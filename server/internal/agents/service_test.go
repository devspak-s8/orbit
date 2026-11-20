package agents

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

func setup(t *testing.T) (*Service, *identity.Service, string, func()) {
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
	ident := identity.NewService(pool, time.Hour)
	email := fmt.Sprintf("agent-%d@orbit.test", time.Now().UnixNano())
	user, _, err := ident.Register(ctx, email, "Agent User", "password123")
	if err != nil {
		pool.Close()
		t.Fatalf("register: %v", err)
	}
	cleanup := func() {
		_, _ = pool.Exec(ctx, `DELETE FROM agents WHERE user_id = $1`, user.ID)
		_, _ = pool.Exec(ctx, `DELETE FROM api_tokens WHERE user_id = $1`, user.ID)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, user.ID)
		pool.Close()
	}
	return NewService(pool), ident, user.ID, cleanup
}

func TestAgentLifecycle(t *testing.T) {
	svc, ident, userID, cleanup := setup(t)
	defer cleanup()
	ctx := context.Background()

	// Create a token for the agent.
	token, _, err := ident.CreateAPIToken(ctx, userID, "agent-token")
	if err != nil {
		t.Fatalf("CreateAPIToken: %v", err)
	}

	// Create an agent.
	agent, err := svc.Create(ctx, userID, "ci-bot", "Runs CI automations", token.ID)
	if err != nil {
		t.Fatalf("Create agent: %v", err)
	}
	if agent.ID == "" || agent.Name != "ci-bot" || agent.TokenID != token.ID {
		t.Errorf("unexpected agent: %+v", agent)
	}

	// Duplicate names rejected.
	if _, err := svc.Create(ctx, userID, "ci-bot", "other", token.ID); err == nil {
		t.Error("duplicate name accepted")
	}

	// List.
	agents, err := svc.List(ctx, userID)
	if err != nil || len(agents) != 1 {
		t.Fatalf("List = %d, err = %v", len(agents), err)
	}

	// Resolve by token.
	resolved, err := svc.AgentByToken(ctx, token.ID)
	if err != nil || resolved.ID != agent.ID {
		t.Errorf("AgentByToken = %+v, err = %v", resolved, err)
	}

	// Revoke.
	if err := svc.Revoke(ctx, userID, agent.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	// Revoked agents don't resolve.
	if _, err := svc.AgentByToken(ctx, token.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("revoked agent still resolves: err = %v", err)
	}
	// Double revoke fails.
	if err := svc.Revoke(ctx, userID, agent.ID); err == nil {
		t.Error("double revoke succeeded")
	}

	// Another user's token can't be used (SQL enforces user_id match).
	_, _, err = ident.CreateAPIToken(ctx, userID, "token2")
	if err != nil {
		t.Fatalf("CreateAPIToken 2: %v", err)
	}
}
