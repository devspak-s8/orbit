package work

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/orbit-org/orbit/server/internal/events"
)

// Comment is one comment on a task.
type Comment struct {
	ID         string    `json:"id"`
	TaskID     string    `json:"taskId"`
	Body       string    `json:"body"`
	AuthorID   string    `json:"authorId"`
	AuthorName string    `json:"authorName"`
	CreatedAt  time.Time `json:"createdAt"`
}

// Label is a colored project label.
type Label struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Color   string `json:"color"`
}

// Label colors are a fixed palette — no user-supplied CSS anywhere.
var labelColors = map[string]bool{
	"emerald": true, "sky": true, "amber": true, "rose": true,
	"violet": true, "lime": true, "neutral": true,
}

// AddComment posts a comment on a task and publishes task.commented
// transactionally; the assignee is notified unless they are the author.
func (s *Service) AddComment(ctx context.Context, userID, workspaceID, taskID, body string) (Comment, error) {
	body = strings.TrimSpace(body)
	if body == "" || len(body) > 4000 {
		return Comment{}, errors.New("comment must be 1-4000 characters")
	}
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return Comment{}, err
	}
	task, err := s.loadTask(ctx, workspaceID, taskID)
	if err != nil {
		return Comment{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Comment{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	var c Comment
	if err := tx.QueryRow(ctx,
		`INSERT INTO module_work.comments (task_id, workspace_id, body, author_id)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id::text, task_id::text, body, author_id::text, created_at`,
		taskID, workspaceID, body, userID,
	).Scan(&c.ID, &c.TaskID, &c.Body, &c.AuthorID, &c.CreatedAt); err != nil {
		return Comment{}, fmt.Errorf("insert comment: %w", err)
	}
	if err := tx.QueryRow(ctx,
		`SELECT display_name FROM users WHERE id = $1`, userID,
	).Scan(&c.AuthorName); err != nil {
		return Comment{}, fmt.Errorf("author name: %w", err)
	}

	snippet := body
	if len(snippet) > 120 {
		snippet = snippet[:120] + "…"
	}
	if err := s.events.PublishTx(ctx, tx, events.Event{
		Type: "task.commented", WorkspaceID: workspaceID, ActorUserID: userID,
		Payload: map[string]any{"taskId": taskID, "title": task.Title, "commentSnippet": snippet},
	}); err != nil {
		return Comment{}, err
	}
	if task.AssigneeID != "" && task.AssigneeID != userID {
		if err := s.notif.InsertTx(ctx, tx, task.AssigneeID, workspaceID, "task.commented", map[string]any{
			"taskId": taskID, "title": task.Title, "commentSnippet": snippet, "actorUserId": userID,
		}); err != nil {
			return Comment{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Comment{}, fmt.Errorf("commit: %w", err)
	}
	return c, nil
}

// ListComments returns a task's comment thread, oldest first.
func (s *Service) ListComments(ctx context.Context, userID, workspaceID, taskID string) ([]Comment, error) {
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return nil, err
	}
	if _, err := s.loadTask(ctx, workspaceID, taskID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx,
		`SELECT c.id::text, c.task_id::text, c.body, c.author_id::text, COALESCE(u.display_name, ''), c.created_at
		 FROM module_work.comments c
		 LEFT JOIN users u ON u.id = c.author_id
		 WHERE c.task_id = $1 AND c.workspace_id = $2
		 ORDER BY c.created_at`,
		taskID, workspaceID,
	)
	if err != nil {
		return nil, fmt.Errorf("list comments: %w", err)
	}
	defer rows.Close()
	items := []Comment{}
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.TaskID, &c.Body, &c.AuthorID, &c.AuthorName, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan comment: %w", err)
		}
		items = append(items, c)
	}
	return items, rows.Err()
}

// CreateLabel adds a colored label to a project.
func (s *Service) CreateLabel(ctx context.Context, userID, workspaceID, projectID, name, color string) (Label, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 40 {
		return Label{}, errors.New("label name must be 1-40 characters")
	}
	if !labelColors[color] {
		return Label{}, errors.New("unknown label color")
	}
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return Label{}, err
	}
	if ws, err := s.projectWorkspace(ctx, projectID); err != nil {
		return Label{}, err
	} else if ws != workspaceID {
		return Label{}, ErrNotFound
	}

	var l Label
	err := s.pool.QueryRow(ctx,
		`INSERT INTO module_work.labels (project_id, workspace_id, name, color)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (project_id, name) DO UPDATE SET color = EXCLUDED.color
		 RETURNING id::text, name, color`,
		projectID, workspaceID, name, color,
	).Scan(&l.ID, &l.Name, &l.Color)
	if err != nil {
		return Label{}, fmt.Errorf("insert label: %w", err)
	}
	return l, nil
}

// ListLabels returns a project's labels.
func (s *Service) ListLabels(ctx context.Context, userID, workspaceID, projectID string) ([]Label, error) {
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return nil, err
	}
	if ws, err := s.projectWorkspace(ctx, projectID); err != nil {
		return nil, err
	} else if ws != workspaceID {
		return nil, ErrNotFound
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, name, color FROM module_work.labels
		 WHERE project_id = $1 AND workspace_id = $2
		 ORDER BY created_at`,
		projectID, workspaceID,
	)
	if err != nil {
		return nil, fmt.Errorf("list labels: %w", err)
	}
	defer rows.Close()
	items := []Label{}
	for rows.Next() {
		var l Label
		if err := rows.Scan(&l.ID, &l.Name, &l.Color); err != nil {
			return nil, fmt.Errorf("scan label: %w", err)
		}
		items = append(items, l)
	}
	return items, rows.Err()
}

// SetTaskLabels replaces a task's label set atomically. Every label must
// belong to the task's project.
func (s *Service) SetTaskLabels(ctx context.Context, userID, workspaceID, taskID string, labelIDs []string) error {
	if len(labelIDs) > 10 {
		return errors.New("at most 10 labels per task")
	}
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return err
	}
	task, err := s.loadTask(ctx, workspaceID, taskID)
	if err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	if len(labelIDs) > 0 {
		// All labels must belong to this project — one bad id rejects all.
		var count int
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM module_work.labels
			 WHERE id = ANY($1::uuid[]) AND project_id = $2`,
			labelIDs, task.ProjectID,
		).Scan(&count); err != nil {
			return fmt.Errorf("check labels: %w", err)
		}
		if count != len(labelIDs) {
			return ErrNotFound
		}
	}

	if _, err := tx.Exec(ctx, `DELETE FROM module_work.task_labels WHERE task_id = $1`, taskID); err != nil {
		return fmt.Errorf("clear labels: %w", err)
	}
	for _, lid := range labelIDs {
		if _, err := tx.Exec(ctx,
			`INSERT INTO module_work.task_labels (task_id, label_id) VALUES ($1, $2)`,
			taskID, lid,
		); err != nil {
			return fmt.Errorf("assign label: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// taskLabels loads labels for tasks in one query (task_id -> labels).
func (s *Service) taskLabels(ctx context.Context, taskIDs []string) (map[string][]Label, error) {
	if len(taskIDs) == 0 {
		return map[string][]Label{}, nil
	}
	rows, err := s.pool.Query(ctx,
		`SELECT tl.task_id::text, l.id::text, l.name, l.color
		 FROM module_work.task_labels tl
		 JOIN module_work.labels l ON l.id = tl.label_id
		 WHERE tl.task_id = ANY($1::uuid[])
		 ORDER BY l.name`,
		taskIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("load task labels: %w", err)
	}
	defer rows.Close()
	out := map[string][]Label{}
	for rows.Next() {
		var taskID string
		var l Label
		if err := rows.Scan(&taskID, &l.ID, &l.Name, &l.Color); err != nil {
			return nil, fmt.Errorf("scan task label: %w", err)
		}
		out[taskID] = append(out[taskID], l)
	}
	return out, rows.Err()
}