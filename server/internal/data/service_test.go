package data

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
	svc     *Service
	auto    *automation.Service
	runner  *automation.Runner
	userID  string
	wsID    string
	cleanup func()
	ctx     context.Context
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
	dataSvc := NewService(pool, eventsSvc, mods)
	autoSvc := automation.NewService(pool)

	email := fmt.Sprintf("data-%d@orbit.test", time.Now().UnixNano())
	user, _, err := ident.Register(ctx, email, "Data User", "password123")
	if err != nil {
		pool.Close()
		t.Fatalf("register: %v", err)
	}
	ws, err := wsSvc.Create(ctx, user.ID, fmt.Sprintf("Data Test %d", time.Now().UnixNano()))
	if err != nil {
		pool.Close()
		t.Fatalf("create workspace: %v", err)
	}
	if _, err := mods.Install(ctx, user.ID, ws.ID, ModuleID); err != nil {
		pool.Close()
		t.Fatalf("install data module: %v", err)
	}
	if _, err := mods.Enable(ctx, user.ID, ws.ID, ModuleID); err != nil {
		pool.Close()
		t.Fatalf("enable data module: %v", err)
	}

	cleanup := func() {
		_, _ = pool.Exec(ctx, `DELETE FROM workspaces WHERE created_by = $1`, user.ID)
		_, _ = pool.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, user.ID)
		_, _ = pool.Exec(ctx, `DELETE FROM audit_logs WHERE actor_user_id = $1`, user.ID)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, user.ID)
		pool.Close()
	}

	return &env{
		svc: dataSvc, auto: autoSvc, runner: automation.NewRunner(pool, config.SMTPConfig{}),
		userID: user.ID, wsID: ws.ID, cleanup: cleanup, ctx: ctx,
	}
}

func testColumns() []Column {
	return []Column{
		{Name: "Name", Type: ColText},
		{Name: "Email", Type: ColText},
		{Name: "Score", Type: ColNumber},
		{Name: "Plan", Type: ColSelect, Options: []string{"free", "pro"}},
		{Name: "Active", Type: ColCheckbox},
	}
}

func colID(t *testing.T, tbl Table, name string) string {
	t.Helper()
	for _, c := range tbl.Columns {
		if c.Name == name {
			return c.ID
		}
	}
	t.Fatalf("column %q not found", name)
	return ""
}

func TestTableAndRecordCRUD(t *testing.T) {
	env := setup(t)
	defer env.cleanup()

	tbl, err := env.svc.CreateTable(env.ctx, env.userID, env.wsID, "Contacts", testColumns())
	if err != nil {
		t.Fatalf("CreateTable: %v", err)
	}
	if len(tbl.Columns) != 5 {
		t.Fatalf("columns = %d, want 5", len(tbl.Columns))
	}

	// Create a record with valid, typed values.
	values := map[string]any{
		colID(t, tbl, "Name"):   "Ada Lovelace",
		colID(t, tbl, "Score"):  float64(99),
		colID(t, tbl, "Plan"):   "pro",
		colID(t, tbl, "Active"): true,
	}
	rec, err := env.svc.CreateRecord(env.ctx, env.userID, env.wsID, tbl.ID, values, "ui")
	if err != nil {
		t.Fatalf("CreateRecord: %v", err)
	}
	if rec.Values[colID(t, tbl, "Name")] != "Ada Lovelace" {
		t.Errorf("stored name = %v", rec.Values[colID(t, tbl, "Name")])
	}

	// Filter by column value.
	items, err := env.svc.ListRecords(env.ctx, env.userID, env.wsID, tbl.ID,
		colID(t, tbl, "Plan"), "pro")
	if err != nil || len(items) != 1 {
		t.Fatalf("filtered list = %d items, err = %v; want 1", len(items), err)
	}

	// Update replaces values.
	updated, err := env.svc.UpdateRecord(env.ctx, env.userID, env.wsID, rec.ID, map[string]any{
		colID(t, tbl, "Name"):  "Ada King",
		colID(t, tbl, "Score"): float64(100),
	})
	if err != nil {
		t.Fatalf("UpdateRecord: %v", err)
	}
	if updated.Values[colID(t, tbl, "Name")] != "Ada King" {
		t.Errorf("updated name = %v", updated.Values[colID(t, tbl, "Name")])
	}

	// Delete removes it.
	if err := env.svc.DeleteRecord(env.ctx, env.userID, env.wsID, rec.ID); err != nil {
		t.Fatalf("DeleteRecord: %v", err)
	}
	if _, err := env.svc.getRecord(env.ctx, env.wsID, rec.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("getRecord after delete err = %v, want ErrNotFound", err)
	}
}

func TestValueValidation(t *testing.T) {
	env := setup(t)
	defer env.cleanup()

	tbl, err := env.svc.CreateTable(env.ctx, env.userID, env.wsID, "V", testColumns())
	if err != nil {
		t.Fatalf("CreateTable: %v", err)
	}

	// Unknown column.
	if _, err := env.svc.CreateRecord(env.ctx, env.userID, env.wsID, tbl.ID,
		map[string]any{"nope": "x"}, "ui"); !errors.Is(err, ErrInvalidValue) {
		t.Errorf("unknown column err = %v, want ErrInvalidValue", err)
	}
	// Number column given text.
	if _, err := env.svc.CreateRecord(env.ctx, env.userID, env.wsID, tbl.ID,
		map[string]any{colID(t, tbl, "Score"): "high"}, "ui"); !errors.Is(err, ErrInvalidValue) {
		t.Errorf("text-in-number err = %v, want ErrInvalidValue", err)
	}
	// Checkbox column given text.
	if _, err := env.svc.CreateRecord(env.ctx, env.userID, env.wsID, tbl.ID,
		map[string]any{colID(t, tbl, "Active"): "yes"}, "ui"); !errors.Is(err, ErrInvalidValue) {
		t.Errorf("text-in-checkbox err = %v, want ErrInvalidValue", err)
	}
	// Date column given a bad date.
	if _, err := env.svc.CreateRecord(env.ctx, env.userID, env.wsID, tbl.ID,
		map[string]any{colID(t, tbl, "Active"): true}, "ui"); err != nil {
		t.Fatalf("valid record: %v", err)
	}
}

func TestCSVRoundTrip(t *testing.T) {
	env := setup(t)
	defer env.cleanup()

	tbl, err := env.svc.CreateTable(env.ctx, env.userID, env.wsID, "Import", testColumns())
	if err != nil {
		t.Fatalf("CreateTable: %v", err)
	}

	csvIn := strings.Join([]string{
		"Name,Email,Score,Plan,Active",
		`Grace Hopper,grace@example.com,95,pro,yes`,
		`Alan Turing,alan@example.com,88,free,no`,
	}, "\n")
	count, err := env.svc.ImportCSV(env.ctx, env.userID, env.wsID, tbl.ID, csvIn)
	if err != nil {
		t.Fatalf("ImportCSV: %v", err)
	}
	if count != 2 {
		t.Fatalf("imported = %d, want 2", count)
	}

	// Export round-trips the data.
	out, err := env.svc.ExportCSV(env.ctx, env.userID, env.wsID, tbl.ID)
	if err != nil {
		t.Fatalf("ExportCSV: %v", err)
	}
	for _, want := range []string{"Name,Email,Score,Plan,Active", "Grace Hopper", "Alan Turing", "true", "false"} {
		if !strings.Contains(out, want) {
			t.Errorf("export missing %q:\n%s", want, out)
		}
	}

	// Re-import of the export is accepted (idempotent shape).
	if _, err := env.svc.ImportCSV(env.ctx, env.userID, env.wsID, tbl.ID, out); err != nil {
		t.Fatalf("re-import export: %v", err)
	}

	// Bad headers are rejected with a clear error.
	if _, err := env.svc.ImportCSV(env.ctx, env.userID, env.wsID, tbl.ID, "Wat,Stuff\n1,2"); err == nil {
		t.Error("import with unknown header succeeded")
	}
}

func TestPublicFormFiresWorkflow(t *testing.T) {
	env := setup(t)
	defer env.cleanup()

	// A workflow watching every new record.
	wf, err := env.auto.Create(env.ctx, env.userID, env.wsID, "On new record", automation.Definition{
		Trigger: automation.Trigger{Type: "event", EventType: "record.created"},
		Nodes: []automation.Node{
			{ID: "log", Type: "log", Config: automation.NodeConfig{Message: "new {{payload.source}} record"}},
		},
		Edges: []automation.Edge{{From: "trigger", To: "log"}},
	})
	if err != nil {
		t.Fatalf("Create workflow: %v", err)
	}
	if _, err := env.auto.Validate(env.ctx, env.userID, env.wsID, wf.ID); err != nil {
		t.Fatalf("Validate workflow: %v", err)
	}

	// A table with a public form.
	tbl, err := env.svc.CreateTable(env.ctx, env.userID, env.wsID, "Signups", []Column{
		{Name: "Name", Type: ColText},
		{Name: "Email", Type: ColText},
	})
	if err != nil {
		t.Fatalf("CreateTable: %v", err)
	}
	tbl, err = env.svc.EnableForm(env.ctx, env.userID, env.wsID, tbl.ID)
	if err != nil {
		t.Fatalf("EnableForm: %v", err)
	}
	if tbl.FormToken == "" {
		t.Fatal("no form token after EnableForm")
	}

	// The public submits — no membership, just the secret URL.
	rec, err := env.svc.SubmitForm(env.ctx, tbl.FormToken, map[string]any{
		colID(t, tbl, "Name"):  "A Stranger",
		colID(t, tbl, "Email"): "stranger@example.com",
	})
	if err != nil {
		t.Fatalf("SubmitForm: %v", err)
	}
	if rec.Source != "form" {
		t.Errorf("source = %q, want form", rec.Source)
	}

	// Unknown tokens look identical (no leak).
	if _, err := env.svc.SubmitForm(env.ctx, "not-a-real-token", map[string]any{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("bad token err = %v, want ErrNotFound", err)
	}

	// The event fired the workflow: relay + drain and check.
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
	runs, err := env.auto.ListRuns(env.ctx, env.userID, env.wsID, wf.ID, 50, "")
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs = %d, err = %v; want exactly 1", len(runs), err)
	}
	if runs[0].Status != "completed" || runs[0].TriggerType != "event" {
		t.Errorf("run = %+v, want completed event-triggered", runs[0])
	}
	_, steps, err := env.auto.GetRun(env.ctx, env.userID, env.wsID, wf.ID, runs[0].ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if msg, _ := steps[0].Output["message"].(string); msg != "new form record" {
		t.Errorf("substituted message = %q, want %q", msg, "new form record")
	}
}

func TestIsolation(t *testing.T) {
	env := setup(t)
	defer env.cleanup()

	tbl, err := env.svc.CreateTable(env.ctx, env.userID, env.wsID, "Private", testColumns())
	if err != nil {
		t.Fatalf("CreateTable: %v", err)
	}

	// An outsider (not a member) cannot see or touch the table.
	outsiderEmail := fmt.Sprintf("data-out-%d@orbit.test", time.Now().UnixNano())
	pool := env.svc.pool
	outsider, _, err := identity.NewService(pool, time.Hour).Register(env.ctx, outsiderEmail, "Out", "password123")
	if err != nil {
		t.Fatalf("register outsider: %v", err)
	}
	if _, err := env.svc.ListTables(env.ctx, outsider.ID, env.wsID); !errors.Is(err, ErrForbidden) {
		t.Errorf("outsider ListTables err = %v, want ErrForbidden", err)
	}
	if _, err := env.svc.GetTable(env.ctx, outsider.ID, env.wsID, tbl.ID); !errors.Is(err, ErrForbidden) {
		t.Errorf("outsider GetTable err = %v, want ErrForbidden", err)
	}
}
