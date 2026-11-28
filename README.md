# ORBIT

**Universal digital operations platform** — free, self-hosted, modular.

ORBIT replaces a stack of disconnected subscriptions (task management, automation, data, docs) with one platform: independent modules sharing a single login, one data model, and a built-in automation engine. A task event in one module can trigger a workflow that updates data in another — natively, without glue code.

> **Status: v1.2.0 — released and self-hostable today.** Twelve tagged releases so far. The platform core plus four product modules are live, hardened by a [published security audit](docs/security/audit-2026-11-21.md) with a live attack suite that runs against every release. Decisions are documented as [ADRs](docs/adr); every feature ships from a written [plan](docs/plans) with end-to-end verification.

## What works today

- **Accounts & workspaces** — signup, login (Argon2id, revocable sessions, rate limiting), tenant-isolated workspaces, shareable invites, in-app notifications
- **Work Management** — projects, boards (drag-and-drop), list/calendar/timeline views, tasks, subtasks, dependencies, labels, comments, time tracking
- **Data** — tables with typed columns, records, CSV import/export, public forms on secret URLs
- **Docs** — pages attached to projects, feeding the event log
- **Automation engine** — durable Postgres-backed workflows: event/manual/webhook/schedule triggers, HTTP, log, email, and branch nodes, **connector presets** (Slack, Discord, Telegram, generic webhook), per-step durability, restart recovery, **workflow evals** (dry-run replays with assertions), self-healing failure notifications with one-click retry
- **AI-native surface** — built-in **MCP server** (JSON-RPC, 10 tools) so Claude/Cursor/ChatGPT can operate your workspace, plus named **agent identities** with audit attribution
- **API tokens & security** — Bearer tokens (SHA-256 at rest), live security audit over every route, uniform 404s on secret URLs
- **Desktop app** — Electron shell for any self-hosted instance (`apps/desktop`)

## Run the full stack

Requires Docker with Compose:

```bash
git clone https://github.com/devspak-s8/orbit.git
cd orbit
docker compose up -d
```

- Web UI: http://localhost:8081
- API: http://localhost:8082/healthz
- PostgreSQL: `localhost:5434` (user `orbit`, password `orbit`, db `orbit`)

The API applies database migrations automatically at startup.

## Development

Prerequisites: Go 1.26+, Node 22+, Docker.

```bash
docker compose up -d db   # database only

cd server
go run ./cmd/api          # API on :8080 (uses localhost:5434)

cd apps/web
npm install
npm run dev               # web on :5173, proxies /v1 to :8080
```

### Tests

```bash
cd server && go test ./...
```

Requires the dev database (`docker compose up -d db`). CI runs the same suite against a fresh PostgreSQL on every push.

### Configuration

| Variable | Default | Purpose |
|---|---|---|
| `ORBIT_HTTP_ADDR` | `:8080` | API listen address |
| `ORBIT_DATABASE_URL` | `postgres://orbit:orbit@localhost:5434/orbit?sslmode=disable` | PostgreSQL connection string |
| `ORBIT_CORS_ORIGINS` | `http://localhost:5173` | Allowed browser origins (comma-separated) |
| `ORBIT_SMTP_HOST` | *(empty)* | Outbound SMTP relay for email nodes — empty disables email with a clear error |
| `ORBIT_SMTP_PORT` | `587` | SMTP relay port |
| `ORBIT_SMTP_USER` / `ORBIT_SMTP_PASS` | *(empty)* | SMTP credentials |
| `ORBIT_SMTP_FROM` | `orbit@localhost` | Envelope sender |

### Webhooks

Any workflow can be triggered from anywhere on the internet: give it the
`webhook` trigger, validate it, and the workflow page shows a unique URL:

```bash
curl -X POST -H "Content-Type: application/json" \
  -d '{"name":"Ada","message":"hello"}' \
  http://your-orbit/hooks/<token>
```

The JSON body becomes the run's payload — `{{payload.name}}` in any node
config substitutes from it. The secret is the URL; unknown URLs return
the same 404 as missing ones. Deliveries are rate-limited and audited.

### Desktop app

ORBIT also ships as a desktop app: an Electron shell for any self-hosted
instance. Enter your server URL once — the app verifies `/healthz`, then
opens your workspace in its own window with a tray icon.

```bash
cd apps/desktop
npm install
npm start         # run from source
npm run dist      # build the installer (Windows NSIS; mac/linux configured)
```

## Contributing

The highest-leverage contribution is a **new module** — ORBIT is designed
so a module is a self-contained Go package plus a web page, with the
platform providing accounts, workspaces, events, and automation:

1. Read `docs/adr/0002-modular-monolith-and-monorepo.md`, then study
   `server/internal/demo` (the reference module: schema, manifest, routes,
   events, uninstall hook — under 150 lines of Go)
2. Pick from the post-1.0 module list in `docs/ROADMAP.md` — Infrastructure
   Studio, Observability, Security Lab, AI Studio, File Studio — or propose
   your own
3. Connector presets (Twilio, GitHub, PagerDuty…) are self-contained
   entries in `server/internal/automation/connectors.go`

Every PR runs the full suite (Go + web build, serialized against a fresh
PostgreSQL). A live security audit (`TestSecurityAudit`) runs against a
booted instance — add your routes to its inventory and it will verify
your auth and tenant isolation for you.

## Backups

The database is the single source of truth; backing it up backs up everything.

```bash
# Backup
docker compose exec -T db pg_dump -U orbit -d orbit > backups/orbit-$(date +%Y%m%d-%H%M%S).sql

# Restore (into a fresh volume: docker compose down -v, up -d db, then)
cat backups/orbit-YYYYMMDD-HHMMSS.sql | docker compose exec -T db psql -U orbit -d orbit
```

The restore path is verified: a dump of the alpha schema (15 tables) restores cleanly with all data intact.

## Architecture

```
apps/web/         React + TypeScript + Tailwind (Vite)
apps/desktop/     Electron shell for any self-hosted instance
server/           Go API — domains: identity, workspaces, modules, events,
                  automation (workflow engine), work (Work Management),
                  demo (reference module)
compose.yaml      Full stack: web (nginx) -> api (Go) -> db (PostgreSQL)
docs/adr/         Architecture decision records
docs/plans/       Implementation plans (e.g. Work Management)
```

Key decisions (see [docs/adr](docs/adr)): modular monolith, standard-library HTTP, PostgreSQL as the only infrastructure, workflows as durable Postgres-backed executions, event publishing transactional with the change that produced it.

## Licence

Apache License 2.0 — see [LICENSE](LICENSE).