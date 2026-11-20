-- +goose Up
-- V1.0 completion: docs pages (orbit.docs module), agent identity.

CREATE SCHEMA module_docs;

CREATE TABLE module_docs.pages (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id   UUID        NOT NULL REFERENCES module_work.projects(id) ON DELETE CASCADE,
    workspace_id UUID        NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    title        TEXT        NOT NULL,
    body         TEXT        NOT NULL DEFAULT '',
    created_by   UUID        REFERENCES users(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX module_docs_pages_project_idx ON module_docs.pages (project_id, updated_at DESC);
CREATE INDEX module_docs_pages_ws_idx ON module_docs.pages (workspace_id, updated_at DESC);

-- Agent identity: a named API token with human-readable identity. Every
-- audit row from an agent's action carries the agent_id. Revoking the
-- token kills the agent.
CREATE TABLE agents (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         TEXT        NOT NULL,
    description  TEXT        NOT NULL DEFAULT '',
    token_id     UUID        NOT NULL REFERENCES api_tokens(id) ON DELETE CASCADE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at   TIMESTAMPTZ,
    UNIQUE (user_id, name)
);

-- Audit rows can now be attributed to an agent.
ALTER TABLE audit_logs ADD COLUMN agent_id UUID REFERENCES agents(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE audit_logs DROP COLUMN IF EXISTS agent_id;
DROP TABLE IF EXISTS agents;
DROP TABLE IF EXISTS module_docs.pages;
DROP SCHEMA IF EXISTS module_docs;