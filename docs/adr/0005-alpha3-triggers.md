# ADR-0005: Alpha 3 trigger expansion

**Status:** Accepted
**Date:** 2026-11-15

## Context

The engine reacts only to internal events and manual runs. "Replace Zapier"
requires inbound triggers from outside ORBIT (webhooks, time) and outbound
effects beyond HTTP (email). Three design questions needed answers before
implementation (see docs/plans/alpha3-automation-hardening.md).

## Decision 1: webhook authentication is the secret URL

Webhook triggers authenticate by an unguessable path token (256-bit,
base64url), minted at workflow validation. `POST /hooks/{token}` is
otherwise unauthenticated: rate-limited per IP, audit-logged, payload
capped at 1 MB, unknown tokens return an identical 404 (no existence
leak). Tokens are stored on the workflow row and displayed in the UI with
a copy button; rotation is a future action.

*Rejected:* signed payloads (breaks the "paste a URL into any tool"
UX), per-workspace API keys (couples hooks to the auth system), and
hashed token storage (the token is the routing key, not a password
equivalent — it grants only "run this one workflow").

## Decision 2: schedules are intervals, not cron

Schedule triggers fire every `intervalSec` (60s–7d). The cursor
(`next_due_at` on the workflow row) advances in the same transaction as
the run enqueue, so a crash can neither double-fire nor burst-fire missed
intervals; missed intervals are skipped, documented.

*Rejected:* cron expressions — a parser dependency and a timezone/ DST
surface that the workflow editor milestone should own, while fixed
intervals cover the overwhelming majority of real automations. Revisit
when the visual editor ships.

## Decision 3: SMTP is server-level configuration

Outbound email relays through one server-wide SMTP configuration
(`ORBIT_SMTP_*` environment). The self-hoster configures it once; an empty
host fails email nodes with a clear, inspectable error rather than a
silent skip.

*Rejected:* per-workspace SMTP credentials — they require an encrypted
credential store, a permissions model, and UI that belong to a dedicated
"connections" milestone, not a trigger-expansion milestone. That
milestone will move email onto workspace-scoped credentials.

## Consequences

- Webhooks are simple to consume from any tool that can POST, at the cost
  of token-in-URL exposure (TLS and rotation mitigate).
- Scheduling is predictable and restart-safe but coarse; cron arrives later.
- Multi-tenant email routing is deferred; single-tenant self-hosting is the
  alpha's deployment model anyway.
