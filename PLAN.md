# ORBIT — 90-Day Plan

**Version:** 0.1 (pre-development)
**Owner:** solo engineer, full-time
**Source:** ORBIT PRD v1.0
**Status:** planning — no implementation has started

This document converts the PRD into an executable 90-day plan. Where it disagrees with the PRD's §17 roadmap, it disagrees deliberately and the reason is recorded.

---

## 1. Goal

**Produce a working platform skeleton that satisfies every item in PRD §19 (First Implementation Acceptance Criteria).** Nothing more.

The 15 criteria, in order, are:

1. A developer can clone and run the project.
2. Desktop and web clients can connect to the backend.
3. A user can register and authenticate.
4. A user can create a workspace.
5. Workspace membership and permissions are enforced.
6. The platform can discover a module manifest.
7. The platform can install and enable a module.
8. Module routes and interfaces appear in the workspace.
9. A module can register an event.
10. The automation engine can subscribe to that event.
11. A workflow can execute an HTTP action.
12. Execution history is persisted.
13. Failed operations can be inspected.
14. The system can restart without losing persisted workflow state.
15. The platform can run using Docker Compose.

§19 is the single definition of done. Any feature that appears on neither the PRD's §19 list nor a dependency of an item on that list is out of scope for these 90 days.

---

## 2. Non-goals (explicit cuts)

The PRD describes ten modules. For these 90 days, **zero modules ship at full product quality.** The only module work is the minimal Automation module and a tiny demonstration module used to prove the module system.

| Cut | Why | When it comes back |
|---|---|---|
| Work Management, Data Studio, Developer Studio, Infrastructure Studio, Observability, Security Lab, Analytics Studio, AI Studio, File Studio | Each is a multi-quarter product; building any of them now starves the skeleton | Post-skeleton, one module at a time (§5 below) |
| Third-party plugin sandboxing (WASM/WASI, signed releases, resource quotas) | Interface must be proven before isolation is engineered; untrusted third-party code is a research project | After the module API is stable at v1 |
| Desktop local runtime (local DB, local automation, local modules) | Doubles every feature's surface area; PRD §8.8 is the *last* thing to build, not the first | Before public beta |
| Workflow editor UI (drag-and-drop canvas) | Definition format + validator + engine first; editor is a presentation layer | After the engine executes JSON-defined workflows |
| Redis/Valkey, OpenSearch, object storage | PostgreSQL alone covers the skeleton; fewer moving parts | When the first benchmark justifies each |
| AI gateway and any AI features | PRD §6.9 already mandates AI-optional | Post-1.0 |
| Billing, cloud services, enterprise SSO | PRD §11/§13 make these optional | Commercial phase |
| Subflows, parallel execution, loops in the workflow engine | §9 scope for later; the skeleton needs linear + branch + retry only | Engine hardening phase |
| Cross-platform desktop polish (Windows/macOS/Linux installers) | Development happens on one platform; packaging matrix comes later | Beta |

**Rule:** if scope is slipping, the plan cuts features, never the §19 checklist. Cuts are recorded in `DECISIONS.md` so the PRD's deferred items stay visible instead of silently vanishing.

---

## 3. Team reality and what it implies

One person, full-time, 90 days ≈ 12 productive weeks ≈ roughly 400–500 engineering hours after meetings, admin, and life.

Implications, stated so the plan respects them:

- **Strictly sequential phases.** No parallel tracks. A phase is finished when its checklist passes, not when the calendar says so.
- **One week of buffer is reserved** (Week 13). Buffer absorbs reality; it is not headroom for new scope.
- **Decisions are made in Phase 0 and written down.** A solo engineer cannot afford to revisit architecture mid-build. ADRs exist so "why" survives.
- **Public repo early.** The PRD (§12) and pragmatism agree: a public repo from day 1 invites help, creates accountability, and makes bus-factor irrelevant.

---

## 4. Architecture decisions (resolved in Phase 0)

The PRD's §21 table is adopted. The open decisions below are resolved **now** so implementation never blocks on them. Each becomes an ADR in `docs/adr/` during Phase 0.

| # | Decision | Choice | Rationale |
|---|---|---|---|
| D1 | Repo layout | Single monorepo: `server/` (Go), `apps/web/` (React + TS), `apps/desktop/` (Tauri 2), `packages/` (shared types, design tokens), `docs/` | Solo maintainability; PRD's modular monolith has one deployable |
| D2 | Automation engine | **Build a minimal durable engine in Go**: Postgres-backed queue tables, in-process worker pollers, JSON workflow definitions, checkpointed step states | Embedding Temporal adds ops burden and API surface for a solo dev; minimal scope (HTTP + webhook + manual triggers) is achievable; the queue/worker boundary is designed so a future engine swap is possible. Decision revisited at engine hardening |
| D3 | Event system | Transactional outbox table in Postgres + in-process relay that dispatches to subscribers and (later) signed webhooks | PRD §8.6 compliant; no new infra; replayable; swap for a broker later without changing the event contract |
| D4 | Jobs/queue | Same Postgres-backed queue mechanism as D2; no separate job system | One queue primitive for automation steps and background jobs |
| D5 | Caching | None in the skeleton | Nothing needs it yet |
| D6 | Search | PostgreSQL ILIKE/FTS on demand; no search service | PRD §8.2 already says Postgres first |
| D7 | Real-time | SSE, not WebSockets, for execution updates | Simpler; sufficient for the skeleton |
| D8 | Workspace isolation | Workspace-scoped ownership columns on every table + application-layer authorization checks; RLS deferred | RLS as the *only* mechanism is banned by the PRD; app-layer checks are the skeleton's requirement |
| D9 | Secrets | AES-256-GCM at rest via a server-side key, scoped per workspace, never in workflow definitions or logs | Meets PRD §10 minimum without a KMS dependency |
| D10 | Local DB | PostgreSQL via Docker Compose, both dev and self-host; no SQLite path in the skeleton | One persistence target reduces variance; SQLite for local runtime is a later decision |
| D11 | Desktop scope | Thin Tauri shell: workspace login, same web UI embedded, desktop notifications, secure token storage. No local runtime | Preserves "desktop-first" surface while deferring §8.8 |
| D12 | Language/formatting conventions | Go (server), TypeScript strict (frontend); taskfiles/Make for orchestration; formatting + linting enforced in CI from day 1 | Non-negotiable for a solo codebase that others will read |

Spikes required before architecture locks: (a) prove Postgres queue can handle concurrent worker polling without lost/duplicate jobs (SKIP LOCKED), and (b) validate the Tauri shell can host the web build and authenticate. Both are Phase 0 items, timeboxed to three days combined.

---

## 5. Phases

### Phase 0 — Foundations and decisions (Weeks 1–2)

**Exit criteria:** repo is public; CI runs lint/test on push; `make dev` starts Postgres + server + web; ADRs for D1–D12 exist; both spikes resolved; a "hello workspace" end-to-end ping works.

- Create GitHub repo `ORBIT-ORG/orbit` (name check pending — see risk R2), license, CoC, security policy, contributing guide, roadmap stub.
- Monorepo skeleton: Go server (chi or stdlib router), React + TS + Tailwind + a minimal design system (tokens, 3–5 primitives, keyboard/screen-reader basics per PRD §14).
- Tauri 2 shell app that loads the web UI and stores an auth token (D11).
- Postgres migrations tooling; initial schema for `users`, `workspaces`, `memberships`.
- CI: build, lint, test, and a Docker Compose smoke test.
- Timeboxed spikes for D2 queue mechanics and D11 shell auth.

### Phase 1 — Platform core (Weeks 3–5)

**Exit criteria:** criteria 3, 4, 5 pass. A second user cannot see or modify the first user's workspace. Audit rows are written for auth and membership events.

- Registration, login, session management, logout (criterion 3).
- Password hashing (Argon2id), session rotation, basic rate limiting on auth endpoints.
- Workspace create/list/rename (criterion 4).
- Memberships: owner/member roles, invite by email (self-join code is acceptable), permission checks enforced at API and data layers (criterion 5).
- API gateway conventions: versioned routes, unified error format, request IDs, pagination envelope, OpenAPI generation from the Go types.
- Module registry tables and manifest schema (criterion 6 data model), seeded with built-in manifests only.
- Minimal audit log domain; written to from auth, workspace, and membership flows.

### Phase 2 — Module system, minimal (Weeks 6–7)

**Exit criteria:** criteria 6, 7, 8, 9 pass. A "Demo" module ships entirely through the manifest pipeline.

- Manifest validation, dependency check against `orbit.core`, compatibility check (`apiVersion`).
- Install → configure → enable lifecycle APIs (PRD §7.2 reduced to: install, enable, disable, uninstall — upgrade/rollback deferred but the version column exists).
- UI registration: the module declares frontend routes; the web shell renders them under `/workspace/:ws/modules/:module` (criterion 8).
- Backend registration: the module declares HTTP handlers and event subscriptions; the core registers them at enable time (criteria 6, 9).
- Event bus with transactional outbox (D3): `event` schema {id, type, schemaVersion, workspace, actor, correlationId, payload}; the Demo module emits `demo.record.created`.
- Module data is owned by the module (PRD §8.5); module tables live under the module's schema namespace.

### Phase 3 — Automation engine, minimal (Weeks 8–10)

**Exit criteria:** criteria 10, 11, 12, 13, 14 pass. A workflow defined as JSON subscribes to `demo.record.created`, calls an HTTP endpoint, and survives a server kill mid-flight.

Scope of the engine this phase (everything else is deferred per §2):

- Workflow definition format (JSON, versioned): metadata, trigger, nodes, edges, input/output schemas, retry and timeout policies, required permissions (PRD §9.3).
- Validator: structural, cycle, missing-node, and permission checks. The engine **never executes** a workflow in `DRAFT` (PRD §6.2 requirement: editing separated from execution).
- Triggers: manual (API + minimal UI button) and internal event subscription (criterion 10).
- Node types: HTTP request and a log/output node. That is all.
- Execution: Postgres queue (D2), workflow instance state persisted per step, linear execution with a simple conditional branch, one retry policy, timeouts.
- Execution history API + UI table: instances, steps, inputs/outputs (redacted), timings, status (criteria 12, 13).
- Cancellation and failure inspection UI (criterion 13).
- Restart recovery: on boot, the worker resumes `QUEUED`/`RUNNING` instances marked by a stale heartbeat; test kills the process mid-run (criterion 14).
- Webhook trigger: deferred if Weeks 8–10 run long; manual + event triggers satisfy §19.

### Phase 4 — Hardening and public alpha (Weeks 11–12)

**Exit criteria:** criteria 1, 2, 15 pass, plus the §18 security tests that map to §19. The internal-alpha definition from PRD §16 is met.

- Docker Compose: `postgres` + `server` + `worker` + `web`; `docker compose up` from a clean clone reaches a login screen (criteria 1, 15).
- Desktop shell connects to a remote backend as well as localhost (criterion 2).
- Backup/restore procedure documented and executed against the Compose stack (restore verified, not just written).
- Security tests per PRD §18 scoped to §19: cross-workspace access, unauthorized API access, webhook forgery (deferred with webhooks), injection basics, permission escalation.
- Integration tests: auth → workspace → module install → event → workflow → HTTP action → history (the §18 end-to-end journey).
- README, quickstart, architecture overview, API docs, upgrade notes.
- Tag `v0.1.0-alpha`; publish release notes. Done is the §19 checklist, demonstrated in a recorded walkthrough.

### Week 13 — Buffer

Reserved. If unused: the highest-value §19-adjacent item (recommended: webhook trigger) or documentation depth — decided at the time, never pulled forward.

---

## 6. Working method

- **Monday:** pick the phase's checklist items for the week. **Daily:** code + tests in the same change. **Friday:** run the §19 checklist as far as it goes; update this file's status table.
- Every architecture decision gets an ADR with context and consequences. Decisions may be revisited with new evidence; they are never changed silently.
- The PRD is the source of truth for *intent*; this document is the source of truth for *sequencing*. Conflicts resolve toward this document, and the resolution is recorded.
- Deploy-to-production thinking from day 1: migrations are forward-only and tested in CI; secrets are never in logs; every table has workspace ownership where user data lives.

---

## 7. Risk register

| ID | Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|---|
| R1 | Scope creep toward the PRD's ten modules | High | High | §2 cut list is policy; new scope requires displacing an existing §19 item, never adding load |
| R2 | "ORBIT" name conflicts (trademark/domain/repo) | Medium | Medium | Phase 0 check; rename is cheap now, expensive later |
| R3 | Durable execution is harder than planned (worker races, partial failures) | Medium | High | D2 spike in Phase 0; engine scope is tiny; swap-to-Temporal escape hatch documented as ADR D2 |
| R4 | Tauri adds build friction for minimal payoff in the skeleton | Medium | Medium | D11 keeps desktop thin; if Tauri blocks, ship web-only for alpha and document the deviation (criterion 2 reworded, not silently dropped) |
| R5 | Burnout / lost weeks (solo factor) | Medium | High | Week 13 buffer; public repo lowers isolation; phases are sequential so slippage is visible |
| R6 | Dependency licence incompatibility (PRD §12) | Low | Medium | Licence check in CI from Phase 0 |
| R7 | Postgres queue contention under concurrency | Low | Medium | SKIP LOCKED spike; single-worker default in the skeleton |
| R8 | Over-engineering (microservices, premature services) | Medium | Medium | D1–D12 forbid it; ADR review gate before any new infra is added |

---

## 8. Definition of done

The 90 days are successful when, on a clean machine:

```bash
git clone https://github.com/ORBIT-ORG/orbit.git
cd orbit
docker compose up -d
```

…leads to: registering an account, creating a workspace, installing and enabling a module, building a workflow that subscribes to a module event and executes an HTTP action, inspecting persisted execution history (including a failure), and killing/restarting the stack without losing workflow state — with desktop and web clients both connected to the backend.

Everything else in the PRD is future work, recorded and sequenced, not forgotten.