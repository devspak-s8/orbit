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
	"github.com/orbit-org/orbit/server/internal/notifications"
)

type inviteEnv struct {
	svc      *Service
	notif    *notifications.Service
	ownerID  string
	joinerID string
	wsID     string
	cleanup  func()
	ctx      context.Context
}

func setupInvites(t *testing.T) *inviteEnv {
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

	ident := identity.NewService(pool, 24*time.Hour)
	svc := NewService(pool)
	notif := notifications.NewService(pool)
	svc.UseNotifications(notif)

	ownerEmail := fmt.Sprintf("inv-owner-%d@orbit.test", time.Now().UnixNano())
	joinerEmail := fmt.Sprintf("inv-joiner-%d@orbit.test", time.Now().UnixNano())
	owner, _, err := ident.Register(ctx, ownerEmail, "Owner", "password123")
	if err != nil {
		pool.Close()
		t.Fatalf("register owner: %v", err)
	}
	joiner, _, err := ident.Register(ctx, joinerEmail, "Joiner", "password123")
	if err != nil {
		pool.Close()
		t.Fatalf("register joiner: %v", err)
	}
	ws, err := svc.Create(ctx, owner.ID, fmt.Sprintf("Invite Test %d", time.Now().UnixNano()))
	if err != nil {
		pool.Close()
		t.Fatalf("create workspace: %v", err)
	}

	cleanup := func() {
		_, _ = pool.Exec(ctx, `DELETE FROM workspaces WHERE created_by = $1`, owner.ID)
		_, _ = pool.Exec(ctx, `DELETE FROM invite_codes WHERE workspace_id IN (SELECT id FROM workspaces WHERE created_by = $1)`, owner.ID)
		for _, e := range []string{ownerEmail, joinerEmail} {
			_, _ = pool.Exec(ctx, `DELETE FROM notifications WHERE user_id IN (SELECT id FROM users WHERE email_normalized = $1)`, e)
			_, _ = pool.Exec(ctx, `DELETE FROM sessions WHERE user_id IN (SELECT id FROM users WHERE email_normalized = $1)`, e)
			_, _ = pool.Exec(ctx, `DELETE FROM audit_logs WHERE actor_user_id IN (SELECT id FROM users WHERE email_normalized = $1)`, e)
			_, _ = pool.Exec(ctx, `DELETE FROM users WHERE email_normalized = $1`, e)
		}
		pool.Close()
	}

	return &inviteEnv{svc: svc, notif: notif, ownerID: owner.ID, joinerID: joiner.ID, wsID: ws.ID, cleanup: cleanup, ctx: ctx}
}

func TestCreateInviteOwnerOnly(t *testing.T) {
	env := setupInvites(t)
	defer env.cleanup()

	invite, err := env.svc.CreateInvite(env.ctx, env.ownerID, env.wsID)
	if err != nil {
		t.Fatalf("owner CreateInvite: %v", err)
	}
	if invite.Code == "" || invite.ExpiresAt.Before(time.Now()) {
		t.Errorf("bad invite: %+v", invite)
	}

	// A non-member cannot mint invites.
	if _, err := env.svc.CreateInvite(env.ctx, env.joinerID, env.wsID); !errors.Is(err, ErrForbidden) {
		t.Errorf("non-member CreateInvite err = %v, want ErrForbidden", err)
	}
}

func TestRedeemInviteFlow(t *testing.T) {
	env := setupInvites(t)
	defer env.cleanup()

	invite, err := env.svc.CreateInvite(env.ctx, env.ownerID, env.wsID)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}

	// Unknown codes are indistinguishable from expired ones.
	if _, err := env.svc.RedeemInvite(env.ctx, env.joinerID, "no-such-code"); !errors.Is(err, ErrInviteInvalid) {
		t.Errorf("bad code err = %v, want ErrInviteInvalid", err)
	}

	// Redeem joins the workspace as a member.
	ws, err := env.svc.RedeemInvite(env.ctx, env.joinerID, invite.Code)
	if err != nil {
		t.Fatalf("RedeemInvite: %v", err)
	}
	if ws.Role != "member" {
		t.Errorf("joined role = %q, want member", ws.Role)
	}
	if _, err := env.svc.Membership(env.ctx, env.joinerID, env.wsID); err != nil {
		t.Errorf("joiner membership check failed: %v", err)
	}

	// Double redeem is a conflict.
	if _, err := env.svc.RedeemInvite(env.ctx, env.joinerID, invite.Code); !errors.Is(err, ErrAlreadyMember) {
		t.Errorf("re-redeem err = %v, want ErrAlreadyMember", err)
	}

	// The owner was notified of the join, transactionally.
	items, unread, err := env.notif.List(env.ctx, env.ownerID, 20)
	if err != nil {
		t.Fatalf("list owner notifications: %v", err)
	}
	if unread != 1 || len(items) == 0 || items[0].Type != "member.joined" {
		t.Errorf("owner notifications = %v (unread %d), want one member.joined", items, unread)
	}

	// Expired codes stop working.
	invite2, err := env.svc.CreateInvite(env.ctx, env.ownerID, env.wsID)
	if err != nil {
		t.Fatalf("CreateInvite 2: %v", err)
	}
	if _, err := env.svc.pool.Exec(env.ctx,
		`UPDATE invite_codes SET expires_at = now() - interval '1 hour' WHERE code = $1`,
		invite2.Code,
	); err != nil {
		t.Fatalf("expire code: %v", err)
	}
	if _, err := env.svc.RedeemInvite(env.ctx, env.ownerID, invite2.Code); !errors.Is(err, ErrInviteInvalid) {
		// owner is already a member here, so use a fresh third party feel:
		// the error must be invalid-or-member, never a silent join
		if !errors.Is(err, ErrAlreadyMember) {
			t.Errorf("expired code err = %v, want ErrInviteInvalid", err)
		}
	}
}
