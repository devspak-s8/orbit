package workspaces

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Invite is a shareable workspace join code.
type Invite struct {
	Code      string    `json:"code"`
	Workspace string    `json:"workspaceId"`
	ExpiresAt time.Time `json:"expiresAt"`
	CreatedAt time.Time `json:"createdAt"`
}

var (
	// ErrInviteInvalid covers expired, unknown, and malformed codes alike —
	// callers must not distinguish them (no existence leak).
	ErrInviteInvalid = errors.New("invite is invalid or has expired")
	// ErrAlreadyMember is returned when the redeemer is already in.
	ErrAlreadyMember = errors.New("you are already a member of this workspace")
)

// CreateInvite mints a single shareable code for the workspace. Only the
// owner may invite (alpha: member role only).
func (s *Service) CreateInvite(ctx context.Context, userID, workspaceID string) (Invite, error) {
	role, err := s.Membership(ctx, userID, workspaceID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			// Non-members must not learn the workspace exists.
			return Invite{}, ErrForbidden
		}
		return Invite{}, err
	}
	if role != "owner" {
		return Invite{}, ErrForbidden
	}

	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return Invite{}, fmt.Errorf("generate invite code: %w", err)
	}
	inv := Invite{
		Code:      base64.RawURLEncoding.EncodeToString(b),
		Workspace: workspaceID,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}
	if err := s.pool.QueryRow(ctx,
		`INSERT INTO invite_codes (workspace_id, code, created_by, expires_at)
		 VALUES ($1, $2, $3, $4)
		 RETURNING created_at`,
		workspaceID, inv.Code, userID, inv.ExpiresAt,
	).Scan(&inv.CreatedAt); err != nil {
		return Invite{}, fmt.Errorf("insert invite: %w", err)
	}
	return inv, nil
}

// RedeemInvite joins the caller to the code's workspace as a member, and
// notifies the workspace owner — transactionally with the join.
func (s *Service) RedeemInvite(ctx context.Context, userID, code string) (Workspace, error) {
	var wsID string
	var expiresAt time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT workspace_id, expires_at FROM invite_codes WHERE code = $1`,
		code,
	).Scan(&wsID, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Workspace{}, ErrInviteInvalid
	}
	if err != nil {
		return Workspace{}, fmt.Errorf("lookup invite: %w", err)
	}
	if time.Now().After(expiresAt) {
		return Workspace{}, ErrInviteInvalid
	}

	role, err := s.Membership(ctx, userID, wsID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return Workspace{}, err
	}
	if err == nil {
		_ = role
		return Workspace{}, ErrAlreadyMember
	}

	// Join + owner notification in one transaction.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Workspace{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`INSERT INTO memberships (workspace_id, user_id, role) VALUES ($1, $2, 'member')`,
		wsID, userID,
	); err != nil {
		return Workspace{}, fmt.Errorf("insert membership: %w", err)
	}

	if s.notif != nil {
		var ownerID string
		if err := tx.QueryRow(ctx,
			`SELECT created_by::text FROM workspaces WHERE id = $1`, wsID,
		).Scan(&ownerID); err == nil && ownerID != userID {
			if err := s.notif.InsertTx(ctx, tx, ownerID, wsID, "member.joined", map[string]any{
				"workspaceId": wsID,
				"memberId":    userID,
			}); err != nil {
				return Workspace{}, err
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Workspace{}, fmt.Errorf("commit: %w", err)
	}

	ws, err := s.Get(ctx, userID, wsID)
	if err != nil {
		return Workspace{}, err
	}
	ws.Role = "member"
	return ws, nil
}
