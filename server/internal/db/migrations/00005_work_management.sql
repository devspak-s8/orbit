-- +goose Up
-- Work Management module data (orbit.work). Module-owned schema per PRD 8.5:
-- the module owns everything under module_work, including its migrations.
CREATE SCHEMA module_work;

CREATE TABLE module_work.projects (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID        NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name         TEXT        NOT NULL,
    description  TEXT        NOT NULL DEFAULT '',
    created_by   UUID        REFERENCES users(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    archived_at  TIMESTAMPTZ
);

CREATE INDEX module_work_projects_ws_idx ON module_work.projects (workspace_id, created_at DESC);

CREATE TABLE module_work.tasks (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id   UUID        NOT NULL REFERENCES module_work.projects(id) ON DELETE CASCADE,
    workspace_id UUID        NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    title        TEXT        NOT NULL,
    description  TEXT        NOT NULL DEFAULT '',
    status       TEXT        NOT NULL DEFAULT 'todo'
                             CHECK (status IN ('todo', 'in_progress', 'done')),
    priority     TEXT        NOT NULL DEFAULT 'medium'
                             CHECK (priority IN ('low', 'medium', 'high', 'urgent')),
    assignee_id  UUID        REFERENCES users(id) ON DELETE SET NULL,
    due_date     DATE,
    position     INT         NOT NULL DEFAULT 0,
    completed_at TIMESTAMPTZ,
    created_by   UUID        REFERENCES users(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX module_work_tasks_project_idx ON module_work.tasks (project_id, position);
CREATE INDEX module_work_tasks_ws_idx ON module_work.tasks (workspace_id, created_at DESC);
CREATE INDEX module_work_tasks_assignee_idx ON module_work.tasks (assignee_id) WHERE assignee_id IS NOT NULL;

-- +goose Down
DROP TABLE IF EXISTS module_work.tasks;
DROP TABLE IF EXISTS module_work.projects;
DROP SCHEMA IF EXISTS module_work;