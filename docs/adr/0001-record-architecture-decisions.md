# ADR-0001: Record architecture decisions

**Status:** Accepted
**Date:** 2026-11-06

## Context

ORBIT is a long-lived platform with a small initial team (one engineer). Decisions made today about stack, structure, and boundaries will constrain everything built later. Without records, future contributors cannot reconstruct *why* a choice was made and may unknowingly reverse it.

## Decision

Every significant, hard-to-reverse technical decision is recorded as an ADR in this directory, using this template:

- **Status** — Proposed / Accepted / Superseded by ADR-XXXX
- **Context** — what forces shaped the decision
- **Decision** — what was chosen
- **Consequences** — the resulting trade-offs, positive and negative

ADRs are immutable once accepted. Changing a decision means writing a new ADR that supersedes the old one — never editing the old record.

## Consequences

- Slightly more writing up front; large savings in future arguments and onboarding.
- The ADR index below is the canonical history of the project's architecture.

## Index

- ADR-0002: Modular monolith and monorepo layout
- ADR-0003: Standard library router for the API
- ADR-0004: Build the durable automation engine in Go