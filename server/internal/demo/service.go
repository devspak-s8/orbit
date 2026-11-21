// Package demo is the first end-to-end module: it owns a table in its own
// schema, exposes member routes, and publishes events transactionally with
// its writes. It exists to prove the module pipeline (PRD acceptance
// criteria 6-9).
package demo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/orbit-org/orbit/server/internal/events"
	"github.com/orbit-org/orbit/server/internal/modules"
)

// ErrTitleRequired is returned when adding a record without a title.
var ErrTitleRequired = errors.New("title is required")

// Record is one demo record.
type Record struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"createdAt"`
}

// Service is the Demo module backend.
type Service struct {
	pool   *pgxpool.Pool
	events *events.Service
	mods   *modules.Service
}

func NewService(pool *pgxpool.Pool, events *events.Service, mods *modules.Service) *Service {
	return &Service{pool: pool, events: events, mods: mods}
}

// requireMember verifies the caller is a member of the workspace.
// SECURITY (audit finding #3): the demo gate previously checked only
// authentication and module status, letting any logged-in user read or
// write another workspace's records.
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
		return errors.New("not a member")
	}
	return nil
}

// Enabled reports whether the Demo module is installed and enabled in the
// workspace; handlers use this to gate access.
func (s *Service) Enabled(ctx context.Context, workspaceID string) bool {
	return s.mods.IsEnabled(ctx, workspaceID, "orbit.demo")
}

// List returns this workspace's records, newest first.
func (s *Service) List(ctx context.Context, workspaceID string) ([]Record, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, title, created_at
		 FROM module_demo.records
		 WHERE workspace_id = $1
		 ORDER BY created_at DESC, id DESC`,
		workspaceID,
	)
	if err != nil {
		return nil, fmt.Errorf("list demo records: %w", err)
	}
	defer rows.Close()

	items := []Record{}
	for rows.Next() {
		var r Record
		if err := rows.Scan(&r.ID, &r.Title, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan demo record: %w", err)
		}
		items = append(items, r)
	}
	return items, rows.Err()
}

// Add inserts a record and publishes demo.record.created in the same
// transaction, so the event is exactly as durable as the record itself.
func (s *Service) Add(ctx context.Context, workspaceID, userID, title string) (Record, error) {
	if title == "" {
		return Record{}, ErrTitleRequired
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Record{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	var r Record
	if err := tx.QueryRow(ctx,
		`INSERT INTO module_demo.records (workspace_id, title, created_by)
		 VALUES ($1, $2, $3)
		 RETURNING id::text, title, created_at`,
		workspaceID, title, userID,
	).Scan(&r.ID, &r.Title, &r.CreatedAt); err != nil {
		return Record{}, fmt.Errorf("insert demo record: %w", err)
	}

	if err := s.events.PublishTx(ctx, tx, events.Event{
		Type:        "demo.record.created",
		WorkspaceID: workspaceID,
		ActorUserID: userID,
		Payload:     map[string]any{"recordId": r.ID, "title": title},
	}); err != nil {
		return Record{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Record{}, fmt.Errorf("commit: %w", err)
	}
	return r, nil
}

// Cleanup removes the module's data for a workspace on uninstall.
func (s *Service) Cleanup(ctx context.Context, workspaceID string) error {
	if _, err := s.pool.Exec(ctx,
		`DELETE FROM module_demo.records WHERE workspace_id = $1`,
		workspaceID,
	); err != nil {
		return fmt.Errorf("delete demo records: %w", err)
	}
	return nil
}
