package work

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrSubtaskTitle  = errors.New("subtask title is required (max 500 characters)")
	ErrDependency    = errors.New("dependency invalid: tasks must differ and belong to the same project")
	ErrCycle         = errors.New("adding this dependency would create a cycle")
	ErrTimeRunning   = errors.New("a timer is already running for this task")
	ErrTimeNotActive = errors.New("no active timer on this task")
	ErrTimeMinutes   = errors.New("minutes must be between 1 and 1440")
)

// Subtask is one checklist item on a task.
type Subtask struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"taskId"`
	Title     string    `json:"title"`
	Done      bool      `json:"done"`
	Position  int       `json:"position"`
	CreatedAt time.Time `json:"createdAt"`
}

// DependencyInfo summarizes a task's blockers and dependents.
type DependencyInfo struct {
	BlockedBy []TaskBrief `json:"blockedBy"`
	Blocks    []TaskBrief `json:"blocks"`
}

// TaskBrief is a lightweight task reference for lists.
type TaskBrief struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

// TimeEntry is one logged period of work.
type TimeEntry struct {
	ID        string     `json:"id"`
	TaskID    string     `json:"taskId"`
	UserID    string     `json:"userId"`
	UserName  string     `json:"userName"`
	Minutes   int        `json:"minutes"`
	Note      string     `json:"note"`
	StartedAt time.Time  `json:"startedAt"`
	EndedAt   *time.Time `json:"endedAt"`
}

// --- subtasks ---

func (s *Service) CreateSubtask(ctx context.Context, userID, workspaceID, taskID, title string) (Subtask, error) {
	title = strings.TrimSpace(title)
	if title == "" || len(title) > 500 {
		return Subtask{}, ErrSubtaskTitle
	}
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return Subtask{}, err
	}
	if _, err := s.loadTask(ctx, workspaceID, taskID); err != nil {
		return Subtask{}, err
	}
	var st Subtask
	err := s.pool.QueryRow(ctx,
		`INSERT INTO module_work.subtasks (task_id, workspace_id, title, created_by)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id::text, task_id::text, title, done, position, created_at`,
		taskID, workspaceID, title, userID,
	).Scan(&st.ID, &st.TaskID, &st.Title, &st.Done, &st.Position, &st.CreatedAt)
	if err != nil {
		return Subtask{}, fmt.Errorf("insert subtask: %w", err)
	}
	return st, nil
}

func (s *Service) ListSubtasks(ctx context.Context, userID, workspaceID, taskID string) ([]Subtask, error) {
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, task_id::text, title, done, position, created_at
		 FROM module_work.subtasks WHERE task_id = $1 AND workspace_id = $2
		 ORDER BY position, created_at`,
		taskID, workspaceID,
	)
	if err != nil {
		return nil, fmt.Errorf("list subtasks: %w", err)
	}
	defer rows.Close()
	items := []Subtask{}
	for rows.Next() {
		var st Subtask
		if err := rows.Scan(&st.ID, &st.TaskID, &st.Title, &st.Done, &st.Position, &st.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan subtask: %w", err)
		}
		items = append(items, st)
	}
	return items, rows.Err()
}

func (s *Service) UpdateSubtask(ctx context.Context, userID, workspaceID, subtaskID string, title *string, done *bool) (Subtask, error) {
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return Subtask{}, err
	}
	sets := []string{}
	args := []any{subtaskID, workspaceID}
	n := 2
	if title != nil {
		t := strings.TrimSpace(*title)
		if t == "" || len(t) > 500 {
			return Subtask{}, ErrSubtaskTitle
		}
		n++
		sets = append(sets, fmt.Sprintf("title = $%d", n))
		args = append(args, t)
	}
	if done != nil {
		n++
		sets = append(sets, fmt.Sprintf("done = $%d", n))
		args = append(args, *done)
	}
	if len(sets) == 0 {
		return Subtask{}, errors.New("nothing to update")
	}
	var st Subtask
	err := s.pool.QueryRow(ctx,
		"UPDATE module_work.subtasks SET "+strings.Join(sets, ", ")+
			" WHERE id = $1 AND workspace_id = $2"+
			" RETURNING id::text, task_id::text, title, done, position, created_at",
		args...,
	).Scan(&st.ID, &st.TaskID, &st.Title, &st.Done, &st.Position, &st.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Subtask{}, ErrNotFound
	}
	if err != nil {
		return Subtask{}, fmt.Errorf("update subtask: %w", err)
	}
	return st, nil
}

func (s *Service) DeleteSubtask(ctx context.Context, userID, workspaceID, subtaskID string) error {
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM module_work.subtasks WHERE id = $1 AND workspace_id = $2`,
		subtaskID, workspaceID,
	)
	if err != nil {
		return fmt.Errorf("delete subtask: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// --- dependencies ---

// AddDependency records "taskID is blocked by blocksTaskID". Cycles are
// rejected with a recursive check: can blocksTaskID reach taskID by
// following blockers?
func (s *Service) AddDependency(ctx context.Context, userID, workspaceID, taskID, blocksTaskID string) error {
	if taskID == blocksTaskID {
		return ErrDependency
	}
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return err
	}
	blocked, err := s.loadTask(ctx, workspaceID, taskID)
	if err != nil {
		return err
	}
	blocker, err := s.loadTask(ctx, workspaceID, blocksTaskID)
	if err != nil {
		return err
	}
	if blocked.ProjectID != blocker.ProjectID {
		return ErrDependency
	}

	// Cycle check: walk blockers of blocksTaskID; if we ever reach taskID, it's a cycle.
	visited := map[string]bool{}
	var walk func(id string) (bool, error)
	walk = func(id string) (bool, error) {
		if id == taskID {
			return true, nil
		}
		if visited[id] {
			return false, nil
		}
		visited[id] = true
		rows, err := s.pool.Query(ctx,
			`SELECT blocks_task_id::text FROM module_work.dependencies WHERE task_id = $1`, id,
		)
		if err != nil {
			return false, fmt.Errorf("walk dependency: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var next string
			if err := rows.Scan(&next); err != nil {
				return false, err
			}
			found, err := walk(next)
			if err != nil || found {
				return found, err
			}
		}
		return false, rows.Err()
	}
	cycle, err := walk(blocksTaskID)
	if err != nil {
		return err
	}
	if cycle {
		return ErrCycle
	}

	_, err = s.pool.Exec(ctx,
		`INSERT INTO module_work.dependencies (task_id, blocks_task_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		taskID, blocksTaskID,
	)
	if err != nil {
		return fmt.Errorf("insert dependency: %w", err)
	}
	return nil
}

func (s *Service) RemoveDependency(ctx context.Context, userID, workspaceID, taskID, blocksTaskID string) error {
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM module_work.dependencies WHERE task_id = $1 AND blocks_task_id = $2`,
		taskID, blocksTaskID,
	)
	if err != nil {
		return fmt.Errorf("remove dependency: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) DependencyInfo(ctx context.Context, userID, workspaceID, taskID string) (DependencyInfo, error) {
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return DependencyInfo{}, err
	}
	if _, err := s.loadTask(ctx, workspaceID, taskID); err != nil {
		return DependencyInfo{}, err
	}
	info := DependencyInfo{BlockedBy: []TaskBrief{}, Blocks: []TaskBrief{}}

	rows, err := s.pool.Query(ctx,
		`SELECT t.id::text, t.title, t.status
		 FROM module_work.dependencies d
		 JOIN module_work.tasks t ON t.id = d.blocks_task_id
		 WHERE d.task_id = $1`,
		taskID,
	)
	if err != nil {
		return info, fmt.Errorf("list blockers: %w", err)
	}
	for rows.Next() {
		var b TaskBrief
		if err := rows.Scan(&b.ID, &b.Title, &b.Status); err != nil {
			return info, err
		}
		info.BlockedBy = append(info.BlockedBy, b)
	}
	rows.Close()

	rows, err = s.pool.Query(ctx,
		`SELECT t.id::text, t.title, t.status
		 FROM module_work.dependencies d
		 JOIN module_work.tasks t ON t.id = d.task_id
		 WHERE d.blocks_task_id = $1`,
		taskID,
	)
	if err != nil {
		return info, fmt.Errorf("list blocked: %w", err)
	}
	for rows.Next() {
		var b TaskBrief
		if err := rows.Scan(&b.ID, &b.Title, &b.Status); err != nil {
			return info, err
		}
		info.Blocks = append(info.Blocks, b)
	}
	return info, rows.Err()
}

// isTaskBlocked reports whether the task has any incomplete blocker.
func (s *Service) isTaskBlocked(ctx context.Context, taskID string) (bool, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM module_work.dependencies d
		 JOIN module_work.tasks t ON t.id = d.blocks_task_id
		 WHERE d.task_id = $1 AND t.status <> 'done'`,
		taskID,
	).Scan(&count)
	return count > 0, err
}

// --- time tracking ---

// StartTimer opens a running entry on the task for the user.
func (s *Service) StartTimer(ctx context.Context, userID, workspaceID, taskID string) (TimeEntry, error) {
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return TimeEntry{}, err
	}
	if _, err := s.loadTask(ctx, workspaceID, taskID); err != nil {
		return TimeEntry{}, err
	}
	var existing int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM module_work.time_entries WHERE task_id = $1 AND user_id = $2 AND ended_at IS NULL`,
		taskID, userID,
	).Scan(&existing); err != nil {
		return TimeEntry{}, err
	}
	if existing > 0 {
		return TimeEntry{}, ErrTimeRunning
	}
	var e TimeEntry
	var name string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO module_work.time_entries (task_id, workspace_id, user_id)
		 VALUES ($1, $2, $3)
		 RETURNING id::text, task_id::text, user_id::text, minutes, note, started_at, ended_at`,
		taskID, workspaceID, userID,
	).Scan(&e.ID, &e.TaskID, &e.UserID, &e.Minutes, &e.Note, &e.StartedAt, &e.EndedAt)
	if err != nil {
		return TimeEntry{}, fmt.Errorf("start timer: %w", err)
	}
	if err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(display_name, email) FROM users WHERE id = $1`, userID,
	).Scan(&name); err == nil {
		e.UserName = name
	}
	return e, nil
}

// StopTimer closes the user's running entry on the task, computing minutes.
func (s *Service) StopTimer(ctx context.Context, userID, workspaceID, taskID string) (TimeEntry, error) {
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return TimeEntry{}, err
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE module_work.time_entries
		 SET ended_at = now(),
		     minutes = GREATEST(1, EXTRACT(EPOCH FROM (now() - started_at)) / 60)::int
		 WHERE task_id = $1 AND user_id = $2 AND workspace_id = $3 AND ended_at IS NULL`,
		taskID, userID, workspaceID,
	)
	if err != nil {
		return TimeEntry{}, fmt.Errorf("stop timer: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return TimeEntry{}, ErrTimeNotActive
	}
	var e TimeEntry
	err = s.pool.QueryRow(ctx,
		`SELECT e.id::text, e.task_id::text, e.user_id::text, e.minutes, e.note, e.started_at, e.ended_at,
		        COALESCE(u.display_name, '')
		 FROM module_work.time_entries e
		 LEFT JOIN users u ON u.id = e.user_id
		 WHERE e.task_id = $1 AND e.user_id = $2 AND e.ended_at IS NOT NULL
		 ORDER BY e.ended_at DESC LIMIT 1`,
		taskID, userID,
	).Scan(&e.ID, &e.TaskID, &e.UserID, &e.Minutes, &e.Note, &e.StartedAt, &e.EndedAt, &e.UserName)
	if err != nil {
		return TimeEntry{}, fmt.Errorf("fetch stopped: %w", err)
	}
	return e, nil
}

// LogTime adds a manual entry (minutes + note).
func (s *Service) LogTime(ctx context.Context, userID, workspaceID, taskID string, minutes int, note string) (TimeEntry, error) {
	if minutes < 1 || minutes > 1440 {
		return TimeEntry{}, ErrTimeMinutes
	}
	if len(note) > 500 {
		return TimeEntry{}, errors.New("note must be at most 500 characters")
	}
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return TimeEntry{}, err
	}
	if _, err := s.loadTask(ctx, workspaceID, taskID); err != nil {
		return TimeEntry{}, err
	}
	var e TimeEntry
	err := s.pool.QueryRow(ctx,
		`INSERT INTO module_work.time_entries (task_id, workspace_id, user_id, minutes, note, ended_at)
		 VALUES ($1, $2, $3, $4, $5, now())
		 RETURNING id::text, task_id::text, user_id::text, minutes, note, started_at, ended_at`,
		taskID, workspaceID, userID, minutes, strings.TrimSpace(note),
	).Scan(&e.ID, &e.TaskID, &e.UserID, &e.Minutes, &e.Note, &e.StartedAt, &e.EndedAt)
	if err != nil {
		return TimeEntry{}, fmt.Errorf("log time: %w", err)
	}
	_ = s.pool.QueryRow(ctx,
		`SELECT COALESCE(display_name, '') FROM users WHERE id = $1`, userID,
	).Scan(&e.UserName)
	return e, nil
}

// ListTime returns a task's time log, newest first.
func (s *Service) ListTime(ctx context.Context, userID, workspaceID, taskID string) ([]TimeEntry, error) {
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return nil, err
	}
	if _, err := s.loadTask(ctx, workspaceID, taskID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx,
		`SELECT e.id::text, e.task_id::text, e.user_id::text, e.minutes, e.note, e.started_at, e.ended_at,
		        COALESCE(u.display_name, '')
		 FROM module_work.time_entries e
		 LEFT JOIN users u ON u.id = e.user_id
		 WHERE e.task_id = $1 AND e.workspace_id = $2
		 ORDER BY e.ended_at DESC NULLS FIRST, e.created_at DESC`,
		taskID, workspaceID,
	)
	if err != nil {
		return nil, fmt.Errorf("list time: %w", err)
	}
	defer rows.Close()
	items := []TimeEntry{}
	for rows.Next() {
		var e TimeEntry
		if err := rows.Scan(&e.ID, &e.TaskID, &e.UserID, &e.Minutes, &e.Note, &e.StartedAt, &e.EndedAt, &e.UserName); err != nil {
			return nil, fmt.Errorf("scan time: %w", err)
		}
		items = append(items, e)
	}
	return items, rows.Err()
}

// DeleteTimeEntry removes a log entry (own entries only).
func (s *Service) DeleteTimeEntry(ctx context.Context, userID, workspaceID, entryID string) error {
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM module_work.time_entries WHERE id = $1 AND workspace_id = $2 AND user_id = $3`,
		entryID, workspaceID, userID,
	)
	if err != nil {
		return fmt.Errorf("delete time entry: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
