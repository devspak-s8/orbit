package workspaces

import (
	"errors"
	"testing"
)

func TestWorkspaceSettings(t *testing.T) {
	env := setupInvites(t)
	defer env.cleanup()
	ctx := env.ctx

	// Non-owners can't rename — either forbidden or not found (no leak).
	if _, err := env.svc.UpdateWorkspace(ctx, env.joinerID, env.wsID, "New Name"); err == nil {
		t.Error("non-owner rename succeeded")
	}

	// Rename works.
	renamed, err := env.svc.UpdateWorkspace(ctx, env.ownerID, env.wsID, "Renamed Workspace")
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if renamed.Name != "Renamed Workspace" {
		t.Errorf("renamed to %q", renamed.Name)
	}

	// Empty name rejected.
	if _, err := env.svc.UpdateWorkspace(ctx, env.ownerID, env.wsID, "  "); !errors.Is(err, ErrInvalidName) {
		t.Errorf("empty rename err = %v", err)
	}

	// Delete requires exact name match.
	if err := env.svc.DeleteWorkspace(ctx, env.ownerID, env.wsID, "Wrong Name"); err == nil {
		t.Error("wrong confirmation name accepted")
	}

	// Non-owners can't delete — either forbidden or not found.
	if err := env.svc.DeleteWorkspace(ctx, env.joinerID, env.wsID, "Renamed Workspace"); err == nil {
		t.Error("non-owner delete succeeded")
	}

	// Delete with correct name succeeds (cascades everything).
	if err := env.svc.DeleteWorkspace(ctx, env.ownerID, env.wsID, "Renamed Workspace"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	// The workspace is gone.
	if _, err := env.svc.Get(ctx, env.ownerID, env.wsID); !errors.Is(err, ErrNotFound) {
		t.Errorf("after delete err = %v, want ErrNotFound", err)
	}
}
