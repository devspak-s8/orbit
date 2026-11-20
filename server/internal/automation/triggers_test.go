package automation

import (
	"bufio"
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/orbit-org/orbit/server/internal/config"
)

// --- webhook trigger ---

func TestWebhookTriggerDeliversAndRuns(t *testing.T) {
	env := setup(t)
	defer env.cleanup()

	wf, err := env.svc.Create(env.ctx, env.userID, env.wsID, "Hook", Definition{
		Trigger: Trigger{Type: "webhook"},
		Nodes: []Node{
			{ID: "echo", Type: "log", Config: NodeConfig{Message: "hook said: {{payload.value}}"}},
		},
		Edges: []Edge{{From: "trigger", To: "echo"}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Drafts never run: no token before validation.
	if wf.WebhookToken != "" {
		t.Errorf("draft workflow already has a token: %q", wf.WebhookToken)
	}
	if _, _, err := env.svc.EnqueueWebhook(env.ctx, "some-token", nil); err == nil {
		t.Error("enqueue with unknown token succeeded, want not-found")
	}

	validated, err := env.svc.Validate(env.ctx, env.userID, env.wsID, wf.ID)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if validated.WebhookToken == "" || len(validated.WebhookToken) < 32 {
		t.Fatalf("validated webhook workflow has no usable token: %q", validated.WebhookToken)
	}

	run, hooked, err := env.svc.EnqueueWebhook(env.ctx, validated.WebhookToken, map[string]any{"value": 42})
	if err != nil {
		t.Fatalf("EnqueueWebhook: %v", err)
	}
	if hooked.ID != wf.ID {
		t.Errorf("token resolved to workflow %q, want %q", hooked.ID, wf.ID)
	}

	env.drain(t)

	_, steps, err := env.svc.GetRun(env.ctx, env.userID, env.wsID, wf.ID, run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if steps[0].Status != "completed" {
		t.Errorf("step status = %q, want completed: %+v", steps[0].Status, steps[0])
	}
	if msg, _ := steps[0].Output["message"].(string); msg != "hook said: 42" {
		t.Errorf("substituted message = %q, want %q", msg, "hook said: 42")
	}
	runs, _ := env.svc.ListRuns(env.ctx, env.userID, env.wsID, wf.ID, 50, "")
	if len(runs) != 1 || runs[0].TriggerType != "webhook" {
		t.Errorf("runs = %+v, want one webhook-triggered run", runs)
	}
}

// --- schedule trigger ---

func TestScheduleFiresWhenDue(t *testing.T) {
	env := setup(t)
	defer env.cleanup()

	wf, err := env.svc.Create(env.ctx, env.userID, env.wsID, "Every minute", Definition{
		Trigger: Trigger{Type: "schedule", IntervalSec: 60},
		Nodes: []Node{
			{ID: "tick", Type: "log", Config: NodeConfig{Message: "tick"}},
		},
		Edges: []Edge{{From: "trigger", To: "tick"}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	validated, err := env.svc.Validate(env.ctx, env.userID, env.wsID, wf.ID)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if validated.NextDueAt == nil {
		t.Fatal("schedule workflow has no next_due_at after validation")
	}
	if validated.NextDueAt.Before(time.Now().Add(55 * time.Second)) {
		t.Errorf("first due = %v, want a full interval away", validated.NextDueAt)
	}

	// Not due yet: nothing fires.
	if err := env.runner.RunDueSchedules(env.ctx); err != nil {
		t.Fatalf("RunDueSchedules: %v", err)
	}
	if runs, _ := env.svc.ListRuns(env.ctx, env.userID, env.wsID, wf.ID, 50, ""); len(runs) != 0 {
		t.Fatalf("premature runs = %d, want 0", len(runs))
	}

	// Force it due, as an hour of wall-clock time would.
	if _, err := env.runner.pool.Exec(env.ctx,
		`UPDATE workflows SET next_due_at = now() - interval '1 second' WHERE id = $1`, wf.ID,
	); err != nil {
		t.Fatalf("force due: %v", err)
	}
	if err := env.runner.RunDueSchedules(env.ctx); err != nil {
		t.Fatalf("RunDueSchedules: %v", err)
	}

	runs, _ := env.svc.ListRuns(env.ctx, env.userID, env.wsID, wf.ID, 50, "")
	if len(runs) != 1 || runs[0].TriggerType != "schedule" {
		t.Fatalf("runs = %+v, want one schedule-triggered run", runs)
	}

	// The cursor advanced: a second pass does not double-fire.
	if err := env.runner.RunDueSchedules(env.ctx); err != nil {
		t.Fatalf("RunDueSchedules second: %v", err)
	}
	runs, _ = env.svc.ListRuns(env.ctx, env.userID, env.wsID, wf.ID, 50, "")
	if len(runs) != 1 {
		t.Fatalf("double-fire: runs = %d, want 1", len(runs))
	}

	// The advanced cursor is a full interval in the future.
	var next time.Time
	if err := env.runner.pool.QueryRow(env.ctx,
		`SELECT next_due_at FROM workflows WHERE id = $1`, wf.ID,
	).Scan(&next); err != nil {
		t.Fatalf("read cursor: %v", err)
	}
	if !next.After(time.Now().Add(55 * time.Second)) {
		t.Errorf("advanced cursor = %v, want >= now+55s", next)
	}
}

// --- email node ---

func TestEmailNodeFailsClearlyWhenUnconfigured(t *testing.T) {
	env := setup(t)
	defer env.cleanup()

	wf, err := env.svc.Create(env.ctx, env.userID, env.wsID, "Mail", Definition{
		Trigger: Trigger{Type: "manual"},
		Nodes: []Node{
			{ID: "mail", Type: "email", Config: NodeConfig{To: "a@b.c", Subject: "hi"}},
		},
		Edges: []Edge{{From: "trigger", To: "mail"}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := env.svc.Validate(env.ctx, env.userID, env.wsID, wf.ID); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	run, err := env.svc.StartRun(env.ctx, env.userID, env.wsID, wf.ID)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	env.drain(t)

	_, steps, err := env.svc.GetRun(env.ctx, env.userID, env.wsID, wf.ID, run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if steps[0].Status != "failed" {
		t.Fatalf("step = %+v, want failed when SMTP is unconfigured", steps[0])
	}
	if !strings.Contains(steps[0].Error, "email is not configured") {
		t.Errorf("error = %q, want a clear unconfigured message", steps[0].Error)
	}
}

// smtpStub is a minimal SMTP server capturing one message: enough to
// prove the engine performs a real SMTP transaction with substitution.
type smtpStub struct {
	listen  net.Listener
	message string
	mu      sync.Mutex
	got     chan struct{}
}

func startSMTPStub(t *testing.T) (*smtpStub, int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("smtp stub listen: %v", err)
	}
	stub := &smtpStub{listen: ln, got: make(chan struct{}, 1)}
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		write := func(s string) { _, _ = conn.Write([]byte(s)) }
		write("220 stub\r\n")
		r := bufio.NewReader(conn)
		inData := false
		var msg strings.Builder
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			if inData {
				msg.WriteString(line)
				if line == ".\r\n" {
					write("250 queued\r\n")
					inData = false
					stub.mu.Lock()
					stub.message = msg.String()
					stub.mu.Unlock()
					select {
					case stub.got <- struct{}{}:
					default:
					}
				}
				continue
			}
			upper := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
				write("250-stub\r\n250 OK\r\n")
			case strings.HasPrefix(upper, "DATA"):
				write("354 go\r\n")
				inData = true
			case strings.HasPrefix(upper, "QUIT"):
				write("221 bye\r\n")
				return
			default: // MAIL, RCPT
				write("250 OK\r\n")
			}
		}
	}()
	t.Cleanup(func() { ln.Close() })
	return stub, ln.Addr().(*net.TCPAddr).Port
}

func TestEmailNodeSendsRealSMTP(t *testing.T) {
	env := setup(t)
	defer env.cleanup()

	stub, port := startSMTPStub(t)
	mailer := NewRunner(env.runner.pool, config.SMTPConfig{Host: "127.0.0.1", Port: port, From: "orbit@orbit.test"})

	wf, err := env.svc.Create(env.ctx, env.userID, env.wsID, "Real mail", Definition{
		Trigger: Trigger{Type: "manual"},
		Nodes: []Node{
			{ID: "mail", Type: "email", Config: NodeConfig{
				To:      "{{payload.email}}",
				Subject: "Hello {{payload.name}}",
				Text:    "the workflow works",
			}},
		},
		Edges: []Edge{{From: "trigger", To: "mail"}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := env.svc.Validate(env.ctx, env.userID, env.wsID, wf.ID); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	// Manual run with a payload the email fields substitute from.
	wfFull, err := env.svc.Get(env.ctx, env.userID, env.wsID, wf.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	_, err = env.svc.enqueueWithPayload(env.ctx, wfFull, map[string]any{"email": "ada@example.com", "name": "Ada"})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	for i := 0; i < 50; i++ {
		ran, err := mailer.ProcessNext(env.ctx)
		if err != nil {
			t.Fatalf("ProcessNext: %v", err)
		}
		if !ran {
			break
		}
	}

	// The stub captured a real SMTP transaction.
	select {
	case <-stub.got:
	case <-time.After(10 * time.Second):
		t.Fatal("SMTP stub never received the message")
	}
	stub.mu.Lock()
	msg := stub.message
	stub.mu.Unlock()
	if !strings.Contains(msg, "Subject: Hello Ada\r\n") {
		t.Errorf("captured message missing substituted subject:\n%s", msg)
	}
	if !strings.Contains(msg, "To: ada@example.com") {
		t.Errorf("captured message missing substituted recipient:\n%s", msg)
	}
	if !strings.Contains(msg, "the workflow works") {
		t.Errorf("captured message missing body:\n%s", msg)
	}
}

// enqueueWithPayload is a test helper: enqueue a run for a ready workflow
// with an explicit trigger payload (the manual StartRun only sets actor).
func (s *Service) enqueueWithPayload(ctx context.Context, wf Workflow, payload map[string]any) (Run, error) {
	return s.enqueueRun(ctx, wf, "manual", payload)
}
