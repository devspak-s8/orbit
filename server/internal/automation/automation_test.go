package automation

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/orbit-org/orbit/server/internal/config"
	"github.com/orbit-org/orbit/server/internal/db"
	"github.com/orbit-org/orbit/server/internal/events"
	"github.com/orbit-org/orbit/server/internal/identity"
	"github.com/orbit-org/orbit/server/internal/workspaces"
)

type env struct {
	svc    *Service
	runner *Runner
	events *events.Service
	userID string
	wsID   string
	cleanup func()
	ctx    context.Context
}

func setup(t *testing.T) *env {
	t.Helper()
	ctx := context.Background()
	cfg := config.Load()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		t.Fatalf("connect database (is `docker compose up -d db` running?): %v", err)
	}
	if err := db.Migrate(ctx, cfg.DatabaseURL); err != nil {
		pool.Close()
		t.Fatalf("migrate: %v", err)
	}

	ident := identity.NewService(pool, 24*time.Hour)
	wsSvc := workspaces.NewService(pool)
	eventsSvc := events.NewService(pool)

	email := fmt.Sprintf("auto-%d@orbit.test", time.Now().UnixNano())
	user, _, err := ident.Register(ctx, email, "Auto User", "password123")
	if err != nil {
		pool.Close()
		t.Fatalf("register: %v", err)
	}
	ws, err := wsSvc.Create(ctx, user.ID, fmt.Sprintf("Auto Test %d", time.Now().UnixNano()))
	if err != nil {
		pool.Close()
		t.Fatalf("create workspace: %v", err)
	}

	cleanup := func() {
		_, _ = pool.Exec(ctx, `DELETE FROM workspaces WHERE created_by = $1`, user.ID)
		_, _ = pool.Exec(ctx, `DELETE FROM audit_logs WHERE actor_user_id = $1`, user.ID)
		_, _ = pool.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, user.ID)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, user.ID)
		pool.Close()
	}

	return &env{
		svc:    NewService(pool),
		runner: NewRunner(pool),
		events: eventsSvc,
		userID: user.ID,
		wsID:   ws.ID,
		cleanup: cleanup,
		ctx:    ctx,
	}
}

// drain processes the queue until empty, with a bounded wait.
func (e *env) drain(t *testing.T) {
	t.Helper()
	for i := 0; i < 50; i++ {
		ran, err := e.runner.ProcessNext(e.ctx)
		if err != nil {
			t.Fatalf("ProcessNext: %v", err)
		}
		if !ran {
			return
		}
	}
	t.Fatal("queue did not drain")
}

func waitFor(t *testing.T, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("condition not met in time")
}

func TestManualRunExecutesAndPersistsHistory(t *testing.T) {
	env := setup(t)
	defer env.cleanup()

	// A test endpoint the workflow will call.
	var called int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&called, 1)
		w.Write([]byte(`{"ok": true}`))
	}))
	defer srv.Close()

	wf, err := env.svc.Create(env.ctx, env.userID, env.wsID, "Notify", Definition{
		Trigger: Trigger{Type: "manual"},
		Nodes: []Node{
			{ID: "call", Type: "http", Config: NodeConfig{Method: "GET", URL: srv.URL}},
			{ID: "done", Type: "log", Config: NodeConfig{Message: "workflow finished"}},
		},
		Edges: []Edge{
			{From: "trigger", To: "call"},
			{From: "call", To: "done"},
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Draft workflows never run (editing and execution are separate).
	if _, err := env.svc.StartRun(env.ctx, env.userID, env.wsID, wf.ID); err == nil {
		t.Fatal("draft workflow ran, want ErrNotReady")
	}

	if _, err := env.svc.Validate(env.ctx, env.userID, env.wsID, wf.ID); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	run, err := env.svc.StartRun(env.ctx, env.userID, env.wsID, wf.ID)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	env.drain(t)

	if atomic.LoadInt32(&called) != 1 {
		t.Errorf("endpoint called %d times, want 1", called)
	}

	_, steps, err := env.svc.GetRun(env.ctx, env.userID, env.wsID, wf.ID, run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("steps = %d, want 2", len(steps))
	}
	if steps[0].Status != "completed" || steps[0].Output["statusCode"] != float64(200) {
		t.Errorf("step 1 = %+v, want completed with 200", steps[0])
	}
	if steps[1].Status != "completed" {
		t.Errorf("step 2 = %+v, want completed", steps[1])
	}

	// The run itself is persisted as completed.
	runs, err := env.svc.ListRuns(env.ctx, env.userID, env.wsID, wf.ID)
	if err != nil || len(runs) != 1 {
		t.Fatalf("ListRuns = %v, err = %v; want 1 run", runs, err)
	}
	if runs[0].Status != "completed" {
		t.Errorf("run status = %q, want completed", runs[0].Status)
	}
}

func TestEventTriggerFiresWorkflow(t *testing.T) {
	env := setup(t)
	defer env.cleanup()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	wf, err := env.svc.Create(env.ctx, env.userID, env.wsID, "On record", Definition{
		Trigger: Trigger{Type: "event", EventType: "demo.record.created"},
		Nodes: []Node{
			{ID: "call", Type: "http", Config: NodeConfig{Method: "POST", URL: srv.URL}},
		},
		Edges: []Edge{{From: "trigger", To: "call"}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := env.svc.Validate(env.ctx, env.userID, env.wsID, wf.ID); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	// Publish the event the workflow subscribes to.
	tx, err := env.runner.pool.Begin(env.ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := env.events.PublishTx(env.ctx, tx, events.Event{
		Type:        "demo.record.created",
		WorkspaceID: env.wsID,
		Payload:     map[string]any{"recordId": "r-1", "title": "test"},
	}); err != nil {
		t.Fatalf("PublishTx: %v", err)
	}
	if err := tx.Commit(env.ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// The relay enqueues a run; the worker executes it.
	if err := env.runner.RelayEvents(env.ctx); err != nil {
		t.Fatalf("RelayEvents: %v", err)
	}
	env.drain(t)

	runs, err := env.svc.ListRuns(env.ctx, env.userID, env.wsID, wf.ID)
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs = %d, err = %v; want 1 event-triggered run", len(runs), err)
	}
	if runs[0].TriggerType != "event" {
		t.Errorf("trigger type = %q, want event", runs[0].TriggerType)
	}
	if runs[0].Status != "completed" {
		t.Errorf("run status = %q, want completed", runs[0].Status)
	}
}

func TestFailedRunIsInspectable(t *testing.T) {
	env := setup(t)
	defer env.cleanup()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	wf, err := env.svc.Create(env.ctx, env.userID, env.wsID, "Will fail", Definition{
		Trigger: Trigger{Type: "manual"},
		Nodes: []Node{
			{ID: "bad", Type: "http", Config: NodeConfig{Method: "GET", URL: srv.URL}},
			{ID: "never", Type: "log", Config: NodeConfig{Message: "unreachable"}},
		},
		Edges: []Edge{
			{From: "trigger", To: "bad"},
			{From: "bad", To: "never"},
		},
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
	if steps[0].Status != "failed" || steps[0].Error == "" {
		t.Errorf("step 1 = %+v, want failed with an error message", steps[0])
	}
	if steps[1].Status != "skipped" {
		t.Errorf("step 2 = %+v, want skipped after failure", steps[1])
	}

	runs, _ := env.svc.ListRuns(env.ctx, env.userID, env.wsID, wf.ID)
	if len(runs) != 1 || runs[0].Status != "failed" || runs[0].Error == "" {
		t.Errorf("run = %+v, want failed with inspectable error", runs[0])
	}
}

func TestRestartRecoveryResumesMidRun(t *testing.T) {
	env := setup(t)
	defer env.cleanup()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	wf, err := env.svc.Create(env.ctx, env.userID, env.wsID, "Two steps", Definition{
		Trigger: Trigger{Type: "manual"},
		Nodes: []Node{
			{ID: "a", Type: "http", Config: NodeConfig{Method: "GET", URL: srv.URL}},
			{ID: "b", Type: "log", Config: NodeConfig{Message: "second"}},
		},
		Edges: []Edge{
			{From: "trigger", To: "a"},
			{From: "a", To: "b"},
		},
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

	// Simulate a crash after step 1 completed: mark step 1 completed and
	// the run running, as a killed process would leave them.
	if _, err := env.runner.pool.Exec(env.ctx,
		`UPDATE workflow_steps SET status = 'completed', started_at = now(), finished_at = now() WHERE run_id = $1 AND seq = 1`,
		run.ID,
	); err != nil {
		t.Fatalf("simulate step 1: %v", err)
	}
	if _, err := env.runner.pool.Exec(env.ctx,
		`UPDATE workflow_runs SET status = 'running' WHERE id = $1`,
		run.ID,
	); err != nil {
		t.Fatalf("simulate running run: %v", err)
	}

	// "Reboot": recovery requeues the run; the worker resumes at step 2.
	if err := env.runner.Recover(env.ctx); err != nil {
		t.Fatalf("Recover: %v", err)
	}
	env.drain(t)

	_, steps, err := env.svc.GetRun(env.ctx, env.userID, env.wsID, wf.ID, run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if steps[0].Status != "completed" {
		t.Errorf("step 1 = %q, want completed (preserved across restart)", steps[0].Status)
	}
	if steps[1].Status != "completed" {
		t.Errorf("step 2 = %q, want completed after resume", steps[1].Status)
	}
	runs, _ := env.svc.ListRuns(env.ctx, env.userID, env.wsID, wf.ID)
	if len(runs) != 1 || runs[0].Status != "completed" {
		t.Errorf("run = %+v, want exactly one completed run", runs)
	}
}

func TestRetriesRecoverTransientFailure(t *testing.T) {
	env := setup(t)
	defer env.cleanup()

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			http.Error(w, "flaky", http.StatusBadGateway)
			return
		}
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	wf, err := env.svc.Create(env.ctx, env.userID, env.wsID, "Flaky", Definition{
		Trigger: Trigger{Type: "manual"},
		Nodes: []Node{
			{ID: "call", Type: "http", Config: NodeConfig{Method: "GET", URL: srv.URL, Retries: 1}},
		},
		Edges: []Edge{{From: "trigger", To: "call"}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := env.svc.Validate(env.ctx, env.userID, env.wsID, wf.ID); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if _, err := env.svc.StartRun(env.ctx, env.userID, env.wsID, wf.ID); err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	env.drain(t)

	if atomic.LoadInt32(&calls) != 2 {
		t.Errorf("endpoint called %d times, want 2 (initial + 1 retry)", calls)
	}
	runs, _ := env.svc.ListRuns(env.ctx, env.userID, env.wsID, wf.ID)
	if len(runs) != 1 || runs[0].Status != "completed" {
		t.Errorf("run = %+v, want completed after retry", runs[0])
	}
}

func TestEventTriggerPayloadSubstitution(t *testing.T) {
	env := setup(t)
	defer env.cleanup()

	var gotTitle string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 512)
		n, _ := r.Body.Read(buf)
		gotTitle = fmt.Sprintf("%s", buf[:n])
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	wf, err := env.svc.Create(env.ctx, env.userID, env.wsID, "With payload", Definition{
		Trigger: Trigger{Type: "event", EventType: "demo.record.created"},
		Nodes: []Node{
			{ID: "call", Type: "http", Config: NodeConfig{
				Method: "POST", URL: srv.URL,
				Body: map[string]any{"title": "{{payload.title}}"},
			}},
		},
		Edges: []Edge{{From: "trigger", To: "call"}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := env.svc.Validate(env.ctx, env.userID, env.wsID, wf.ID); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	tx, _ := env.runner.pool.Begin(env.ctx)
	if err := env.events.PublishTx(env.ctx, tx, events.Event{
		Type: "demo.record.created", WorkspaceID: env.wsID,
		Payload: map[string]any{"title": "hello substitution"},
	}); err != nil {
		t.Fatalf("PublishTx: %v", err)
	}
	if err := tx.Commit(env.ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	if err := env.runner.RelayEvents(env.ctx); err != nil {
		t.Fatalf("RelayEvents: %v", err)
	}
	env.drain(t)

	if gotTitle == "" {
		t.Fatal("workflow HTTP node received no body")
	}
	want := `"title":"hello substitution"`
	if !strings.Contains(gotTitle, want) {
		t.Errorf("body = %q, want it to contain %q", gotTitle, want)
	}
}