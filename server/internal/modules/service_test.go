package modules

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/orbit-org/orbit/server/internal/config"
	"github.com/orbit-org/orbit/server/internal/db"
	"github.com/orbit-org/orbit/server/internal/identity"
	"github.com/orbit-org/orbit/server/internal/workspaces"
)

type env struct {
	mods     *Service
	ownerID  string
	memberID string
	workspace string
	cleanup  func()
	ctx      context.Context
}

// setup builds a workspace with an owner and a plain member.
func setup(t *testing.T) *env {
	t.Helper()
	ctx := context.Background()
	cfg := config.Load()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		t.Fatalf("connect database (is `docker compose up -d db` running?): %v", err)
	}
	if err := db.Migrate(ctx, cfg.DatabaseURL); err != nil {
		pool.Close()
		t.Fatalf("migrate: %v", err)
	}

	ident := identity.NewService(pool, 24*time.Hour)
	wsSvc := workspaces.NewService(pool)
	catalog, err := LoadCatalog()
	if err != nil {
		pool.Close()
		t.Fatalf("LoadCatalog: %v", err)
	}
	mods := NewService(pool, catalog)

	ownerEmail := fmt.Sprintf("mods-owner-%d@orbit.test", time.Now().UnixNano())
	memberEmail := fmt.Sprintf("mods-member-%d@orbit.test", time.Now().UnixNano())
	owner, _, err := ident.Register(ctx, ownerEmail, "Owner", "password123")
	if err != nil {
		pool.Close()
		t.Fatalf("register owner: %v", err)
	}
	member, _, err := ident.Register(ctx, memberEmail, "Member", "password123")
	if err != nil {
		pool.Close()
		t.Fatalf("register member: %v", err)
	}
	ws, err := wsSvc.Create(ctx, owner.ID, fmt.Sprintf("Module Test %d", time.Now().UnixNano()))
	if err != nil {
		pool.Close()
		t.Fatalf("create workspace: %v", err)
	}
	// Add the second user as a plain member.
	if _, err := pool.Exec(ctx,
		`INSERT INTO memberships (workspace_id, user_id, role) VALUES ($1, $2, 'member')`,
		ws.ID, member.ID,
	); err != nil {
		pool.Close()
		t.Fatalf("add member: %v", err)
	}

	cleanup := func() {
		for _, email := range []string{ownerEmail, memberEmail} {
			_, _ = pool.Exec(ctx, `DELETE FROM audit_logs WHERE actor_user_id IN (SELECT id FROM users WHERE email_normalized = $1)`, email)
			_, _ = pool.Exec(ctx, `DELETE FROM sessions WHERE user_id IN (SELECT id FROM users WHERE email_normalized = $1)`, email)
			_, _ = pool.Exec(ctx, `DELETE FROM memberships WHERE user_id IN (SELECT id FROM users WHERE email_normalized = $1)`, email)
			_, _ = pool.Exec(ctx, `DELETE FROM users WHERE email_normalized = $1`, email)
		}
		_, _ = pool.Exec(ctx, `DELETE FROM workspaces WHERE id = $1`, ws.ID)
		pool.Close()
	}

	return &env{mods: mods, ownerID: owner.ID, memberID: member.ID, workspace: ws.ID, cleanup: cleanup, ctx: ctx}
}

func TestModuleLifecycle(t *testing.T) {
	env := setup(t)
	defer env.cleanup()

	// Initially: available, not installed, not enabled.
	items, err := env.mods.Available(env.ctx, env.workspace)
	if err != nil {
		t.Fatalf("Available: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("catalog empty")
	}
	if items[0].Installed || items[0].Enabled {
		t.Errorf("fresh workspace shows module installed/enabled: %+v", items[0])
	}

	// Install: inert until enabled.
	item, err := env.mods.Install(env.ctx, env.ownerID, env.workspace, "orbit.demo")
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !item.Installed || item.Enabled {
		t.Errorf("after install: %+v, want installed=true enabled=false", item)
	}
	if env.mods.IsEnabled(env.ctx, env.workspace, "orbit.demo") {
		t.Error("IsEnabled true before enable")
	}

	// Enable → active.
	item, err = env.mods.Enable(env.ctx, env.ownerID, env.workspace, "orbit.demo")
	if err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if !item.Enabled {
		t.Errorf("after enable: %+v, want enabled=true", item)
	}
	if !env.mods.IsEnabled(env.ctx, env.workspace, "orbit.demo") {
		t.Error("IsEnabled false after enable")
	}

	// Disable → inactive.
	item, err = env.mods.Disable(env.ctx, env.ownerID, env.workspace, "orbit.demo")
	if err != nil {
		t.Fatalf("Disable: %v", err)
	}
	if item.Enabled {
		t.Errorf("after disable: %+v, want enabled=false", item)
	}

	// Uninstall → gone.
	if err := env.mods.Uninstall(env.ctx, env.ownerID, env.workspace, "orbit.demo"); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if env.mods.IsEnabled(env.ctx, env.workspace, "orbit.demo") {
		t.Error("IsEnabled true after uninstall")
	}

	// Lifecycle ops on a non-installed module are conflicts.
	if _, err := env.mods.Enable(env.ctx, env.ownerID, env.workspace, "orbit.demo"); !errors.Is(err, ErrNotInstalled) {
		t.Errorf("Enable after uninstall err = %v, want ErrNotInstalled", err)
	}
}

func TestOwnerEnforcement(t *testing.T) {
	env := setup(t)
	defer env.cleanup()

	if _, err := env.mods.Install(env.ctx, env.memberID, env.workspace, "orbit.demo"); !errors.Is(err, ErrForbidden) {
		t.Errorf("member Install err = %v, want ErrForbidden", err)
	}
	// The owner installs, then the member tries to enable.
	if _, err := env.mods.Install(env.ctx, env.ownerID, env.workspace, "orbit.demo"); err != nil {
		t.Fatalf("owner Install: %v", err)
	}
	if _, err := env.mods.Enable(env.ctx, env.memberID, env.workspace, "orbit.demo"); !errors.Is(err, ErrForbidden) {
		t.Errorf("member Enable err = %v, want ErrForbidden", err)
	}
	if err := env.mods.Uninstall(env.ctx, env.memberID, env.workspace, "orbit.demo"); !errors.Is(err, ErrForbidden) {
		t.Errorf("member Uninstall err = %v, want ErrForbidden", err)
	}
	// A non-member is also forbidden (no existence leak).
	if err := env.mods.Uninstall(env.ctx, "00000000-0000-0000-0000-000000000000", env.workspace, "orbit.demo"); !errors.Is(err, ErrForbidden) {
		t.Errorf("outsider Uninstall err = %v, want ErrForbidden", err)
	}
}

func TestUnknownModule(t *testing.T) {
	env := setup(t)
	defer env.cleanup()

	if _, err := env.mods.Install(env.ctx, env.ownerID, env.workspace, "orbit.nonexistent"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Install unknown err = %v, want ErrNotFound", err)
	}
}