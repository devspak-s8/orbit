// Connector presets turn the generic HTTP node into "send a Slack message"
// with one pasted credential. The catalog is the single source of truth:
// the API serves it to the web editor, validation checks nodes against it,
// and the runner expands presets into plain HTTP requests at execution
// time (docs/plans/connector-presets.md).
package automation

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// ConnectorField describes one preset form field.
type ConnectorField struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Type        string `json:"type"` // "text" | "textarea" | "url"
	Required    bool   `json:"required"`
	Placeholder string `json:"placeholder,omitempty"`
	Description string `json:"description,omitempty"`
	// URLPrefix, when set, requires url-typed values to start with it
	// (catches paste errors and keeps the preset promise).
	URLPrefix string `json:"urlPrefix,omitempty"`
}

// Connector is one preset: metadata, its form, and how params become an
// HTTP request. Build receives validated params and returns the plain HTTP
// config; {{payload.*}} substitution happens later, in the runner.
type Connector struct {
	ID       string           `json:"id"`
	Name     string           `json:"name"`
	Category string           `json:"category"`
	Fields   []ConnectorField `json:"fields"`
	Build    func(params map[string]string) (method, reqURL string, headers map[string]string, body map[string]any)
}

func (c Connector) field(key string) (ConnectorField, bool) {
	for _, f := range c.Fields {
		if f.Key == key {
			return f, true
		}
	}
	return ConnectorField{}, false
}

// Catalog is the built-in preset registry. Order is the display order.
var Catalog = []Connector{
	{
		ID:       "slack",
		Name:     "Slack",
		Category: "chat",
		Fields: []ConnectorField{
			{Key: "url", Label: "Webhook URL", Type: "url", Required: true, URLPrefix: "https://hooks.slack.com/", Placeholder: "https://hooks.slack.com/services/T…/B…/…", Description: "Create one in Slack: Apps → Incoming Webhooks."},
			{Key: "text", Label: "Message", Type: "textarea", Required: true, Placeholder: "Task {{payload.title}} changed status", Description: "Supports {{payload.*}} tokens."},
		},
		Build: func(p map[string]string) (string, string, map[string]string, map[string]any) {
			return "POST", p["url"], nil, map[string]any{"text": p["text"]}
		},
	},
	{
		ID:       "discord",
		Name:     "Discord",
		Category: "chat",
		Fields: []ConnectorField{
			{Key: "url", Label: "Webhook URL", Type: "url", Required: true, URLPrefix: "https://discord.com/api/webhooks/", Placeholder: "https://discord.com/api/webhooks/…", Description: "Server Settings → Integrations → Webhooks → Copy Webhook URL."},
			{Key: "content", Label: "Message", Type: "textarea", Required: true, Placeholder: "Task {{payload.title}} changed status", Description: "Supports {{payload.*}} tokens."},
		},
		Build: func(p map[string]string) (string, string, map[string]string, map[string]any) {
			return "POST", p["url"], nil, map[string]any{"content": p["content"]}
		},
	},
	{
		ID:       "telegram",
		Name:     "Telegram",
		Category: "chat",
		Fields: []ConnectorField{
			{Key: "token", Label: "Bot token", Type: "text", Required: true, Placeholder: "123456789:AAE…", Description: "From @BotFather."},
			{Key: "chatId", Label: "Chat ID", Type: "text", Required: true, Placeholder: "@mychannel or -1001234567890", Description: "Chat, group, or channel the bot can post to."},
			{Key: "text", Label: "Message", Type: "textarea", Required: true, Placeholder: "Task {{payload.title}} changed status", Description: "Supports {{payload.*}} tokens."},
		},
		Build: func(p map[string]string) (string, string, map[string]string, map[string]any) {
			return "POST", "https://api.telegram.org/bot" + p["token"] + "/sendMessage", nil,
				map[string]any{"chat_id": p["chatId"], "text": p["text"]}
		},
	},
	{
		ID:       "webhook",
		Name:     "Webhook POST",
		Category: "webhook",
		Fields: []ConnectorField{
			{Key: "url", Label: "Request URL", Type: "url", Required: true, Placeholder: "https://example.com/hook", Description: "Any absolute http(s) URL."},
			{Key: "body", Label: "JSON body", Type: "textarea", Required: false, Placeholder: `{"event": "{{payload.type}}"}`, Description: "Optional JSON object. Supports {{payload.*}} tokens."},
		},
		Build: func(p map[string]string) (string, string, map[string]string, map[string]any) {
			var body map[string]any
			if raw := strings.TrimSpace(p["body"]); raw != "" {
				_ = json.Unmarshal([]byte(raw), &body) // validated at save time
			}
			if body == nil {
				body = map[string]any{}
			}
			return "POST", p["url"], nil, body
		},
	},
}

func connectorByID(id string) (Connector, bool) {
	for _, c := range Catalog {
		if c.ID == id {
			return c, true
		}
	}
	return Connector{}, false
}

// connectorInfo is the wire form of a catalog entry: metadata and the form
// spec only. Build funcs are server-side and must not leak into JSON
// (encoding/json cannot marshal funcs).
type connectorInfo struct {
	ID       string           `json:"id"`
	Name     string           `json:"name"`
	Category string           `json:"category"`
	Fields   []ConnectorField `json:"fields"`
}

// WireCatalog returns the catalog in its serializable form.
func WireCatalog() []connectorInfo {
	out := make([]connectorInfo, len(Catalog))
	for i, c := range Catalog {
		out[i] = connectorInfo{ID: c.ID, Name: c.Name, Category: c.Category, Fields: c.Fields}
	}
	return out
}

// validateConnectorNode checks a preset node against the catalog. Problems
// are returned in the same style as Definition.Validate.
func validateConnectorNode(nodeID, connector string, params map[string]string) []string {
	var problems []string
	c, ok := connectorByID(connector)
	if !ok {
		known := make([]string, len(Catalog))
		for i, cc := range Catalog {
			known[i] = cc.ID
		}
		return []string{fmt.Sprintf("node %q: unknown connector %q (available: %s)", nodeID, connector, strings.Join(known, ", "))}
	}
	if params == nil {
		params = map[string]string{}
	}
	for _, f := range c.Fields {
		v := strings.TrimSpace(params[f.Key])
		if f.Required && v == "" {
			problems = append(problems, fmt.Sprintf("node %q: connector %q requires %q (%s)", nodeID, c.ID, f.Key, f.Label))
			continue
		}
		if v == "" {
			continue
		}
		if f.Type == "url" {
			u, err := url.Parse(v)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				problems = append(problems, fmt.Sprintf("node %q: %q must be an absolute http(s) URL", nodeID, f.Key))
				continue
			}
			if f.URLPrefix != "" && !strings.HasPrefix(v, f.URLPrefix) {
				problems = append(problems, fmt.Sprintf("node %q: %q must start with %s", nodeID, f.Key, f.URLPrefix))
			}
		}
	}
	// Extra keys are rejected: typos silently dropping data is worse.
	known := map[string]bool{}
	for _, f := range c.Fields {
		known[f.Key] = true
	}
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !known[k] {
			problems = append(problems, fmt.Sprintf("node %q: connector %q has no field %q", nodeID, c.ID, k))
		}
	}
	// The generic webhook preset parses its body at save time.
	if c.ID == "webhook" {
		if raw := strings.TrimSpace(params["body"]); raw != "" {
			var probe map[string]any
			if err := json.Unmarshal([]byte(raw), &probe); err != nil {
				problems = append(problems, fmt.Sprintf("node %q: %q must be a JSON object", nodeID, "body"))
			}
		}
	}
	return problems
}

// expandConnector turns a preset node's params into a plain HTTP config.
// Called from the runner after validation; substitution of {{payload.*}}
// tokens happens downstream, unchanged.
func expandConnector(cfg NodeConfig) (NodeConfig, error) {
	c, ok := connectorByID(cfg.Connector)
	if !ok {
		return NodeConfig{}, fmt.Errorf("unknown connector %q", cfg.Connector)
	}
	method, reqURL, headers, body := c.Build(cfg.Params)
	return NodeConfig{
		Method:    method,
		URL:       reqURL,
		Headers:   headers,
		Body:      body,
		Retries:   cfg.Retries,
		TimeoutMs: cfg.TimeoutMs,
	}, nil
}
