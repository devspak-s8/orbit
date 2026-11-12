# ORBIT

**Universal digital operations platform** — free, self-hosted, modular.

ORBIT replaces a stack of disconnected subscriptions (task management, automation, data, docs) with one platform: independent modules sharing a single login, one data model, and a built-in automation engine. A task event in one module can trigger a workflow that updates data in another — natively, without glue code.

> **Status: pre-alpha (v0.1.0-alpha).** The platform core is working end to end: identity, workspaces, a module system with install/enable lifecycle, a platform event log, and a durable workflow engine with event triggers and HTTP actions. The first user-facing module (Work Management) is next. See the [roadmap](docs/ROADMAP.md) and [plan](PLAN.md).

## What works today

- **Accounts** — signup, login, sessions (Argon2id, server-side revocable sessions, rate limiting, audit log)
- **Workspaces** — tenant-isolated environments; cross-workspace access is enforced at the data layer
- **Work Management** — the first product module: projects, tasks, boards, assignments, due dates, priorities; lifecycle events feed the automation engine
- **Modules** — manifest-based registry, install → enable → disable → uninstall lifecycle (owner-only), UI registration
- **Event log** — transactional, workspace-scoped events
- **Automation engine** — JSON workflow definitions with validation (drafts never execute), event and manual triggers, HTTP/log nodes, per-node retries and timeouts, per-step durable execution history, and restart recovery that resumes interrupted runs

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