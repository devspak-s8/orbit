-- +goose Up
-- Work Management expansion: task comments and project labels.

CREATE TABLE module_work.comments (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id      UUID        NOT NULL REFERENCES module_work.tasks(id) ON DELETE CASCADE,
    workspace_id UUID        NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    body         TEXT        NOT NULL,
    author_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX module_work_comments_task_idx ON module_work.comments (task_id, created_at);

CREATE TABLE module_work.labels (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id   UUID        NOT NULL REFERENCES module_work.projects(id) ON DELETE CASCADE,
    workspace_id UUID        NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name         TEXT        NOT NULL,
    color        TEXT        NOT NULL DEFAULT 'neutral',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, name)
);

CREATE TABLE module_work.task_labels (
    task_id  UUID NOT NULL REFERENCES module_work.tasks(id) ON DELETE CASCADE,
    label_id UUID NOT NULL REFERENCES module_work.labels(id) ON DELETE CASCADE,
    PRIMARY KEY (task_id, label_id)
);

-- +goose Down
DROP TABLE IF EXISTS module_work.task_labels;
DROP TABLE IF EXISTS module_work.labels;
DROP TABLE IF EXISTS module_work.comments;