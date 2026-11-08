# ADR-0004: Build the durable automation engine in Go

**Status:** Accepted
**Date:** 2026-11-06

## Context

The automation engine is the flagship subsystem (PRD §9). It requires durable execution: persisted workflow state, recovery after crash, retries, and execution history. Alternatives considered:

- **Embed an existing engine** (Temporal, River, Hatchet): mature semantics but adds an operational dependency, a new API surface, and (for Temporal) significant server infrastructure a solo team must run and understand.
- **Build a minimal engine in Go**: Postgres-backed queue tables, in-process worker pollers, JSON workflow definitions, checkpointed step states.

## Decision

Build the minimal engine in Go, scoped to: manual triggers, internal event subscriptions, and an HTTP action node. PostgreSQL tables provide the queue (`FOR UPDATE SKIP LOCKED`), execution state, and history. The queue/worker boundary is designed so a future engine swap does not require changing the workflow definition format or the execution history schema.

This decision is revisited at engine hardening if the minimal engine shows fundamental limits.

## Consequences

- Full control over semantics and no new infrastructure to operate.
- We own correctness for the hard parts (races, partial failures, stale heartbeats) — mitigated by tiny scope, a Phase 0 concurrency spike, and single-worker default.
- Postgres becomes a bottleneck only at concurrency levels far beyond the roadmap horizon.