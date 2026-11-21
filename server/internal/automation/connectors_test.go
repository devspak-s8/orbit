package automation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// --- catalog expansion ---

func TestConnectorCatalogExpansion(t *testing.T) {
	cases := []struct {
		name      string
		connector string
		params    map[string]string
		wantURL   string
		wantBody  map[string]any
	}{
		{
			"slack", "slack",
			map[string]string{"url": "https://hooks.slack.com/services/T/B/x", "text": "hi {{payload.title}}"},
			"https://hooks.slack.com/services/T/B/x",
			map[string]any{"text": "hi {{payload.title}}"},
		},
		{
			"discord", "discord",
			map[string]string{"url": "https://discord.com/api/webhooks/123/abc", "content": "hi"},
			"https://discord.com/api/webhooks/123/abc",
			map[string]any{"content": "hi"},
		},
		{
			"telegram", "telegram",
			map[string]string{"token": "111:AAA", "chatId": "@chan", "text": "hi"},
			"https://api.telegram.org/bot111:AAA/sendMessage",
			map[string]any{"chat_id": "@chan", "text": "hi"},
		},
		{
			"webhook with body", "webhook",
			map[string]string{"url": "http://example.com/h", "body": `{"a":"{{payload.x}}"}`},
			"http://example.com/h",
			map[string]any{"a": "{{payload.x}}"},
		},
		{
			"webhook without body", "webhook",
			map[string]string{"url": "http://example.com/h"},
			"http://example.com/h",
			map[string]any{},
		},
	}
	for _, tc := range cases {
		c, ok := connectorByID(tc.connector)
		if !ok {
			t.Fatalf("connector %q not in catalog", tc.connector)
		}
		method, url, headers, body := c.Build(tc.params)
		if method != "POST" {
			t.Errorf("%s: method = %q, want POST", tc.name, method)
		}
		if url != tc.wantURL {
			t.Errorf("%s: url = %q, want %q", tc.name, url, tc.wantURL)
		}
		if len(headers) != 0 {
			t.Errorf("%s: unexpected headers %v", tc.name, headers)
		}
		want, _ := json.Marshal(tc.wantBody)
		got, _ := json.Marshal(body)
		if string(got) != string(want) {
			t.Errorf("%s: body = %s, want %s", tc.name, got, want)
		}
	}
}

// --- definition validation ---

func TestConnectorValidation(t *testing.T) {
	node := func(connector string, params map[string]string, extra NodeConfig) Node {
		return Node{ID: "n", Type: "http", Config: NodeConfig{Connector: connector, Params: params, Retries: extra.Retries, Method: extra.Method, URL: extra.URL}}
	}
	base := func(n Node) Definition {
		return Definition{Trigger: Trigger{Type: "manual"}, Nodes: []Node{n}, Edges: []Edge{{From: "trigger", To: "n"}}}
	}

	valid := []Node{
		node("slack", map[string]string{"url": "https://hooks.slack.com/services/T/B/x", "text": "hi"}, NodeConfig{}),
		node("discord", map[string]string{"url": "https://discord.com/api/webhooks/1/a", "content": "hi"}, NodeConfig{}),
		node("telegram", map[string]string{"token": "1:A", "chatId": "@c", "text": "hi"}, NodeConfig{}),
		node("webhook", map[string]string{"url": "http://127.0.0.1:9/h", "body": `{"a":1}`}, NodeConfig{}),
		node("webhook", map[string]string{"url": "https://any.example/x"}, NodeConfig{}), // no prefix requirement
	}
	for _, n := range valid {
		d := base(n)
		if err := d.Validate(); err != nil {
			t.Errorf("valid connector node rejected: %v", err)
		}
	}

	invalid := []struct {
		name string
		node Node
		want string
	}{
		{"unknown connector", node("teams", nil, NodeConfig{}), "unknown connector"},
		{"missing required", node("slack", map[string]string{"url": "https://hooks.slack.com/x"}, NodeConfig{}), `requires "text"`},
		{"wrong prefix", node("slack", map[string]string{"url": "https://evil.example/x", "text": "hi"}, NodeConfig{}), "must start with"},
		{"discord wrong prefix", node("discord", map[string]string{"url": "https://hooks.slack.com/x", "content": "hi"}, NodeConfig{}), "must start with"},
		{"not a url", node("webhook", map[string]string{"url": "not-a-url"}, NodeConfig{}), "absolute http(s)"},
		{"unknown param", node("slack", map[string]string{"url": "https://hooks.slack.com/x", "text": "hi", "extra": "x"}, NodeConfig{}), "no field"},
		{"bad json body", node("webhook", map[string]string{"url": "http://x.example/h", "body": `{no`}, NodeConfig{}), "JSON object"},
		{"mixed with url", node("slack", map[string]string{"url": "https://hooks.slack.com/x", "text": "hi"}, NodeConfig{Method: "POST", URL: "http://x.example"}), "mutually exclusive"},
	}
	for _, tc := range invalid {
		d := base(tc.node)
		err := d.Validate()
		if err == nil {
			t.Errorf("%s: invalid connector node accepted", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error %q does not contain %q", tc.name, err, tc.want)
		}
	}

	// Plain HTTP nodes are untouched.
	plain := Definition{Trigger: Trigger{Type: "manual"}, Nodes: []Node{
		{ID: "n", Type: "http", Config: NodeConfig{Method: "GET", URL: "http://example.com"}},
	}, Edges: []Edge{{From: "trigger", To: "n"}}}
	if err := plain.Validate(); err != nil {
		t.Errorf("plain http node rejected: %v", err)
	}
}

// --- end to end through the real runner ---

func TestConnectorPresetRunsThroughEngine(t *testing.T) {
	env := setup(t)
	defer env.cleanup()
	ctx := context.Background()

	// A local receiver stands in for the external service.
	var mu sync.Mutex
	var gotReq struct {
		method string
		path   string
		body   map[string]any
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := map[string]any{}
		_ = json.NewDecoder(r.Body).Decode(&b)
		mu.Lock()
		gotReq.method = r.Method
		gotReq.path = r.URL.Path
		gotReq.body = b
		mu.Unlock()
		w.WriteHeader(200)
	}))
	defer srv.Close()

	def := Definition{
		Trigger: Trigger{Type: "webhook"},
		Nodes: []Node{
			{ID: "notify", Type: "http", Config: NodeConfig{
				Connector: "webhook",
				Params: map[string]string{
					"url":  srv.URL + "/hook",
					"body": `{"title": "{{payload.title}}", "status": "{{payload.status}}", "n": 42}`,
				},
			}},
		},
		Edges: []Edge{{From: "trigger", To: "notify"}},
	}
	if err := def.Validate(); err != nil {
		t.Fatalf("definition invalid: %v", err)
	}
	wf, err := env.svc.Create(ctx, env.userID, env.wsID, "Preset E2E", def)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := env.svc.Validate(ctx, env.userID, env.wsID, wf.ID); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	// Run through the engine (manual run path; trigger type stays webhook).
	run, err := env.svc.StartRun(ctx, env.userID, env.wsID, wf.ID)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	env.drain(t)

	_, steps, err := env.svc.GetRun(ctx, env.userID, env.wsID, wf.ID, run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if len(steps) != 1 || steps[0].Status != "completed" {
		t.Fatalf("preset step = %+v, want completed", steps)
	}

	mu.Lock()
	defer mu.Unlock()
	if gotReq.method != "POST" {
		t.Errorf("receiver got %q, want POST", gotReq.method)
	}
	if gotReq.path != "/hook" {
		t.Errorf("receiver path = %q, want /hook", gotReq.path)
	}
	// Manual trigger runs carry an empty payload, so tokens substitute to
	// "" — the request must still carry the literal structure and n.
	if gotReq.body["n"] != float64(42) {
		t.Errorf("body.n = %v, want 42 (full body: %v)", gotReq.body["n"], gotReq.body)
	}
	if _, ok := gotReq.body["title"]; !ok {
		t.Errorf("body missing title key: %v", gotReq.body)
	}
}

func TestConnectorPresetSubstitutesPayload(t *testing.T) {
	env := setup(t)
	defer env.cleanup()
	ctx := context.Background()

	var mu sync.Mutex
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := map[string]any{}
		_ = json.NewDecoder(r.Body).Decode(&b)
		mu.Lock()
		gotBody = b
		mu.Unlock()
		w.WriteHeader(200)
	}))
	defer srv.Close()

	def := Definition{
		Trigger: Trigger{Type: "webhook"},
		Nodes: []Node{
			{ID: "notify", Type: "http", Config: NodeConfig{
				Connector: "webhook",
				Params: map[string]string{
					"url":  srv.URL,
					"body": `{"title": "{{payload.title}}"}`,
				},
			}},
		},
		Edges: []Edge{{From: "trigger", To: "notify"}},
	}
	wf, err := env.svc.Create(ctx, env.userID, env.wsID, "Preset Sub", def)
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
	if _, err := env.svc.enqueueWithPayload(ctx, wfFull, map[string]any{"title": "Fix the login bug"}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	env.drain(t)

	mu.Lock()
	defer mu.Unlock()
	if gotBody["title"] != "Fix the login bug" {
		t.Errorf("title = %v, want substituted payload value", gotBody["title"])
	}
}
