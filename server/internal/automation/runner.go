package automation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/orbit-org/orbit/server/internal/config"
)

// Runner is the workflow execution worker: it relays platform events into
// runs, claims queued runs with SKIP LOCKED, and executes steps with
// per-step durability so restarts resume instead of losing work (ADR-0004).
type Runner struct {
	pool   *pgxpool.Pool
	client *http.Client
	smtp   config.SMTPConfig
}

func NewRunner(pool *pgxpool.Pool, smtp config.SMTPConfig) *Runner {
	return &Runner{
		pool:   pool,
		client: &http.Client{}, // per-request timeouts come from node config
		smtp:   smtp,
	}
}

// Run drives the worker loop until the context is cancelled. It is safe to
// start multiple loops; SKIP LOCKED arbitrates claims.
func (r *Runner) Run(ctx context.Context) {
	if err := r.Recover(ctx); err != nil {
		slog.Error("automation recovery", "error", err)
	} else {
		slog.Info("automation worker started")
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("automation worker stopped")
			return
		case <-ticker.C:
			if err := r.RelayEvents(ctx); err != nil && ctx.Err() == nil {
				slog.Error("automation event relay", "error", err)
			}
			if err := r.RunDueSchedules(ctx); err != nil && ctx.Err() == nil {
				slog.Error("automation scheduler", "error", err)
			}
			for {
				ran, err := r.ProcessNext(ctx)
				if err != nil && ctx.Err() == nil {
					slog.Error("automation execution", "error", err)
				}
				if !ran || ctx.Err() != nil {
					break
				}
			}
		}
	}
}

// Recover requeues runs a dead process left in flight. Completed steps stay
// completed; the run resumes from the next pending step.
func (r *Runner) Recover(ctx context.Context) error {
	if _, err := r.pool.Exec(ctx,
		`UPDATE workflow_steps SET status = 'pending', error = '' WHERE status = 'running'`,
	); err != nil {
		return fmt.Errorf("reset in-flight steps: %w", err)
	}
	tag, err := r.pool.Exec(ctx,
		`UPDATE workflow_runs SET status = 'queued', error = 'interrupted by restart; requeued'
		 WHERE status = 'running'`,
	)
	if err != nil {
		return fmt.Errorf("requeue interrupted runs: %w", err)
	}
	if tag.RowsAffected() > 0 {
		slog.Info("requeued interrupted workflow runs", "count", tag.RowsAffected())
	}
	return nil
}

// RelayEvents consumes new platform events and enqueues runs for ready
// workflows subscribed to them. The cursor makes consumption resumable.
func (r *Runner) RelayEvents(ctx context.Context) error {
	var last int64
	if err := r.pool.QueryRow(ctx,
		`SELECT last_event_seq FROM automation_cursor WHERE id = 1`,
	).Scan(&last); err != nil {
		return fmt.Errorf("read cursor: %w", err)
	}

	rows, err := r.pool.Query(ctx,
		`SELECT seq, type, workspace_id::text, payload
		 FROM events WHERE seq > $1 ORDER BY seq LIMIT 500`,
		last,
	)
	if err != nil {
		return fmt.Errorf("query events: %w", err)
	}
	type evt struct {
		seq     int64
		typ     string
		ws      string
		payload map[string]any
	}
	var evts []evt
	for rows.Next() {
		var e evt
		if err := rows.Scan(&e.seq, &e.typ, &e.ws, &e.payload); err != nil {
			rows.Close()
			return fmt.Errorf("scan event: %w", err)
		}
		evts = append(evts, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(evts) == 0 {
		return nil
	}

	// Ready workflows with event triggers; matched in Go (alpha scale).
	wfRows, err := r.pool.Query(ctx,
		`SELECT w.id::text, w.workspace_id::text, w.current_version, v.definition
		 FROM workflows w
		 JOIN workflow_versions v ON v.workflow_id = w.id AND v.version = w.current_version
		 WHERE w.status = 'ready'`,
	)
	if err != nil {
		return fmt.Errorf("query ready workflows: %w", err)
	}
	type readyWF struct {
		Workflow
		def Definition
	}
	var ready []readyWF
	for wfRows.Next() {
		var raw []byte
		w := readyWF{}
		if err := wfRows.Scan(&w.ID, &w.WorkspaceID, &w.CurrentVersion, &raw); err != nil {
			wfRows.Close()
			return fmt.Errorf("scan workflow: %w", err)
		}
		if err := json.Unmarshal(raw, &w.def); err != nil {
			continue // unreadable definition never runs
		}
		ready = append(ready, w)
	}
	wfRows.Close()

	for _, e := range evts {
		for _, w := range ready {
			if w.WorkspaceID != e.ws || w.def.Trigger.Type != "event" || w.def.Trigger.EventType != e.typ {
				continue
			}
			// The run's trigger payload exposes the event payload
			// fields at the top level (plus event metadata), so
			// {{payload.title}} refers directly to event fields.
			payload := map[string]any{
				"type":    e.typ,
				"eventId": e.seq,
			}
			for k, v := range e.payload {
				payload[k] = v
			}
			wf := w.Workflow
			wf.Definition = &w.def
			if _, err := r.enqueue(ctx, wf, "event", payload); err != nil {
				slog.Error("enqueue event-triggered run", "workflow", w.ID, "error", err)
			} else {
				slog.Info("workflow triggered by event", "workflow", w.ID, "event", e.typ)
			}
		}
		last = e.seq
	}

	if _, err := r.pool.Exec(ctx,
		`UPDATE automation_cursor SET last_event_seq = $1, updated_at = now() WHERE id = 1`,
		last,
	); err != nil {
		return fmt.Errorf("advance cursor: %w", err)
	}
	return nil
}

// ProcessNext claims one queued run and executes it. Returns false when the
// queue is empty.
// ProcessNext claims one queued run and executes it as a graph walk:
// from the trigger, node by node, following single edges — and at a
// branch node, following the yes/no edge the condition selects. Steps
// for untaken paths are marked skipped; cancellation is observed
// between steps. Returns false when the queue is empty.
func (r *Runner) ProcessNext(ctx context.Context) (bool, error) {
	run, ok, err := r.claim(ctx)
	if err != nil || !ok {
		return false, err
	}

	wf, err := r.loadForRun(ctx, run)
	if err != nil {
		r.finishRun(ctx, run.ID, "failed", err.Error())
		return true, nil
	}

	steps, err := r.loadSteps(ctx, run.ID)
	if err != nil {
		r.finishRun(ctx, run.ID, "failed", err.Error())
		return true, nil
	}

	defErr := wf.Definition.Validate()
	if defErr != nil {
		// Definitions are validated before ready; a bad one here means
		// data corruption — fail loudly, never execute.
		r.finishRun(ctx, run.ID, "failed", "definition no longer valid: "+defErr.Error())
		return true, nil
	}

	def := wf.Definition
	byID := map[string]Node{}
	for _, n := range def.Nodes {
		byID[n.ID] = n
	}
	outEdges := map[string][]Edge{}
	for _, e := range def.Edges {
		outEdges[e.From] = append(outEdges[e.From], e)
	}
	stepByNode := map[string]*Step{}
	for i := range steps {
		stepByNode[steps[i].NodeID] = &steps[i]
	}

	executed := map[string]bool{}
	fail := func(nodeID string, stepErr error) {
		r.skipPending(ctx, steps, executed)
		r.finishRun(ctx, run.ID, "failed", fmt.Sprintf("node %q failed: %s", nodeID, stepErr))
	}

	current := "trigger"
	for {
		// Cancellation is observed between steps.
		if status, err := r.runStatus(ctx, run.ID); err == nil && status == "cancelled" {
			r.skipPending(ctx, steps, executed)
			return true, nil // finishRun already stamped cancelled
		}

		// Pick the next node: branches follow their taken label; every
		// other node follows its single edge.
		edges := outEdges[current]
		if len(edges) == 0 {
			break // end of the path
		}
		var next string
		if current != "trigger" && byID[current].Type == "branch" {
			st := stepByNode[current]
			taken, _ := st.Output["taken"].(string)
			for _, e := range edges {
				if e.Label == taken {
					next = e.To
					break
				}
			}
			if next == "" {
				fail(current, fmt.Errorf("branch has no %q edge", taken))
				return true, nil
			}
		} else {
			next = edges[0].To
		}
		if executed[next] {
			fail(next, fmt.Errorf("cycle detected during execution"))
			return true, nil
		}

		// Execute the next node.
		node := byID[next]
		st := stepByNode[next]
		if st == nil {
			fail(next, fmt.Errorf("node missing from definition"))
			return true, nil
		}
		if st.Status != "completed" { // resumed runs skip done steps
			r.markStepRunning(ctx, st)
			if node.Type == "branch" {
				// A branch step evaluates its condition and records the
				// taken label — it is not executed like action nodes.
				taken, terr := evalBranch(node.Config.Condition, run.TriggerPayload)
				if terr != nil {
					r.failStep(ctx, st, terr.Error())
					fail(next, terr)
					return true, nil
				}
				r.completeStep(ctx, st, map[string]any{"taken": taken})
			} else {
				out, stepErr := r.execWithRetries(ctx, node, run.TriggerPayload)
				if stepErr != nil {
					r.failStep(ctx, st, stepErr.Error())
					fail(next, stepErr)
					return true, nil
				}
				r.completeStep(ctx, st, out)
			}
		}
		executed[next] = true
		current = next
	}

	r.skipPending(ctx, steps, executed)
	r.finishRun(ctx, run.ID, "completed", "")
	return true, nil
}

// evalBranch evaluates a branch condition against the run payload.
// Numeric comparison when both sides parse as numbers; == and != fall
// back to string comparison.
func evalBranch(c *Condition, payload map[string]any) (string, error) {
	if c == nil {
		return "", fmt.Errorf("branch condition is missing")
	}
	left := strings.TrimSpace(substituteTokens(c.Field, payload))

	var rn float64
	rIsNum := false
	switch v := c.Value.(type) {
	case float64:
		rn, rIsNum = v, true
	case int:
		rn, rIsNum = float64(v), true
	case int64:
		rn, rIsNum = float64(v), true
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			rn, rIsNum = f, true
		}
	}
	if ln, err := strconv.ParseFloat(left, 64); err == nil && rIsNum {
		switch c.Op {
		case ">":
			return boolLabel(ln > rn), nil
		case ">=":
			return boolLabel(ln >= rn), nil
		case "<":
			return boolLabel(ln < rn), nil
		case "<=":
			return boolLabel(ln <= rn), nil
		case "==":
			return boolLabel(ln == rn), nil
		case "!=":
			return boolLabel(ln != rn), nil
		}
	}
	ls := left
	rs := fmt.Sprintf("%v", c.Value)
	switch c.Op {
	case "==":
		return boolLabel(ls == rs), nil
	case "!=":
		return boolLabel(ls != rs), nil
	default:
		return "", fmt.Errorf("cannot compare %q %s %v (not numeric)", left, c.Op, c.Value)
	}
}

func boolLabel(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// runStatus reads a run's current status (for cancellation checks).
func (r *Runner) runStatus(ctx context.Context, runID string) (string, error) {
	var status string
	err := r.pool.QueryRow(ctx,
		`SELECT status FROM workflow_runs WHERE id = $1`, runID,
	).Scan(&status)
	return status, err
}

func (r *Runner) claim(ctx context.Context) (Run, bool, error) {
	var run Run
	err := r.pool.QueryRow(ctx,
		`UPDATE workflow_runs SET status = 'running', started_at = COALESCE(started_at, now())
		 WHERE id = (
		   SELECT id FROM workflow_runs WHERE status = 'queued'
		   ORDER BY created_at, id LIMIT 1 FOR UPDATE SKIP LOCKED
		 )
		 RETURNING id::text, workflow_id::text, workspace_id::text, version, status, trigger_type, trigger_payload, error, created_at`,
	).Scan(&run.ID, &run.WorkflowID, &run.WorkspaceID, &run.Version, &run.Status, &run.TriggerType, &run.TriggerPayload, &run.Error, &run.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Run{}, false, nil
		}
		return Run{}, false, fmt.Errorf("claim run: %w", err)
	}
	return run, true, nil
}

func (r *Runner) loadForRun(ctx context.Context, run Run) (Workflow, error) {
	var wf Workflow
	var raw []byte
	err := r.pool.QueryRow(ctx,
		`SELECT w.id::text, w.workspace_id::text, w.name, w.status, w.current_version, v.definition
		 FROM workflows w
		 JOIN workflow_versions v ON v.workflow_id = w.id AND v.version = $1
		 WHERE w.id = $2`,
		run.Version, run.WorkflowID,
	).Scan(&wf.ID, &wf.WorkspaceID, &wf.Name, &wf.Status, &wf.CurrentVersion, &raw)
	if err != nil {
		return Workflow{}, fmt.Errorf("load workflow for run: %w", err)
	}
	var def Definition
	if err := json.Unmarshal(raw, &def); err != nil {
		return Workflow{}, fmt.Errorf("decode definition: %w", err)
	}
	wf.Definition = &def
	return wf, nil
}

func (r *Runner) loadSteps(ctx context.Context, runID string) ([]Step, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT run_id::text, seq, node_id, node_type, status, input, output, error, started_at, finished_at
		 FROM workflow_steps WHERE run_id = $1 ORDER BY seq`,
		runID,
	)
	if err != nil {
		return nil, fmt.Errorf("load steps: %w", err)
	}
	defer rows.Close()
	var steps []Step
	for rows.Next() {
		var st Step
		if err := rows.Scan(&st.RunID, &st.Seq, &st.NodeID, &st.NodeType, &st.Status, &st.Input, &st.Output, &st.Error, &st.StartedAt, &st.FinishedAt); err != nil {
			return nil, fmt.Errorf("scan step: %w", err)
		}
		steps = append(steps, st)
	}
	return steps, rows.Err()
}

func (r *Runner) findNode(wf Workflow, nodeID string) (Node, bool) {
	for _, n := range wf.Definition.Nodes {
		if n.ID == nodeID {
			return n, true
		}
	}
	return Node{}, false
}

func (r *Runner) execWithRetries(ctx context.Context, node Node, payload map[string]any) (map[string]any, error) {
	attempts := node.Config.Retries + 1
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		out, err := r.execNode(ctx, node, payload)
		if err == nil {
			return out, nil
		}
		lastErr = err
		if attempt < attempts {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(200*attempt) * time.Millisecond):
			}
		}
	}
	return nil, fmt.Errorf("%w (after %d attempt(s))", lastErr, attempts)
}

func (r *Runner) execNode(ctx context.Context, node Node, payload map[string]any) (map[string]any, error) {
	switch node.Type {
	case "http":
		return r.execHTTP(ctx, node, payload)
	case "log":
		return execLog(node, payload)
	case "email":
		return r.execEmail(ctx, node, payload)
	default:
		return nil, fmt.Errorf("unknown node type %q", node.Type)
	}
}

// execEmail sends one plain-text email through the server's SMTP relay.
// All fields support {{payload.*}} substitution. An unconfigured relay
// fails the node with a clear error — never a silent skip.
func (r *Runner) execEmail(ctx context.Context, node Node, payload map[string]any) (map[string]any, error) {
	if r.smtp.Host == "" {
		return nil, fmt.Errorf("email is not configured on this server: set ORBIT_SMTP_HOST (see README)")
	}

	to := strings.Split(substituteTokens(node.Config.To, payload), ",")
	recipients := make([]string, 0, len(to))
	for _, addr := range to {
		if addr = strings.TrimSpace(addr); addr != "" {
			recipients = append(recipients, addr)
		}
	}
	if len(recipients) == 0 {
		return nil, fmt.Errorf("email node has no recipients after substitution")
	}
	subject := substituteTokens(node.Config.Subject, payload)
	text := substituteTokens(node.Config.Text, payload)

	msg := &bytes.Buffer{}
	fmt.Fprintf(msg, "From: %s\r\n", r.smtp.From)
	fmt.Fprintf(msg, "To: %s\r\n", strings.Join(recipients, ", "))
	fmt.Fprintf(msg, "Subject: %s\r\n", subject)
	fmt.Fprintf(msg, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	fmt.Fprintf(msg, "Content-Type: text/plain; charset=utf-8\r\n")
	fmt.Fprintf(msg, "\r\n%s\r\n", text)

	addr := net.JoinHostPort(r.smtp.Host, strconv.Itoa(r.smtp.Port))
	var auth smtp.Auth
	if r.smtp.User != "" {
		auth = smtp.PlainAuth("", r.smtp.User, r.smtp.Pass, r.smtp.Host)
	}
	if err := smtp.SendMail(addr, auth, r.smtp.From, recipients, msg.Bytes()); err != nil {
		return nil, fmt.Errorf("send mail via %s: %w", addr, err)
	}
	return map[string]any{"to": recipients, "subject": subject}, nil
}

// execHTTP performs the node's HTTP request. {{payload.key}} tokens in the
// URL and body are substituted from the trigger payload. Responses with
// status >= 400 fail the node.
func (r *Runner) execHTTP(ctx context.Context, node Node, payload map[string]any) (map[string]any, error) {
	cfg := node.Config
	url := substituteTokens(cfg.URL, payload)

	var bodyReader io.Reader
	if cfg.Body != nil {
		raw, err := json.Marshal(cfg.Body)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		bodyReader = bytes.NewReader([]byte(substituteTokens(string(raw), payload)))
	}

	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.nodeTimeout())*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, cfg.Method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if cfg.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range cfg.Headers {
		req.Header.Set(k, substituteTokens(v, payload))
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))

	out := map[string]any{
		"statusCode": resp.StatusCode,
		"url":        url,
		"method":     cfg.Method,
	}
	if len(body) > 0 {
		out["body"] = string(body)
	}
	if resp.StatusCode >= 400 {
		return out, fmt.Errorf("http status %d", resp.StatusCode)
	}
	return out, nil
}

func execLog(node Node, payload map[string]any) (map[string]any, error) {
	out := map[string]any{"message": substituteTokens(node.Config.Message, payload)}
	return out, nil
}

// substituteTokens replaces {{payload.key}} tokens with values from the
// trigger payload. Unknown tokens are replaced with an empty string.
func substituteTokens(s string, payload map[string]any) string {
	return placeholderPattern.ReplaceAllStringFunc(s, func(m string) string {
		key := placeholderPattern.FindStringSubmatch(m)[1]
		v, ok := payload[key]
		if !ok {
			return ""
		}
		if s, ok := v.(string); ok {
			return s
		}
		return fmt.Sprintf("%v", v)
	})
}

// --- step/run state transitions ---

func (r *Runner) markStepRunning(ctx context.Context, st *Step) {
	st.Status = "running"
	if _, err := r.pool.Exec(ctx,
		`UPDATE workflow_steps SET status = 'running', started_at = COALESCE(started_at, now()) WHERE run_id = $1 AND seq = $2`,
		st.RunID, st.Seq,
	); err != nil {
		slog.Error("mark step running", "error", err)
	}
}

func (r *Runner) completeStep(ctx context.Context, st *Step, out map[string]any) {
	st.Status = "completed"
	if out == nil {
		out = map[string]any{}
	}
	st.Output = out
	if _, err := r.pool.Exec(ctx,
		`UPDATE workflow_steps SET status = 'completed', output = $3, finished_at = now() WHERE run_id = $1 AND seq = $2`,
		st.RunID, st.Seq, out,
	); err != nil {
		slog.Error("complete step", "error", err)
	}
}

func (r *Runner) failStep(ctx context.Context, st *Step, msg string) {
	st.Status = "failed"
	if _, err := r.pool.Exec(ctx,
		`UPDATE workflow_steps SET status = 'failed', error = $3, finished_at = now() WHERE run_id = $1 AND seq = $2`,
		st.RunID, st.Seq, msg,
	); err != nil {
		slog.Error("fail step", "error", err)
	}
}

// skipPending marks every step that this run did not execute (the untaken
// branch path, or the tail after a failure) as skipped.
func (r *Runner) skipPending(ctx context.Context, steps []Step, executed map[string]bool) {
	for i := range steps {
		st := &steps[i]
		if st.Status == "pending" && !executed[st.NodeID] {
			if _, err := r.pool.Exec(ctx,
				`UPDATE workflow_steps SET status = 'skipped', finished_at = now() WHERE run_id = $1 AND seq = $2`,
				st.RunID, st.Seq,
			); err != nil {
				slog.Error("skip step", "error", err)
			}
			st.Status = "skipped"
		}
	}
}

func (r *Runner) finishRun(ctx context.Context, runID, status, errMsg string) {
	if _, err := r.pool.Exec(ctx,
		`UPDATE workflow_runs SET status = $2, error = $3, finished_at = now() WHERE id = $1`,
		runID, status, strings.TrimSpace(errMsg),
	); err != nil {
		slog.Error("finish run", "error", err)
	}
}

// enqueue is the runner-side reimplementation of Service.enqueueRun for
// event-triggered runs (the service method enqueues with user context).
func (r *Runner) enqueue(ctx context.Context, wf Workflow, triggerType string, payload map[string]any) (Run, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Run{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)
	run, err := r.enqueueTx(ctx, tx, wf, triggerType, payload)
	if err != nil {
		return Run{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Run{}, fmt.Errorf("commit: %w", err)
	}
	return run, nil
}

// enqueueTx inserts a run and its pending steps inside the caller's
// transaction, so trigger state and queued work commit atomically.
func (r *Runner) enqueueTx(ctx context.Context, tx pgx.Tx, wf Workflow, triggerType string, payload map[string]any) (Run, error) {
	var run Run
	if err := tx.QueryRow(ctx,
		`INSERT INTO workflow_runs (workflow_id, workspace_id, version, trigger_type, trigger_payload)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id::text, workflow_id::text, workspace_id::text, version, status, trigger_type, trigger_payload, created_at`,
		wf.ID, wf.WorkspaceID, wf.CurrentVersion, triggerType, payload,
	).Scan(&run.ID, &run.WorkflowID, &run.WorkspaceID, &run.Version, &run.Status, &run.TriggerType, &run.TriggerPayload, &run.CreatedAt); err != nil {
		return Run{}, fmt.Errorf("insert run: %w", err)
	}
	for i, node := range wf.Definition.Order() {
		if _, err := tx.Exec(ctx,
			`INSERT INTO workflow_steps (run_id, seq, node_id, node_type) VALUES ($1, $2, $3, $4)`,
			run.ID, i+1, node.ID, node.Type,
		); err != nil {
			return Run{}, fmt.Errorf("insert step: %w", err)
		}
	}
	return run, nil
}

// RunDueSchedules enqueues runs for schedule-triggered workflows whose
// next_due_at has passed. The cursor advances in the same transaction as
// the enqueue, so a crash can neither double-fire nor lose the fire.
func (r *Runner) RunDueSchedules(ctx context.Context) error {
	for {
		enqueued, err := r.runOneDueSchedule(ctx)
		if err != nil {
			return err
		}
		if !enqueued {
			return nil
		}
	}
}

func (r *Runner) runOneDueSchedule(ctx context.Context) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	var wf Workflow
	var raw []byte
	err = tx.QueryRow(ctx,
		`SELECT w.id::text, w.workspace_id::text, w.current_version, v.definition
		 FROM workflows w
		 JOIN workflow_versions v ON v.workflow_id = w.id AND v.version = w.current_version
		 WHERE w.status = 'ready' AND w.next_due_at IS NOT NULL AND w.next_due_at <= now()
		 ORDER BY w.next_due_at
		 LIMIT 1
		 FOR UPDATE OF w SKIP LOCKED`,
	).Scan(&wf.ID, &wf.WorkspaceID, &wf.CurrentVersion, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claim due workflow: %w", err)
	}

	var def Definition
	if err := json.Unmarshal(raw, &def); err != nil {
		// Unreadable definition: clear the cursor so it never fires again
		// silently. Loudly logged; the workflow shows as ready but never
		// schedules — visible in history by the absence of runs.
		if _, err := tx.Exec(ctx, `UPDATE workflows SET next_due_at = NULL WHERE id = $1`, wf.ID); err != nil {
			return false, fmt.Errorf("clear bad schedule: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return false, fmt.Errorf("commit: %w", err)
		}
		slog.Error("schedule workflow has unreadable definition; cursor cleared", "workflow", wf.ID)
		return true, nil
	}

	if def.Trigger.Type != "schedule" || def.Trigger.IntervalSec < minScheduleSecs {
		// The trigger changed shape without revalidation: clear the cursor.
		if _, err := tx.Exec(ctx, `UPDATE workflows SET next_due_at = NULL WHERE id = $1`, wf.ID); err != nil {
			return false, fmt.Errorf("clear mismatched schedule: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return false, fmt.Errorf("commit: %w", err)
		}
		slog.Error("schedule cursor on non-schedule workflow; cleared", "workflow", wf.ID)
		return true, nil
	}

	// Advance the cursor and enqueue the run in one transaction.
	if _, err := tx.Exec(ctx,
		`UPDATE workflows SET next_due_at = now() + make_interval(secs => $2) WHERE id = $1`,
		wf.ID, def.Trigger.IntervalSec,
	); err != nil {
		return false, fmt.Errorf("advance schedule: %w", err)
	}

	wf.Definition = &def
	if _, err := r.enqueueTx(ctx, tx, wf, "schedule", map[string]any{"scheduled": true}); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit: %w", err)
	}
	slog.Info("scheduled workflow fired", "workflow", wf.ID)
	return true, nil
}
