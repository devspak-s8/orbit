package demo

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/orbit-org/orbit/server/internal/config"
	"github.com/orbit-org/orbit/server/internal/db"
	"github.com/orbit-org/orbit/server/internal/events"
	"github.com/orbit-org/orbit/server/internal/identity"
	"github.com/orbit-org/orbit/server/internal/modules"
	"github.com/orbit-org/orbit/server/internal/workspaces"
)

type env struct {
	demo      *Service
	events    *events.Service
	mods      *modules.Service
	userID    string
	workspace string
	cleanup   func()
}

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
	catalog, err := modules.LoadCatalog()
	if err != nil {
		pool.Close()
		t.Fatalf("LoadCatalog: %v", err)
	}
	mods := modules.NewService(pool, catalog)
	eventsSvc := events.NewService(pool)
	demoSvc := NewService(pool, eventsSvc, mods)

	email := fmt.Sprintf("demo-%d@orbit.test", time.Now().UnixNano())
	user, _, err := ident.Register(ctx, email, "Demo User", "password123")
	if err != nil {
		pool.Close()
		t.Fatalf("register: %v", err)
	}
	ws, err := wsSvc.Create(ctx, user.ID, fmt.Sprintf("Demo Workspace %d", time.Now().UnixNano()))
	if err != nil {
		pool.Close()
		t.Fatalf("create workspace: %v", err)
	}
	if _, err := mods.Install(ctx, user.ID, ws.ID, "orbit.demo"); err != nil {
		pool.Close()
		t.Fatalf("install demo: %v", err)
	}
	if _, err := mods.Enable(ctx, user.ID, ws.ID, "orbit.demo"); err != nil {
		pool.Close()
		t.Fatalf("enable demo: %v", err)
	}

	cleanup := func() {
		_, _ = pool.Exec(ctx, `DELETE FROM workspaces WHERE created_by = $1`, user.ID)
		_, _ = pool.Exec(ctx, `DELETE FROM audit_logs WHERE actor_user_id = $1`, user.ID)
		_, _ = pool.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, user.ID)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, user.ID)
		pool.Close()
	}

	return &env{demo: demoSvc, events: eventsSvc, mods: mods, userID: user.ID, workspace: ws.ID, cleanup: cleanup}
}

func TestAddRecordPublishesEvent(t *testing.T) {
	env := setup(t)
	defer env.cleanup()
	ctx := context.Background()

	if !env.demo.Enabled(ctx, env.workspace) {
		t.Fatal("demo module not enabled")
	}

	record, err := env.demo.Add(ctx, env.workspace, env.userID, "First record")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if record.ID == "" || record.Title != "First record" {
		t.Errorf("unexpected record: %+v", record)
	}

	// The event must be durable alongside the record.
	evts, err := env.events.List(ctx, env.workspace, 50)
	if err != nil {
		t.Fatalf("List events: %v", err)
	}
	if len(evts) != 1 {
		t.Fatalf("events = %d, want 1", len(evts))
	}
	e := evts[0]
	if e.Type != "demo.record.created" {
		t.Errorf("event type = %q, want demo.record.created", e.Type)
	}
	if e.ActorUserID != env.userID {
		t.Errorf("event actor = %q, want %q", e.ActorUserID, env.userID)
	}
	if rid, _ := e.Payload["recordId"].(string); rid != record.ID {
		t.Errorf("payload recordId = %v, want %q", e.Payload["recordId"], record.ID)
	}

	// Records are listed for the workspace.
	records, err := env.demo.List(ctx, env.workspace)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(records) != 1 || records[0].ID != record.ID {
		t.Errorf("records = %+v, want the added record", records)
	}
}

func TestAddRequiresTitle(t *testing.T) {
	env := setup(t)
	defer env.cleanup()

	if _, err := env.demo.Add(context.Background(), env.workspace, env.userID, ""); err == nil {
		t.Error("Add with empty title succeeded, want error")
	}
}

func TestUninstallCleansModuleData(t *testing.T) {
	env := setup(t)
	defer env.cleanup()
	ctx := context.Background()

	if _, err := env.demo.Add(ctx, env.workspace, env.userID, "to be cleaned"); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Register the same hook main.go registers, then uninstall.
	env.mods.RegisterUninstallHook("orbit.demo", env.demo.Cleanup)
	if err := env.mods.Uninstall(ctx, env.userID, env.workspace, "orbit.demo"); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}

	records, err := env.demo.List(ctx, env.workspace)
	if err != nil {
		t.Fatalf("List after uninstall: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("records after uninstall = %d, want 0 (module data must be cleaned)", len(records))
	}
	if env.demo.Enabled(ctx, env.workspace) {
		t.Error("demo still enabled after uninstall")
	}
}