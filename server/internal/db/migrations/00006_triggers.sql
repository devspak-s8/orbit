-- +goose Up
-- Alpha 3: webhook trigger tokens and durable schedule state, both owned
-- by the workflow row. No new tables: tokens are workflow-scoped and the
-- schedule cursor lives where the scheduler reads it.
ALTER TABLE workflows
  ADD COLUMN webhook_token TEXT UNIQUE,
  ADD COLUMN next_due_at   TIMESTAMPTZ;

-- +goose Down
ALTER TABLE workflows
  DROP COLUMN IF EXISTS webhook_token,
  DROP COLUMN IF EXISTS next_due_at;