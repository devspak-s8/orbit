// Dry-run simulation for workflow evals: walks a definition against a
// payload exactly like the runner, but intercepts every side effect. HTTP
// requests are recorded (post-expansion, post-substitution) and never sent;
// emails are recorded and never delivered. Branches evaluate for real —
// they are pure. Untaken paths are marked skipped, like the runner does.
package automation

import (
	"encoding/json"
	"fmt"
	"strings"
)

// SimRequest is what an http node would have sent.
type SimRequest struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    map[string]any    `json:"body,omitempty"`
}

// SimEmail is what an email node would have delivered.
type SimEmail struct {
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Text    string   `json:"text"`
}

// SimNode is one node's simulated outcome, in walk order.
type SimNode struct {
	NodeID  string         `json:"nodeId"`
	Type    string         `json:"type"`
	Status  string         `json:"status"` // completed | skipped | failed
	Output  map[string]any `json:"output,omitempty"`
	Request *SimRequest    `json:"request,omitempty"`
	Email   *SimEmail      `json:"email,omitempty"`
	Error   string         `json:"error,omitempty"`
}

// SimResult is a whole dry-run.
type SimResult struct {
	Nodes []SimNode `json:"nodes"`
}

// Assertion checks one property of one node's simulated outcome.
type Assertion struct {
	NodeID   string `json:"nodeId"`
	Property string `json:"property"`
	Op       string `json:"op"` // == | != | contains
	Value    any    `json:"value"`
}

// AssertionResult is the evaluated outcome of one assertion.
type AssertionResult struct {
	Assertion Assertion `json:"assertion"`
	Passed    bool      `json:"passed"`
	Actual    any       `json:"actual,omitempty"`
	Reason    string    `json:"reason,omitempty"`
}

// Simulate walks the definition against the payload, intercepting side
// effects. It never touches the network or the database.
func Simulate(def *Definition, payload map[string]any) SimResult {
	byID := map[string]Node{}
	for _, n := range def.Nodes {
		byID[n.ID] = n
	}
	outEdges := map[string][]Edge{}
	for _, e := range def.Edges {
		outEdges[e.From] = append(outEdges[e.From], e)
	}

	visited := map[string]bool{}
	result := SimResult{Nodes: []SimNode{}}
	record := func(n Node, status string) *SimNode {
		sn := SimNode{NodeID: n.ID, Type: n.Type, Status: status}
		result.Nodes = append(result.Nodes, sn)
		return &result.Nodes[len(result.Nodes)-1]
	}

	current := "trigger"
	for {
		edges := outEdges[current]
		if len(edges) == 0 {
			break // end of the path
		}
		var next string
		if current != "trigger" && byID[current].Type == "branch" {
			taken, err := evalBranch(byID[current].Config.Condition, payload)
			if err != nil {
				sn := record(byID[current], "failed")
				sn.Error = err.Error()
				break
			}
			for _, e := range edges {
				if e.Label == taken {
					next = e.To
					break
				}
			}
			if next == "" {
				break
			}
		} else {
			next = edges[0].To
		}
		if visited[next] {
			sn := record(byID[next], "failed")
			sn.Error = "cycle detected during simulation"
			break
		}

		node := byID[next]
		switch node.Type {
		case "branch":
			taken, err := evalBranch(node.Config.Condition, payload)
			if err != nil {
				sn := record(node, "failed")
				sn.Error = err.Error()
			} else {
				sn := record(node, "completed")
				sn.Output = map[string]any{"taken": taken}
				visited[next] = true
				current = next
			}
		case "http":
			sn := record(node, "completed")
			req, err := simRequest(node.Config, payload)
			if err != nil {
				sn.Status = "failed"
				sn.Error = err.Error()
			} else {
				sn.Request = req
				visited[next] = true
				current = next
			}
		case "email":
			sn := record(node, "completed")
			sn.Email = simEmail(node.Config, payload)
			visited[next] = true
			current = next
		case "log":
			sn := record(node, "completed")
			sn.Output = map[string]any{"message": substituteTokens(node.Config.Message, payload)}
			visited[next] = true
			current = next
		default:
			sn := record(node, "failed")
			sn.Error = fmt.Sprintf("unknown node type %q", node.Type)
		}
	}

	// Every node not on the taken path is skipped, mirroring the runner.
	for _, n := range def.Nodes {
		onPath := false
		for i := range result.Nodes {
			if result.Nodes[i].NodeID == n.ID {
				onPath = true
				break
			}
		}
		if !onPath {
			record(n, "skipped")
		}
	}
	return result
}

// simRequest expands connector presets and builds the request the runner
// would send — substituted, never delivered.
func simRequest(cfg NodeConfig, payload map[string]any) (*SimRequest, error) {
	if cfg.Connector != "" {
		expanded, err := expandConnector(cfg)
		if err != nil {
			return nil, err
		}
		cfg = expanded
	}
	req := &SimRequest{
		Method:  cfg.Method,
		URL:     substituteTokens(cfg.URL, payload),
		Headers: map[string]string{},
		Body:    map[string]any{},
	}
	for k, v := range cfg.Headers {
		req.Headers[k] = substituteTokens(v, payload)
	}
	if cfg.Body != nil {
		raw, err := json.Marshal(cfg.Body)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		if err := json.Unmarshal([]byte(substituteTokens(string(raw), payload)), &req.Body); err != nil {
			return nil, fmt.Errorf("substitute body: %w", err)
		}
	}
	return req, nil
}

// simEmail substitutes every email field without delivering.
func simEmail(cfg NodeConfig, payload map[string]any) *SimEmail {
	recipients := []string{}
	for _, addr := range strings.Split(substituteTokens(cfg.To, payload), ",") {
		if addr = strings.TrimSpace(addr); addr != "" {
			recipients = append(recipients, addr)
		}
	}
	return &SimEmail{
		To:      recipients,
		Subject: substituteTokens(cfg.Subject, payload),
		Text:    substituteTokens(cfg.Text, payload),
	}
}

// EvaluateAssertions checks every assertion against the simulation result.
// An assertion about a node that never ran fails with a clear reason.
func EvaluateAssertions(sim SimResult, assertions []Assertion) []AssertionResult {
	results := []AssertionResult{}
	byNode := map[string]SimNode{}
	for _, n := range sim.Nodes {
		byNode[n.NodeID] = n
	}
	for _, a := range assertions {
		res := AssertionResult{Assertion: a}
		node, ok := byNode[a.NodeID]
		if !ok {
			res.Reason = fmt.Sprintf("node %q not found in simulation", a.NodeID)
			results = append(results, res)
			continue
		}
		actual, ok := simProperty(node, a.Property)
		if !ok {
			res.Reason = fmt.Sprintf("node %q has no property %q", a.NodeID, a.Property)
			results = append(results, res)
			continue
		}
		res.Actual = actual
		res.Passed = compareAssertion(actual, a.Op, a.Value)
		if !res.Passed {
			res.Reason = fmt.Sprintf("node %q: %v %s %v", a.NodeID, actual, a.Op, a.Value)
		}
		results = append(results, res)
	}
	return results
}

// simProperty resolves a property path against a node's simulated outcome.
func simProperty(node SimNode, property string) (any, bool) {
	switch {
	case property == "status":
		return node.Status, true
	case property == "taken":
		return node.Output["taken"], node.Output["taken"] != nil
	case property == "message":
		return node.Output["message"], node.Output["message"] != nil
	case property == "request.url" && node.Request != nil:
		return node.Request.URL, true
	case property == "request.method" && node.Request != nil:
		return node.Request.Method, true
	case strings.HasPrefix(property, "request.body.") && node.Request != nil:
		key := strings.TrimPrefix(property, "request.body.")
		v, ok := node.Request.Body[key]
		return v, ok
	case property == "email.subject" && node.Email != nil:
		return node.Email.Subject, true
	case property == "email.text" && node.Email != nil:
		return node.Email.Text, true
	case strings.HasPrefix(property, "output."):
		key := strings.TrimPrefix(property, "output.")
		v, ok := node.Output[key]
		return v, ok
	default:
		return nil, false
	}
}

// compareAssertion applies the operator. Values compare as strings when
// either side is non-numeric; contains is substring matching.
func compareAssertion(actual any, op string, expected any) bool {
	aStr := fmt.Sprintf("%v", actual)
	eStr := fmt.Sprintf("%v", expected)
	switch op {
	case "==":
		return aStr == eStr
	case "!=":
		return aStr != eStr
	case "contains":
		return strings.Contains(aStr, eStr)
	default:
		return false
	}
}

// ValidateAssertions checks shape: known op, non-empty node/property.
func ValidateAssertions(assertions []Assertion) error {
	for _, a := range assertions {
		if strings.TrimSpace(a.NodeID) == "" {
			return fmt.Errorf("assertion needs nodeId")
		}
		if strings.TrimSpace(a.Property) == "" {
			return fmt.Errorf("assertion for node %q needs property", a.NodeID)
		}
		switch a.Op {
		case "==", "!=", "contains":
		default:
			return fmt.Errorf("assertion op must be ==, !=, or contains (got %q)", a.Op)
		}
		if a.Value == nil {
			return fmt.Errorf("assertion for node %q needs value", a.NodeID)
		}
	}
	return nil
}
