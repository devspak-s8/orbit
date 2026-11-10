package workspaces

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/orbit-org/orbit/server/internal/config"
	"github.com/orbit-org/orbit/server/internal/db"
	"github.com/orbit-org/orbit/server/internal/identity"
)

type testEnv struct {
	ws      *Service
	userA   identity.User
	userB   identity.User
	cleanup func()
}

func setupEnv(t *testing.T) *testEnv {
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

	identSvc := identity.NewService(pool, 24*time.Hour)
	emailA := fmt.Sprintf("ws-a-%d@orbit.test", time.Now().UnixNano())
	emailB := fmt.Sprintf("ws-b-%d@orbit.test", time.Now().UnixNano())
	userA, _, err := identSvc.Register(ctx, emailA, "User A", "password123")
	if err != nil {
		pool.Close()
		t.Fatalf("register user A: %v", err)
	}
	userB, _, err := identSvc.Register(ctx, emailB, "User B", "password123")
	if err != nil {
		pool.Close()
		t.Fatalf("register user B: %v", err)
	}

	cleanup := func() {
		for _, email := range []string{emailA, emailB} {
			_, _ = pool.Exec(ctx,
				`DELETE FROM audit_logs WHERE actor_user_id IN (SELECT id FROM users WHERE email_normalized = $1)`, email)
			_, _ = pool.Exec(ctx,
				`DELETE FROM sessions WHERE user_id IN (SELECT id FROM users WHERE email_normalized = $1)`, email)
			_, _ = pool.Exec(ctx,
				`DELETE FROM memberships WHERE user_id IN (SELECT id FROM users WHERE email_normalized = $1)`, email)
			_, _ = pool.Exec(ctx,
				`DELETE FROM workspaces WHERE created_by IN (SELECT id FROM users WHERE email_normalized = $1)`, email)
			_, _ = pool.Exec(ctx, `DELETE FROM users WHERE email_normalized = $1`, email)
		}
		pool.Close()
	}

	return &testEnv{ws: NewService(pool), userA: userA, userB: userB, cleanup: cleanup}
}

func TestCreateListGet(t *testing.T) {
	env := setupEnv(t)
	defer env.cleanup()
	ctx := context.Background()

	ws, err := env.ws.Create(ctx, env.userA.ID, "Design Team")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if ws.ID == "" || ws.Role != "owner" {
		t.Errorf("unexpected workspace: %+v", ws)
	}

	items, err := env.ws.List(ctx, env.userA.ID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 || items[0].ID != ws.ID {
		t.Errorf("List = %+v, want the created workspace", items)
	}

	got, err := env.ws.Get(ctx, env.userA.ID, ws.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "Design Team" {
		t.Errorf("Get name = %q, want %q", got.Name, "Design Team")
	}
}

func TestWorkspaceIsolation(t *testing.T) {
	env := setupEnv(t)
	defer env.cleanup()
	ctx := context.Background()

	ws, err := env.ws.Create(ctx, env.userA.ID, "Private")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// User B must not see A's workspace in their list.
	items, err := env.ws.List(ctx, env.userB.ID)
	if err != nil {
		t.Fatalf("List (B): %v", err)
	}
	if len(items) != 0 {
		t.Errorf("user B sees %d workspaces, want 0 (tenant isolation)", len(items))
	}

	// User B must not be able to fetch A's workspace directly.
	if _, err := env.ws.Get(ctx, env.userB.ID, ws.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("user B Get err = %v, want ErrNotFound", err)
	}

	// Membership check must also fail for B.
	if _, err := env.ws.Membership(ctx, env.userB.ID, ws.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("user B Membership err = %v, want ErrNotFound", err)
	}
}

func TestSlugGeneration(t *testing.T) {
	env := setupEnv(t)
	defer env.cleanup()
	ctx := context.Background()

	ws, err := env.ws.Create(ctx, env.userA.ID, "  My Team! (Dev)  ")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if ws.Slug != "my-team-dev" {
		t.Errorf("slug = %q, want %q", ws.Slug, "my-team-dev")
	}

	// A second workspace with the same name must get a unique slug.
	ws2, err := env.ws.Create(ctx, env.userA.ID, "  My Team! (Dev)  ")
	if err != nil {
		t.Fatalf("Create duplicate-name: %v", err)
	}
	if ws2.Slug == ws.Slug {
		t.Errorf("duplicate name produced identical slug %q", ws.Slug)
	}
}

func TestCreateRejectsInvalidName(t *testing.T) {
	env := setupEnv(t)
	defer env.cleanup()
	ctx := context.Background()

	if _, err := env.ws.Create(ctx, env.userA.ID, "   "); !errors.Is(err, ErrInvalidName) {
		t.Errorf("empty name err = %v, want ErrInvalidName", err)
	}
	long := make([]byte, 81)
	for i := range long {
		long[i] = 'x'
	}
	if _, err := env.ws.Create(ctx, env.userA.ID, string(long)); !errors.Is(err, ErrInvalidName) {
		t.Errorf("long name err = %v, want ErrInvalidName", err)
	}
}
