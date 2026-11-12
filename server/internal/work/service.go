// Package work implements the Work Management module (orbit.work):
// projects, tasks, boards, assignments — with platform events on the task
// lifecycle that feed the automation engine.
package work

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/orbit-org/orbit/server/internal/events"
	"github.com/orbit-org/orbit/server/internal/modules"
)

var (
	ErrNotFound        = errors.New("not found")
	ErrForbidden       = errors.New("you do not have access to this workspace")
	ErrInvalidTitle    = errors.New("title is required (max 500 characters)")
	ErrInvalidName     = errors.New("name is required (max 120 characters)")
	ErrInvalidStatus   = errors.New("status must be todo, in_progress, or done")
	ErrInvalidPriority = errors.New("priority must be low, medium, high, or urgent")
	ErrInvalidDate     = errors.New("due date must be formatted YYYY-MM-DD")
	ErrModuleDisabled  = errors.New("Work Management is not enabled in this workspace")
)

// ModuleID identifies this module in the registry.
const ModuleID = "orbit.work"

// TaskCounts summarises a project's board for lists.
type TaskCounts struct {
	Todo       int `json:"todo"`
	InProgress int `json:"inProgress"`
	Done       int `json:"done"`
}

// Project is a container for tasks.
type Project struct {
	ID          string     `json:"id"`
	WorkspaceID string     `json:"workspaceId"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	CreatedBy   string     `json:"createdBy"`
	TaskCounts  TaskCounts `json:"taskCounts"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	ArchivedAt  *time.Time `json:"archivedAt"`
}

// Task is one unit of work.
type Task struct {
	ID           string     `json:"id"`
	ProjectID    string     `json:"projectId"`
	WorkspaceID  string     `json:"workspaceId"`
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	Status       string     `json:"status"`
	Priority     string     `json:"priority"`
	AssigneeID   string     `json:"assigneeId"`
	AssigneeName string     `json:"assigneeName"`
	DueDate      *string    `json:"dueDate"` // YYYY-MM-DD or null
	Position     int        `json:"position"`
	CompletedAt  *time.Time `json:"completedAt"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

// TaskPatch carries optional updates; nil fields are left unchanged.
type TaskPatch struct {
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	Priority    *string `json:"priority,omitempty"`
	AssigneeID  *string `json:"assigneeId,omitempty"` // "" clears
	DueDate     *string `json:"dueDate,omitempty"`    // "" clears
}

// Service is the Work Management backend.
type Service struct {
	pool   *pgxpool.Pool
	events *events.Service
	mods   *modules.Service
}

func NewService(pool *pgxpool.Pool, events *events.Service, mods *modules.Service) *Service {
	return &Service{pool: pool, events: events, mods: mods}
}

// Enabled reports whether the module is installed and enabled.
func (s *Service) Enabled(ctx context.Context, workspaceID string) bool {
	return s.mods.IsEnabled(ctx, workspaceID, ModuleID)
}

// Cleanup removes module data on uninstall.
func (s *Service) Cleanup(ctx context.Context, workspaceID string) error {
	if _, err := s.pool.Exec(ctx,
		`DELETE FROM module_work.tasks WHERE workspace_id = $1`, workspaceID,
	); err != nil {
		return fmt.Errorf("delete tasks: %w", err)
	}
	if _, err := s.pool.Exec(ctx,
		`DELETE FROM module_work.projects WHERE workspace_id = $1`, workspaceID,
	); err != nil {
		return fmt.Errorf("delete projects: %w", err)
	}
	return nil
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

// --- projects ---

// CreateProject makes a project and emits project.created transactionally.
func (s *Service) CreateProject(ctx context.Context, userID, workspaceID, name, description string) (Project, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 120 {
		return Project{}, ErrInvalidName
	}
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return Project{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Project{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	var p Project
	if err := tx.QueryRow(ctx,
		`INSERT INTO module_work.projects (workspace_id, name, description, created_by)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id::text, workspace_id::text, name, description, created_by::text, created_at, updated_at`,
		workspaceID, name, strings.TrimSpace(description), userID,
	).Scan(&p.ID, &p.WorkspaceID, &p.Name, &p.Description, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return Project{}, fmt.Errorf("insert project: %w", err)
	}

	if err := s.events.PublishTx(ctx, tx, events.Event{
		Type: "project.created", WorkspaceID: workspaceID, ActorUserID: userID,
		Payload: map[string]any{"projectId": p.ID, "name": p.Name},
	}); err != nil {
		return Project{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Project{}, fmt.Errorf("commit: %w", err)
	}
	return p, nil
}

// ListProjects returns the workspace's unarchived projects with board counts.
func (s *Service) ListProjects(ctx context.Context, userID, workspaceID string) ([]Project, error) {
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx,
		`SELECT p.id::text, p.workspace_id::text, p.name, p.description, p.created_by::text,
		        p.created_at, p.updated_at,
		        count(*) FILTER (WHERE t.status = 'todo')::int,
		        count(*) FILTER (WHERE t.status = 'in_progress')::int,
		        count(*) FILTER (WHERE t.status = 'done')::int
		 FROM module_work.projects p
		 LEFT JOIN module_work.tasks t ON t.project_id = p.id
		 WHERE p.workspace_id = $1 AND p.archived_at IS NULL
		 GROUP BY p.id
		 ORDER BY p.created_at DESC`,
		workspaceID,
	)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()
	items := []Project{}
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.WorkspaceID, &p.Name, &p.Description, &p.CreatedBy,
			&p.CreatedAt, &p.UpdatedAt, &p.TaskCounts.Todo, &p.TaskCounts.InProgress, &p.TaskCounts.Done); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		items = append(items, p)
	}
	return items, rows.Err()
}

// GetProject returns one project (with counts), enforcing workspace scope.
func (s *Service) GetProject(ctx context.Context, userID, workspaceID, projectID string) (Project, error) {
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return Project{}, err
	}
	row := s.pool.QueryRow(ctx,
		`SELECT p.id::text, p.workspace_id::text, p.name, p.description, p.created_by::text,
		        p.created_at, p.updated_at,
		        count(t.id) FILTER (WHERE t.status = 'todo')::int,
		        count(t.id) FILTER (WHERE t.status = 'in_progress')::int,
		        count(t.id) FILTER (WHERE t.status = 'done')::int
		 FROM module_work.projects p
		 LEFT JOIN module_work.tasks t ON t.project_id = p.id
		 WHERE p.id = $1 AND p.workspace_id = $2 AND p.archived_at IS NULL
		 GROUP BY p.id`,
		projectID, workspaceID,
	)
	var p Project
	err := row.Scan(&p.ID, &p.WorkspaceID, &p.Name, &p.Description, &p.CreatedBy,
		&p.CreatedAt, &p.UpdatedAt, &p.TaskCounts.Todo, &p.TaskCounts.InProgress, &p.TaskCounts.Done)
	if errors.Is(err, pgx.ErrNoRows) {
		return Project{}, ErrNotFound
	}
	if err != nil {
		return Project{}, fmt.Errorf("get project: %w", err)
	}
	return p, nil
}

func (s *Service) projectWorkspace(ctx context.Context, projectID string) (string, error) {
	var wsID string
	err := s.pool.QueryRow(ctx,
		`SELECT workspace_id::text FROM module_work.projects WHERE id = $1`, projectID,
	).Scan(&wsID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return wsID, err
}

// --- tasks ---

// CreateTask adds a task and emits task.created (plus task.assigned when an
// assignee is given) transactionally.
func (s *Service) CreateTask(ctx context.Context, userID, workspaceID, projectID string, title string, patch TaskPatch) (Task, error) {
	title = strings.TrimSpace(title)
	if title == "" || len(title) > 500 {
		return Task{}, ErrInvalidTitle
	}
	if err := s.validatePatch(ctx, workspaceID, patch); err != nil {
		return Task{}, err
	}
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return Task{}, err
	}
	if ws, err := s.projectWorkspace(ctx, projectID); err != nil {
		return Task{}, err
	} else if ws != workspaceID {
		return Task{}, ErrNotFound
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Task{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	var t Task
	var description, assigneeID string
	var dueDate *string
	if patch.Description != nil {
		description = *patch.Description
	}
	if patch.AssigneeID != nil && *patch.AssigneeID != "" {
		assigneeID = *patch.AssigneeID
	}
	dueDate = patch.DueDate
	if dueDate != nil && *dueDate == "" {
		dueDate = nil
	}

	if err := tx.QueryRow(ctx,
		`INSERT INTO module_work.tasks (project_id, workspace_id, title, description, priority, assignee_id, due_date, position, created_by)
		 VALUES ($1, $2, $3, $4, $5, NULLIF($6, '')::uuid, NULLIF($7, '')::date,
		         COALESCE((SELECT max(position) + 1 FROM module_work.tasks WHERE project_id = $1 AND status = 'todo'), 1),
		         $8)
		 RETURNING id`,
		projectID, workspaceID, title, description, priorityOr(patch.Priority, "medium"), assigneeID, strPtrToVal(dueDate), userID,
	).Scan(&t.ID); err != nil {
		return Task{}, fmt.Errorf("insert task: %w", err)
	}

	if err := s.events.PublishTx(ctx, tx, events.Event{
		Type: "task.created", WorkspaceID: workspaceID, ActorUserID: userID,
		Payload: map[string]any{"taskId": t.ID, "projectId": t.ProjectID, "title": t.Title},
	}); err != nil {
		return Task{}, err
	}
	if t.AssigneeID != "" {
		if err := s.events.PublishTx(ctx, tx, events.Event{
			Type: "task.assigned", WorkspaceID: workspaceID, ActorUserID: userID,
			Payload: map[string]any{"taskId": t.ID, "assigneeId": t.AssigneeID, "title": t.Title},
		}); err != nil {
			return Task{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Task{}, fmt.Errorf("commit: %w", err)
	}
	// Re-load after commit to pick up the enriched row (assignee name).
	return s.loadTask(ctx, workspaceID, t.ID)
}

// ListTasks returns a project's tasks, optionally filtered by status.
func (s *Service) ListTasks(ctx context.Context, userID, workspaceID, projectID, status string) ([]Task, error) {
	if status != "" && status != "todo" && status != "in_progress" && status != "done" {
		return nil, ErrInvalidStatus
	}
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return nil, err
	}
	if ws, err := s.projectWorkspace(ctx, projectID); err != nil {
		return nil, err
	} else if ws != workspaceID {
		return nil, ErrNotFound
	}

	rows, err := s.pool.Query(ctx,
		`SELECT t.id::text, t.project_id::text, t.workspace_id::text, t.title, t.description,
		        t.status, t.priority, COALESCE(t.assignee_id::text, ''), COALESCE(u.display_name, ''),
		        t.due_date::text, t.position, t.completed_at, t.created_at, t.updated_at
		 FROM module_work.tasks t
		 LEFT JOIN users u ON u.id = t.assignee_id
		 WHERE t.project_id = $1 AND ($2 = '' OR t.status = $2)
		 ORDER BY t.position, t.created_at`,
		projectID, status,
	)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()
	items := []Task{}
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, t)
	}
	return items, rows.Err()
}

// GetTask loads one task, enforcing workspace scope.
func (s *Service) GetTask(ctx context.Context, userID, workspaceID, taskID string) (Task, error) {
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return Task{}, err
	}
	return s.loadTask(ctx, workspaceID, taskID)
}

func (s *Service) loadTask(ctx context.Context, workspaceID, taskID string) (Task, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT t.id::text, t.project_id::text, t.workspace_id::text, t.title, t.description,
		        t.status, t.priority, COALESCE(t.assignee_id::text, ''), COALESCE(u.display_name, ''),
		        t.due_date::text, t.position, t.completed_at, t.created_at, t.updated_at
		 FROM module_work.tasks t
		 LEFT JOIN users u ON u.id = t.assignee_id
		 WHERE t.id = $1 AND t.workspace_id = $2`,
		taskID, workspaceID,
	)
	t, err := scanTask(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	return t, err
}

// UpdateTask applies a patch. Assignee changes emit task.assigned.
// Status is changed through SetTaskStatus, never here.
func (s *Service) UpdateTask(ctx context.Context, userID, workspaceID, taskID string, patch TaskPatch) (Task, error) {
	if err := s.validatePatch(ctx, workspaceID, patch); err != nil {
		return Task{}, err
	}
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return Task{}, err
	}
	current, err := s.loadTask(ctx, workspaceID, taskID)
	if err != nil {
		return Task{}, err
	}
	if patch.Title != nil {
		t := strings.TrimSpace(*patch.Title)
		if t == "" || len(t) > 500 {
			return Task{}, ErrInvalidTitle
		}
		current.Title = t
	}
	if patch.Description != nil {
		current.Description = strings.TrimSpace(*patch.Description)
	}
	if patch.Priority != nil && *patch.Priority != "" {
		current.Priority = *patch.Priority
	}
	var newAssignee string
	assigneeChanged := false
	if patch.AssigneeID != nil {
		newAssignee = strings.TrimSpace(*patch.AssigneeID)
		assigneeChanged = newAssignee != current.AssigneeID
		current.AssigneeID = newAssignee
	}
	var dueDate *string
	if patch.DueDate != nil {
		if *patch.DueDate == "" {
			dueDate = nil
		} else {
			dueDate = patch.DueDate
		}
	} else if current.DueDate != nil {
		dueDate = current.DueDate
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Task{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`UPDATE module_work.tasks
		 SET title = $3, description = $4, priority = $5,
		     assignee_id = NULLIF($6, '')::uuid,
		     due_date = NULLIF($7, '')::date,
		     updated_at = now()
		 WHERE id = $1 AND workspace_id = $2`,
		taskID, workspaceID, current.Title, current.Description, current.Priority,
		current.AssigneeID, strPtrToVal(dueDate),
	); err != nil {
		return Task{}, fmt.Errorf("update task: %w", err)
	}

	if assigneeChanged && current.AssigneeID != "" {
		if err := s.events.PublishTx(ctx, tx, events.Event{
			Type: "task.assigned", WorkspaceID: workspaceID, ActorUserID: userID,
			Payload: map[string]any{"taskId": taskID, "assigneeId": current.AssigneeID, "title": current.Title},
		}); err != nil {
			return Task{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Task{}, fmt.Errorf("commit: %w", err)
	}
	return s.loadTask(ctx, workspaceID, taskID)
}

// SetTaskStatus moves a task on the board. Transitioning into done emits
// task.completed; leaving done emits task.reopened. todo <-> in_progress is
// silent (no event firehose for non-lifecycle moves).
func (s *Service) SetTaskStatus(ctx context.Context, userID, workspaceID, taskID, status string) (Task, error) {
	if status != "todo" && status != "in_progress" && status != "done" {
		return Task{}, ErrInvalidStatus
	}
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return Task{}, err
	}
	current, err := s.loadTask(ctx, workspaceID, taskID)
	if err != nil {
		return Task{}, err
	}
	if current.Status == status {
		return current, nil
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Task{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`UPDATE module_work.tasks
		 SET status = $3,
		     completed_at = CASE WHEN $3 = 'done' THEN now() ELSE NULL END,
		     position = COALESCE((SELECT max(position) + 1 FROM module_work.tasks
		                          WHERE project_id = $4 AND status = $3), 1),
		     updated_at = now()
		 WHERE id = $1 AND workspace_id = $2`,
		taskID, workspaceID, status, current.ProjectID,
	); err != nil {
		return Task{}, fmt.Errorf("set status: %w", err)
	}

	evtType := ""
	switch {
	case status == "done":
		evtType = "task.completed"
	case current.Status == "done":
		evtType = "task.reopened"
	}
	if evtType != "" {
		if err := s.events.PublishTx(ctx, tx, events.Event{
			Type: evtType, WorkspaceID: workspaceID, ActorUserID: userID,
			Payload: map[string]any{
				"taskId": taskID, "projectId": current.ProjectID,
				"title": current.Title, "assigneeId": current.AssigneeID,
			},
		}); err != nil {
			return Task{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Task{}, fmt.Errorf("commit: %w", err)
	}
	return s.loadTask(ctx, workspaceID, taskID)
}

// DeleteTask removes a task.
func (s *Service) DeleteTask(ctx context.Context, userID, workspaceID, taskID string) error {
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM module_work.tasks WHERE id = $1 AND workspace_id = $2`,
		taskID, workspaceID,
	)
	if err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// --- helpers ---

type rowScanner interface{ Scan(dest ...any) error }

func scanTask(row rowScanner) (Task, error) {
	var t Task
	err := row.Scan(&t.ID, &t.ProjectID, &t.WorkspaceID, &t.Title, &t.Description,
		&t.Status, &t.Priority, &t.AssigneeID, &t.AssigneeName,
		&t.DueDate, &t.Position, &t.CompletedAt, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}

// validatePatch checks patch values, including that an assignee is a member
// of the workspace.
func (s *Service) validatePatch(ctx context.Context, workspaceID string, patch TaskPatch) error {
	if patch.Priority != nil && *patch.Priority != "" &&
		*patch.Priority != "low" && *patch.Priority != "medium" && *patch.Priority != "high" && *patch.Priority != "urgent" {
		return ErrInvalidPriority
	}
	if patch.DueDate != nil && *patch.DueDate != "" {
		if _, err := time.Parse("2006-01-02", *patch.DueDate); err != nil {
			return ErrInvalidDate
		}
	}
	if patch.AssigneeID != nil && *patch.AssigneeID != "" {
		var member bool
		if err := s.pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM memberships WHERE workspace_id = $1 AND user_id = $2)`,
			workspaceID, *patch.AssigneeID,
		).Scan(&member); err != nil {
			return fmt.Errorf("check assignee: %w", err)
		}
		if !member {
			return fmt.Errorf("%w: assignee must be a workspace member", ErrForbidden)
		}
	}
	return nil
}

func priorityOr(p *string, fallback string) string {
	if p == nil || *p == "" {
		return fallback
	}
	return *p
}

func strPtrToVal(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
