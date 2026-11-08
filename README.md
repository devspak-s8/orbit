# ORBIT

**Universal digital operations platform** — free, self-hosted, modular.

ORBIT replaces a stack of disconnected subscriptions (task management, automation, data, docs) with one platform: independent modules sharing a single login, one data model, and a built-in automation engine. A task event in one module can trigger a workflow that updates data in another — natively, without glue code.

> **Status: pre-alpha.** The repository currently contains the platform skeleton: Go API server, React web shell, PostgreSQL schema, and CI. See [PLAN.md](PLAN.md) for the build plan and [the roadmap](docs/ROADMAP.md) for what ships next.

## Stack

| Layer | Technology |
|---|---|
| Backend | Go |
| Frontend | React + TypeScript + Tailwind CSS |
| Database | PostgreSQL |
| Deployment | Docker Compose |
| Licence | Apache 2.0 |

## Development

Prerequisites: Go 1.26+, Node 22+, Docker with Compose.

### 1. Start the database

```bash
docker compose up -d db
```

### 2. Start the API server

```bash
cd server
go run ./cmd/api
```

The API listens on `http://localhost:8080`. Migrations apply automatically at startup.

### 3. Start the web app

```bash
cd apps/web
npm install
npm run dev
```

The web app runs on `http://localhost:5173` and proxies `/v1` to the API server.

### 4. Check it works

- API health: `http://localhost:8080/healthz`
- API ping: `http://localhost:8080/v1/ping`
- Web shell: `http://localhost:5173`

## Configuration

The server reads configuration from environment variables:

| Variable | Default | Purpose |
|---|---|---|
| `ORBIT_HTTP_ADDR` | `:8080` | API listen address |
| `ORBIT_DATABASE_URL` | `postgres://orbit:orbit@localhost:5434/orbit?sslmode=disable` | PostgreSQL connection string |
| `ORBIT_CORS_ORIGINS` | `http://localhost:5173` | Allowed browser origins (comma-separated) |

## Testing

```bash
cd server && go vet ./... && go test ./...
cd apps/web && npm run build
```

CI runs the same checks on every push.

## Repository layout

```
server/           Go API server
  cmd/api/        Entry point
  internal/       Config, database, HTTP API
apps/web/         React web application
compose.yaml      Development database
```

## Licence

Apache License 2.0 — see [LICENSE](LICENSE).