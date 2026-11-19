package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// APIToken is a Bearer credential for programmatic access. The secret is
// shown once at creation; only the digest is stored.
type APIToken struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
	RevokedAt  *time.Time `json:"revokedAt"`
}

var ErrTokenNameRequired = errors.New("token name is required (max 80 characters)")

// CreateAPIToken mints a new Bearer token and returns (record, secret).
func (s *Service) CreateAPIToken(ctx context.Context, userID, name string) (APIToken, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 80 {
		return APIToken{}, "", ErrTokenNameRequired
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return APIToken{}, "", fmt.Errorf("generate token: %w", err)
	}
	secret := "ob_" + base64.RawURLEncoding.EncodeToString(raw)
	digest := sha256.Sum256([]byte(secret))

	var t APIToken
	err := s.pool.QueryRow(ctx,
		`INSERT INTO api_tokens (user_id, name, token_hash)
		 VALUES ($1, $2, $3)
		 RETURNING id::text, name, created_at`,
		userID, name, hexEncode(digest[:]),
	).Scan(&t.ID, &t.Name, &t.CreatedAt)
	if err != nil {
		return APIToken{}, "", fmt.Errorf("insert token: %w", err)
	}
	return t, secret, nil
}

// ListAPITokens returns the caller's tokens, newest first, never secrets.
func (s *Service) ListAPITokens(ctx context.Context, userID string) ([]APIToken, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, name, created_at, last_used_at, revoked_at
		 FROM api_tokens WHERE user_id = $1
		 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list tokens: %w", err)
	}
	defer rows.Close()
	items := []APIToken{}
	for rows.Next() {
		var t APIToken
		if err := rows.Scan(&t.ID, &t.Name, &t.CreatedAt, &t.LastUsedAt, &t.RevokedAt); err != nil {
			return nil, fmt.Errorf("scan token: %w", err)
		}
		items = append(items, t)
	}
	return items, rows.Err()
}

// RevokeAPIToken marks a token revoked; the caller must own it.
func (s *Service) RevokeAPIToken(ctx context.Context, userID, tokenID string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE api_tokens SET revoked_at = now()
		 WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL`,
		tokenID, userID,
	)
	if err != nil {
		return fmt.Errorf("revoke token: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSessionExpired // deliberately indistinguishable from unknown ids
	}
	return nil
}

// UserByBearerToken authenticates an Authorization: Bearer secret and
// stamps last_used_at (best effort).
func (s *Service) UserByBearerToken(ctx context.Context, secret string) (User, bool) {
	if secret == "" || !strings.HasPrefix(secret, "ob_") {
		return User{}, false
	}
	digest := sha256.Sum256([]byte(secret))
	var user User
	err := s.pool.QueryRow(ctx,
		`SELECT u.id, u.email, u.display_name, u.created_at
		 FROM api_tokens t
		 JOIN users u ON u.id = t.user_id
		 WHERE t.token_hash = $1 AND t.revoked_at IS NULL`,
		hexEncode(digest[:]),
	).Scan(&user.ID, &user.Email, &user.DisplayName, &user.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, false
	}
	if err != nil {
		return User{}, false
	}
	// Best-effort usage stamp; never blocks the request.
	_, _ = s.pool.Exec(ctx,
		`UPDATE api_tokens SET last_used_at = now() WHERE token_hash = $1`,
		hexEncode(digest[:]),
	)
	return user, true
}

func hexEncode(b []byte) string {
	const hexDigits = "0123456789abcdef"
	out := make([]byte, 0, len(b)*2)
	for _, v := range b {
		out = append(out, hexDigits[v>>4], hexDigits[v&0xf])
	}
	return string(out)
}
