package automation

import (
	"context"
	"errors"
	"testing"
)

func TestBranchRoutesYesAndNo(t *testing.T) {
	env := setup(t)
	defer env.cleanup()
	ctx := context.Background()

	// A branch: score > 50 -> "big" log node; else "small" log node.
	def := Definition{
		Trigger: Trigger{Type: "manual"},
		Nodes: []Node{
			{ID: "gate", Type: "branch", Config: NodeConfig{Condition: &Condition{
				Field: "{{payload.score}}", Op: ">", Value: 50,
			}}},
			{ID: "big", Type: "log", Config: NodeConfig{Message: "big score"}},
			{ID: "small", Type: "log", Config: NodeConfig{Message: "small score"}},
		},
		Edges: []Edge{
			{From: "trigger", To: "gate"},
			{From: "gate", To: "big", Label: "yes"},
			{From: "gate", To: "small", Label: "no"},
		},
	}
	if err := def.Validate(); err != nil {
		t.Fatalf("branch definition invalid: %v", err)
	}
	wf, err := env.svc.Create(ctx, env.userID, env.wsID, "Gated", def)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := env.svc.Validate(ctx, env.userID, env.wsID, wf.ID); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	wfFull, err := env.svc.Get(ctx, env.userID, env.wsID, wf.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	// High score takes the yes path.
	high, err := env.svc.enqueueWithPayload(ctx, wfFull, map[string]any{"score": 99})
	if err != nil {
		t.Fatalf("enqueue high: %v", err)
	}
	// Low score takes the no path.
	low, err := env.svc.enqueueWithPayload(ctx, wfFull, map[string]any{"score": 10})
	if err != nil {
		t.Fatalf("enqueue low: %v", err)
	}
	for i := 0; i < 50; i++ {
		ran, err := env.runner.ProcessNext(ctx)
		if err != nil {
			t.Fatalf("ProcessNext: %v", err)
		}
		if !ran {
			break
		}
	}

	_, highSteps, err := env.svc.GetRun(ctx, env.userID, env.wsID, wf.ID, high.ID)
	if err != nil {
		t.Fatalf("GetRun high: %v", err)
	}
	byNode := map[string]Step{}
	for _, st := range highSteps {
		byNode[st.NodeID] = st
	}
	if byNode["gate"].Status != "completed" || byNode["gate"].Output["taken"] != "yes" {
		t.Errorf("gate on high = %+v, want completed/yes", byNode["gate"])
	}
	if byNode["big"].Status != "completed" {
		t.Errorf("big on high = %+v, want completed", byNode["big"])
	}
	if byNode["small"].Status != "skipped" {
		t.Errorf("small on high = %+v, want skipped", byNode["small"])
	}

	_, lowSteps, err := env.svc.GetRun(ctx, env.userID, env.wsID, wf.ID, low.ID)
	if err != nil {
		t.Fatalf("GetRun low: %v", err)
	}
	byNode = map[string]Step{}
	for _, st := range lowSteps {
		byNode[st.NodeID] = st
	}
	if byNode["gate"].Output["taken"] != "no" {
		t.Errorf("gate on low = %+v, want no", byNode["gate"])
	}
	if byNode["small"].Status != "completed" || byNode["big"].Status != "skipped" {
		t.Errorf("low run routing wrong: small=%+v big=%+v", byNode["small"], byNode["big"])
	}
}

func TestBranchStringEquality(t *testing.T) {
	// == on strings, and a mismatched numeric comparison error.
	if got, err := evalBranch(&Condition{Field: "{{payload.plan}}", Op: "==", Value: "pro"}, map[string]any{"plan": "pro"}); err != nil || got != "yes" {
		t.Errorf("string ==: got %q, err %v; want yes", got, err)
	}
	if got, err := evalBranch(&Condition{Field: "{{payload.plan}}", Op: "!=", Value: "pro"}, map[string]any{"plan": "pro"}); err != nil || got != "no" {
		t.Errorf("string !=: got %q, err %v; want no", got, err)
	}
	if _, err := evalBranch(&Condition{Field: "abc", Op: ">", Value: "notanumber"}, nil); err == nil {
		t.Error("ordered op on non-numerics should error")
	}
	if got, err := evalBranch(&Condition{Field: "{{payload.n}}", Op: "<=", Value: 10}, map[string]any{"n": 10}); err != nil || got != "yes" {
		t.Errorf("<= 10: got %q, err %v; want yes", got, err)
	}
}

func TestBranchValidation(t *testing.T) {
	base := Definition{
		Trigger: Trigger{Type: "manual"},
		Nodes: []Node{
			{ID: "g", Type: "branch", Config: NodeConfig{Condition: &Condition{Field: "{{payload.x}}", Op: ">", Value: 1}}},
			{ID: "a", Type: "log"},
			{ID: "b", Type: "log"},
		},
		Edges: []Edge{
			{From: "trigger", To: "g"},
			{From: "g", To: "a", Label: "yes"},
			{From: "g", To: "b", Label: "no"},
		},
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid branch rejected: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*Definition)
	}{
		{"missing no edge", func(d *Definition) {
			d.Edges = []Edge{{From: "trigger", To: "g"}, {From: "g", To: "a", Label: "yes"}}
		}},
		{"unlabeled fork", func(d *Definition) {
			d.Edges = []Edge{{From: "trigger", To: "g"}, {From: "g", To: "a", Label: "yes"}, {From: "g", To: "b"}}
		}},
		{"two branches", func(d *Definition) {
			d.Nodes = append(d.Nodes, Node{ID: "g2", Type: "branch", Config: NodeConfig{Condition: &Condition{Field: "x", Op: "==", Value: 1}}})
			d.Edges = append(d.Edges, Edge{From: "a", To: "g2", Label: "yes"}, Edge{From: "a", To: "b", Label: "no"})
		}},
		{"non-branch fork", func(d *Definition) {
			d.Nodes = append(d.Nodes, Node{ID: "c", Type: "log"})
			d.Edges = append(d.Edges, Edge{From: "a", To: "b"}, Edge{From: "a", To: "c"})
		}},
		{"missing condition", func(d *Definition) {
			d.Nodes[0].Config.Condition = nil
		}},
		{"bad op", func(d *Definition) {
			d.Nodes[0].Config.Condition.Op = "~="
		}},
		{"cycle", func(d *Definition) {
			d.Edges = []Edge{{From: "trigger", To: "g"}, {From: "g", To: "a", Label: "yes"}, {From: "g", To: "b", Label: "no"}, {From: "b", To: "g"}}
		}},
	}
	for _, tc := range cases {
		// Deep copy: slices share backing arrays across cases otherwise.
		d := base
		d.Nodes = append([]Node(nil), base.Nodes...)
		d.Edges = append([]Edge(nil), base.Edges...)
		tc.mutate(&d)
		if err := d.Validate(); err == nil {
			t.Errorf("%s: invalid branch graph accepted", tc.name)
		}
	}
}

func TestWorkflowEditingReturnsToDraft(t *testing.T) {
	env := setup(t)
	defer env.cleanup()
	ctx := context.Background()

	original := Definition{
		Trigger: Trigger{Type: "manual"},
		Nodes:   []Node{{ID: "a", Type: "log", Config: NodeConfig{Message: "v1"}}},
		Edges:   []Edge{{From: "trigger", To: "a"}},
	}
	wf, err := env.svc.Create(ctx, env.userID, env.wsID, "Editable", original)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := env.svc.Validate(ctx, env.userID, env.wsID, wf.ID); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	// Editing the definition drops the workflow to draft with a new version.
	edited := Definition{
		Trigger: Trigger{Type: "manual"},
		Nodes:   []Node{{ID: "a", Type: "log", Config: NodeConfig{Message: "v2"}}, {ID: "b", Type: "log"}},
		Edges:   []Edge{{From: "trigger", To: "a"}, {From: "a", To: "b"}},
	}
	updated, err := env.svc.UpdateWorkflow(ctx, env.userID, env.wsID, wf.ID, nil, &edited)
	if err != nil {
		t.Fatalf("UpdateWorkflow: %v", err)
	}
	if updated.Status != "draft" {
		t.Errorf("status after edit = %q, want draft", updated.Status)
	}
	if updated.CurrentVersion != 2 {
		t.Errorf("version after edit = %d, want 2", updated.CurrentVersion)
	}

	// Draft workflows refuse to run.
	if _, err := env.svc.StartRun(ctx, env.userID, env.wsID, wf.ID); !errors.Is(err, ErrNotReady) {
		t.Errorf("run after edit err = %v, want ErrNotReady", err)
	}

	// Re-validation restores readiness with the new definition.
	if _, err := env.svc.Validate(ctx, env.userID, env.wsID, wf.ID); err != nil {
		t.Fatalf("re-validate: %v", err)
	}
	run, err := env.svc.StartRun(ctx, env.userID, env.wsID, wf.ID)
	if err != nil {
		t.Fatalf("run after re-validate: %v", err)
	}
	for i := 0; i < 50; i++ {
		ran, err := env.runner.ProcessNext(ctx)
		if err != nil {
			t.Fatalf("ProcessNext: %v", err)
		}
		if !ran {
			break
		}
	}
	_, steps, err := env.svc.GetRun(ctx, env.userID, env.wsID, wf.ID, run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if len(steps) != 2 || steps[0].Output["message"] != "v2" {
		t.Errorf("edited workflow executed wrong definition: %+v", steps)
	}

	// Invalid edits are rejected outright.
	bad := Definition{Trigger: Trigger{Type: "bogus"}}
	if _, err := env.svc.UpdateWorkflow(ctx, env.userID, env.wsID, wf.ID, nil, &bad); err == nil {
		t.Error("invalid edit accepted")
	}
}

func TestRunCancellation(t *testing.T) {
	env := setup(t)
	defer env.cleanup()
	ctx := context.Background()

	wf, err := env.svc.Create(ctx, env.userID, env.wsID, "Cancellable", Definition{
		Trigger: Trigger{Type: "manual"},
		Nodes:   []Node{{ID: "a", Type: "log", Config: NodeConfig{Message: "x"}}},
		Edges:   []Edge{{From: "trigger", To: "a"}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := env.svc.Validate(ctx, env.userID, env.wsID, wf.ID); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	// Enqueue, cancel before the worker drains.
	run, err := env.svc.StartRun(ctx, env.userID, env.wsID, wf.ID)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if err := env.svc.CancelRun(ctx, env.userID, env.wsID, wf.ID, run.ID); err != nil {
		t.Fatalf("CancelRun: %v", err)
	}
	for i := 0; i < 50; i++ {
		ran, err := env.runner.ProcessNext(ctx)
		if err != nil {
			t.Fatalf("ProcessNext: %v", err)
		}
		if !ran {
			break
		}
	}
	runs, _ := env.svc.ListRuns(ctx, env.userID, env.wsID, wf.ID, 50, "")
	if len(runs) != 1 || runs[0].Status != "cancelled" {
		t.Errorf("run after cancel = %+v, want cancelled", runs[0])
	}
	// Cancelling twice is a not-found (nothing left to cancel).
	if err := env.svc.CancelRun(ctx, env.userID, env.wsID, wf.ID, run.ID); !errors.Is(err, ErrRunNotFound) {
		t.Errorf("re-cancel err = %v, want ErrRunNotFound", err)
	}
}
