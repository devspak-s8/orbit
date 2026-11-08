# ORBIT development entry points. On Windows, run the equivalent commands
# documented in README.md; this Makefile is the canonical interface for
# Unix shells and CI.

.PHONY: db-up db-down server web test lint ci

db-up:
	docker compose up -d db

db-down:
	docker compose down

server:
	cd server && go run ./cmd/api

web:
	cd apps/web && npm run dev

test:
	cd server && go vet ./... && go test ./...
	cd apps/web && npm run build

lint:
	cd server && go vet ./...
	cd apps/web && npx tsc --noEmit

ci: test lint