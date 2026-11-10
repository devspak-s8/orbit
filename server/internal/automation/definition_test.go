package automation

import (
	"encoding/json"
	"testing"
)

func validDefinition() Definition {
	return Definition{
		Trigger: Trigger{Type: "event", EventType: "demo.record.created"},
		Nodes: []Node{
			{ID: "call", Type: "http", Config: NodeConfig{Method: "POST", URL: "http://example.com/hook"}},
			{ID: "done", Type: "log", Config: NodeConfig{Message: "finished"}},
		},
		Edges: []Edge{
			{From: "trigger", To: "call"},
			{From: "call", To: "done"},
		},
	}
}

func TestValidateAcceptsValid(t *testing.T) {
	d := validDefinition()
	if err := d.Validate(); err != nil {
		t.Fatalf("valid definition rejected: %v", err)
	}
	order := d.Order()
	if len(order) != 2 || order[0].ID != "call" || order[1].ID != "done" {
		t.Errorf("Order() = %+v, want [call done]", order)
	}
}

func TestValidateRejectsBroken(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Definition)
	}{
		{"bad trigger type", func(d *Definition) { d.Trigger.Type = "cron" }},
		{"event trigger without type", func(d *Definition) { d.Trigger.EventType = "" }},
		{"manual with event type", func(d *Definition) { d.Trigger.Type = "manual"; d.Trigger.EventType = "demo.record.created" }},
		{"no nodes", func(d *Definition) { d.Nodes = nil; d.Edges = nil }},
		{"unknown node type", func(d *Definition) { d.Nodes[0].Type = "grpc" }},
		{"bad http url", func(d *Definition) { d.Nodes[0].Config.URL = "not a url" }},
		{"bad http method", func(d *Definition) { d.Nodes[0].Config.Method = "BREW" }},
		{"negative retries", func(d *Definition) { d.Nodes[0].Config.Retries = -1 }},
		{"too many retries", func(d *Definition) { d.Nodes[0].Config.Retries = 9 }},
		{"node id trigger", func(d *Definition) { d.Nodes[0].ID = "trigger" }},
		{"duplicate node ids", func(d *Definition) { d.Nodes[1].ID = d.Nodes[0].ID }},
		{"unreachable node", func(d *Definition) { d.Nodes = append(d.Nodes, Node{ID: "orphan", Type: "log"}) }},
		{"cycle", func(d *Definition) {
			d.Edges = []Edge{{From: "trigger", To: "call"}, {From: "call", To: "call"}}
		}},
		{"fork", func(d *Definition) {
			d.Edges = append(d.Edges, Edge{From: "call", To: "done"})
		}},
		{"edge to unknown node", func(d *Definition) {
			d.Edges = []Edge{{From: "trigger", To: "ghost"}}
		}},
	}
	for _, tc := range cases {
		d := validDefinition()
		tc.mutate(&d)
		if err := d.Validate(); err == nil {
			t.Errorf("%s: invalid definition accepted", tc.name)
		}
	}
}

func TestDefinitionJSONRoundTrip(t *testing.T) {
	raw := `{
	  "trigger": {"type": "manual"},
	  "nodes": [{"id": "a", "type": "log", "config": {"message": "hi"}}],
	  "edges": [{"from": "trigger", "to": "a"}]
	}`
	var d Definition
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := d.Validate(); err != nil {
		t.Fatalf("round-tripped definition invalid: %v", err)
	}
}
