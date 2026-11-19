package data

import (
	"errors"
	"strings"
	"testing"
)

func TestColumnRenameAndDelete(t *testing.T) {
	env := setup(t)
	defer env.cleanup()
	ctx := env.ctx

	tbl, err := env.svc.CreateTable(ctx, env.userID, env.wsID, "Cols", testColumns())
	if err != nil {
		t.Fatalf("CreateTable: %v", err)
	}
	score := colID(t, tbl, "Score")
	rec, err := env.svc.CreateRecord(ctx, env.userID, env.wsID, tbl.ID, map[string]any{
		colID(t, tbl, "Name"):  "Ada",
		score:                  float64(42),
	}, "ui")
	if err != nil {
		t.Fatalf("CreateRecord: %v", err)
	}

	// Rename preserves values (keyed by id).
	renamed, err := env.svc.RenameColumn(ctx, env.userID, env.wsID, score, "Points")
	if err != nil {
		t.Fatalf("RenameColumn: %v", err)
	}
	if renamed.Name != "Points" {
		t.Errorf("renamed to %q", renamed.Name)
	}
	got, err := env.svc.getRecord(ctx, env.wsID, rec.ID)
	if err != nil {
		t.Fatalf("getRecord: %v", err)
	}
	if got.Values[score] != float64(42) {
		t.Errorf("value after rename = %v, want 42", got.Values[score])
	}

	// Delete wipes the key from every record.
	if err := env.svc.DeleteColumn(ctx, env.userID, env.wsID, score); err != nil {
		t.Fatalf("DeleteColumn: %v", err)
	}
	got, err = env.svc.getRecord(ctx, env.wsID, rec.ID)
	if err != nil {
		t.Fatalf("getRecord after delete: %v", err)
	}
	if _, exists := got.Values[score]; exists {
		t.Error("deleted column's value survived in records")
	}
	// The column is gone from the table spec.
	t2, err := env.svc.GetTable(ctx, env.userID, env.wsID, tbl.ID)
	if err != nil {
		t.Fatalf("GetTable: %v", err)
	}
	for _, c := range t2.Columns {
		if c.ID == score {
			t.Error("column still in spec after delete")
		}
	}
	// Deleting again is not-found.
	if err := env.svc.DeleteColumn(ctx, env.userID, env.wsID, score); !errors.Is(err, ErrNotFound) {
		t.Errorf("double delete err = %v, want ErrNotFound", err)
	}
}

func TestFormShapeEndpoint(t *testing.T) {
	env := setup(t)
	defer env.cleanup()
	ctx := env.ctx

	tbl, err := env.svc.CreateTable(ctx, env.userID, env.wsID, "Shape", []Column{
		{Name: "Name", Type: ColText},
		{Name: "Plan", Type: ColSelect, Options: []string{"free", "pro"}},
	})
	if err != nil {
		t.Fatalf("CreateTable: %v", err)
	}
	// No token yet: no shape.
	if _, err := env.svc.FormShape(ctx, "no-token"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("shape without token err = %v, want not found", err)
	}
	tbl, err = env.svc.EnableForm(ctx, env.userID, env.wsID, tbl.ID)
	if err != nil {
		t.Fatalf("EnableForm: %v", err)
	}

	shape, err := env.svc.FormShape(ctx, tbl.FormToken)
	if err != nil {
		t.Fatalf("FormShape: %v", err)
	}
	if len(shape.Columns) != 2 {
		t.Fatalf("shape columns = %d, want 2", len(shape.Columns))
	}
	var names []string
	for _, c := range shape.Columns {
		names = append(names, c.Name)
		if c.ID == "" {
			t.Error("shape column missing id (needed for submit validation)")
		}
	}
	if !strings.Contains(strings.Join(names, ","), "Plan") {
		t.Errorf("shape names = %v", names)
	}
	// Select options survive for rendering.
	for _, c := range shape.Columns {
		if c.Name == "Plan" && len(c.Options) != 2 {
			t.Errorf("Plan options = %v", c.Options)
		}
	}
}