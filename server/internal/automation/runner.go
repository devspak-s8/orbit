package automation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Runner is the workflow execution worker: it relays platform events into
// runs, claims queued runs with SKIP LOCKED, and executes steps with
// per-step durability so restarts resume instead of losing work (ADR-0004).
type Runner struct {
	pool   *pgxpool.Pool
	client *http.Client
}

func NewRunner(pool *pgxpool.Pool) *Runner {
	return &Runner{
		pool:   pool,
		client: &http.Client{}, // per-request timeouts come from node config
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

	for i := range steps {
		st := &steps[i]
		if st.Status == "completed" {
			continue // resumed after restart
		}
		node, found := r.findNode(wf, st.NodeID)
		if !found {
			r.failStep(ctx, st, "node missing from definition")
			r.finishRun(ctx, run.ID, "failed", "node missing from definition")
			return true, nil
		}

		r.markStepRunning(ctx, st)
		out, stepErr := r.execWithRetries(ctx, node, run.TriggerPayload)
		if stepErr != nil {
			r.failStep(ctx, st, stepErr.Error())
			r.skipRemaining(ctx, steps, i+1)
			r.finishRun(ctx, run.ID, "failed", fmt.Sprintf("node %q failed: %s", st.NodeID, stepErr))
			return true, nil
		}
		r.completeStep(ctx, st, out)
	}

	r.finishRun(ctx, run.ID, "completed", "")
	return true, nil
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
	default:
		return nil, fmt.Errorf("unknown node type %q", node.Type)
	}
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
	if _, err := r.pool.Exec(ctx,
		`UPDATE workflow_steps SET status = 'running', started_at = COALESCE(started_at, now()) WHERE run_id = $1 AND seq = $2`,
		st.RunID, st.Seq,
	); err != nil {
		slog.Error("mark step running", "error", err)
	}
}

func (r *Runner) completeStep(ctx context.Context, st *Step, out map[string]any) {
	if _, err := r.pool.Exec(ctx,
		`UPDATE workflow_steps SET status = 'completed', output = $3, finished_at = now() WHERE run_id = $1 AND seq = $2`,
		st.RunID, st.Seq, out,
	); err != nil {
		slog.Error("complete step", "error", err)
	}
}

func (r *Runner) failStep(ctx context.Context, st *Step, msg string) {
	if _, err := r.pool.Exec(ctx,
		`UPDATE workflow_steps SET status = 'failed', error = $3, finished_at = now() WHERE run_id = $1 AND seq = $2`,
		st.RunID, st.Seq, msg,
	); err != nil {
		slog.Error("fail step", "error", err)
	}
}

func (r *Runner) skipRemaining(ctx context.Context, steps []Step, from int) {
	for i := from; i < len(steps); i++ {
		st := steps[i]
		if _, err := r.pool.Exec(ctx,
			`UPDATE workflow_steps SET status = 'skipped', finished_at = now() WHERE run_id = $1 AND seq = $2`,
			st.RunID, st.Seq,
		); err != nil {
			slog.Error("skip step", "error", err)
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
	if err := tx.Commit(ctx); err != nil {
		return Run{}, fmt.Errorf("commit: %w", err)
	}
	return run, nil
}
