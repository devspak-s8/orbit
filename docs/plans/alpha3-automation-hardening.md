# Alpha 3 — Automation Hardening Plan

**Milestone:** Alpha 3
**Scope:** inbound webhook triggers, schedule triggers, outbound email connector
**Goal:** make "replace Zapier" concrete — things happen from *outside* ORBIT (webhooks, time) and reach *outside* ORBIT (email)
**Status:** planned — no code yet

---

## 1. What Alpha 3 adds

The engine today reacts only to things that happen *inside* ORBIT (module events, manual
runs). Alpha 3 closes both directions:

```
              ┌─────────────────┐
 webhooks ──▶ │                 │ ──▶ HTTP requests (exists)
 schedules ─▶ │   ORBIT engine  │ ──▶ email (new)
   events  ──▶ │   (exists)      │ ──▶ log (exists)
   manual  ──▶ └─────────────────┘
```

The demo loop after Alpha 3: *a form on any website POSTs to your webhook URL →
workflow runs → sends you an email.* That's the Zapier replacement pitch, working.

## 2. Trigger: inbound webhooks

- A workflow with `trigger: { "type": "webhook" }` gets a unique token at validation
  time: `POST /hooks/{token}` (top-level route, no auth — the secret is the URL)
- Token: 32 random bytes, base64url; stored on the workflow row, `UNIQUE`
- The hook accepts any JSON body (≤ 1 MB), enqueues a run with `triggerType:
  "webhook"` and the body as the trigger payload, responds `202 { "runId": ... }`
  immediately — execution is asynchronous, as with every trigger
- `{{payload.*}}` substitution works on webhook bodies like on events
- **Security:** unknown token → 404 with a generic body (no existence leak);
  per-IP rate limiting reuses the identity limiter; every delivery is audit-logged;
  payload size capped; regenerating the token is a future "rotate" action
- Non-ready workflows (draft/archived) reject deliveries — same rule as every
  trigger: validation gates execution

## 3. Trigger: schedules

- `trigger: { "type": "schedule", "intervalSec": 900 }`
- **Intervals, not cron, for Alpha 3** — deliberate scope cut: fixed intervals
  (60s–7d) cover the overwhelming majority of real automation and need no cron
  parser dependency. Cron arrives with the workflow editor UI (ADR note)
- Scheduling state is durable: `next_due_at` on the workflow row. The worker tick
  enqueues runs when due, then advances `next_due_at` — so restarts neither
  double-fire nor burst-fire missed intervals (missed runs are skipped, documented)
- Minimum interval 60s to prevent footguns

## 4. Node: outbound email

- `email` node: `config: { "to", "subject", "body" }` — all support `{{payload.*}}`
  substitution; `to` may be a comma-separated list
- **SMTP is server-level config** (env vars), not per-workspace — the self-hoster
  configures it once in Compose: `ORBIT_SMTP_HOST/PORT/USER/PASS/FROM`
- Missing SMTP config fails the node with a clear, inspectable error (not a
  silent skip) — consistent with the "expose uncertain outcomes" principle
- No inbound email. No templating engines. Plain text body for alpha.

## 5. Schema (migration 00006)

```sql
ALTER TABLE workflows
  ADD COLUMN webhook_token TEXT UNIQUE,
  ADD COLUMN next_due_at   TIMESTAMPTZ;
```

No other tables: tokens and schedule state are workflow-owned, runs/steps unchanged.

## 6. Definition + validator changes

- Trigger types: `manual | event | webhook | schedule` (+ `intervalSec` required
  for schedule, 60–604800)
- Node types: `http | log | email` (+ `to`/`subject` required for email)
- Trigger type is immutable after creation for alpha (a webhook can't silently
  become an event trigger) — enforced on... actually there is no edit yet, so this
  is free; note for the editor milestone

## 7. UI

- Workflow detail page: webhook workflows show their URL with a copy button and a
  curl example — the page becomes the integration surface
- Template examples in the create form: add webhook + schedule + email samples
- Execution history already renders trigger types generically — `webhook` and
  `schedule` appear without UI changes

## 8. Configuration

| Variable | Purpose |
|---|---|
| `ORBIT_SMTP_HOST` | SMTP relay host (empty = email nodes fail with clear error) |
| `ORBIT_SMTP_PORT` | default 587 |
| `ORBIT_SMTP_USER` / `ORBIT_SMTP_PASS` | relay credentials |
| `ORBIT_SMTP_FROM` | envelope from |

Documented in README + Compose environment block (empty defaults).

## 9. Implementation order

1. Migration 00006 + definition/validator extension (new trigger + node types)
2. Webhook route + token generation at validation + tests
3. Scheduler in the worker tick + `next_due_at` advance + tests
4. Email node with net/smtp + in-test SMTP stub + tests
5. UI: webhook URL panel, templates
6. Live e2e: `curl` a webhook → run in history; schedule fires on a 60s interval;
   email against a local SMTP capture
7. README + ADR-0005 (trigger expansion decisions) + tag `v0.3.0-alpha`

## 10. Acceptance criteria

1. A validated webhook workflow's URL accepts a JSON POST and the payload appears
   in the run's steps and substitution
2. Bad/unknown token → 404; draft workflow's token → rejected
3. A 60s schedule fires without any manual action, survives a worker restart
   without double-firing (test)
4. Email node delivers a message through a real SMTP transaction (test stub) and
   fails visibly when SMTP is unconfigured
5. All four trigger types visible in execution history
6. Full suite green; live curl e2e recorded in the browser history page

## 11. Risks

| Risk | Mitigation |
|---|---|
| Webhook abuse (scraping tokens) | 256-bit tokens, rate limit, audit log, 404-no-leak |
| Scheduler drift/missed runs | `next_due_at` persisted; missed intervals skipped, not burst |
| SMTP credential sprawl | Server-level only for alpha; per-workspace connections are a later "credentials" milestone |
| Scope creep toward cron | Intervals only, written down here |
