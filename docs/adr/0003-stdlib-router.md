# ADR-0003: Standard library router for the API

**Status:** Accepted
**Date:** 2026-11-06

## Context

The API needs routing, middleware, and response conventions. Go 1.22+ includes method-aware pattern routing in `net/http` (`GET /v1/ping`). Popular routers (chi, gin, echo) add convenience but also a dependency and, in gin/echo's case, a non-standard handler signature that complicates middleware reuse.

## Decision

Use the standard library `net/http` ServeMux with method patterns. Write the small amount of shared middleware (request ID, logging, CORS, recovery) in `internal/httpapi`. Revisit only if concrete needs emerge (e.g., extensive route grouping or middleware per-route).

## Consequences

- Zero framework dependency; all handlers are plain `http.HandlerFunc`.
- Middleware is ours to maintain — currently ~100 lines.
- If a framework becomes justified later, handlers port with minimal changes.