package identity

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrInvalidCredentials is returned for unknown email or wrong password.
	ErrInvalidCredentials = errors.New("invalid email or password")
	// ErrEmailTaken is returned when registering an already-used email.
	ErrEmailTaken = errors.New("an account with this email already exists")
	// ErrSessionExpired is returned when a session token is unknown, expired, or revoked.
	ErrSessionExpired = errors.New("session expired")
	// ErrPasswordTooShort is returned for passwords under the minimum length.
	ErrPasswordTooShort = errors.New("password must be at least 8 characters")
	// ErrInvalidEmail is returned for malformed email addresses.
	ErrInvalidEmail = errors.New("invalid email address")
)

const minPasswordLen = 8

// User is the public representation of an account.
type User struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"displayName"`
	CreatedAt   time.Time `json:"createdAt"`
}

// Service owns registration, authentication, and sessions.
type Service struct {
	pool       *pgxpool.Pool
	sessionTTL time.Duration
}

func NewService(pool *pgxpool.Pool, sessionTTL time.Duration) *Service {
	return &Service{pool: pool, sessionTTL: sessionTTL}
}

// normalizeEmail lowercases and trims an email for storage and comparison.
func normalizeEmail(email string) (string, error) {
	addr, err := mail.ParseAddress(strings.TrimSpace(email))
	if err != nil || addr.Address != strings.TrimSpace(email) || !strings.Contains(addr.Address, "@") {
		return "", ErrInvalidEmail
	}
	return strings.ToLower(addr.Address), nil
}

// Register creates an account and a session for it (auto-login on signup).
// It returns the new user and the raw session token.
func (s *Service) Register(ctx context.Context, email, displayName, password string) (User, string, error) {
	normalized, err := normalizeEmail(email)
	if err != nil {
		return User{}, "", err
	}
	if len(password) < minPasswordLen {
		return User{}, "", ErrPasswordTooShort
	}
	hash, err := hashPassword(password)
	if err != nil {
		return User{}, "", fmt.Errorf("hash password: %w", err)
	}
	raw, tokenHash, err := newSessionToken()
	if err != nil {
		return User{}, "", err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return User{}, "", fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	var user User
	err = tx.QueryRow(ctx,
		`INSERT INTO users (email, email_normalized, password_hash, display_name)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, email, display_name, created_at`,
		strings.TrimSpace(email), normalized, hash, strings.TrimSpace(displayName),
	).Scan(&user.ID, &user.Email, &user.DisplayName, &user.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return User{}, "", ErrEmailTaken
		}
		return User{}, "", fmt.Errorf("insert user: %w", err)
	}

	if err := s.createSession(ctx, tx, user.ID, tokenHash); err != nil {
		return User{}, "", err
	}
	if err := s.audit(ctx, tx, user.ID, "user.registered", "user", user.ID, nil); err != nil {
		return User{}, "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return User{}, "", fmt.Errorf("commit: %w", err)
	}
	return user, raw, nil
}

// Login verifies credentials and issues a session.
func (s *Service) Login(ctx context.Context, email, password string) (User, string, error) {
	normalized, err := normalizeEmail(email)
	if err != nil {
		return User{}, "", ErrInvalidCredentials
	}

	var user User
	var passwordHash string
	err = s.pool.QueryRow(ctx,
		`SELECT id, email, display_name, created_at, password_hash
		 FROM users WHERE email_normalized = $1`,
		normalized,
	).Scan(&user.ID, &user.Email, &user.DisplayName, &user.CreatedAt, &passwordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		// Record the failure; no actor id exists for unknown emails.
		if auditErr := s.audit(ctx, nil, "", "user.login_failed", "user", "", map[string]any{"email": normalized}); auditErr != nil {
			return User{}, "", fmt.Errorf("audit login failure: %w", auditErr)
		}
		return User{}, "", ErrInvalidCredentials
	}
	if err != nil {
		return User{}, "", fmt.Errorf("lookup user: %w", err)
	}

	if !verifyPassword(password, passwordHash) {
		if auditErr := s.audit(ctx, nil, user.ID, "user.login_failed", "user", user.ID, nil); auditErr != nil {
			return User{}, "", fmt.Errorf("audit login failure: %w", auditErr)
		}
		return User{}, "", ErrInvalidCredentials
	}

	raw, tokenHash, err := newSessionToken()
	if err != nil {
		return User{}, "", err
	}
	if err := s.createSession(ctx, nil, user.ID, tokenHash); err != nil {
		return User{}, "", err
	}
	if err := s.audit(ctx, nil, user.ID, "user.logged_in", "user", user.ID, nil); err != nil {
		return User{}, "", err
	}
	return user, raw, nil
}

// Logout revokes the session identified by the raw token, if any.
func (s *Service) Logout(ctx context.Context, raw string) error {
	if raw == "" {
		return nil
	}
	res, err := s.pool.Exec(ctx,
		`UPDATE sessions SET revoked_at = now()
		 WHERE token_hash = $1 AND revoked_at IS NULL`,
		hashToken(raw),
	)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	if res.RowsAffected() > 0 {
		// Best-effort audit; the user id is fetched from the session row.
		var userID string
		if err := s.pool.QueryRow(ctx,
			`SELECT user_id::text FROM sessions WHERE token_hash = $1`,
			hashToken(raw),
		).Scan(&userID); err == nil {
			_ = s.audit(ctx, nil, userID, "user.logged_out", "user", userID, nil)
		}
	}
	return nil
}

// UserBySessionToken returns the user owning a valid, unrevoked, unexpired
// session, or ErrSessionExpired.
func (s *Service) UserBySessionToken(ctx context.Context, raw string) (User, error) {
	if raw == "" {
		return User{}, ErrSessionExpired
	}
	var user User
	err := s.pool.QueryRow(ctx,
		`SELECT u.id, u.email, u.display_name, u.created_at
		 FROM sessions s
		 JOIN users u ON u.id = s.user_id
		 WHERE s.token_hash = $1
		   AND s.revoked_at IS NULL
		   AND s.expires_at > now()`,
		hashToken(raw),
	).Scan(&user.ID, &user.Email, &user.DisplayName, &user.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrSessionExpired
	}
	if err != nil {
		return User{}, fmt.Errorf("lookup session: %w", err)
	}
	return user, nil
}

// createSession inserts a session row. tx may be nil to use the pool.
func (s *Service) createSession(ctx context.Context, tx pgx.Tx, userID, tokenHash string) error {
	exec := s.pool.Exec
	if tx != nil {
		exec = tx.Exec
	}
	_, err := exec(ctx,
		`INSERT INTO sessions (user_id, token_hash, expires_at)
		 VALUES ($1, $2, now() + $3::interval)`,
		userID, tokenHash, fmt.Sprintf("%d seconds", int(s.sessionTTL.Seconds())),
	)
	if err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	return nil
}

// audit writes an audit_logs row. tx may be nil to use the pool.
func (s *Service) audit(ctx context.Context, tx pgx.Tx, actorUserID, action, resourceType, resourceID string, metadata map[string]any) error {
	exec := s.pool.Exec
	if tx != nil {
		exec = tx.Exec
	}
	_, err := exec(ctx,
		`INSERT INTO audit_logs (actor_user_id, action, resource_type, resource_id, metadata)
		 VALUES (NULLIF($1, '')::uuid, $2, $3, $4, COALESCE($5, '{}'::jsonb))`,
		actorUserID, action, resourceType, resourceID, metadata,
	)
	if err != nil {
		return fmt.Errorf("insert audit row: %w", err)
	}
	return nil
}