package work

import (
	"errors"
	"testing"
)

func TestSubtasks(t *testing.T) {
	env := setup(t)
	defer env.cleanup()
	ctx := env.ctx

	p, err := env.svc.CreateProject(ctx, env.userID, env.wsID, "Subtask project", "")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	task, err := env.svc.CreateTask(ctx, env.userID, env.wsID, p.ID, "Big task", TaskPatch{})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	// Create subtasks.
	s1, err := env.svc.CreateSubtask(ctx, env.userID, env.wsID, task.ID, "Step one")
	if err != nil {
		t.Fatalf("CreateSubtask 1: %v", err)
	}
	if _, err := env.svc.CreateSubtask(ctx, env.userID, env.wsID, task.ID, "Step two"); err != nil {
		t.Fatalf("CreateSubtask 2: %v", err)
	}

	items, err := env.svc.ListSubtasks(ctx, env.userID, env.wsID, task.ID)
	if err != nil || len(items) != 2 {
		t.Fatalf("ListSubtasks = %d, err = %v; want 2", len(items), err)
	}

	// Toggle done.
	done := true
	updated, err := env.svc.UpdateSubtask(ctx, env.userID, env.wsID, s1.ID, nil, &done)
	if err != nil {
		t.Fatalf("UpdateSubtask: %v", err)
	}
	if !updated.Done {
		t.Error("subtask not done after toggle")
	}

	// Title update.
	newTitle := "Step one (revised)"
	updated, err = env.svc.UpdateSubtask(ctx, env.userID, env.wsID, s1.ID, &newTitle, nil)
	if err != nil {
		t.Fatalf("UpdateSubtask title: %v", err)
	}
	if updated.Title != newTitle {
		t.Errorf("title = %q", updated.Title)
	}

	// Empty titles rejected.
	if _, err := env.svc.CreateSubtask(ctx, env.userID, env.wsID, task.ID, "  "); !errors.Is(err, ErrSubtaskTitle) {
		t.Errorf("empty title err = %v, want ErrSubtaskTitle", err)
	}

	// Delete.
	if err := env.svc.DeleteSubtask(ctx, env.userID, env.wsID, s1.ID); err != nil {
		t.Fatalf("DeleteSubtask: %v", err)
	}
	items, _ = env.svc.ListSubtasks(ctx, env.userID, env.wsID, task.ID)
	if len(items) != 1 {
		t.Errorf("after delete = %d, want 1", len(items))
	}

	// Subtasks cascade with task deletion.
	if err := env.svc.DeleteTask(ctx, env.userID, env.wsID, task.ID); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	if items, err := env.svc.ListSubtasks(ctx, env.userID, env.wsID, task.ID); err == nil && len(items) > 0 {
		t.Error("subtasks survived task deletion")
	}
}

func TestDependencies(t *testing.T) {
	env := setup(t)
	defer env.cleanup()
	ctx := env.ctx

	p, err := env.svc.CreateProject(ctx, env.userID, env.wsID, "Dep project", "")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	a, err := env.svc.CreateTask(ctx, env.userID, env.wsID, p.ID, "First", TaskPatch{})
	if err != nil {
		t.Fatalf("CreateTask A: %v", err)
	}
	b, err := env.svc.CreateTask(ctx, env.userID, env.wsID, p.ID, "Second", TaskPatch{})
	if err != nil {
		t.Fatalf("CreateTask B: %v", err)
	}
	c, err := env.svc.CreateTask(ctx, env.userID, env.wsID, p.ID, "Third", TaskPatch{})
	if err != nil {
		t.Fatalf("CreateTask C: %v", err)
	}

	// B is blocked by A; C is blocked by B.
	if err := env.svc.AddDependency(ctx, env.userID, env.wsID, b.ID, a.ID); err != nil {
		t.Fatalf("AddDependency B←A: %v", err)
	}
	if err := env.svc.AddDependency(ctx, env.userID, env.wsID, c.ID, b.ID); err != nil {
		t.Fatalf("AddDependency C←B: %v", err)
	}

	// Cycle: A blocked by C would cycle (A→B→C→A).
	if err := env.svc.AddDependency(ctx, env.userID, env.wsID, a.ID, c.ID); !errors.Is(err, ErrCycle) {
		t.Errorf("cycle err = %v, want ErrCycle", err)
	}

	// Self-dependency rejected.
	if err := env.svc.AddDependency(ctx, env.userID, env.wsID, a.ID, a.ID); !errors.Is(err, ErrDependency) {
		t.Errorf("self-dep err = %v, want ErrDependency", err)
	}

	// Info shows the graph.
	info, err := env.svc.DependencyInfo(ctx, env.userID, env.wsID, b.ID)
	if err != nil {
		t.Fatalf("DependencyInfo: %v", err)
	}
	if len(info.BlockedBy) != 1 || info.BlockedBy[0].ID != a.ID {
		t.Errorf("B blockedBy = %+v", info.BlockedBy)
	}
	if len(info.Blocks) != 1 || info.Blocks[0].ID != c.ID {
		t.Errorf("B blocks = %+v", info.Blocks)
	}

	// Completing A unblocks B (B's only blocker is done) and fires the event.
	if _, err := env.svc.SetTaskStatus(ctx, env.userID, env.wsID, a.ID, "done"); err != nil {
		t.Fatalf("complete A: %v", err)
	}
	blocked, err := env.svc.isTaskBlocked(ctx, b.ID)
	if err != nil {
		t.Fatalf("isTaskBlocked B: %v", err)
	}
	if blocked {
		t.Error("B still blocked after A completed")
	}
	// C is still blocked by B.
	blockedC, _ := env.svc.isTaskBlocked(ctx, c.ID)
	if !blockedC {
		t.Error("C unblocked prematurely")
	}

	// The event log has task.unblocked for B.
	evts, _ := env.events.List(ctx, env.wsID, 50)
	found := false
	for _, e := range evts {
		if e.Type == "task.unblocked" && e.Payload["taskId"] == b.ID {
			found = true
		}
	}
	if !found {
		t.Error("task.unblocked event missing for B")
	}

	// Remove works.
	if err := env.svc.RemoveDependency(ctx, env.userID, env.wsID, c.ID, b.ID); err != nil {
		t.Fatalf("RemoveDependency: %v", err)
	}
	info, _ = env.svc.DependencyInfo(ctx, env.userID, env.wsID, c.ID)
	if len(info.BlockedBy) != 0 {
		t.Errorf("C still has blockers: %+v", info.BlockedBy)
	}
}

func TestTimeTracking(t *testing.T) {
	env := setup(t)
	defer env.cleanup()
	ctx := env.ctx

	p, err := env.svc.CreateProject(ctx, env.userID, env.wsID, "Time project", "")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	task, err := env.svc.CreateTask(ctx, env.userID, env.wsID, p.ID, "Timed task", TaskPatch{})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	// Start a timer.
	e1, err := env.svc.StartTimer(ctx, env.userID, env.wsID, task.ID)
	if err != nil {
		t.Fatalf("StartTimer: %v", err)
	}
	if e1.EndedAt != nil {
		t.Error("fresh entry should be running")
	}

	// Second start on the same task fails.
	if _, err := env.svc.StartTimer(ctx, env.userID, env.wsID, task.ID); !errors.Is(err, ErrTimeRunning) {
		t.Errorf("double start err = %v, want ErrTimeRunning", err)
	}

	// Stop computes minutes >= 1.
	stopped, err := env.svc.StopTimer(ctx, env.userID, env.wsID, task.ID)
	if err != nil {
		t.Fatalf("StopTimer: %v", err)
	}
	if stopped.Minutes < 1 {
		t.Errorf("minutes = %d, want >= 1", stopped.Minutes)
	}

	// Stop with no active timer fails.
	if _, err := env.svc.StopTimer(ctx, env.userID, env.wsID, task.ID); !errors.Is(err, ErrTimeNotActive) {
		t.Errorf("stop-none err = %v, want ErrTimeNotActive", err)
	}

	// Manual entry.
	manual, err := env.svc.LogTime(ctx, env.userID, env.wsID, task.ID, 90, "deep work")
	if err != nil {
		t.Fatalf("LogTime: %v", err)
	}
	if manual.Minutes != 90 || manual.Note != "deep work" {
		t.Errorf("manual = %+v", manual)
	}

	// Bounds.
	if _, err := env.svc.LogTime(ctx, env.userID, env.wsID, task.ID, 0, ""); !errors.Is(err, ErrTimeMinutes) {
		t.Errorf("0 minutes err = %v", err)
	}
	if _, err := env.svc.LogTime(ctx, env.userID, env.wsID, task.ID, 2000, ""); !errors.Is(err, ErrTimeMinutes) {
		t.Errorf("2000 minutes err = %v", err)
	}

	// List shows both entries.
	entries, err := env.svc.ListTime(ctx, env.userID, env.wsID, task.ID)
	if err != nil || len(entries) != 2 {
		t.Fatalf("ListTime = %d, err = %v; want 2", len(entries), err)
	}

	// Delete an entry.
	if err := env.svc.DeleteTimeEntry(ctx, env.userID, env.wsID, manual.ID); err != nil {
		t.Fatalf("DeleteTimeEntry: %v", err)
	}
	entries, _ = env.svc.ListTime(ctx, env.userID, env.wsID, task.ID)
	if len(entries) != 1 {
		t.Errorf("after delete = %d, want 1", len(entries))
	}

	// Completing the task clears active timers.
	if _, err := env.svc.StartTimer(ctx, env.userID, env.wsID, task.ID); err != nil {
		t.Fatalf("re-start: %v", err)
	}
	if _, err := env.svc.SetTaskStatus(ctx, env.userID, env.wsID, task.ID, "done"); err != nil {
		t.Fatalf("complete: %v", err)
	}
	entries, _ = env.svc.ListTime(ctx, env.userID, env.wsID, task.ID)
	for _, e := range entries {
		if e.EndedAt == nil {
			t.Error("active timer survived task completion")
		}
	}
}
