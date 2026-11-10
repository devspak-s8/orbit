package modules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Item is a module as seen from a workspace: catalog metadata merged with
// that workspace's installation state.
type Item struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Installed   bool     `json:"installed"`
	Enabled     bool     `json:"enabled"`
	Frontend    string   `json:"frontend,omitempty"`
	Events      []string `json:"events,omitempty"`
}

// Service owns the module registry and the workspace installation lifecycle.
type Service struct {
	pool    *pgxpool.Pool
	catalog *Catalog
	// uninstallHooks lets first-party modules clean up their workspace data
	// when uninstalled (PRD 7.2: uninstallation must handle user data).
	uninstallHooks map[string]func(ctx context.Context, workspaceID string) error
}

func NewService(pool *pgxpool.Pool, catalog *Catalog) *Service {
	return &Service{pool: pool, catalog: catalog, uninstallHooks: map[string]func(context.Context, string) error{}}
}

// RegisterUninstallHook adds a cleanup callback for a module. Called by
// first-party modules at startup.
func (s *Service) RegisterUninstallHook(moduleID string, fn func(ctx context.Context, workspaceID string) error) {
	s.uninstallHooks[moduleID] = fn
}

// Seed upserts the embedded catalog into the registry. Idempotent; runs at
// startup so the catalog and database stay in lockstep.
func (s *Service) Seed(ctx context.Context) error {
	for _, id := range s.catalog.IDs() {
		m, _ := s.catalog.Get(id)
		if err := s.upsertModule(ctx, m); err != nil {
			return fmt.Errorf("seed module %q: %w", id, err)
		}
	}
	return nil
}

func (s *Service) upsertModule(ctx context.Context, m Manifest) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal manifest %q: %w", m.ID, err)
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO modules (id, name, version, api_version, description, manifest)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (id) DO UPDATE SET
		   name = EXCLUDED.name, version = EXCLUDED.version,
		   api_version = EXCLUDED.api_version, description = EXCLUDED.description,
		   manifest = EXCLUDED.manifest`,
		m.ID, m.Name, m.Version, m.APIVersion, m.Description, raw,
	)
	if err != nil {
		return fmt.Errorf("upsert module %q: %w", m.ID, err)
	}
	return nil
}

// Available lists every catalog module with this workspace's install state.
func (s *Service) Available(ctx context.Context, workspaceID string) ([]Item, error) {
	state := map[string]bool{}
	rows, err := s.pool.Query(ctx,
		`SELECT module_id, enabled FROM module_installations WHERE workspace_id = $1`,
		workspaceID,
	)
	if err != nil {
		return nil, fmt.Errorf("load installations: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var enabled bool
		if err := rows.Scan(&id, &enabled); err != nil {
			return nil, fmt.Errorf("scan installation: %w", err)
		}
		state[id] = enabled
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	items := []Item{}
	for _, id := range s.catalog.IDs() {
		m, _ := s.catalog.Get(id)
		enabled, installed := state[id]
		items = append(items, Item{
			ID:          m.ID,
			Name:        m.Name,
			Version:     m.Version,
			Description: m.Description,
			Installed:   installed,
			Enabled:     enabled,
			Frontend:    m.Entrypoints.Frontend,
			Events:      m.Events,
		})
	}
	return items, nil
}

// Install records a workspace installation. Installation is inert — the
// module must be enabled separately before it executes (PRD 7.2).
func (s *Service) Install(ctx context.Context, userID, workspaceID, moduleID string) (Item, error) {
	if err := s.requireOwner(ctx, userID, workspaceID); err != nil {
		return Item{}, err
	}
	m, ok := s.catalog.Get(moduleID)
	if !ok {
		return Item{}, ErrNotFound
	}
	// Ensure the registry row exists even if Seed has not run in this
	// process (defense in depth; the FK on module_installations demands it).
	if err := s.upsertModule(ctx, m); err != nil {
		return Item{}, err
	}
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO module_installations (workspace_id, module_id)
		 VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		workspaceID, moduleID,
	); err != nil {
		return Item{}, fmt.Errorf("install module: %w", err)
	}
	if err := s.audit(ctx, userID, workspaceID, "module.installed", moduleID); err != nil {
		return Item{}, err
	}
	return Item{
		ID: m.ID, Name: m.Name, Version: m.Version, Description: m.Description,
		Installed: true, Enabled: false, Frontend: m.Entrypoints.Frontend, Events: m.Events,
	}, nil
}

// Enable marks an installed module active.
func (s *Service) Enable(ctx context.Context, userID, workspaceID, moduleID string) (Item, error) {
	return s.setEnabled(ctx, userID, workspaceID, moduleID, true)
}

// Disable marks an installed module inactive.
func (s *Service) Disable(ctx context.Context, userID, workspaceID, moduleID string) (Item, error) {
	return s.setEnabled(ctx, userID, workspaceID, moduleID, false)
}

func (s *Service) setEnabled(ctx context.Context, userID, workspaceID, moduleID string, enabled bool) (Item, error) {
	if err := s.requireOwner(ctx, userID, workspaceID); err != nil {
		return Item{}, err
	}
	m, ok := s.catalog.Get(moduleID)
	if !ok {
		return Item{}, ErrNotFound
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE module_installations SET enabled = $3, updated_at = now()
		 WHERE workspace_id = $1 AND module_id = $2`,
		workspaceID, moduleID, enabled,
	)
	if err != nil {
		return Item{}, fmt.Errorf("set enabled: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return Item{}, ErrNotInstalled
	}
	action := "module.disabled"
	if enabled {
		action = "module.enabled"
	}
	if err := s.audit(ctx, userID, workspaceID, action, moduleID); err != nil {
		return Item{}, err
	}
	return Item{
		ID: m.ID, Name: m.Name, Version: m.Version, Description: m.Description,
		Installed: true, Enabled: enabled, Frontend: m.Entrypoints.Frontend, Events: m.Events,
	}, nil
}

// Uninstall removes the installation and runs the module's cleanup hook so
// the module's workspace data is handled, not orphaned (PRD 7.2).
func (s *Service) Uninstall(ctx context.Context, userID, workspaceID, moduleID string) error {
	if err := s.requireOwner(ctx, userID, workspaceID); err != nil {
		return err
	}
	if _, ok := s.catalog.Get(moduleID); !ok {
		return ErrNotFound
	}
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM module_installations WHERE workspace_id = $1 AND module_id = $2`,
		workspaceID, moduleID,
	)
	if err != nil {
		return fmt.Errorf("uninstall module: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotInstalled
	}
	if hook, ok := s.uninstallHooks[moduleID]; ok {
		if err := hook(ctx, workspaceID); err != nil {
			return fmt.Errorf("module %q cleanup: %w", moduleID, err)
		}
	}
	if err := s.audit(ctx, userID, workspaceID, "module.uninstalled", moduleID); err != nil {
		return err
	}
	return nil
}

// IsEnabled reports whether a module is installed and enabled in the
// workspace. Modules gate their own routes on this.
func (s *Service) IsEnabled(ctx context.Context, workspaceID, moduleID string) bool {
	var enabled bool
	err := s.pool.QueryRow(ctx,
		`SELECT enabled FROM module_installations WHERE workspace_id = $1 AND module_id = $2`,
		workspaceID, moduleID,
	).Scan(&enabled)
	return err == nil && enabled
}

func (s *Service) requireOwner(ctx context.Context, userID, workspaceID string) error {
	var role string
	err := s.pool.QueryRow(ctx,
		`SELECT role FROM memberships WHERE workspace_id = $1 AND user_id = $2`,
		workspaceID, userID,
	).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) || role != "owner" {
		return ErrForbidden
	}
	if err != nil {
		return fmt.Errorf("check membership: %w", err)
	}
	return nil
}

func (s *Service) audit(ctx context.Context, actorUserID, workspaceID, action, moduleID string) error {
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO audit_logs (workspace_id, actor_user_id, action, resource_type, resource_id)
		 VALUES ($1, $2, $3, 'module', $4)`,
		workspaceID, actorUserID, action, moduleID,
	); err != nil {
		return fmt.Errorf("insert audit row: %w", err)
	}
	return nil
}