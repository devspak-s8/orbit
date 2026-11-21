# Connector Presets — Plan

> **Status: shipped in v1.1.0.** Verified: 4 backend tests (expansion,
> validation, full-engine run, payload substitution), a live end-to-end pass
> (catalog auth-gated, webhook fired through the real engine → expanded
> request with substituted payload received), and a browser round-trip
> (preset picker → form → save → stored definition, plus a canvas
> dirty-state fix this verification surfaced).

Ship the first connector presets for the automation engine's HTTP node so
"replace Zapier" becomes concrete: pick a service, paste one credential,
write a message with `{{payload.*}}` tokens — done.

## Problem

The HTTP node is generic and powerful, but building a "notify Slack when a
task completes" automation today means knowing Slack's incoming-webhook
format and hand-writing method/URL/JSON body. That is the gap between a
workflow engine and a product people adopt.

## Scope (v1.1.0)

Four presets, chosen because they need **one pasted credential and zero
OAuth**:

| Preset | Credential | Sends |
|---|---|---|
| `slack` | incoming-webhook URL (`https://hooks.slack.com/…`) | `{"text": …}` |
| `discord` | webhook URL (`https://discord.com/api/webhooks/…`) | `{"content": …}` |
| `telegram` | bot token + chat ID | bot `sendMessage` API |
| `webhook` | any absolute http(s) URL | any JSON body (generic POST) |

## Design

**Run-time expansion, not save-time.** A node keeps its identity as a
preset:

```json
{"id":"notify","type":"http","config":{
  "connector":"slack",
  "params":{
    "url":"https://hooks.slack.com/services/T…",
    "text":"Done: {{payload.title}}"
  }}}
```

At execution the runner expands `connector` + `params` into method/URL/
headers/body, then runs the existing HTTP path — so `{{payload.*}}`
substitution, retries, timeouts, and step logging work unchanged. Round-trip
editing is lossless (the canvas re-renders preset fields), and preset layout
fixes propagate to existing workflows.

**Catalog in the server, one source of truth.** `internal/automation/
connectors.go` defines the catalog (id, name, category, field specs, build
function). `GET /v1/connectors` (authenticated, not workspace-scoped — it
is static) serves it to the UI, so form labels/types never drift between
backend and frontend.

**Validation.** A node with `connector` set must not also set method/url
(ambiguous definitions are rejected). Required params must be present and
non-empty. URL fields must be absolute http(s); the chat presets enforce
their host prefix (catches paste errors and keeps the preset promise).
The generic `webhook` preset parses its JSON body at save time.

**Changes by file:**

- `server/internal/automation/connectors.go` (new) — catalog, field specs,
  `expand` (params → NodeConfig), validation helper.
- `server/internal/automation/definition.go` — `NodeConfig` gains
  `Connector string` + `Params map[string]string`; http-node validation
  branches on preset vs. plain config.
- `server/internal/automation/runner.go` — `execHTTP` expands preset
  configs before building the request.
- `server/internal/automation/http.go` — `GET /v1/connectors` handler.
- `server/internal/httpapi/router.go` — mount the catalog route behind
  `RequireUser`.
- `apps/web/src/lib/api.ts` — `connector`/`params` in the node config
  type, `listConnectors()`, `Connector`/`ConnectorField` types.
- `apps/web/src/components/WorkflowCanvas.tsx` — connector buttons, preset
  form in the properties panel (labels/types from the catalog), preset
  badge on node cards.

## Tests

- Unit: catalog expansion (URLs, bodies) per preset; validation cases
  (unknown connector, missing required, wrong prefix, connector+url mix,
  bad JSON body on the generic preset, valid definitions pass).
- E2E through the real runner: workflow with the generic `webhook` preset
  targeting a local httptest server → validate → run → assert the server
  received the expanded request with substituted payload values.
- Live: full flow against the dev API (create → validate → run against a
  local listener), plus canvas UI check in the browser.

## Non-goals (explicitly deferred)

- OAuth flows and stored accounts (Slack app installs, etc.) — v2, needs
  a credentials vault and per-workspace secrets.
- Response mapping (using a connector's response in later nodes).
- More presets (Twilio, GitHub, PagerDuty) — follow the same pattern once
  the framework is proven.
