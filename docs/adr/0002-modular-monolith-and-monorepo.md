# ADR-0002: Modular monolith and monorepo layout

**Status:** Accepted
**Date:** 2026-11-06

## Context

The PRD (§8.1) requires a modular monolith initially, not microservices, with the option to extract services later. The team is one engineer; operating cost of many deployables is prohibitive. The desktop app and web app must share a frontend; the module system requires clear in-process boundaries between domains.

## Decision

- One repository containing `server/` (Go), `apps/web/` (React), and future `apps/desktop/` (Tauri).
- The Go server is a single deployable organized into domain packages under `internal/` (`config`, `db`, `httpapi`, and later `identity`, `workspaces`, `modules`, `automation`, `events`, `audit`).
- Domains expose narrow interfaces to each other; no package may import another's internals. Events cross domain boundaries through the outbox rather than direct calls.
- Modules are first-party Go packages with declared manifests; the plugin runtime abstraction is deferred (see PLAN.md §2).

## Consequences

- One process to run, debug, and deploy; horizontal scaling comes later via workers.
- Domain discipline must be enforced by convention and review, not the compiler — acceptable at this size.
- If a service is extracted later, its domain package and event contract move without redesign.