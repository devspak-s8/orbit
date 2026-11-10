-- +goose Up
-- The module registry: every module known to the platform. For the alpha
-- the catalog is embedded first-party manifests; third-party distribution
-- arrives with the registry service (PLAN.md defers sandboxing).
CREATE TABLE modules (
    id          TEXT        PRIMARY KEY,
    name        TEXT        NOT NULL,
    version     TEXT        NOT NULL,
    api_version TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    manifest    JSONB       NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Workspace-scoped installation state. Install and enable are separate
-- steps: installation never implies execution (PRD 7.2).
CREATE TABLE module_installations (
    workspace_id UUID        NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    module_id    TEXT        NOT NULL REFERENCES modules(id) ON DELETE CASCADE,
    enabled      BOOLEAN     NOT NULL DEFAULT false,
    config       JSONB       NOT NULL DEFAULT '{}',
    installed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, module_id)
);

-- The platform event log. Writers insert rows transactionally with their
-- domain change (outbox pattern); readers are triggers and the events feed.
CREATE TABLE events (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    type           TEXT        NOT NULL,
    schema_version INTEGER     NOT NULL DEFAULT 1,
    workspace_id   UUID        NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    actor_user_id  UUID        REFERENCES users(id) ON DELETE SET NULL,
    correlation_id TEXT        NOT NULL DEFAULT '',
    payload        JSONB       NOT NULL DEFAULT '{}',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX events_workspace_idx ON events (workspace_id, created_at DESC);
CREATE INDEX events_type_idx       ON events (type, created_at DESC);

-- Module-owned tables live in their own schema (PRD 8.5): the Demo module
-- owns everything under module_demo, including its migrations.
CREATE SCHEMA module_demo;

CREATE TABLE module_demo.records (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID        NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    title        TEXT        NOT NULL,
    created_by   UUID        REFERENCES users(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX module_demo_records_ws_idx ON module_demo.records (workspace_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS module_demo.records;
DROP SCHEMA IF EXISTS module_demo;
DROP TABLE IF EXISTS events;
DROP TABLE IF EXISTS module_installations;
DROP TABLE IF EXISTS modules;