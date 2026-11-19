# Competitive Research: Work Management & Automation, Late 2026

**Date:** November 2026
**Sources:** 20+ comparison articles, vendor roadmaps (Airtable, OpenProject, n8n, monday), user review studies (2,597 low-star reviews), industry reports (Deloitte, Asana Work Innovation Lab, Smartsheet PPM survey)
**Purpose:** inform ORBIT's roadmap through 2027

---

## 1. Work management: state of the art

### Table stakes (every serious tool has them)

Multiple views (list, kanban, Gantt/timeline, calendar), task hierarchy with subtasks and
dependencies, native docs or wiki, workload/capacity views, per-project custom fields,
saved filters, integrations (200–1,000+), mobile apps.

### The 2026 layer: AI in every tier

Every major tool now ships AI, and the pricing games are the story:

| Tool | AI product | Billing model |
|---|---|---|
| Asana | AI Teammates (30+), AI Studio ("words to workflows") | Bundled from Starter ($10.99/seat) |
| Linear | Linear Agent + Triage Intelligence (auto-labels/assigns from patterns) + **MCP** | Every plan incl. Basic ($10/seat) |
| ClickUp | Brain + Autopilot + Super Agents; multimodel (GPT-5, Claude, o3) | $7–9/seat add-on; 1,000 credits/user/mo pooled |
| Monday | AI Blocks inside automations, Sidekick | **Credit-metered** — AI in automations costs ~8 credits/action; invoices surprise teams |
| Notion | Custom Agents (Feb 2026), multimodel | Moved from cheap add-on into Business ($15–20/seat) |

**MCP is the differentiator.** Linear's Model Context Protocol integration lets external
AI pull context from codebases and docs. Airtable's upgraded MCP connects Claude, ChatGPT,
Gemini, and Cursor directly to bases — read/write records, build automations, submit
forms from inside your AI session.

### What users actually complain about (2,597 low-star reviews, Aug 2026)

| Complaint | Share |
|---|---|
| **Missing capability** | 16.2% |
| Reliability (crashes, freezes) | 15.9% |
| Mobile-vs-desktop gap | 8.3% |
| Ads & upsell pressure | 6.9% |
| Team & permissions | 6.9% |

The PM community's recurring gaps (Reddit r/projectmanagement, Feb 2026):
**capacity forecasting** ("tools manage resources today but lack robust future-facing
forecast"), cross-project dependencies, and reporting that requires exports to Excel.

### Pricing dark patterns users hate

- Monday: 3-seat minimum on every paid tier; AI credits metered inside automations
- Asana: Starter→Advanced more than doubles per-user ($10.99→$24.99) to get portfolio/goals
- Basecamp: time tracking gated at Pro Plus ($99/mo); no Gantt, no dependencies, ever
- ClickUp: AI as a separate paid add-on on top of per-seat; whole workspace must move up a tier together
- Microsoft: killing To Do (June 2027) with **no migration path** for personal accounts

### Where work management is going (2027)

1. **Agents as colleagues, not features.** Airtable's stated direction: agents with
   "their own identity, access, and accountability, just like the people they work
   alongside." Asana's research: 77% of knowledge workers already use AI agents; only
   12% of organizations review them; a third of workers don't know who's accountable
   when an agent errs.
2. **Agents counted as capacity.** Planview announced agent resource management
   (fall 2026) — agents, their compute and token cost, enter the resource model.
   Dayforce scenario-models skills/capacity/cost across people *and* agents.
3. **Predictive resource management.** Monday's positioning: "from scheduling to
   strategy" — predictive capacity planning, intelligent assignment matching
   (skills + availability + team dynamics), proactive risk detection.
4. **Self-hosted AI with sovereignty.** OpenProject's 2026 roadmap: MCP server
   first, AI text assistance, semantic search — all processing inside your own
   environment, no external data sharing.

---

## 2. Automation: state of the art

### The three billing models (this is the real product decision)

| | Zapier | Make | n8n |
|---|---|---|---|
| You pay per | Task (each action) | Credit (each module run; **polls cost credits even when nothing found**) | Execution (whole run, any steps) |
| 10k actions/mo (10-step × 1k runs) | $129/mo | ~$9/mo | $20/mo (2.5k executions) |
| Self-host | No | No | Yes (fair-code, not OSI open source) |

### n8n's 2026 trajectory (the most instructive competitor)

n8n shipped **Agents as a product** (Sept 2026): describe an agent in plain language,
give it a model, tools, channels (Slack/Telegram/schedule), skills, sub-agents, knowledge
files, and memory. One agent turn = one execution. Sessions are reviewable with every
step, tool call, input, and output visible.

Then they announced (In The Loop event, late 2026) a **ground-up engine rewrite** built on
six reliability rules — and every one is a feature ORBIT's engine already has:

| n8n's reliability rule | ORBIT today |
|---|---|
| "Write it down first" (persist before executing) | ✅ per-step checkpoints in Postgres |
| "Only one version of the truth" | ✅ single durable run state |
| "Every job must reach an end" | ✅ finishRun on all paths |
| "Contain the damage" | ✅ step isolation; failures don't leak |
| "Safe to try again" | ✅ per-node retry policies |
| "Leave a trail" | ✅ execution history with outputs |

They're also building: reconciliation sweeps for stuck jobs (we call this Recover),
sandboxed code nodes, worker pools, cross-instance observability ("Manifest"), agent
identity separate from borrowed human logins, SCIM provisioning for agents, policy
gates (stop things before they run), hard cost caps, and evals for workflows.

**Translation: the market leader's two-year engine roadmap is ORBIT's v0.1.** Our
durable-execution bet was correct. Their differentiators we lack: the visual canvas,
the node library (400+), agents as first-class objects, and the community.

### Where automation is going (2027)

1. **Agents inside workflows.** All three platforms converge here: the AI decides
   which tools to call instead of following fixed steps. n8n: "Message an Agent" node
   calls a published agent from a workflow step. Zapier folded agents into the Zap
   editor (July 2026) with per-tool approvals and model tiers.
2. **Governance as product.** n8n's community is begging for it (a governance node
   request — PII scanning, cost budgets, tool authorization, audit evidence — got
   built by a user as a community node). The platforms are answering with policies,
   limits, and attribution.
3. **Self-healing.** n8n explicitly named it: automations that don't need you pouring
   over execution logs. Evals, automated.
4. **Cost control.** Monday's credit model is the cautionary tale; n8n's hard caps
   and Zapier's per-tool approvals are the response. Nobody has solved metering.

---

## 3. The convergence

Work management and automation are merging into the same product — agents with identity,
permissions, and audit trails operating on a shared system of record. Airtable said it
plainly: *"a unified platform where customers can seamlessly bring their teams, data,
workflows, and agents together in one place."*

That is ORBIT's thesis, stated by a $11B company.

The gap nobody has filled: **all of this is cloud-only or license-crippled.** n8n
self-hosting excludes projects, sharing, SSO, environments, Git version control,
and the license bans reselling. OpenProject is sovereign but its AI roadmap is still
server-side text assistance. Nothing self-hostable gives you agents + governance +
work + data in one system with an Apache license.

---

## 4. What ORBIT should do with this (roadmap implications)

### Already validated (keep, deepen)

- Durable per-step engine with restart recovery — n8n is rewriting *toward* this
- Event log + automation as one transaction — no competitor does this natively
- Module system with uninstall cleanup — our differentiator
- Bearer tokens — the "agent identity" story starts here (agents need credentials
  that aren't a person's login; n8n named this exact gap)

### The next wave (2027 targets, in order)

1. **MCP server** — ORBIT's API exposed over Model Context Protocol, so Claude,
   Cursor, ChatGPT can operate tasks, records, and workflows directly. This is the
   single highest-leverage integration in the market right now; Airtable and Linear
   both bet on it. Our Bearer tokens are the auth layer.
2. **Agent identity + audit** — first-class "agents" table: named, scoped,
   revocable, attributed in every audit row (was it the human or the agent?).
   We have the audit log and token infrastructure; this is the natural extension.
3. **Workflow testing (evals)** — replay a recorded trigger payload against a
   workflow in dry-run; assert on step outputs. n8n called evals "boring, complex,
   tedious" — which means automate them. Our execution history already stores
   everything needed.
4. **Visual workflow canvas** — the gap between our JSON editor and n8n/Make is
   the single biggest UX deficit. Branch nodes make a canvas meaningful (yes/no
   edges render naturally).
5. **Node library growth** — presets first (Slack/Discord/Telegram webhook layouts
   are 90% of real usage), then a connector SDK.
6. **Self-healing lite** — failed-run digest notifications with one-click retry;
   anomaly detection on the event log (a workflow that failed 5× in a row pages
   the owner).

### Positioning (the pitch writes itself)

> "The other guys charge per task, per credit, per seat, and per AI action.
> ORBIT runs on your server, counts nothing, and the engine the market leader is
> rewriting toward? We shipped it in v0.1."

The open-source sovereignty angle (Apache 2.0, no Sustainable Use restrictions,
unlike n8n's fair-code license) is a wedge no incumbent can copy without
relicensing.

---

*Sources: structflows.com, shoopp.store, deskdeploy.com, aitoolalliance.com,
stork.ai, propicked.com, marktis.com, getorchestra.io, automationatlas.io,
the-aischool.com, dev.to, newsprint.com (workforce planning map), openproject.org
roadmap, smartsheet.com PPM report, workmanagementinstitute.org, deloitte.com,
airtable.com roadmap + fall release, saasflags.com (Basecamp), research.district-ai.com
(2,597 low-star reviews), reddit r/projectmanagement, cotera.co (Jira vs Monday),
blog.n8n.io (Agents, 3.0), n8n "In The Loop" event transcript, tryorbye.com,
glms.com.au. All accessed November 2026.*