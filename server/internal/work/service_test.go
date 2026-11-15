package work

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/orbit-org/orbit/server/internal/automation"
	"github.com/orbit-org/orbit/server/internal/config"
	"github.com/orbit-org/orbit/server/internal/db"
	"github.com/orbit-org/orbit/server/internal/events"
	"github.com/orbit-org/orbit/server/internal/identity"
	"github.com/orbit-org/orbit/server/internal/modules"
	"github.com/orbit-org/orbit/server/internal/workspaces"
)

type env struct {
	svc        *Service
	mods       *modules.Service
	events     *events.Service
	auto       *automation.Service
	runner     *automation.Runner
	userID     string
	outsiderID string
	wsID       string
	cleanup    func()
	ctx        context.Context
}

func setup(t *testing.T) *env {
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
	wsSvc := workspaces.NewService(pool)
	catalog, err := modules.LoadCatalog()
	if err != nil {
		pool.Close()
		t.Fatalf("LoadCatalog: %v", err)
	}
	mods := modules.NewService(pool, catalog)
	eventsSvc := events.NewService(pool)
	workSvc := NewService(pool, eventsSvc, mods)
	autoSvc := automation.NewService(pool)
	runner := automation.NewRunner(pool, config.SMTPConfig{})

	email := fmt.Sprintf("work-%d@orbit.test", time.Now().UnixNano())
	outsiderEmail := fmt.Sprintf("work-out-%d@orbit.test", time.Now().UnixNano())
	user, _, err := ident.Register(ctx, email, "Work User", "password123")
	if err != nil {
		pool.Close()
		t.Fatalf("register: %v", err)
	}
	outsider, _, err := ident.Register(ctx, outsiderEmail, "Outsider", "password123")
	if err != nil {
		pool.Close()
		t.Fatalf("register outsider: %v", err)
	}
	ws, err := wsSvc.Create(ctx, user.ID, fmt.Sprintf("Work Test %d", time.Now().UnixNano()))
	if err != nil {
		pool.Close()
		t.Fatalf("create workspace: %v", err)
	}
	if _, err := mods.Install(ctx, user.ID, ws.ID, ModuleID); err != nil {
		pool.Close()
		t.Fatalf("install work module: %v", err)
	}
	if _, err := mods.Enable(ctx, user.ID, ws.ID, ModuleID); err != nil {
		pool.Close()
		t.Fatalf("enable work module: %v", err)
	}

	cleanup := func() {
		_, _ = pool.Exec(ctx, `DELETE FROM workspaces WHERE created_by = $1`, user.ID)
		for _, e := range []string{email, outsiderEmail} {
			_, _ = pool.Exec(ctx, `DELETE FROM sessions WHERE user_id IN (SELECT id FROM users WHERE email_normalized = $1)`, e)
			_, _ = pool.Exec(ctx, `DELETE FROM audit_logs WHERE actor_user_id IN (SELECT id FROM users WHERE email_normalized = $1)`, e)
			_, _ = pool.Exec(ctx, `DELETE FROM users WHERE email_normalized = $1`, e)
		}
		pool.Close()
	}

	return &env{
		svc: workSvc, mods: mods, events: eventsSvc, auto: autoSvc, runner: runner,
		userID: user.ID, outsiderID: outsider.ID, wsID: ws.ID, cleanup: cleanup, ctx: ctx,
	}
}

func recentEventTypes(t *testing.T, e *env) []string {
	t.Helper()
	evts, err := e.events.List(e.ctx, e.wsID, 50)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	types := make([]string, len(evts))
	for i, evt := range evts {
		types[i] = evt.Type
	}
	return types
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func TestProjectTaskLifecycleWithEvents(t *testing.T) {
	env := setup(t)
	defer env.cleanup()

	if !env.svc.Enabled(env.ctx, env.wsID) {
		t.Fatal("work module not enabled")
	}

	p, err := env.svc.CreateProject(env.ctx, env.userID, env.wsID, "Launch Website", "")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	// Tasks
	due := "2026-12-01"
	t1, err := env.svc.CreateTask(env.ctx, env.userID, env.wsID, p.ID, "Buy domain", TaskPatch{DueDate: &due})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if t1.Status != "todo" || t1.DueDate == nil || *t1.DueDate != due {
		t.Errorf("unexpected task: %+v", t1)
	}
	if _, err := env.svc.CreateTask(env.ctx, env.userID, env.wsID, p.ID, "Write copy", TaskPatch{}); err != nil {
		t.Fatalf("CreateTask 2: %v", err)
	}

	// Events so far: project.created, task.created x2 (no firehose).
	types := recentEventTypes(t, env)
	for _, want := range []string{"task.created", "project.created"} {
		if !contains(types, want) {
			t.Errorf("missing event %q in %v", want, types)
		}
	}
	if contains(types, "task.completed") || contains(types, "task.assigned") {
		t.Errorf("unexpected lifecycle events already: %v", types)
	}

	// Assignment emits task.assigned.
	updated, err := env.svc.UpdateTask(env.ctx, env.userID, env.wsID, t1.ID, TaskPatch{AssigneeID: &env.userID})
	if err != nil {
		t.Fatalf("UpdateTask assign: %v", err)
	}
	if updated.AssigneeID != env.userID {
		t.Errorf("assignee = %q, want %q", updated.AssigneeID, env.userID)
	}
	if !contains(recentEventTypes(t, env), "task.assigned") {
		t.Error("task.assigned not emitted")
	}

	// Project list carries counts.
	projects, err := env.svc.ListProjects(env.ctx, env.userID, env.wsID)
	if err != nil || len(projects) != 1 {
		t.Fatalf("ListProjects = %v, err = %v", projects, err)
	}
	if projects[0].TaskCounts.Todo != 2 {
		t.Errorf("todo count = %d, want 2", projects[0].TaskCounts.Todo)
	}

	// Completing emits task.completed and stamps completed_at.
	done, err := env.svc.SetTaskStatus(env.ctx, env.userID, env.wsID, t1.ID, "done")
	if err != nil {
		t.Fatalf("SetTaskStatus done: %v", err)
	}
	if done.CompletedAt == nil {
		t.Error("completed_at not set")
	}
	if !contains(recentEventTypes(t, env), "task.completed") {
		t.Error("task.completed not emitted")
	}

	// Reopening emits task.reopened and clears completed_at.
	reopened, err := env.svc.SetTaskStatus(env.ctx, env.userID, env.wsID, t1.ID, "todo")
	if err != nil {
		t.Fatalf("SetTaskStatus reopen: %v", err)
	}
	if reopened.CompletedAt != nil {
		t.Error("completed_at not cleared on reopen")
	}
	if !contains(recentEventTypes(t, env), "task.reopened") {
		t.Error("task.reopened not emitted")
	}

	// A silent move emits nothing.
	if _, err := env.svc.SetTaskStatus(env.ctx, env.userID, env.wsID, t1.ID, "in_progress"); err != nil {
		t.Fatalf("SetTaskStatus silent: %v", err)
	}
	types = recentEventTypes(t, env)
	if contains(types, "task.updated") {
		t.Error("unexpected event on non-lifecycle move")
	}

	// Delete removes the task.
	if err := env.svc.DeleteTask(env.ctx, env.userID, env.wsID, t1.ID); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	if _, err := env.svc.GetTask(env.ctx, env.userID, env.wsID, t1.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetTask after delete err = %v, want ErrNotFound", err)
	}
}

func TestWorkspaceIsolation(t *testing.T) {
	env := setup(t)
	defer env.cleanup()

	p, err := env.svc.CreateProject(env.ctx, env.userID, env.wsID, "Private project", "")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	t1, err := env.svc.CreateTask(env.ctx, env.userID, env.wsID, p.ID, "Secret task", TaskPatch{})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	// The outsider sees no projects.
	items, err := env.svc.ListProjects(env.ctx, env.outsiderID, env.wsID)
	if err == nil {
		t.Fatalf("outsider listed projects: err = nil, items = %v; want ErrForbidden", items)
	}
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("outsider err = %v, want ErrForbidden", err)
	}
	if _, err := env.svc.GetTask(env.ctx, env.outsiderID, env.wsID, t1.ID); !errors.Is(err, ErrForbidden) {
		t.Errorf("outsider GetTask err = %v, want ErrForbidden", err)
	}
	if err := env.svc.DeleteTask(env.ctx, env.outsiderID, env.wsID, t1.ID); !errors.Is(err, ErrForbidden) {
		t.Errorf("outsider DeleteTask err = %v, want ErrForbidden", err)
	}
}

func TestAssigneeMustBeMember(t *testing.T) {
	env := setup(t)
	defer env.cleanup()

	p, err := env.svc.CreateProject(env.ctx, env.userID, env.wsID, "P", "")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if _, err := env.svc.CreateTask(env.ctx, env.userID, env.wsID, p.ID, "T", TaskPatch{AssigneeID: &env.outsiderID}); err == nil {
		t.Error("assigned outsider succeeded; want error")
	}
}

func TestTaskCompletedFiresWorkflow(t *testing.T) {
	env := setup(t)
	defer env.cleanup()

	// A workflow that fires whenever a task completes.
	wf, err := env.auto.Create(env.ctx, env.userID, env.wsID, "On task done", automation.Definition{
		Trigger: automation.Trigger{Type: "event", EventType: "task.completed"},
		Nodes: []automation.Node{
			{ID: "done", Type: "log", Config: automation.NodeConfig{Message: "task {{payload.title}} completed"}},
		},
		Edges: []automation.Edge{{From: "trigger", To: "done"}},
	})
	if err != nil {
		t.Fatalf("Create workflow: %v", err)
	}
	if _, err := env.auto.Validate(env.ctx, env.userID, env.wsID, wf.ID); err != nil {
		t.Fatalf("Validate workflow: %v", err)
	}

	// Create a task and complete it.
	p, err := env.svc.CreateProject(env.ctx, env.userID, env.wsID, "Ship it", "")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	task, err := env.svc.CreateTask(env.ctx, env.userID, env.wsID, p.ID, "Write release notes", TaskPatch{})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if _, err := env.svc.SetTaskStatus(env.ctx, env.userID, env.wsID, task.ID, "done"); err != nil {
		t.Fatalf("SetTaskStatus: %v", err)
	}

	// Relay the event and drain the queue.
	if err := env.runner.RelayEvents(env.ctx); err != nil {
		t.Fatalf("RelayEvents: %v", err)
	}
	for i := 0; i < 50; i++ {
		ran, err := env.runner.ProcessNext(env.ctx)
		if err != nil {
			t.Fatalf("ProcessNext: %v", err)
		}
		if !ran {
			break
		}
	}

	runs, err := env.auto.ListRuns(env.ctx, env.userID, env.wsID, wf.ID)
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs = %d, err = %v; want exactly 1 event-triggered run", len(runs), err)
	}
	if runs[0].Status != "completed" {
		t.Errorf("run status = %q, want completed", runs[0].Status)
	}
	if runs[0].TriggerType != "event" {
		t.Errorf("trigger type = %q, want event", runs[0].TriggerType)
	}
}
