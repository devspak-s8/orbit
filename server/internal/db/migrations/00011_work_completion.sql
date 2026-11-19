-- +goose Up
-- Work Management completion: subtasks, dependencies, time tracking.

CREATE TABLE module_work.subtasks (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id      UUID        NOT NULL REFERENCES module_work.tasks(id) ON DELETE CASCADE,
    workspace_id UUID        NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    title        TEXT        NOT NULL,
    done         BOOLEAN     NOT NULL DEFAULT false,
    position     INT         NOT NULL DEFAULT 0,
    created_by   UUID        REFERENCES users(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX module_work_subtasks_task_idx ON module_work.subtasks (task_id, position);

-- "task_id is blocked by blocks_task_id": the row's task cannot start until
-- the blocking task completes.
CREATE TABLE module_work.dependencies (
    task_id       UUID NOT NULL REFERENCES module_work.tasks(id) ON DELETE CASCADE,
    blocks_task_id UUID NOT NULL REFERENCES module_work.tasks(id) ON DELETE CASCADE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (task_id, blocks_task_id),
    CHECK (task_id <> blocks_task_id)
);

CREATE INDEX module_work_dependencies_blocks_idx ON module_work.dependencies (blocks_task_id);

CREATE TABLE module_work.time_entries (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id      UUID        NOT NULL REFERENCES module_work.tasks(id) ON DELETE CASCADE,
    workspace_id UUID        NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id      UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    minutes     INT         NOT NULL DEFAULT 0,
    note         TEXT        NOT NULL DEFAULT '',
    started_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at     TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX module_work_time_task_idx ON module_work.time_entries (task_id, created_at DESC);
CREATE INDEX module_work_time_active_idx ON module_work.time_entries (user_id) WHERE ended_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS module_work.time_entries;
DROP TABLE IF EXISTS module_work.dependencies;
DROP TABLE IF EXISTS module_work.subtasks;