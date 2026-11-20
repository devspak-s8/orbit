// Package docs implements the Docs module (orbit.docs): pages and notes
// on projects — the wiki half of the workspace, with platform events.
package docs

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
	ErrNotFound     = errors.New("not found")
	ErrForbidden    = errors.New("you do not have access to this workspace")
	ErrInvalidTitle = errors.New("title is required (max 200 characters)")
	ErrModuleOff    = errors.New("Docs is not enabled in this workspace")
)

const ModuleID = "orbit.docs"

// Page is one document attached to a project.
type Page struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"projectId"`
	WorkspaceID string    `json:"workspaceId"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	CreatedBy   string    `json:"createdBy"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// Service is the Docs module backend.
type Service struct {
	pool   *pgxpool.Pool
	events *events.Service
	mods   *modules.Service
}

func NewService(pool *pgxpool.Pool, events *events.Service, mods *modules.Service) *Service {
	return &Service{pool: pool, events: events, mods: mods}
}

func (s *Service) Enabled(ctx context.Context, workspaceID string) bool {
	return s.mods.IsEnabled(ctx, workspaceID, ModuleID)
}

func (s *Service) Cleanup(ctx context.Context, workspaceID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM module_docs.pages WHERE workspace_id = $1`, workspaceID)
	return err
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

// CreatePage makes a page on a project and publishes page.created.
func (s *Service) CreatePage(ctx context.Context, userID, workspaceID, projectID, title, body string) (Page, error) {
	title = strings.TrimSpace(title)
	if title == "" || len(title) > 200 {
		return Page{}, ErrInvalidTitle
	}
	if len(body) > 100_000 {
		return Page{}, errors.New("body must be at most 100,000 characters")
	}
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return Page{}, err
	}
	// Project must be in this workspace.
	var wsID string
	err := s.pool.QueryRow(ctx,
		`SELECT workspace_id::text FROM module_work.projects WHERE id = $1`, projectID,
	).Scan(&wsID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && wsID != workspaceID) {
		return Page{}, ErrNotFound
	}
	if err != nil {
		return Page{}, fmt.Errorf("check project: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Page{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	var p Page
	if err := tx.QueryRow(ctx,
		`INSERT INTO module_docs.pages (project_id, workspace_id, title, body, created_by)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id::text, project_id::text, workspace_id::text, title, body, COALESCE(created_by::text, ''), created_at, updated_at`,
		projectID, workspaceID, title, body, userID,
	).Scan(&p.ID, &p.ProjectID, &p.WorkspaceID, &p.Title, &p.Body, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return Page{}, fmt.Errorf("insert page: %w", err)
	}
	if err := s.events.PublishTx(ctx, tx, events.Event{
		Type: "page.created", WorkspaceID: workspaceID, ActorUserID: userID,
		Payload: map[string]any{"pageId": p.ID, "projectId": projectID, "title": title},
	}); err != nil {
		return Page{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Page{}, fmt.Errorf("commit: %w", err)
	}
	return p, nil
}

// UpdatePage edits a page and publishes page.updated.
func (s *Service) UpdatePage(ctx context.Context, userID, workspaceID, pageID, title, body string) (Page, error) {
	title = strings.TrimSpace(title)
	if title == "" || len(title) > 200 {
		return Page{}, ErrInvalidTitle
	}
	if len(body) > 100_000 {
		return Page{}, errors.New("body must be at most 100,000 characters")
	}
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return Page{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Page{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	var p Page
	err = tx.QueryRow(ctx,
		`UPDATE module_docs.pages SET title = $3, body = $4, updated_at = now()
		 WHERE id = $1 AND workspace_id = $2
		 RETURNING id::text, project_id::text, workspace_id::text, title, body, COALESCE(created_by::text, ''), created_at, updated_at`,
		pageID, workspaceID, title, body,
	).Scan(&p.ID, &p.ProjectID, &p.WorkspaceID, &p.Title, &p.Body, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Page{}, ErrNotFound
	}
	if err != nil {
		return Page{}, fmt.Errorf("update page: %w", err)
	}
	if err := s.events.PublishTx(ctx, tx, events.Event{
		Type: "page.updated", WorkspaceID: workspaceID, ActorUserID: userID,
		Payload: map[string]any{"pageId": p.ID, "projectId": p.ProjectID, "title": title},
	}); err != nil {
		return Page{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Page{}, fmt.Errorf("commit: %w", err)
	}
	return p, nil
}

// DeletePage removes a page.
func (s *Service) DeletePage(ctx context.Context, userID, workspaceID, pageID string) error {
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM module_docs.pages WHERE id = $1 AND workspace_id = $2`,
		pageID, workspaceID,
	)
	if err != nil {
		return fmt.Errorf("delete page: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GetPage returns one page.
func (s *Service) GetPage(ctx context.Context, userID, workspaceID, pageID string) (Page, error) {
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return Page{}, err
	}
	var p Page
	err := s.pool.QueryRow(ctx,
		`SELECT id::text, project_id::text, workspace_id::text, title, body, COALESCE(created_by::text, ''), created_at, updated_at
		 FROM module_docs.pages WHERE id = $1 AND workspace_id = $2`,
		pageID, workspaceID,
	).Scan(&p.ID, &p.ProjectID, &p.WorkspaceID, &p.Title, &p.Body, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Page{}, ErrNotFound
	}
	if err != nil {
		return Page{}, fmt.Errorf("get page: %w", err)
	}
	return p, nil
}

// ListPages returns a project's pages or the workspace's pages, newest first.
func (s *Service) ListPages(ctx context.Context, userID, workspaceID, projectID string) ([]Page, error) {
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, project_id::text, workspace_id::text, title, body, COALESCE(created_by::text, ''), created_at, updated_at
		 FROM module_docs.pages
		 WHERE workspace_id = $1 AND ($2 = '' OR project_id = $2::uuid)
		 ORDER BY updated_at DESC`,
		workspaceID, projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("list pages: %w", err)
	}
	defer rows.Close()
	items := []Page{}
	for rows.Next() {
		var p Page
		if err := rows.Scan(&p.ID, &p.ProjectID, &p.WorkspaceID, &p.Title, &p.Body, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan page: %w", err)
		}
		items = append(items, p)
	}
	return items, rows.Err()
}
