// Package workspaces owns workspace creation, listing, and membership.
// Workspaces are the tenant boundary: every query is scoped by membership.
package workspaces

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrNotFound is returned when a workspace does not exist for the caller.
	ErrNotFound = errors.New("workspace not found")
	// ErrInvalidName is returned for empty or oversized names.
	ErrInvalidName = errors.New("workspace name must be 1-80 characters")
)

// Workspace is the public representation of a workspace the caller belongs to.
type Workspace struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
}

// Service manages workspaces and memberships.
type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

var slugInvalid = regexp.MustCompile(`[^a-z0-9-]+`)

// slugFromName derives a URL-friendly slug; the caller guarantees uniqueness
// by appending a suffix when the base slug collides.
func slugFromName(name string) string {
	slug := strings.ToLower(name)
	slug = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '_' {
			return '-'
		}
		return r
	}, slug)
	slug = slugInvalid.ReplaceAllString(slug, "")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "workspace"
	}
	return slug
}

// Create makes a workspace owned by the given user, who becomes its first
// member with the owner role.
func (s *Service) Create(ctx context.Context, userID, name string) (Workspace, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 80 {
		return Workspace{}, ErrInvalidName
	}

	base := slugFromName(name)
	var ws Workspace

	// Attempt the base slug first, then base-<n> on collision. The unique
	// constraint on workspaces.slug arbitrates under concurrency.
	for i := 0; i < 5; i++ {
		slug := base
		if i > 0 {
			slug = fmt.Sprintf("%s-%d", base, i+1)
		}
		err := s.tryCreate(ctx, userID, name, slug, &ws)
		if err == nil {
			return ws, nil
		}
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
			return Workspace{}, err
		}
	}
	return Workspace{}, fmt.Errorf("generate unique slug for %q", name)
}

func (s *Service) tryCreate(ctx context.Context, userID, name, slug string, ws *Workspace) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	err = tx.QueryRow(ctx,
		`INSERT INTO workspaces (name, slug, created_by)
		 VALUES ($1, $2, $3)
		 RETURNING id, name, slug, created_at`,
		name, slug, userID,
	).Scan(&ws.ID, &ws.Name, &ws.Slug, &ws.CreatedAt)
	if err != nil {
		return err
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO memberships (workspace_id, user_id, role) VALUES ($1, $2, 'owner')`,
		ws.ID, userID,
	); err != nil {
		return fmt.Errorf("insert membership: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO audit_logs (workspace_id, actor_user_id, action, resource_type, resource_id)
		 VALUES ($1, $2, 'workspace.created', 'workspace', $3)`,
		ws.ID, userID, ws.ID,
	); err != nil {
		return fmt.Errorf("insert audit row: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	ws.Role = "owner"
	return nil
}

// List returns every workspace the user is a member of, newest first.
func (s *Service) List(ctx context.Context, userID string) ([]Workspace, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT w.id, w.name, w.slug, m.role, w.created_at
		 FROM workspaces w
		 JOIN memberships m ON m.workspace_id = w.id
		 WHERE m.user_id = $1
		 ORDER BY w.created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	defer rows.Close()

	items := []Workspace{}
	for rows.Next() {
		var w Workspace
		if err := rows.Scan(&w.ID, &w.Name, &w.Slug, &w.Role, &w.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan workspace: %w", err)
		}
		items = append(items, w)
	}
	return items, rows.Err()
}

// Get returns a workspace only if the user is a member.
func (s *Service) Get(ctx context.Context, userID, workspaceID string) (Workspace, error) {
	var ws Workspace
	err := s.pool.QueryRow(ctx,
		`SELECT w.id, w.name, w.slug, m.role, w.created_at
		 FROM workspaces w
		 JOIN memberships m ON m.workspace_id = w.id
		 WHERE w.id = $1 AND m.user_id = $2`,
		workspaceID, userID,
	).Scan(&ws.ID, &ws.Name, &ws.Slug, &ws.Role, &ws.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Workspace{}, ErrNotFound
	}
	if err != nil {
		return Workspace{}, fmt.Errorf("get workspace: %w", err)
	}
	return ws, nil
}

// Membership returns the caller's role, or ErrNotFound if they are not a member.
func (s *Service) Membership(ctx context.Context, userID, workspaceID string) (string, error) {
	var role string
	err := s.pool.QueryRow(ctx,
		`SELECT role FROM memberships WHERE workspace_id = $1 AND user_id = $2`,
		workspaceID, userID,
	).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get membership: %w", err)
	}
	return role, nil
}