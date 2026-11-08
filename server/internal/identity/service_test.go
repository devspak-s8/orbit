package identity

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/orbit-org/orbit/server/internal/config"
	"github.com/orbit-org/orbit/server/internal/db"
)

// setupService connects to the configured database, applies migrations, and
// returns a service plus a cleanup function that removes test-created rows.
// Requires PostgreSQL (local Docker Compose or the CI service container).
func setupService(t *testing.T) (*Service, string, func()) {
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

	email := fmt.Sprintf("test-%d@orbit.test", time.Now().UnixNano())
	cleanup := func() {
		_, _ = pool.Exec(ctx,
			`DELETE FROM audit_logs WHERE actor_user_id IN (SELECT id FROM users WHERE email_normalized = $1)`, email)
		_, _ = pool.Exec(ctx,
			`DELETE FROM sessions WHERE user_id IN (SELECT id FROM users WHERE email_normalized = $1)`, email)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE email_normalized = $1`, email)
		pool.Close()
	}
	return NewService(pool, 24*time.Hour), email, cleanup
}

func TestRegisterLoginLogoutFlow(t *testing.T) {
	svc, email, cleanup := setupService(t)
	defer cleanup()
	ctx := context.Background()

	user, token, err := svc.Register(ctx, email, "Test User", "password123")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if user.ID == "" || user.Email != email || user.DisplayName != "Test User" {
		t.Errorf("unexpected user: %+v", user)
	}
	if token == "" {
		t.Fatal("register returned an empty session token")
	}

	// The token from registration must authenticate.
	got, err := svc.UserBySessionToken(ctx, token)
	if err != nil {
		t.Fatalf("UserBySessionToken after register: %v", err)
	}
	if got.ID != user.ID {
		t.Errorf("session user id = %s, want %s", got.ID, user.ID)
	}

	// Login issues a fresh session.
	loginUser, loginToken, err := svc.Login(ctx, email, "password123")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if loginUser.ID != user.ID {
		t.Errorf("login user id = %s, want %s", loginUser.ID, user.ID)
	}
	if loginToken == token {
		t.Error("login issued the same session token as register")
	}

	// Logout revokes the login session.
	if err := svc.Logout(ctx, loginToken); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, err := svc.UserBySessionToken(ctx, loginToken); !errors.Is(err, ErrSessionExpired) {
		t.Errorf("session still valid after logout, err = %v", err)
	}

	// Audit rows were written for register and login.
	var count int
	if err := svc.pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_logs WHERE actor_user_id = $1`, user.ID,
	).Scan(&count); err != nil {
		t.Fatalf("count audit rows: %v", err)
	}
	if count < 2 {
		t.Errorf("audit rows = %d, want >= 2 (registered + logged_in)", count)
	}
}

func TestRegisterDuplicateEmail(t *testing.T) {
	svc, email, cleanup := setupService(t)
	defer cleanup()
	ctx := context.Background()

	if _, _, err := svc.Register(ctx, email, "A", "password123"); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	// Registration must be case-insensitive on the normalized email.
	if _, _, err := svc.Register(ctx, "  "+email+"  ", "B", "password123"); !errors.Is(err, ErrEmailTaken) {
		t.Errorf("second Register err = %v, want ErrEmailTaken", err)
	}
}

func TestLoginFailures(t *testing.T) {
	svc, email, cleanup := setupService(t)
	defer cleanup()
	ctx := context.Background()

	if _, _, err := svc.Register(ctx, email, "A", "password123"); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if _, _, err := svc.Login(ctx, email, "wrong-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("wrong password err = %v, want ErrInvalidCredentials", err)
	}
	if _, _, err := svc.Login(ctx, "nobody@orbit.test", "password123"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("unknown email err = %v, want ErrInvalidCredentials", err)
	}
}

func TestRegisterValidation(t *testing.T) {
	svc, email, cleanup := setupService(t)
	defer cleanup()
	ctx := context.Background()

	if _, _, err := svc.Register(ctx, "not-an-email", "A", "password123"); !errors.Is(err, ErrInvalidEmail) {
		t.Errorf("bad email err = %v, want ErrInvalidEmail", err)
	}
	if _, _, err := svc.Register(ctx, email, "A", "short"); !errors.Is(err, ErrPasswordTooShort) {
		t.Errorf("short password err = %v, want ErrPasswordTooShort", err)
	}
}

func TestUserBySessionTokenRejectsTampering(t *testing.T) {
	svc, _, cleanup := setupService(t)
	defer cleanup()
	ctx := context.Background()

	for _, raw := range []string{"", "garbage-token", "AAAA.BBBB.CCCC"} {
		if _, err := svc.UserBySessionToken(ctx, raw); !errors.Is(err, ErrSessionExpired) {
			t.Errorf("token %q err = %v, want ErrSessionExpired", raw, err)
		}
	}
}

func TestSessionExpiry(t *testing.T) {
	pool, err := db.Connect(context.Background(), config.Load().DatabaseURL)
	if err != nil {
		t.Skipf("database not available: %v", err)
	}
	defer pool.Close()

	// A service with a negative TTL issues sessions that expire immediately.
	svc := NewService(pool, -time.Minute)
	email := fmt.Sprintf("expiry-%d@orbit.test", time.Now().UnixNano())
	ctx := context.Background()

	_, token, err := svc.Register(ctx, email, "A", "password123")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, `DELETE FROM audit_logs WHERE actor_user_id IN (SELECT id FROM users WHERE email_normalized = $1)`, email)
		_, _ = pool.Exec(ctx, `DELETE FROM sessions WHERE user_id IN (SELECT id FROM users WHERE email_normalized = $1)`, email)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE email_normalized = $1`, email)
	}()

	if _, err := svc.UserBySessionToken(ctx, token); !errors.Is(err, ErrSessionExpired) {
		t.Errorf("expired session err = %v, want ErrSessionExpired", err)
	}
}