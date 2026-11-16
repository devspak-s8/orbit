-- +goose Up
-- Work Management polish: shareable workspace invites and the core
-- in-app notification service (PRD layer 3 shared services).

CREATE TABLE invite_codes (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID        NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    code         TEXT        NOT NULL UNIQUE,
    -- Alpha invites grant the member role only; owner is transfer, not invite.
    role         TEXT        NOT NULL DEFAULT 'member' CHECK (role = 'member'),
    created_by   UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at   TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX invite_codes_workspace_idx ON invite_codes (workspace_id, created_at DESC);

CREATE TABLE notifications (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    workspace_id UUID        NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    type         TEXT        NOT NULL,
    payload      JSONB       NOT NULL DEFAULT '{}',
    read_at      TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX notifications_user_idx ON notifications (user_id, created_at DESC);
CREATE INDEX notifications_unread_idx ON notifications (user_id) WHERE read_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS notifications;
DROP TABLE IF EXISTS invite_codes;