-- +goose Up
-- Data module (orbit.data): user-defined tables, typed columns, and
-- records. The module owns everything under module_data (PRD 8.5).
-- Record values live in JSONB keyed by column id, so column renames and
-- additions never break existing rows.

CREATE SCHEMA module_data;

CREATE TABLE module_data.tables (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID        NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name         TEXT        NOT NULL,
    form_token   TEXT        UNIQUE,
    created_by   UUID        REFERENCES users(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    archived_at  TIMESTAMPTZ
);

CREATE INDEX module_data_tables_ws_idx ON module_data.tables (workspace_id, created_at DESC);

CREATE TABLE module_data.columns (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    table_id   UUID        NOT NULL REFERENCES module_data.tables(id) ON DELETE CASCADE,
    name       TEXT        NOT NULL,
    type       TEXT        NOT NULL CHECK (type IN ('text', 'number', 'date', 'select', 'checkbox')),
    options    JSONB       NOT NULL DEFAULT '[]',  -- select choices
    position   INT         NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX module_data_columns_table_idx ON module_data.columns (table_id, position);

CREATE TABLE module_data.records (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    table_id     UUID        NOT NULL REFERENCES module_data.tables(id) ON DELETE CASCADE,
    workspace_id UUID        NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    data         JSONB       NOT NULL DEFAULT '{}',  -- {columnId: value}
    source       TEXT        NOT NULL DEFAULT 'ui' CHECK (source IN ('ui', 'form', 'csv')),
    created_by   UUID        REFERENCES users(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX module_data_records_table_idx ON module_data.records (table_id, created_at DESC);
CREATE INDEX module_data_records_ws_idx ON module_data.records (workspace_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS module_data.records;
DROP TABLE IF EXISTS module_data.columns;
DROP TABLE IF EXISTS module_data.tables;
DROP SCHEMA IF EXISTS module_data;