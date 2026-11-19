-- +goose Up
-- API tokens: Bearer credentials for CLIs and scripts. Only the SHA-256
-- digest is stored; the plaintext is shown exactly once at creation.
CREATE TABLE api_tokens (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         TEXT        NOT NULL,
    token_hash   TEXT        NOT NULL UNIQUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ,
    revoked_at   TIMESTAMPTZ
);

CREATE INDEX api_tokens_user_idx ON api_tokens (user_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS api_tokens;