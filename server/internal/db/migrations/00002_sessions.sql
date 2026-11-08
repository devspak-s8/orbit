-- +goose Up
-- Sessions are server-side: the client holds only a random token, and the
-- database stores a SHA-256 hash of it. Revocation is a column update.
CREATE TABLE sessions (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT        NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ
);

CREATE INDEX sessions_user_idx   ON sessions (user_id);
CREATE INDEX sessions_expiry_idx ON sessions (expires_at);

-- +goose Down
DROP TABLE IF EXISTS sessions;