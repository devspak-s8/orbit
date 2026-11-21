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
	// Connector preset: when set, the node is a preset (Slack, Discord,
	// Telegram, generic webhook) and method/url/headers/body are derived
	// from the catalog at run time.
	Connector string            `json:"connector,omitempty"`
	Params    map[string]string `json:"params,omitempty"`
	// Log node.
	Message string `json:"message,omitempty"`
	// Email node.
	To      string `json:"to,omitempty"`
	Subject string `json:"subject,omitempty"`
	Text    string `json:"text,omitempty"`
	// Branch node: condition on the trigger payload.
	Condition *Condition `json:"condition,omitempty"`
	// Both.
	Retries   int `json:"retries,omitempty"`
	TimeoutMs int `json:"timeoutMs,omitempty"`
}

// Condition is a branch predicate over the run's trigger payload.
type Condition struct {
	// Field is a template: "{{payload.score}}".
	Field string `json:"field"`
	// Op is one of >, >=, <, <=, ==, !=.
	Op string `json:"op"`
	// Value is the right-hand side (number or string).
	Value any `json:"value"`
}

// Node is one executable step.
type Node struct {
	ID     string     `json:"id"`
	Type   string     `json:"type"` // "http" | "log"
	Config NodeConfig `json:"config"`
}

// Edge connects two nodes; "trigger" is the reserved source of the first.
// Branch nodes carry exactly two labeled edges: "yes" and "no".
type Edge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Label string `json:"label,omitempty"` // "yes" | "no" on branch edges
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
	nodeTypes   = map[string]bool{"http": true, "log": true, "email": true, "branch": true}
	httpMethods = map[string]bool{"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true}
	// placeholderPattern matches {{payload.key}} tokens in URL/body values.
	placeholderPattern = regexp.MustCompile(`\{\{\s*payload\.([a-zA-Z0-9_]+)\s*\}\}`)
	condOps            = map[string]bool{">": true, ">=": true, "<": true, "<=": true, "==": true, "!=": true}
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
			if n.Config.Connector != "" {
				// Preset node: validated against the catalog; plain
				// method/url must not be mixed in.
				if n.Config.Method != "" || n.Config.URL != "" {
					problems = append(problems, fmt.Sprintf("node %q: connector preset and method/url are mutually exclusive", n.ID))
				}
				problems = append(problems, validateConnectorNode(n.ID, n.Config.Connector, n.Config.Params)...)
			} else {
				if !httpMethods[n.Config.Method] {
					problems = append(problems, fmt.Sprintf("node %q: method must be GET/POST/PUT/PATCH/DELETE", n.ID))
				}
				u, err := url.Parse(n.Config.URL)
				if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
					problems = append(problems, fmt.Sprintf("node %q: url must be an absolute http(s) URL", n.ID))
				}
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
		if n.Type == "branch" {
			c := n.Config.Condition
			if c == nil {
				problems = append(problems, fmt.Sprintf("node %q: branch requires condition", n.ID))
			} else {
				if strings.TrimSpace(c.Field) == "" {
					problems = append(problems, fmt.Sprintf("node %q: condition requires field", n.ID))
				}
				if !condOps[c.Op] {
					problems = append(problems, fmt.Sprintf("node %q: condition op must be one of > >= < <= == !=", n.ID))
				}
				if c.Value == nil {
					problems = append(problems, fmt.Sprintf("node %q: condition requires value", n.ID))
				}
			}
		}
	}

	// Graph shape: non-branch nodes have at most one outgoing edge; a
	// branch node has exactly two, labeled "yes" and "no".
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

	branchCount := 0
	for _, n := range d.Nodes {
		if n.Type == "branch" {
			branchCount++
			yes, no := false, false
			for _, e := range out[n.ID] {
				switch e.Label {
				case "yes":
					yes = true
				case "no":
					no = true
				}
			}
			if len(out[n.ID]) != 2 || !yes || !no {
				problems = append(problems, fmt.Sprintf("branch node %q needs exactly two outgoing edges, labeled yes and no", n.ID))
			}
		} else if len(out[n.ID]) > 1 {
			problems = append(problems, fmt.Sprintf("node %q has multiple outgoing edges; only branch nodes fork", n.ID))
		}
	}
	if branchCount > 1 {
		problems = append(problems, "at most one branch node is allowed per workflow")
	}

	// Reachability and acyclicity: DFS from the trigger over all edges
	// (gray = on current path, black = fully explored).
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := map[string]int{"trigger": gray}
	var dfs func(id string)
	dfs = func(id string) {
		for _, e := range out[id] {
			switch color[e.To] {
			case gray:
				problems = append(problems, "cycle detected in workflow graph")
			case white:
				color[e.To] = gray
				dfs(e.To)
			}
		}
		color[id] = black
	}
	dfs("trigger")
	for _, n := range d.Nodes {
		if color[n.ID] == white {
			problems = append(problems, fmt.Sprintf("node %q is not reachable from the trigger", n.ID))
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("workflow definition invalid: %s", strings.Join(problems, "; "))
	}
	return nil
}

// Order returns the nodes for step-creation purposes. With branch support
// the runtime walks the graph per run; every node gets a step row, and
// untaken branch paths are marked skipped during execution.
func (d *Definition) Order() []Node {
	return d.Nodes
}

// nodeTimeout returns the configured timeout in milliseconds, or the default.
func (c NodeConfig) nodeTimeout() int {
	if c.TimeoutMs > 0 {
		return c.TimeoutMs
	}
	return defaultNodeSecs * 1000
}

var errNotFound = errors.New("workflow not found")
