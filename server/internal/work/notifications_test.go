package work

import (
	"testing"

	"github.com/orbit-org/orbit/server/internal/notifications"
)

// TestAssignmentAndCompletionNotify exercises the notification producers:
// assigning a task notifies the assignee; completing someone else's task
// notifies its creator. Self-actions never notify.
func TestAssignmentAndCompletionNotify(t *testing.T) {
	env := setup(t)
	defer env.cleanup()

	// The outsider becomes a member for this test.
	if _, err := env.svc.pool.Exec(env.ctx,
		`INSERT INTO memberships (workspace_id, user_id, role) VALUES ($1, $2, 'member')`,
		env.wsID, env.outsiderID,
	); err != nil {
		t.Fatalf("add outsider as member: %v", err)
	}
	notif := notifications.NewService(env.svc.pool)

	p, err := env.svc.CreateProject(env.ctx, env.userID, env.wsID, "Notify project", "")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	// Creator creates a task assigned to the outsider.
	outsider := env.outsiderID
	task, err := env.svc.CreateTask(env.ctx, env.userID, env.wsID, p.ID, "Review the plan", TaskPatch{AssigneeID: &outsider})
	if err != nil {
		t.Fatalf("CreateTask assigned: %v", err)
	}

	items, unread, err := notif.List(env.ctx, env.outsiderID, 20)
	if err != nil {
		t.Fatalf("list assignee notifications: %v", err)
	}
	if unread != 1 || len(items) == 0 || items[0].Type != "task.assigned" {
		t.Fatalf("assignee notifications = %+v (unread %d), want one task.assigned", items, unread)
	}

	// The assignee completes the task; the creator is told.
	if _, err := env.svc.SetTaskStatus(env.ctx, env.outsiderID, env.wsID, task.ID, "done"); err != nil {
		t.Fatalf("SetTaskStatus by assignee: %v", err)
	}
	items, _, err = notif.List(env.ctx, env.userID, 20)
	if err != nil {
		t.Fatalf("list creator notifications: %v", err)
	}
	found := false
	for _, n := range items {
		if n.Type == "task.completed" && n.Payload["taskId"] == task.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("creator notifications = %+v, want a task.completed for the task", items)
	}

	// Completing your own task never notifies you.
	before, unreadBefore, _ := notif.List(env.ctx, env.userID, 20)
	own, err := env.svc.CreateTask(env.ctx, env.userID, env.wsID, p.ID, "Own task", TaskPatch{})
	if err != nil {
		t.Fatalf("CreateTask own: %v", err)
	}
	if _, err := env.svc.SetTaskStatus(env.ctx, env.userID, env.wsID, own.ID, "done"); err != nil {
		t.Fatalf("SetTaskStatus own: %v", err)
	}
	after, unreadAfter, _ := notif.List(env.ctx, env.userID, 20)
	if unreadAfter != unreadBefore || len(after) != len(before) {
		t.Errorf("self-completion notified: unread %d -> %d, items %d -> %d",
			unreadBefore, unreadAfter, len(before), len(after))
	}
}
