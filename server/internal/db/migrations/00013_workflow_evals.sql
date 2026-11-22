-- +goose Up
-- Workflow evals (PRD quality infrastructure): replay recorded payloads as
-- dry-runs with assertions. Evals are data — the payload and the expected
-- outcomes — while the simulator itself is code (docs/plans/
-- v1.2.0-evals-healing-desktop.md).

CREATE TABLE workflow_evals (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID       NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    workflow_id UUID        NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    name        TEXT        NOT NULL,
    payload     JSONB       NOT NULL DEFAULT '{}',
    assertions  JSONB       NOT NULL DEFAULT '[]',
    created_by  UUID        REFERENCES users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX workflow_evals_workflow_idx ON workflow_evals (workflow_id, created_at DESC);

CREATE TABLE workflow_eval_runs (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    eval_id       UUID        NOT NULL REFERENCES workflow_evals(id) ON DELETE CASCADE,
    workspace_id  UUID        NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    passed        BOOLEAN     NOT NULL,
    results       JSONB       NOT NULL DEFAULT '[]',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX workflow_eval_runs_eval_idx ON workflow_eval_runs (eval_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS workflow_eval_runs;
DROP TABLE IF EXISTS workflow_evals;
