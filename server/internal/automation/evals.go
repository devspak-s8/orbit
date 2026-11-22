// Workflow evals (docs/plans/v1.2.0-evals-healing-desktop.md): recorded
// payloads replayed as dry-runs with assertions, plus one-click retry for
// failed runs. All methods authorize before validating (audit finding #5).
package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Eval is a stored dry-run: the payload to replay and what to assert.
type Eval struct {
	ID         string          `json:"id"`
	WorkflowID string          `json:"workflowId"`
	Name       string          `json:"name"`
	Payload    map[string]any  `json:"payload"`
	Assertions []Assertion     `json:"assertions"`
	LastRun    *EvalRunSummary `json:"lastRun,omitempty"`
	CreatedAt  time.Time       `json:"createdAt"`
}

// EvalRunSummary is the latest (or requested) execution of an eval.
type EvalRunSummary struct {
	ID        string            `json:"id"`
	Passed    bool              `json:"passed"`
	Results   []AssertionResult `json:"results"`
	CreatedAt time.Time         `json:"createdAt"`
}

var (
	ErrEvalNotFound    = errors.New("eval not found")
	ErrRunNotRetryable = errors.New("only failed or cancelled runs can be retried")
)

// CreateEval stores a dry-run definition against the workflow.
func (s *Service) CreateEval(ctx context.Context, userID, workspaceID, workflowID, name string, payload map[string]any, assertions []Assertion) (Eval, error) {
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return Eval{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 120 {
		return Eval{}, ErrInvalidInput
	}
	if payload == nil {
		payload = map[string]any{}
	}
	if assertions == nil {
		assertions = []Assertion{}
	}
	if err := ValidateAssertions(assertions); err != nil {
		return Eval{}, fmt.Errorf("%w: %s", ErrInvalidDefinition, err)
	}
	assertionsJSON, err := json.Marshal(assertions)
	if err != nil {
		return Eval{}, fmt.Errorf("marshal assertions: %w", err)
	}

	var e Eval
	var payloadRaw, assertionsRaw []byte
	err = s.pool.QueryRow(ctx,
		`INSERT INTO workflow_evals (workspace_id, workflow_id, name, payload, assertions, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id::text, workflow_id::text, name, payload, assertions, created_at`,
		workspaceID, workflowID, name, payload, assertionsJSON, userID,
	).Scan(&e.ID, &e.WorkflowID, &e.Name, &payloadRaw, &assertionsRaw, &e.CreatedAt)
	if err != nil {
		return Eval{}, fmt.Errorf("insert eval: %w", err)
	}
	if len(payloadRaw) > 0 {
		_ = json.Unmarshal(payloadRaw, &e.Payload)
	}
	e.Assertions = []Assertion{}
	if len(assertionsRaw) > 0 {
		_ = json.Unmarshal(assertionsRaw, &e.Assertions)
	}
	return e, nil
}

// CreateEvalFromRun replays a recorded run's trigger payload ("replay
// recorded payloads"): the fastest way to capture a real-world case.
func (s *Service) CreateEvalFromRun(ctx context.Context, userID, workspaceID, workflowID, runID, name string) (Eval, error) {
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return Eval{}, err
	}
	var payload []byte
	err := s.pool.QueryRow(ctx,
		`SELECT trigger_payload FROM workflow_runs
		 WHERE id = $1 AND workflow_id = $2 AND workspace_id = $3`,
		runID, workflowID, workspaceID,
	).Scan(&payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return Eval{}, ErrRunNotFound
	}
	if err != nil {
		return Eval{}, fmt.Errorf("load run: %w", err)
	}
	var decoded map[string]any
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &decoded); err != nil {
			return Eval{}, fmt.Errorf("decode trigger payload: %w", err)
		}
	}
	return s.CreateEval(ctx, userID, workspaceID, workflowID, name, decoded, nil)
}

// ListEvals returns the workflow's evals, newest first, each with its
// latest run result.
func (s *Service) ListEvals(ctx context.Context, userID, workspaceID, workflowID string) ([]Eval, error) {
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx,
		`SELECT e.id::text, e.workflow_id::text, e.name, e.payload, e.assertions, e.created_at
		 FROM workflow_evals e
		 WHERE e.workspace_id = $1 AND e.workflow_id = $2
		 ORDER BY e.created_at DESC`,
		workspaceID, workflowID,
	)
	if err != nil {
		return nil, fmt.Errorf("list evals: %w", err)
	}
	defer rows.Close()

	items := []Eval{}
	for rows.Next() {
		var e Eval
		var payload, assertions []byte
		if err := rows.Scan(&e.ID, &e.WorkflowID, &e.Name, &payload, &assertions, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan eval: %w", err)
		}
		if len(payload) > 0 {
			_ = json.Unmarshal(payload, &e.Payload)
		}
		e.Assertions = []Assertion{}
		if len(assertions) > 0 {
			_ = json.Unmarshal(assertions, &e.Assertions)
		}
		items = append(items, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Attach the latest run to each eval (workflows carry few evals).
	for i := range items {
		latest, err := s.latestEvalRun(ctx, items[i].ID)
		if err != nil {
			return nil, err
		}
		items[i].LastRun = latest
	}
	return items, nil
}

func (s *Service) latestEvalRun(ctx context.Context, evalID string) (*EvalRunSummary, error) {
	var run EvalRunSummary
	var results []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id::text, passed, results, created_at
		 FROM workflow_eval_runs WHERE eval_id = $1
		 ORDER BY created_at DESC LIMIT 1`, evalID,
	).Scan(&run.ID, &run.Passed, &results, &run.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("latest eval run: %w", err)
	}
	run.Results = []AssertionResult{}
	if len(results) > 0 {
		_ = json.Unmarshal(results, &run.Results)
	}
	return &run, nil
}

// RunEval simulates the workflow against the eval's payload, checks the
// assertions, and stores the result. No side effects ever execute.
func (s *Service) RunEval(ctx context.Context, userID, workspaceID, workflowID, evalID string) (EvalRunSummary, error) {
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return EvalRunSummary{}, err
	}
	var payload, assertions []byte
	err := s.pool.QueryRow(ctx,
		`SELECT payload, assertions FROM workflow_evals
		 WHERE id = $1 AND workflow_id = $2 AND workspace_id = $3`,
		evalID, workflowID, workspaceID,
	).Scan(&payload, &assertions)
	if errors.Is(err, pgx.ErrNoRows) {
		return EvalRunSummary{}, ErrEvalNotFound
	}
	if err != nil {
		return EvalRunSummary{}, fmt.Errorf("load eval: %w", err)
	}

	_, def, err := s.load(ctx, workspaceID, workflowID)
	if err != nil {
		return EvalRunSummary{}, err
	}

	var decodedPayload map[string]any
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &decodedPayload); err != nil {
			return EvalRunSummary{}, fmt.Errorf("decode eval payload: %w", err)
		}
	}
	decodedAssertions := []Assertion{}
	if len(assertions) > 0 {
		_ = json.Unmarshal(assertions, &decodedAssertions)
	}

	sim := Simulate(&def, decodedPayload)
	results := EvaluateAssertions(sim, decodedAssertions)
	passed := true
	for _, r := range results {
		if !r.Passed {
			passed = false
			break
		}
	}
	resultsJSON, err := json.Marshal(results)
	if err != nil {
		return EvalRunSummary{}, fmt.Errorf("marshal results: %w", err)
	}

	var run EvalRunSummary
	if err := s.pool.QueryRow(ctx,
		`INSERT INTO workflow_eval_runs (eval_id, workspace_id, passed, results)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id::text, passed, results, created_at`,
		evalID, workspaceID, passed, resultsJSON,
	).Scan(&run.ID, &run.Passed, &run.Results, &run.CreatedAt); err != nil {
		return EvalRunSummary{}, fmt.Errorf("insert eval run: %w", err)
	}
	run.Results = results
	return run, nil
}

// DeleteEval removes an eval and its run history.
func (s *Service) DeleteEval(ctx context.Context, userID, workspaceID, workflowID, evalID string) error {
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM workflow_evals WHERE id = $1 AND workflow_id = $2 AND workspace_id = $3`,
		evalID, workflowID, workspaceID,
	)
	if err != nil {
		return fmt.Errorf("delete eval: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrEvalNotFound
	}
	return nil
}

// RetryRun re-enqueues a failed or cancelled run with its original
// trigger payload — one-click self-healing.
func (s *Service) RetryRun(ctx context.Context, userID, workspaceID, workflowID, runID string) (Run, error) {
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return Run{}, err
	}
	var status string
	var payload []byte
	err := s.pool.QueryRow(ctx,
		`SELECT status, trigger_payload FROM workflow_runs
		 WHERE id = $1 AND workflow_id = $2 AND workspace_id = $3`,
		runID, workflowID, workspaceID,
	).Scan(&status, &payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return Run{}, ErrRunNotFound
	}
	if err != nil {
		return Run{}, fmt.Errorf("load run: %w", err)
	}
	if status != "failed" && status != "cancelled" {
		return Run{}, ErrRunNotRetryable
	}

	wf, _, err := s.load(ctx, workspaceID, workflowID)
	if err != nil {
		return Run{}, err
	}
	if wf.Status != "ready" {
		return Run{}, ErrNotReady
	}
	var decoded map[string]any
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &decoded); err != nil {
			return Run{}, fmt.Errorf("decode trigger payload: %w", err)
		}
	}
	return s.enqueueRun(ctx, wf, "retry", decoded)
}
