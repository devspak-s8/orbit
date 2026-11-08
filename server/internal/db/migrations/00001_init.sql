-- +goose Up
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Users are global platform accounts. Workspace membership lives in memberships.
CREATE TABLE users (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    email            TEXT        NOT NULL,
    email_normalized TEXT        NOT NULL UNIQUE,
    password_hash    TEXT        NOT NULL,
    display_name     TEXT        NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Workspaces are the tenant boundary. Every user-data table must be owned by
-- a workspace and enforce that ownership at the application layer.
CREATE TABLE workspaces (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT        NOT NULL,
    slug       TEXT        NOT NULL UNIQUE,
    created_by UUID        NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE memberships (
    workspace_id UUID        NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id      UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role         TEXT        NOT NULL CHECK (role IN ('owner', 'member')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, user_id)
);

-- Platform audit log. Written by core domains from day one.
CREATE TABLE audit_logs (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id  UUID        REFERENCES workspaces(id) ON DELETE CASCADE,
    actor_user_id UUID        REFERENCES users(id) ON DELETE SET NULL,
    action        TEXT        NOT NULL,
    resource_type TEXT        NOT NULL,
    resource_id   TEXT        NOT NULL DEFAULT '',
    metadata      JSONB       NOT NULL DEFAULT '{}',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX audit_logs_workspace_idx ON audit_logs (workspace_id, created_at DESC);
CREATE INDEX audit_logs_actor_idx    ON audit_logs (actor_user_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS memberships;
DROP TABLE IF EXISTS workspaces;
DROP TABLE IF EXISTS users;