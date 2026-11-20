// Package agents implements first-class agent identity: named API tokens
// with a human-readable persona. Every audit row from an agent's action
// carries the agent_id, so "was it the human or the agent?" is always
// answerable.
package agents

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNameRequired = errors.New("agent name is required (max 80 characters)")
	ErrNotFound     = errors.New("agent not found")
)

// Agent is a named identity backed by an API token.
type Agent struct {
	ID          string     `json:"id"`
	UserID      string     `json:"userId"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	TokenID     string     `json:"tokenId"`
	CreatedAt   time.Time  `json:"createdAt"`
	RevokedAt   *time.Time `json:"revokedAt"`
}

// Service manages agent identity.
type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// Create makes a named agent backed by an existing API token.
func (s *Service) Create(ctx context.Context, userID, name, description, tokenID string) (Agent, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 80 {
		return Agent{}, ErrNameRequired
	}
	if len(description) > 500 {
		return Agent{}, errors.New("description must be at most 500 characters")
	}
	// Token must exist and belong to the caller.
	var tokenUser string
	err := s.pool.QueryRow(ctx,
		`SELECT user_id::text FROM api_tokens WHERE id = $1 AND revoked_at IS NULL`,
		tokenID,
	).Scan(&tokenUser)
	if errors.Is(err, pgx.ErrNoRows) {
		return Agent{}, errors.New("token not found or already revoked")
	}
	if err != nil {
		return Agent{}, fmt.Errorf("check token: %w", err)
	}
	if tokenUser != userID {
		return Agent{}, errors.New("token belongs to another user")
	}

	var a Agent
	err = s.pool.QueryRow(ctx,
		`INSERT INTO agents (user_id, name, description, token_id)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id::text, user_id::text, name, description, token_id::text, created_at`,
		userID, name, description, tokenID,
	).Scan(&a.ID, &a.UserID, &a.Name, &a.Description, &a.TokenID, &a.CreatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return Agent{}, fmt.Errorf("you already have an agent named %q", name)
		}
		return Agent{}, fmt.Errorf("insert agent: %w", err)
	}
	return a, nil
}

// List returns the caller's agents.
func (s *Service) List(ctx context.Context, userID string) ([]Agent, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT a.id::text, a.user_id::text, a.name, a.description, a.token_id::text, a.created_at, a.revoked_at
		 FROM agents a WHERE a.user_id = $1
		 ORDER BY a.created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	defer rows.Close()
	items := []Agent{}
	for rows.Next() {
		var a Agent
		if err := rows.Scan(&a.ID, &a.UserID, &a.Name, &a.Description, &a.TokenID, &a.CreatedAt, &a.RevokedAt); err != nil {
			return nil, fmt.Errorf("scan agent: %w", err)
		}
		items = append(items, a)
	}
	return items, rows.Err()
}

// Revoke marks an agent revoked (the token stays until separately revoked).
func (s *Service) Revoke(ctx context.Context, userID, agentID string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE agents SET revoked_at = now() WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL`,
		agentID, userID,
	)
	if err != nil {
		return fmt.Errorf("revoke agent: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// AgentByToken resolves which agent (if any) a Bearer token belongs to,
// so audit rows can carry agent attribution.
func (s *Service) AgentByToken(ctx context.Context, tokenID string) (Agent, error) {
	var a Agent
	err := s.pool.QueryRow(ctx,
		`SELECT id::text, user_id::text, name, description, token_id::text, created_at, revoked_at
		 FROM agents WHERE token_id = $1 AND revoked_at IS NULL`,
		tokenID,
	).Scan(&a.ID, &a.UserID, &a.Name, &a.Description, &a.TokenID, &a.CreatedAt, &a.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Agent{}, ErrNotFound
	}
	if err != nil {
		return Agent{}, fmt.Errorf("agent by token: %w", err)
	}
	return a, nil
}

// AuditAttribution writes an audit row attributed to an agent.
func (s *Service) Audit(ctx context.Context, agentID, action, resourceType, resourceID string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO audit_logs (agent_id, action, resource_type, resource_id)
		 VALUES ($1, $2, $3, $4)`,
		agentID, action, resourceType, resourceID,
	)
	if err != nil {
		return fmt.Errorf("agent audit: %w", err)
	}
	return nil
}
