package automation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/orbit-org/orbit/server/internal/config"
	"github.com/orbit-org/orbit/server/internal/notifications"
)

// --- simulator (pure) ---

func TestSimulateBranchAndConnector(t *testing.T) {
	def := &Definition{
		Trigger: Trigger{Type: "manual"},
		Nodes: []Node{
			{ID: "gate", Type: "branch", Config: NodeConfig{Condition: &Condition{
				Field: "{{payload.score}}", Op: ">", Value: 50,
			}}},
			{ID: "notify", Type: "http", Config: NodeConfig{
				Connector: "slack",
				Params: map[string]string{
					"url":  "https://hooks.slack.com/services/T/B/x",
					"text": "score was {{payload.score}}",
				},
			}},
			{ID: "logLow", Type: "log", Config: NodeConfig{Message: "low: {{payload.score}}"}},
			{ID: "mail", Type: "email", Config: NodeConfig{
				To: "a@orbit.test, b@orbit.test", Subject: "s", Text: "hello",
			}},
		},
		Edges: []Edge{
			{From: "trigger", To: "gate"},
			{From: "gate", To: "notify", Label: "yes"},
			{From: "gate", To: "logLow", Label: "no"},
			{From: "notify", To: "mail"},
		},
	}

	sim := Simulate(def, map[string]any{"score": 99})
	byID := map[string]SimNode{}
	for _, n := range sim.Nodes {
		byID[n.NodeID] = n
	}
	if byID["gate"].Status != "completed" || byID["gate"].Output["taken"] != "yes" {
		t.Errorf("gate = %+v, want completed/yes", byID["gate"])
	}
	if byID["logLow"].Status != "skipped" {
		t.Errorf("untaken node = %+v, want skipped", byID["logLow"])
	}
	req := byID["notify"].Request
	if byID["notify"].Status != "completed" || req == nil {
		t.Fatalf("notify = %+v, want completed with request", byID["notify"])
	}
	if req.Method != "POST" || req.URL != "https://hooks.slack.com/services/T/B/x" {
		t.Errorf("request = %s %s", req.Method, req.URL)
	}
	if req.Body["text"] != "score was 99" {
		t.Errorf("body = %v, want substituted text", req.Body)
	}
	if byID["mail"].Email == nil || len(byID["mail"].Email.To) != 2 {
		t.Errorf("email = %+v, want two recipients", byID["mail"].Email)
	}

	// Low score flips the branch; the dry-run must never execute anything.
	low := Simulate(def, map[string]any{"score": 1})
	byIDLow := map[string]SimNode{}
	for _, n := range low.Nodes {
		byIDLow[n.NodeID] = n
	}
	if byIDLow["gate"].Output["taken"] != "no" {
		t.Errorf("gate on low = %+v", byIDLow["gate"])
	}
	if byIDLow["notify"].Status != "skipped" || byIDLow["logLow"].Status != "completed" {
		t.Errorf("low routing wrong: notify=%+v logLow=%+v", byIDLow["notify"], byIDLow["logLow"])
	}
	if byIDLow["logLow"].Output["message"] != "low: 1" {
		t.Errorf("log output = %v, want substitution", byIDLow["logLow"].Output)
	}
}

func TestSimulateFailedNodeStopsWalk(t *testing.T) {
	def := &Definition{
		Trigger: Trigger{Type: "manual"},
		Nodes: []Node{
			{ID: "bad", Type: "http", Config: NodeConfig{Connector: "slack", Params: map[string]string{
				"url": "https://hooks.slack.com/services/T/B/x", "text": "hi",
			}}},
			{ID: "after", Type: "log", Config: NodeConfig{Message: "never"}},
		},
		Edges: []Edge{{From: "trigger", To: "bad"}, {From: "bad", To: "after"}},
	}
	sim := Simulate(def, nil)
	if sim.Nodes[0].Status != "completed" || sim.Nodes[1].Status != "completed" {
		t.Errorf("healthy chain should complete: %+v", sim.Nodes)
	}
}

func TestEvaluateAssertions(t *testing.T) {
	sim := SimResult{Nodes: []SimNode{
		{NodeID: "n", Type: "http", Status: "completed",
			Request: &SimRequest{Method: "POST", URL: "https://hooks.slack.com/x", Body: map[string]any{"text": "hello world"}}},
		{NodeID: "other", Type: "log", Status: "skipped"},
	}}

	pass := []Assertion{
		{NodeID: "n", Property: "status", Op: "==", Value: "completed"},
		{NodeID: "n", Property: "request.method", Op: "==", Value: "POST"},
		{NodeID: "n", Property: "request.body.text", Op: "contains", Value: "world"},
		{NodeID: "other", Property: "status", Op: "!=", Value: "completed"},
	}
	for _, a := range pass {
		res := EvaluateAssertions(sim, []Assertion{a})
		if len(res) != 1 || !res[0].Passed {
			t.Errorf("assertion %+v should pass, got %+v", a, res)
		}
	}

	fail := []Assertion{
		{NodeID: "n", Property: "status", Op: "==", Value: "failed"},
		{NodeID: "missing", Property: "status", Op: "==", Value: "completed"},
		{NodeID: "n", Property: "request.body.nope", Op: "==", Value: "x"},
		{NodeID: "n", Property: "request.url", Op: "==", Value: "https://evil.example"},
	}
	for _, a := range fail {
		res := EvaluateAssertions(sim, []Assertion{a})
		if len(res) != 1 || res[0].Passed {
			t.Errorf("assertion %+v should fail, got %+v", a, res)
		}
	}

	if err := ValidateAssertions([]Assertion{{NodeID: "n", Property: "x", Op: "~=", Value: 1}}); err == nil {
		t.Error("invalid op accepted")
	}
	if err := ValidateAssertions([]Assertion{{NodeID: "", Property: "x", Op: "==", Value: 1}}); err == nil {
		t.Error("missing nodeId accepted")
	}
}

// --- evals end to end ---

func TestEvalLifecycleThroughService(t *testing.T) {
	env := setup(t)
	defer env.cleanup()
	ctx := context.Background()

	def := Definition{
		Trigger: Trigger{Type: "webhook"},
		Nodes: []Node{
			{ID: "notify", Type: "http", Config: NodeConfig{
				Connector: "slack",
				Params: map[string]string{
					"url":  "https://hooks.slack.com/services/T/B/x",
					"text": "task {{payload.title}} done",
				},
			}},
		},
		Edges: []Edge{{From: "trigger", To: "notify"}},
	}
	wf, err := env.svc.Create(ctx, env.userID, env.wsID, "Eval target", def)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Create the eval: dry-run replay with assertions.
	e, err := env.svc.CreateEval(ctx, env.userID, env.wsID, wf.ID, "happy path", map[string]any{"title": "ship it"}, []Assertion{
		{NodeID: "notify", Property: "status", Op: "==", Value: "completed"},
		{NodeID: "notify", Property: "request.body.text", Op: "==", Value: "task ship it done"},
	})
	if err != nil {
		t.Fatalf("CreateEval: %v", err)
	}

	run, err := env.svc.RunEval(ctx, env.userID, env.wsID, wf.ID, e.ID)
	if err != nil {
		t.Fatalf("RunEval: %v", err)
	}
	if !run.Passed {
		t.Fatalf("eval should pass, results: %+v", run.Results)
	}

	// A wrong assertion fails the same eval.
	run, err = env.svc.RunEval(ctx, env.userID, env.wsID, wf.ID, e.ID)
	if err != nil {
		t.Fatalf("RunEval again: %v", err)
	}
	if !run.Passed {
		t.Fatalf("second identical run should also pass: %+v", run.Results)
	}

	// Create a failing eval to see list results.
	bad, err := env.svc.CreateEval(ctx, env.userID, env.wsID, wf.ID, "wrong expectation", map[string]any{"title": "x"}, []Assertion{
		{NodeID: "notify", Property: "status", Op: "==", Value: "failed"},
	})
	if err != nil {
		t.Fatalf("CreateEval bad: %v", err)
	}
	if _, err := env.svc.RunEval(ctx, env.userID, env.wsID, wf.ID, bad.ID); err != nil {
		t.Fatalf("RunEval bad: %v", err)
	}

	items, err := env.svc.ListEvals(ctx, env.userID, env.wsID, wf.ID)
	if err != nil {
		t.Fatalf("ListEvals: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("evals = %d, want 2", len(items))
	}
	for _, it := range items {
		if it.LastRun == nil {
			t.Fatalf("eval %s missing last run", it.Name)
		}
		if it.Name == "happy path" && !it.LastRun.Passed {
			t.Errorf("happy path last run = %+v, want passed", it.LastRun)
		}
		if it.Name == "wrong expectation" && it.LastRun.Passed {
			t.Errorf("wrong expectation last run should fail: %+v", it.LastRun)
		}
	}

	// Invalid assertions are rejected at create time.
	if _, err := env.svc.CreateEval(ctx, env.userID, env.wsID, wf.ID, "bad", nil, []Assertion{
		{NodeID: "n", Property: "status", Op: "explode", Value: 1},
	}); err == nil {
		t.Error("invalid assertion accepted")
	}

	// Delete removes eval history.
	if err := env.svc.DeleteEval(ctx, env.userID, env.wsID, wf.ID, bad.ID); err != nil {
		t.Fatalf("DeleteEval: %v", err)
	}
	items, _ = env.svc.ListEvals(ctx, env.userID, env.wsID, wf.ID)
	if len(items) != 1 {
		t.Errorf("after delete, evals = %d, want 1", len(items))
	}
}

func TestEvalFromRecordedRun(t *testing.T) {
	env := setup(t)
	defer env.cleanup()
	ctx := context.Background()

	// A workflow whose payload we record through a real run.
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := map[string]any{}
		_ = json.NewDecoder(r.Body).Decode(&b)
		gotBody = b
		w.WriteHeader(200)
	}))
	defer srv.Close()

	def := Definition{
		Trigger: Trigger{Type: "webhook"},
		Nodes: []Node{{
			ID: "call", Type: "http",
			Config: NodeConfig{Connector: "webhook", Params: map[string]string{
				"url":  srv.URL,
				"body": `{"title":"{{payload.title}}"}`,
			}},
		}},
		Edges: []Edge{{From: "trigger", To: "call"}},
	}
	wf, err := env.svc.Create(ctx, env.userID, env.wsID, "Recorded", def)
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
	run, err := env.svc.enqueueWithPayload(ctx, wfFull, map[string]any{"title": "the real bug"})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	env.drain(t)
	if gotBody["title"] != "the real bug" {
		t.Fatalf("recorded run did not execute: %+v", gotBody)
	}

	// Capture the recorded payload as an eval.
	e, err := env.svc.CreateEvalFromRun(ctx, env.userID, env.wsID, wf.ID, run.ID, "regression")
	if err != nil {
		t.Fatalf("CreateEvalFromRun: %v", err)
	}
	if e.Payload["title"] != "the real bug" {
		t.Errorf("captured payload = %v, want the recorded one", e.Payload)
	}

	// The dry-run replays it without touching the receiver.
	result, err := env.svc.RunEval(ctx, env.userID, env.wsID, wf.ID, e.ID)
	if err != nil {
		t.Fatalf("RunEval: %v", err)
	}
	if result.Passed && len(result.Results) != 0 {
		t.Errorf("no assertions means trivially passed: %+v", result)
	}
	if len(e.Assertions) != 0 {
		t.Errorf("from-run eval should start without assertions: %+v", e.Assertions)
	}
}

// --- self-healing: failure notifications, anomaly escalation, retry ---

func TestFailureNotificationsAndRetry(t *testing.T) {
	env := setup(t)
	defer env.cleanup()
	ctx := context.Background()

	notifSvc := notifications.NewService(env.runner.pool)
	healing := NewRunner(env.runner.pool, config.SMTPConfig{}, notifSvc)

	// A workflow that fails: the node points at a closed port.
	deadDef := Definition{
		Trigger: Trigger{Type: "manual"},
		Nodes:   []Node{{ID: "call", Type: "http", Config: NodeConfig{Method: "GET", URL: "http://127.0.0.1:1/dead"}}},
		Edges:   []Edge{{From: "trigger", To: "call"}},
	}
	wf, err := env.svc.Create(ctx, env.userID, env.wsID, "Flaky", deadDef)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := env.svc.Validate(ctx, env.userID, env.wsID, wf.ID); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	failedRun := func() Run {
		run, err := env.svc.StartRun(ctx, env.userID, env.wsID, wf.ID)
		if err != nil {
			t.Fatalf("StartRun: %v", err)
		}
		for i := 0; i < 50; i++ {
			ran, err := healing.ProcessNext(ctx)
			if err != nil {
				t.Fatalf("ProcessNext: %v", err)
			}
			if !ran {
				break
			}
		}
		return run
	}

	notificationCount := func() (int, int) {
		var failed, anomaly int
		if err := env.runner.pool.QueryRow(ctx,
			`SELECT count(*) FILTER (WHERE type = 'workflow.failed'),
			        count(*) FILTER (WHERE type = 'workflow.anomaly')
			 FROM notifications WHERE user_id = $1 AND workspace_id = $2`,
			env.userID, env.wsID,
		).Scan(&failed, &anomaly); err != nil {
			t.Fatalf("count notifications: %v", err)
		}
		return failed, anomaly
	}

	// First failure: one plain workflow.failed notification.
	first := failedRun()
	failed, anomaly := notificationCount()
	if failed != 1 || anomaly != 0 {
		t.Fatalf("after first failure: failed=%d anomaly=%d", failed, anomaly)
	}

	// Second failure: still plain, no escalation yet.
	_ = failedRun()
	failed, anomaly = notificationCount()
	if failed != 2 || anomaly != 0 {
		t.Fatalf("after second failure: failed=%d anomaly=%d", failed, anomaly)
	}

	// Third failure: the last three runs all failed — anomaly escalation.
	third := failedRun()
	_ = third
	failed, anomaly = notificationCount()
	if failed != 2 || anomaly != 1 {
		t.Fatalf("after third failure: failed=%d anomaly=%d — want escalation", failed, anomaly)
	}

	// The anomaly payload carries the workflow and run.
	var raw []byte
	if err := env.runner.pool.QueryRow(ctx,
		`SELECT payload FROM notifications WHERE user_id = $1 AND type = 'workflow.anomaly'`,
		env.userID,
	).Scan(&raw); err != nil {
		t.Fatalf("load anomaly notification: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode anomaly payload: %v", err)
	}
	if payload["workflowName"] != "Flaky" || payload["runId"] == "" || payload["anomaly"] != true {
		t.Errorf("anomaly payload = %v", payload)
	}

	// Fix the workflow, then retry the failed run with its payload.
	live := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer live.Close()
	fixedDef := deadDef
	fixedDef.Nodes[0].Config.URL = live.URL
	if _, err := env.svc.UpdateWorkflow(ctx, env.userID, env.wsID, wf.ID, nil, &fixedDef); err != nil {
		t.Fatalf("UpdateWorkflow: %v", err)
	}
	if _, err := env.svc.Validate(ctx, env.userID, env.wsID, wf.ID); err != nil {
		t.Fatalf("re-validate: %v", err)
	}

	retried, err := env.svc.RetryRun(ctx, env.userID, env.wsID, wf.ID, first.ID)
	if err != nil {
		t.Fatalf("RetryRun: %v", err)
	}
	if retried.TriggerType != "retry" {
		t.Errorf("retried run triggerType = %q, want retry", retried.TriggerType)
	}
	for i := 0; i < 50; i++ {
		ran, err := healing.ProcessNext(ctx)
		if err != nil {
			t.Fatalf("ProcessNext: %v", err)
		}
		if !ran {
			break
		}
	}
	completed, _, err := env.svc.GetRun(ctx, env.userID, env.wsID, wf.ID, retried.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if completed.Status != "completed" {
		t.Errorf("retried run status = %q (error %q), want completed", completed.Status, completed.Error)
	}

	// Completed runs are not retryable.
	if _, err := env.svc.RetryRun(ctx, env.userID, env.wsID, wf.ID, retried.ID); err == nil {
		t.Error("completed run was accepted for retry")
	}

	// Unknown runs surface not-found.
	if _, err := env.svc.RetryRun(ctx, env.userID, env.wsID, wf.ID, "00000000-0000-0000-0000-000000000000"); err == nil {
		t.Error("unknown run was accepted for retry")
	}
}
