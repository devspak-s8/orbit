package work

import (
	"context"
	"strings"
	"testing"

	"github.com/orbit-org/orbit/server/internal/notifications"
)

func TestCommentsAndLabels(t *testing.T) {
	env := setup(t)
	defer env.cleanup()
	ctx := context.Background()

	// A second member to receive notifications.
	if _, err := env.svc.pool.Exec(ctx,
		`INSERT INTO memberships (workspace_id, user_id, role) VALUES ($1, $2, 'member')`,
		env.wsID, env.outsiderID,
	); err != nil {
		t.Fatalf("add member: %v", err)
	}
	notif := notifications.NewService(env.svc.pool)

	p, err := env.svc.CreateProject(ctx, env.userID, env.wsID, "Comment project", "")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	outsider := env.outsiderID
	task, err := env.svc.CreateTask(ctx, env.userID, env.wsID, p.ID, "Discuss the design", TaskPatch{AssigneeID: &outsider})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	// Labels: create, assign, and see them on the task.
	label, err := env.svc.CreateLabel(ctx, env.userID, env.wsID, p.ID, "design", "violet")
	if err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}
	if err := env.svc.SetTaskLabels(ctx, env.userID, env.wsID, task.ID, []string{label.ID}); err != nil {
		t.Fatalf("SetTaskLabels: %v", err)
	}
	got, err := env.svc.GetTask(ctx, env.userID, env.wsID, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if len(got.Labels) != 1 || got.Labels[0].Name != "design" {
		t.Fatalf("task labels = %+v, want one 'design'", got.Labels)
	}
	// ListTasks also carries labels.
	tasks, err := env.svc.ListTasks(ctx, env.userID, env.wsID, p.ID, "")
	if err != nil || len(tasks) != 1 {
		t.Fatalf("ListTasks = %v, err = %v", tasks, err)
	}
	if len(tasks[0].Labels) != 1 || tasks[0].Labels[0].Color != "violet" {
		t.Errorf("ListTasks labels = %+v", tasks[0].Labels)
	}
	// Labels from other projects are rejected.
	p2, err := env.svc.CreateProject(ctx, env.userID, env.wsID, "Other project", "")
	if err != nil {
		t.Fatalf("CreateProject 2: %v", err)
	}
	label2, err := env.svc.CreateLabel(ctx, env.userID, env.wsID, p2.ID, "elsewhere", "amber")
	if err != nil {
		t.Fatalf("CreateLabel 2: %v", err)
	}
	if err := env.svc.SetTaskLabels(ctx, env.userID, env.wsID, task.ID, []string{label2.ID}); err == nil {
		t.Error("cross-project label accepted")
	}

	// Comments: post, list, and the assignee is notified.
	c1, err := env.svc.AddComment(ctx, env.userID, env.wsID, task.ID, "First! Here's the mock.")
	if err != nil {
		t.Fatalf("AddComment 1: %v", err)
	}
	if _, err := env.svc.AddComment(ctx, env.outsiderID, env.wsID, task.ID, "Looks good to me."); err != nil {
		t.Fatalf("AddComment 2: %v", err)
	}
	items, err := env.svc.ListComments(ctx, env.userID, env.wsID, task.ID)
	if err != nil || len(items) != 2 {
		t.Fatalf("ListComments = %d items, err = %v; want 2", len(items), err)
	}
	if items[0].ID != c1.ID || items[0].AuthorName != "Work User" {
		t.Errorf("comment order/author wrong: %+v", items[0])
	}
	if items[1].AuthorName != "Outsider" {
		t.Errorf("second comment author = %q", items[1].AuthorName)
	}
	// Empty bodies are rejected.
	if _, err := env.svc.AddComment(ctx, env.userID, env.wsID, task.ID, "   "); err == nil {
		t.Error("empty comment accepted")
	}
	// Long bodies are rejected.
	if _, err := env.svc.AddComment(ctx, env.userID, env.wsID, task.ID, strings.Repeat("x", 4001)); err == nil {
		t.Error("oversized comment accepted")
	}

	// The assignee was notified about the author's comment (not their own).
	assigneeItems, unread, err := notif.List(ctx, env.outsiderID, 20)
	if err != nil {
		t.Fatalf("assignee notifications: %v", err)
	}
	foundComment := false
	for _, n := range assigneeItems {
		if n.Type == "task.commented" {
			foundComment = true
			break
		}
	}
	if !foundComment || unread < 1 {
		t.Errorf("assignee not notified: items = %+v, unread = %d", assigneeItems, unread)
	}

	// The event log carries task.commented.
	evts, err := env.events.List(ctx, env.wsID, 50)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	seen := false
	for _, e := range evts {
		if e.Type == "task.commented" {
			seen = true
		}
	}
	if !seen {
		t.Error("task.commented event missing from the log")
	}

	// Deleting the task cascades comments.
	if err := env.svc.DeleteTask(ctx, env.userID, env.wsID, task.ID); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	if _, err := env.svc.ListComments(ctx, env.userID, env.wsID, task.ID); err == nil {
		t.Error("comments survived task deletion")
	}
}
