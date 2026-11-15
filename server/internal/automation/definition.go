// Package automation implements the workflow engine: definition format,
// validation, the execution queue, the worker, and execution history
// (PRD 9). Editing and execution are strictly separated — only validated,
// ready workflows can ever run.
package automation

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// Trigger describes what starts a workflow.
type Trigger struct {
	// Type is "event" (subscribes to a platform event type), "manual",
	// "webhook" (inbound POST /hooks/{token}), or "schedule".
	Type string `json:"type"`
	// EventType is required for event triggers, e.g. "demo.record.created".
	EventType string `json:"eventType,omitempty"`
	// IntervalSec is required for schedule triggers: how often the
	// workflow fires (60s minimum, 7d maximum).
	IntervalSec int `json:"intervalSec,omitempty"`
}

// NodeConfig carries the per-type node configuration.
type NodeConfig struct {
	// HTTP node.
	Method  string            `json:"method,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    map[string]any    `json:"body,omitempty"`
	// Log node.
	Message string `json:"message,omitempty"`
	// Email node.
	To      string `json:"to,omitempty"`
	Subject string `json:"subject,omitempty"`
	Text    string `json:"text,omitempty"`
	// Both.
	Retries   int `json:"retries,omitempty"`
	TimeoutMs int `json:"timeoutMs,omitempty"`
}

// Node is one executable step.
type Node struct {
	ID     string     `json:"id"`
	Type   string     `json:"type"` // "http" | "log"
	Config NodeConfig `json:"config"`
}

// Edge connects two nodes; "trigger" is the reserved source of the first.
type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// Definition is the versioned workflow specification (PRD 9.3).
type Definition struct {
	Trigger   Trigger `json:"trigger"`
	Nodes     []Node  `json:"nodes"`
	Edges     []Edge  `json:"edges"`
	TimeoutMs int     `json:"timeoutMs,omitempty"`
}

const (
	maxNodes        = 32
	defaultNodeSecs = 10
	maxNodeSecs     = 60
	minScheduleSecs = 60
	maxScheduleSecs = 7 * 24 * 60 * 60
)

var (
	nodeTypes   = map[string]bool{"http": true, "log": true, "email": true}
	httpMethods = map[string]bool{"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true}
	// placeholderPattern matches {{payload.key}} tokens in URL/body values.
	placeholderPattern = regexp.MustCompile(`\{\{\s*payload\.([a-zA-Z0-9_]+)\s*\}\}`)
)

// Validate checks the definition and returns a combined error listing every
// problem. A valid definition is a simple chain: exactly one path from the
// reserved "trigger" source through every node exactly once.
func (d *Definition) Validate() error {
	var problems []string

	switch d.Trigger.Type {
	case "event":
		if !strings.Contains(d.Trigger.EventType, ".") || len(d.Trigger.EventType) < 3 {
			problems = append(problems, `event trigger requires eventType (e.g. "demo.record.created")`)
		}
	case "manual":
		if d.Trigger.EventType != "" {
			problems = append(problems, "manual trigger must not declare eventType")
		}
	case "webhook":
		if d.Trigger.EventType != "" || d.Trigger.IntervalSec != 0 {
			problems = append(problems, "webhook trigger takes no additional configuration")
		}
	case "schedule":
		if d.Trigger.IntervalSec < minScheduleSecs || d.Trigger.IntervalSec > maxScheduleSecs {
			problems = append(problems, fmt.Sprintf("schedule trigger requires intervalSec %d-%d (seconds)", minScheduleSecs, maxScheduleSecs))
		}
	default:
		problems = append(problems, `trigger.type must be "event", "manual", "webhook", or "schedule"`)
	}

	if len(d.Nodes) == 0 {
		problems = append(problems, "at least one node is required")
	}
	if len(d.Nodes) > maxNodes {
		problems = append(problems, fmt.Sprintf("at most %d nodes are allowed", maxNodes))
	}

	byID := map[string]Node{}
	for _, n := range d.Nodes {
		if n.ID == "" || n.ID == "trigger" {
			problems = append(problems, "every node needs a non-empty id that is not \"trigger\"")
			continue
		}
		if _, dup := byID[n.ID]; dup {
			problems = append(problems, fmt.Sprintf("duplicate node id %q", n.ID))
			continue
		}
		byID[n.ID] = n
		if !nodeTypes[n.Type] {
			problems = append(problems, fmt.Sprintf("node %q: unknown type %q", n.ID, n.Type))
			continue
		}
		if n.Type == "http" {
			if !httpMethods[n.Config.Method] {
				problems = append(problems, fmt.Sprintf("node %q: method must be GET/POST/PUT/PATCH/DELETE", n.ID))
			}
			u, err := url.Parse(n.Config.URL)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				problems = append(problems, fmt.Sprintf("node %q: url must be an absolute http(s) URL", n.ID))
			}
			if n.Config.Retries < 0 || n.Config.Retries > 5 {
				problems = append(problems, fmt.Sprintf("node %q: retries must be 0-5", n.ID))
			}
			if bad := n.Config.TimeoutMs; bad < 0 || bad > maxNodeSecs*1000 {
				problems = append(problems, fmt.Sprintf("node %q: timeoutMs must be 0-%d", n.ID, maxNodeSecs*1000))
			}
		}
		if n.Type == "email" {
			// Template tokens substitute at runtime; a literal address is
			// validated now, a templated one validates on delivery.
			hasPlaceholder := placeholderPattern.MatchString(n.Config.To)
			if strings.TrimSpace(n.Config.To) == "" || (!strings.Contains(n.Config.To, "@") && !hasPlaceholder) {
				problems = append(problems, fmt.Sprintf("node %q: email requires to (comma-separated addresses)", n.ID))
			}
			if strings.TrimSpace(n.Config.Subject) == "" {
				problems = append(problems, fmt.Sprintf("node %q: email requires subject", n.ID))
			}
			if n.Config.Retries < 0 || n.Config.Retries > 5 {
				problems = append(problems, fmt.Sprintf("node %q: retries must be 0-5", n.ID))
			}
		}
	}

	// Walk the chain: exactly one edge out of each source along the way.
	out := map[string][]Edge{}
	for _, e := range d.Edges {
		if e.From == "" || e.To == "" {
			problems = append(problems, "edges need from and to")
			continue
		}
		if e.From != "trigger" && e.From != e.To {
			if _, ok := byID[e.From]; !ok {
				problems = append(problems, fmt.Sprintf("edge from unknown node %q", e.From))
			}
		}
		if _, ok := byID[e.To]; !ok {
			problems = append(problems, fmt.Sprintf("edge to unknown node %q", e.To))
		}
		out[e.From] = append(out[e.From], e)
	}

	// Chain walk from the trigger.
	visited := map[string]bool{}
	cur := "trigger"
	for {
		edges := out[cur]
		if len(edges) == 0 {
			break
		}
		if len(edges) > 1 {
			problems = append(problems, fmt.Sprintf("node %q has multiple outgoing edges; the alpha executes linear chains only", cur))
			break
		}
		next := edges[0].To
		if visited[next] {
			problems = append(problems, "cycle detected in workflow graph")
			break
		}
		visited[next] = true
		cur = next
	}
	for _, n := range d.Nodes {
		if !visited[n.ID] {
			problems = append(problems, fmt.Sprintf("node %q is not reachable from the trigger", n.ID))
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("workflow definition invalid: %s", strings.Join(problems, "; "))
	}
	return nil
}

// Order returns the nodes in execution order along the validated chain.
// Must only be called on a validated definition.
func (d *Definition) Order() []Node {
	out := map[string]Edge{}
	for _, e := range d.Edges {
		out[e.From] = e
	}
	byID := map[string]Node{}
	for _, n := range d.Nodes {
		byID[n.ID] = n
	}
	var order []Node
	cur := out["trigger"].To
	for cur != "" {
		order = append(order, byID[cur])
		if e, ok := out[cur]; ok {
			cur = e.To
		} else {
			cur = ""
		}
	}
	return order
}

// nodeTimeout returns the configured timeout in milliseconds, or the default.
func (c NodeConfig) nodeTimeout() int {
	if c.TimeoutMs > 0 {
		return c.TimeoutMs
	}
	return defaultNodeSecs * 1000
}

var errNotFound = errors.New("workflow not found")
