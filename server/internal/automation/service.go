package automation

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrWorkflowNotFound = errors.New("workflow not found")
	ErrRunNotFound      = errors.New("run not found")
	ErrNotReady         = errors.New("workflow must be validated (ready) before it can run")
	ErrForbidden        = errors.New("you do not have access to this workspace")
	ErrInvalidInput     = errors.New("invalid input")
	// ErrInvalidDefinition wraps validator output so handlers can map it
	// to a 400 with the full problem list.
	ErrInvalidDefinition = errors.New("invalid definition")
)

// Workflow is a workflow's metadata; the definition lives in versions.
type Workflow struct {
	ID             string      `json:"id"`
	WorkspaceID    string      `json:"workspaceId"`
	Name           string      `json:"name"`
	Status         string      `json:"status"`
	CurrentVersion int         `json:"currentVersion"`
	Definition     *Definition `json:"definition,omitempty"`
	// WebhookToken is the secret path segment for webhook-triggered
	// workflows; present once the workflow is validated.
	WebhookToken string `json:"webhookToken,omitempty"`
	// NextDueAt is the durable schedule cursor for schedule triggers.
	NextDueAt *time.Time `json:"nextDueAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

// Run is one execution of a workflow.
type Run struct {
	ID             string         `json:"id"`
	WorkflowID     string         `json:"workflowId"`
	WorkspaceID    string         `json:"workspaceId"`
	Version        int            `json:"version"`
	Status         string         `json:"status"`
	TriggerType    string         `json:"triggerType"`
	TriggerPayload map[string]any `json:"triggerPayload"`
	Error          string         `json:"error"`
	StartedAt      *time.Time     `json:"startedAt"`
	FinishedAt     *time.Time     `json:"finishedAt"`
	CreatedAt      time.Time      `json:"createdAt"`
}

// Step is one node execution inside a run.
type Step struct {
	RunID      string         `json:"-"`
	Seq        int            `json:"seq"`
	NodeID     string         `json:"nodeId"`
	NodeType   string         `json:"nodeType"`
	Status     string         `json:"status"`
	Input      map[string]any `json:"input"`
	Output     map[string]any `json:"output"`
	Error      string         `json:"error"`
	StartedAt  *time.Time     `json:"startedAt"`
	FinishedAt *time.Time     `json:"finishedAt"`
}

// Service manages workflow definitions and run history.
type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// Create stores a new workflow in draft with its first version.
func (s *Service) Create(ctx context.Context, userID, workspaceID, name string, def Definition) (Workflow, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 120 {
		return Workflow{}, ErrInvalidInput
	}
	if err := def.Validate(); err != nil {
		return Workflow{}, fmt.Errorf("%w: %s", ErrInvalidDefinition, err)
	}
	raw, err := json.Marshal(def)
	if err != nil {
		return Workflow{}, fmt.Errorf("marshal definition: %w", err)
	}
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return Workflow{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Workflow{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	var wf Workflow
	if err := tx.QueryRow(ctx,
		`INSERT INTO workflows (workspace_id, name, created_by)
		 VALUES ($1, $2, $3)
		 RETURNING id::text, workspace_id::text, name, status, current_version, created_at, updated_at`,
		workspaceID, name, userID,
	).Scan(&wf.ID, &wf.WorkspaceID, &wf.Name, &wf.Status, &wf.CurrentVersion, &wf.CreatedAt, &wf.UpdatedAt); err != nil {
		return Workflow{}, fmt.Errorf("insert workflow: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO workflow_versions (workflow_id, version, definition) VALUES ($1, 1, $2)`,
		wf.ID, raw,
	); err != nil {
		return Workflow{}, fmt.Errorf("insert workflow version: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Workflow{}, fmt.Errorf("commit: %w", err)
	}
	return wf, nil
}

// Validate re-checks the current definition and promotes draft â†’ ready.
// Validation never executes anything (PRD 6.2: editing and execution are
// separate concerns).
func (s *Service) Validate(ctx context.Context, userID, workspaceID, workflowID string) (Workflow, error) {
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return Workflow{}, err
	}
	wf, def, err := s.load(ctx, workspaceID, workflowID)
	if err != nil {
		return Workflow{}, err
	}
	if err := def.Validate(); err != nil {
		return Workflow{}, fmt.Errorf("%w: %s", ErrInvalidDefinition, err)
	}

	// Trigger activation side effects (ADR-0005): webhook triggers get
	// their secret token at validation; schedule triggers get their first
	// due time. Both live on the workflow row.
	sets := []string{"status = 'ready'", "updated_at = now()"}
	args := []any{workflowID}
	switch def.Trigger.Type {
	case "webhook":
		if wf.WebhookToken == "" {
			token, err := newWebhookToken()
			if err != nil {
				return Workflow{}, err
			}
			sets = append(sets, fmt.Sprintf("webhook_token = $%d", len(args)+1))
			args = append(args, token)
			wf.WebhookToken = token
		}
	case "schedule":
		// Re-validating resets the cursor: the next fire is a full
		// interval away, never immediate.
		sets = append(sets, fmt.Sprintf("next_due_at = now() + make_interval(secs => $%d)", len(args)+1))
		args = append(args, def.Trigger.IntervalSec)
		due := time.Now().Add(time.Duration(def.Trigger.IntervalSec) * time.Second)
		wf.NextDueAt = &due
	}

	if _, err := s.pool.Exec(ctx,
		`UPDATE workflow_versions SET validated_at = now()
		 WHERE workflow_id = $1 AND version = $2`,
		workflowID, wf.CurrentVersion,
	); err != nil {
		return Workflow{}, fmt.Errorf("mark validated: %w", err)
	}
	if _, err := s.pool.Exec(ctx,
		"UPDATE workflows SET "+strings.Join(sets, ", ")+" WHERE id = $1",
		args...,
	); err != nil {
		return Workflow{}, fmt.Errorf("set ready: %w", err)
	}
	wf.Status = "ready"
	wf.Definition = &def
	return wf, nil
}

// UpdateWorkflow edits a workflow's name and/or definition. The new
// definition is validated and stored as a new version; the workflow drops
// to draft and must be re-validated before it runs again (execution
// safety: no definition changes under a ready workflow).
func (s *Service) UpdateWorkflow(ctx context.Context, userID, workspaceID, workflowID string, name *string, def *Definition) (Workflow, error) {
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return Workflow{}, err
	}
	wf, current, err := s.load(ctx, workspaceID, workflowID)
	if err != nil {
		return Workflow{}, err
	}
	if wf.Status == "archived" {
		return Workflow{}, ErrWorkflowNotFound
	}
	if def != nil {
		if err := def.Validate(); err != nil {
			return Workflow{}, fmt.Errorf("%w: %s", ErrInvalidDefinition, err)
		}
	}
	newName := wf.Name
	if name != nil {
		n := strings.TrimSpace(*name)
		if n == "" || len(n) > 120 {
			return Workflow{}, ErrInvalidInput
		}
		newName = n
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Workflow{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	newVersion := wf.CurrentVersion
	if def != nil {
		newVersion++
		raw, err := json.Marshal(def)
		if err != nil {
			return Workflow{}, fmt.Errorf("marshal definition: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO workflow_versions (workflow_id, version, definition) VALUES ($1, $2, $3)`,
			workflowID, newVersion, raw,
		); err != nil {
			return Workflow{}, fmt.Errorf("insert version: %w", err)
		}
	}
	// Editing always returns the workflow to draft: re-validation is the
	// gate that lets it execute again. Webhook tokens and schedule cursors
	// survive, but accept nothing until ready.
	if _, err := tx.Exec(ctx,
		`UPDATE workflows SET name = $3, current_version = $4, status = 'draft', updated_at = now()
		 WHERE id = $1 AND workspace_id = $2`,
		workflowID, workspaceID, newName, newVersion,
	); err != nil {
		return Workflow{}, fmt.Errorf("update workflow: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Workflow{}, fmt.Errorf("commit: %w", err)
	}

	_ = current
	updated, def2, err := s.load(ctx, workspaceID, workflowID)
	if err != nil {
		return Workflow{}, err
	}
	updated.Definition = &def2
	return updated, nil
}

// CancelRun marks a queued or running run cancelled. The worker observes
// the status between steps and stops; remaining steps are skipped.
func (s *Service) CancelRun(ctx context.Context, userID, workspaceID, workflowID, runID string) error {
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return err
	}
	if _, _, err := s.load(ctx, workspaceID, workflowID); err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE workflow_runs SET status = 'cancelled', finished_at = now()
		 WHERE id = $1 AND workflow_id = $2 AND status IN ('queued', 'running')`,
		runID, workflowID,
	)
	if err != nil {
		return fmt.Errorf("cancel run: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrRunNotFound
	}
	return nil
}

// Get returns a workflow with its current definition.
func (s *Service) Get(ctx context.Context, userID, workspaceID, workflowID string) (Workflow, error) {
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return Workflow{}, err
	}
	wf, def, err := s.load(ctx, workspaceID, workflowID)
	if err != nil {
		return Workflow{}, err
	}
	wf.Definition = &def
	return wf, nil
}

// List returns the workspace's workflows, newest first.
func (s *Service) List(ctx context.Context, userID, workspaceID string) ([]Workflow, error) {
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, workspace_id::text, name, status, current_version, created_at, updated_at
		 FROM workflows WHERE workspace_id = $1
		 ORDER BY updated_at DESC`,
		workspaceID,
	)
	if err != nil {
		return nil, fmt.Errorf("list workflows: %w", err)
	}
	defer rows.Close()
	items := []Workflow{}
	for rows.Next() {
		var wf Workflow
		if err := rows.Scan(&wf.ID, &wf.WorkspaceID, &wf.Name, &wf.Status, &wf.CurrentVersion, &wf.CreatedAt, &wf.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan workflow: %w", err)
		}
		items = append(items, wf)
	}
	return items, rows.Err()
}

// StartRun enqueues a manual run. Only ready workflows can run.
func (s *Service) StartRun(ctx context.Context, userID, workspaceID, workflowID string) (Run, error) {
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return Run{}, err
	}
	wf, _, err := s.load(ctx, workspaceID, workflowID)
	if err != nil {
		return Run{}, err
	}
	if wf.Status != "ready" {
		return Run{}, ErrNotReady
	}
	return s.enqueueRun(ctx, wf, "manual", map[string]any{"actorUserId": userID})
}

// enqueueRun inserts a queued run plus pending step rows for every node,
// in execution order. The worker picks it up from there.
func (s *Service) enqueueRun(ctx context.Context, wf Workflow, triggerType string, payload map[string]any) (Run, error) {
	def := wf.Definition
	if def == nil {
		return Run{}, errors.New("definition missing")
	}
	if payload == nil {
		payload = map[string]any{}
	}

	tx, err := s.pool.Begin(ctx)
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

	for i, node := range def.Order() {
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

// ListRuns returns a workflow's runs, newest first, with cursor pagination.
// `?limit=50&before={runId}` — the cursor is the last run's ID from the
// previous page.
func (s *Service) ListRuns(ctx context.Context, userID, workspaceID, workflowID string, limit int, before string) ([]Run, error) {
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return nil, err
	}
	if _, _, err := s.load(ctx, workspaceID, workflowID); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, workflow_id::text, workspace_id::text, version, status, trigger_type, trigger_payload, error, started_at, finished_at, created_at
		 FROM workflow_runs
		 WHERE workflow_id = $1
		   AND ($2 = '' OR created_at < (SELECT created_at FROM workflow_runs WHERE id = $2::uuid))
		 ORDER BY created_at DESC
		 LIMIT $3`,
		workflowID, before, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	defer rows.Close()
	items := []Run{}
	for rows.Next() {
		var r Run
		if err := rows.Scan(&r.ID, &r.WorkflowID, &r.WorkspaceID, &r.Version, &r.Status, &r.TriggerType, &r.TriggerPayload, &r.Error, &r.StartedAt, &r.FinishedAt, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan run: %w", err)
		}
		items = append(items, r)
	}
	return items, rows.Err()
}

// GetRun returns a run with its steps.
func (s *Service) GetRun(ctx context.Context, userID, workspaceID, workflowID, runID string) (Run, []Step, error) {
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return Run{}, nil, err
	}
	if _, _, err := s.load(ctx, workspaceID, workflowID); err != nil {
		return Run{}, nil, err
	}
	var r Run
	err := s.pool.QueryRow(ctx,
		`SELECT id::text, workflow_id::text, workspace_id::text, version, status, trigger_type, trigger_payload, error, started_at, finished_at, created_at
		 FROM workflow_runs WHERE id = $1 AND workflow_id = $2`,
		runID, workflowID,
	).Scan(&r.ID, &r.WorkflowID, &r.WorkspaceID, &r.Version, &r.Status, &r.TriggerType, &r.TriggerPayload, &r.Error, &r.StartedAt, &r.FinishedAt, &r.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Run{}, nil, ErrRunNotFound
	}
	if err != nil {
		return Run{}, nil, fmt.Errorf("get run: %w", err)
	}

	rows, err := s.pool.Query(ctx,
		`SELECT seq, node_id, node_type, status, input, output, error, started_at, finished_at
		 FROM workflow_steps WHERE run_id = $1 ORDER BY seq`,
		runID,
	)
	if err != nil {
		return Run{}, nil, fmt.Errorf("list steps: %w", err)
	}
	defer rows.Close()
	steps := []Step{}
	for rows.Next() {
		var st Step
		if err := rows.Scan(&st.Seq, &st.NodeID, &st.NodeType, &st.Status, &st.Input, &st.Output, &st.Error, &st.StartedAt, &st.FinishedAt); err != nil {
			return Run{}, nil, fmt.Errorf("scan step: %w", err)
		}
		steps = append(steps, st)
	}
	return r, steps, rows.Err()
}

// load fetches a workflow and its current definition, enforcing workspace scope.
func (s *Service) load(ctx context.Context, workspaceID, workflowID string) (Workflow, Definition, error) {
	var wf Workflow
	var raw []byte
	var token *string
	err := s.pool.QueryRow(ctx,
		`SELECT w.id::text, w.workspace_id::text, w.name, w.status, w.current_version,
		        w.webhook_token, w.next_due_at, w.created_at, w.updated_at, v.definition
		 FROM workflows w
		 JOIN workflow_versions v ON v.workflow_id = w.id AND v.version = w.current_version
		 WHERE w.id = $1 AND w.workspace_id = $2`,
		workflowID, workspaceID,
	).Scan(&wf.ID, &wf.WorkspaceID, &wf.Name, &wf.Status, &wf.CurrentVersion,
		&token, &wf.NextDueAt, &wf.CreatedAt, &wf.UpdatedAt, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return Workflow{}, Definition{}, ErrWorkflowNotFound
	}
	if err != nil {
		return Workflow{}, Definition{}, fmt.Errorf("load workflow: %w", err)
	}
	if token != nil {
		wf.WebhookToken = *token
	}
	var def Definition
	if err := json.Unmarshal(raw, &def); err != nil {
		return Workflow{}, Definition{}, fmt.Errorf("decode definition: %w", err)
	}
	wf.Definition = &def
	return wf, def, nil
}

// newWebhookToken generates the secret path segment for a webhook trigger:
// 32 random bytes, base64url (43 characters).
func newWebhookToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate webhook token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// EnqueueWebhook resolves a webhook token to a ready workflow and enqueues a
// run carrying the delivered payload. Unknown tokens are indistinguishable
// from missing ones (no existence leak).
func (s *Service) EnqueueWebhook(ctx context.Context, token string, payload map[string]any) (Run, Workflow, error) {
	var wf Workflow
	var raw []byte
	err := s.pool.QueryRow(ctx,
		`SELECT w.id::text, w.workspace_id::text, w.name, w.status, w.current_version,
		        w.created_at, w.updated_at, v.definition
		 FROM workflows w
		 JOIN workflow_versions v ON v.workflow_id = w.id AND v.version = w.current_version
		 WHERE w.webhook_token = $1 AND w.status = 'ready'`,
		token,
	).Scan(&wf.ID, &wf.WorkspaceID, &wf.Name, &wf.Status, &wf.CurrentVersion,
		&wf.CreatedAt, &wf.UpdatedAt, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return Run{}, Workflow{}, ErrWorkflowNotFound
	}
	if err != nil {
		return Run{}, Workflow{}, fmt.Errorf("lookup webhook: %w", err)
	}
	var def Definition
	if err := json.Unmarshal(raw, &def); err != nil {
		return Run{}, Workflow{}, fmt.Errorf("decode definition: %w", err)
	}
	// Defense in depth: the trigger behind this token really is a webhook.
	if def.Trigger.Type != "webhook" {
		return Run{}, Workflow{}, ErrWorkflowNotFound
	}
	wf.Definition = &def
	run, err := s.enqueueRun(ctx, wf, "webhook", payload)
	if err != nil {
		return Run{}, Workflow{}, err
	}
	return run, wf, nil
}

func (s *Service) requireMember(ctx context.Context, userID, workspaceID string) error {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM memberships WHERE workspace_id = $1 AND user_id = $2)`,
		workspaceID, userID,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check membership: %w", err)
	}
	if !exists {
		return ErrForbidden
	}
	return nil
}
