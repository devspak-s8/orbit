package identity

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/orbit-org/orbit/server/internal/config"
	"github.com/orbit-org/orbit/server/internal/db"
)

func TestAPITokenLifecycle(t *testing.T) {
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
	defer pool.Close()

	svc := NewService(pool, time.Hour)
	email := fmt.Sprintf("token-user-%d@orbit.test", time.Now().UnixNano())
	user, _, err := svc.Register(ctx, email, "Token User", "password123")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	rec, secret, err := svc.CreateAPIToken(ctx, user.ID, "ci-bot")
	if err != nil {
		t.Fatalf("CreateAPIToken: %v", err)
	}
	if len(secret) < 30 || rec.ID == "" {
		t.Fatalf("bad token/secret: %+v %q", rec, secret)
	}

	// The secret authenticates and stamps usage.
	if _, ok := svc.UserByBearerToken(ctx, secret); !ok {
		t.Fatal("fresh token failed to authenticate")
	}
	list, err := svc.ListAPITokens(ctx, user.ID)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListAPITokens = %v, err %v", list, err)
	}
	if list[0].LastUsedAt == nil {
		t.Error("last_used_at not stamped")
	}

	// Wrong secrets fail.
	if _, ok := svc.UserByBearerToken(ctx, "ob_notreal"); ok {
		t.Error("fake token authenticated")
	}

	// Revocation kills the token immediately.
	if err := svc.RevokeAPIToken(ctx, user.ID, rec.ID); err != nil {
		t.Fatalf("RevokeAPIToken: %v", err)
	}
	if _, ok := svc.UserByBearerToken(ctx, secret); ok {
		t.Error("revoked token still authenticates")
	}
	if err := svc.RevokeAPIToken(ctx, user.ID, rec.ID); err == nil {
		t.Error("revoke twice succeeded")
	}
}
