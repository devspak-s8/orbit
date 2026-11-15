# ORBIT Roadmap

**Last updated:** November 2026

ORBIT's goal is a free, self-hosted replacement for a stack of disconnected subscriptions: task management, automation, data, and docs — sharing one login, one data model, and one automation engine. The full PRD vision spans years; this roadmap defines what actually ships and when.

## Release milestones

| Milestone | Target | What it is |
|---|---|---|
| `v0.1.0-alpha` | Nov 2026 ✅ | Platform skeleton + Demo module: auth, workspaces, permissions, module system, minimal automation, Docker Compose |
| Alpha 2 | Dec 2026 ✅ | Work Management: projects, tasks, boards, assignments, due dates; task lifecycle events drive workflows |
| Alpha 3 | Nov 2026 ✅ | Automation hardened: inbound webhook triggers, durable schedules, email connector |
| `v0.2.0-beta` | Apr 2027 | Data module: tables, forms, CSV, "form → record → workflow" |
| `v1.0.0` | Mid 2027 | Docs-lite, backup/restore verified, security audit, install polish |

## Explicitly deferred (post-1.0)

- Infrastructure Studio, Observability, Security Lab, Analytics Studio, AI Studio, File Studio
- Third-party plugin sandboxing (WASM/WASI, signed releases)
- Desktop local runtime and desktop app
- Cloud hosting business, billing, enterprise SSO

## Scope rule

Every milestone must ship something usable. If a milestone is at risk, scope is cut — dates do not move.