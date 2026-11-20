package docs

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/orbit-org/orbit/server/internal/config"
	"github.com/orbit-org/orbit/server/internal/db"
	"github.com/orbit-org/orbit/server/internal/events"
	"github.com/orbit-org/orbit/server/internal/identity"
	"github.com/orbit-org/orbit/server/internal/modules"
	"github.com/orbit-org/orbit/server/internal/work"
	"github.com/orbit-org/orbit/server/internal/workspaces"
)

func setup(t *testing.T) (*Service, string, string, string, func()) {
	t.Helper()
	ctx := context.Background()
	cfg := config.Load()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
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
	workSvc := work.NewService(pool, eventsSvc, mods, nil)
	docsSvc := NewService(pool, eventsSvc, mods)

	email := fmt.Sprintf("docs-%d@orbit.test", time.Now().UnixNano())
	user, _, err := ident.Register(ctx, email, "Docs User", "password123")
	if err != nil {
		pool.Close()
		t.Fatalf("register: %v", err)
	}
	ws, err := wsSvc.Create(ctx, user.ID, fmt.Sprintf("Docs Test %d", time.Now().UnixNano()))
	if err != nil {
		pool.Close()
		t.Fatalf("create workspace: %v", err)
	}
	if _, err := mods.Install(ctx, user.ID, ws.ID, work.ModuleID); err != nil {
		pool.Close()
		t.Fatalf("install work: %v", err)
	}
	if _, err := mods.Enable(ctx, user.ID, ws.ID, work.ModuleID); err != nil {
		pool.Close()
		t.Fatalf("enable work: %v", err)
	}
	proj, err := workSvc.CreateProject(ctx, user.ID, ws.ID, "Docs project", "")
	if err != nil {
		pool.Close()
		t.Fatalf("create project: %v", err)
	}
	if _, err := mods.Install(ctx, user.ID, ws.ID, ModuleID); err != nil {
		pool.Close()
		t.Fatalf("install docs: %v", err)
	}
	if _, err := mods.Enable(ctx, user.ID, ws.ID, ModuleID); err != nil {
		pool.Close()
		t.Fatalf("enable docs: %v", err)
	}

	cleanup := func() {
		_, _ = pool.Exec(ctx, `DELETE FROM workspaces WHERE created_by = $1`, user.ID)
		_, _ = pool.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, user.ID)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, user.ID)
		pool.Close()
	}

	return docsSvc, user.ID, ws.ID, proj.ID, cleanup
}

func TestPageCRUD(t *testing.T) {
	svc, userID, wsID, projID, cleanup := setup(t)
	defer cleanup()
	ctx := context.Background()

	// Create.
	p, err := svc.CreatePage(ctx, userID, wsID, projID, "Architecture", "# Architecture\n\nThe system is...")
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}
	if p.ID == "" || p.Title != "Architecture" {
		t.Errorf("unexpected page: %+v", p)
	}

	// The event log has page.created.
	evts, _ := events.NewService(svc.pool).List(ctx, wsID, 10)
	found := false
	for _, e := range evts {
		if e.Type == "page.created" {
			found = true
		}
	}
	if !found {
		t.Error("page.created event missing")
	}

	// Update publishes page.updated.
	p2, err := svc.UpdatePage(ctx, userID, wsID, p.ID, "Architecture v2", "Updated body")
	if err != nil {
		t.Fatalf("UpdatePage: %v", err)
	}
	if p2.Title != "Architecture v2" {
		t.Errorf("updated title = %q", p2.Title)
	}
	evts, _ = events.NewService(svc.pool).List(ctx, wsID, 10)
	foundUpdated := false
	for _, e := range evts {
		if e.Type == "page.updated" {
			foundUpdated = true
		}
	}
	if !foundUpdated {
		t.Error("page.updated event missing")
	}

	// Get.
	got, err := svc.GetPage(ctx, userID, wsID, p.ID)
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if got.Body != "Updated body" {
		t.Errorf("body = %q", got.Body)
	}

	// List by project.
	items, err := svc.ListPages(ctx, userID, wsID, projID)
	if err != nil || len(items) != 1 {
		t.Fatalf("ListPages = %d, err = %v", len(items), err)
	}

	// Delete.
	if err := svc.DeletePage(ctx, userID, wsID, p.ID); err != nil {
		t.Fatalf("DeletePage: %v", err)
	}
	if _, err := svc.GetPage(ctx, userID, wsID, p.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("after delete err = %v, want ErrNotFound", err)
	}

	// Validation: empty title.
	if _, err := svc.CreatePage(ctx, userID, wsID, projID, "  ", "body"); !errors.Is(err, ErrInvalidTitle) {
		t.Errorf("empty title err = %v", err)
	}
}
