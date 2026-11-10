// Package events implements the platform event log: versioned, typed,
// workspace-scoped events written transactionally with the changes that
// produce them (outbox pattern — PRD 8.6).
package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/orbit-org/orbit/server/internal/apijson"
	"github.com/orbit-org/orbit/server/internal/identity"
)

// Event is the platform event envelope.
type Event struct {
	Type          string         `json:"type"`
	SchemaVersion int            `json:"schemaVersion"`
	WorkspaceID   string         `json:"workspaceId"`
	ActorUserID   string         `json:"actorUserId,omitempty"`
	CorrelationID string         `json:"correlationId,omitempty"`
	Payload       map[string]any `json:"payload"`
	CreatedAt     time.Time      `json:"createdAt"`
}

// Service reads and writes the event log.
type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// PublishTx inserts an event inside the caller's transaction, so the event
// commits or rolls back together with the change that produced it.
func (s *Service) PublishTx(ctx context.Context, tx pgx.Tx, e Event) error {
	if e.Type == "" {
		return errors.New("event type is required")
	}
	if e.SchemaVersion == 0 {
		e.SchemaVersion = 1
	}
	if e.Payload == nil {
		e.Payload = map[string]any{}
	}
	_, err := tx.Exec(ctx,
		`INSERT INTO events (type, schema_version, workspace_id, actor_user_id, correlation_id, payload)
		 VALUES ($1, $2, $3, NULLIF($4, '')::uuid, $5, $6)`,
		e.Type, e.SchemaVersion, e.WorkspaceID, e.ActorUserID, e.CorrelationID, e.Payload,
	)
	if err != nil {
		return fmt.Errorf("publish event %q: %w", e.Type, err)
	}
	return nil
}

// List returns recent events for a workspace, newest first.
func (s *Service) List(ctx context.Context, workspaceID string, limit int) ([]Event, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, type, schema_version, workspace_id::text, actor_user_id::text,
		        correlation_id, payload, created_at
		 FROM events
		 WHERE workspace_id = $1
		 ORDER BY created_at DESC, id DESC
		 LIMIT $2`,
		workspaceID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()

	items := []Event{}
	for rows.Next() {
		var e Event
		var id string
		var actorID *string
		var payload []byte
		if err := rows.Scan(&id, &e.Type, &e.SchemaVersion, &e.WorkspaceID, &actorID, &e.CorrelationID, &payload, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		if actorID != nil {
			e.ActorUserID = *actorID
		}
		if err := json.Unmarshal(payload, &e.Payload); err != nil {
			return nil, fmt.Errorf("decode event payload: %w", err)
		}
		items = append(items, e)
	}
	return items, rows.Err()
}

// Handlers wires event endpoints onto a mux that receives workspace paths.
func (s *Service) Handlers(api *http.ServeMux) {
	api.HandleFunc("GET /v1/workspaces/{id}/events", s.handleList)
}

func (s *Service) handleList(w http.ResponseWriter, r *http.Request) {
	if _, ok := identity.UserFromContext(r.Context()); !ok {
		apijson.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	// The workspace membership guard runs in the router; here we only
	// resolve the limit and query the log for the path workspace.
	limit := 50
	if q := r.URL.Query().Get("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	items, err := s.List(r.Context(), r.PathValue("id"), limit)
	if err != nil {
		apijson.WriteError(w, http.StatusInternalServerError, "internal", "an internal error occurred")
		return
	}
	if items == nil {
		items = []Event{}
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"events": items})
}