// Package notifications is the core in-app notification service (PRD
// layer 3 shared services). Producers write rows transactionally with the
// change that causes them; consumers poll — SSE push arrives with D7.
package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Notification is one in-app notification.
type Notification struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	WorkspaceID string         `json:"workspaceId"`
	Payload     map[string]any `json:"payload"`
	ReadAt      *time.Time     `json:"readAt"`
	CreatedAt   time.Time      `json:"createdAt"`
}

// Service reads and writes user notifications.
type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// InsertTx writes a notification inside the caller's transaction, so the
// notification is exactly as durable as the change that caused it.
func (s *Service) InsertTx(ctx context.Context, tx pgx.Tx, userID, workspaceID, typ string, payload map[string]any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal notification payload: %w", err)
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO notifications (user_id, workspace_id, type, payload)
		 VALUES ($1, $2, $3, $4)`,
		userID, workspaceID, typ, raw,
	)
	if err != nil {
		return fmt.Errorf("insert notification: %w", err)
	}
	return nil
}

// List returns the user's latest notifications and their unread count.
func (s *Service) List(ctx context.Context, userID string, limit int) ([]Notification, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, type, workspace_id::text, payload, read_at, created_at
		 FROM notifications WHERE user_id = $1
		 ORDER BY created_at DESC, id DESC LIMIT $2`,
		userID, limit,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list notifications: %w", err)
	}
	defer rows.Close()

	items := []Notification{}
	for rows.Next() {
		var n Notification
		var raw []byte
		if err := rows.Scan(&n.ID, &n.Type, &n.WorkspaceID, &raw, &n.ReadAt, &n.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan notification: %w", err)
		}
		if err := json.Unmarshal(raw, &n.Payload); err != nil {
			return nil, 0, fmt.Errorf("decode notification payload: %w", err)
		}
		items = append(items, n)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	var unread int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL`,
		userID,
	).Scan(&unread); err != nil {
		return nil, 0, fmt.Errorf("count unread: %w", err)
	}
	return items, unread, nil
}

// MarkAllRead stamps every unread notification of the user.
func (s *Service) MarkAllRead(ctx context.Context, userID string) error {
	if _, err := s.pool.Exec(ctx,
		`UPDATE notifications SET read_at = now() WHERE user_id = $1 AND read_at IS NULL`,
		userID,
	); err != nil {
		return fmt.Errorf("mark notifications read: %w", err)
	}
	return nil
}
