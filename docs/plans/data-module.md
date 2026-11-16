# Data Module (v0.2.0-beta) — Implementation Plan

**Milestone:** beta opener
**Module id:** `orbit.data`
**Replaces:** the Airtable/NocoDB slice — tables, records, CSV, public forms
**The headline loop:** *public form submission → record created → event → workflow → email* — the entire "replace a stack of SaaS" pitch in one chain
**Status:** planned — no code yet

---

## 1. Scope

### In

- **Tables**: workspace-owned, user-defined columns (text, number, date,
  select, checkbox), column add/rename, table rename/archive
- **Records**: CRUD, inline grid editing, filter (column = value) and sort,
  pagination
- **CSV**: import (create/update by first column? no — beta: import =
  append rows) and export per table
- **Public forms**: one form per table, secret URL (like webhooks) with the
  columns toggled visible; submissions create records and publish
  `data.record.created`
- **Events**: `table.created`, `record.created` (payload: tableId, recordId,
  data) — feeding the existing engine with zero new trigger code

### Out (documented, deliberately)

- Column delete (destructive schema change — needs a data-migration story)
- Relations between tables, lookups, formulas
- Multiple views per table, saved filters
- Per-table permissions (module-level membership is the beta boundary)
- Real-time collaborative editing

## 2. Data model (module-owned schema, migration 00008)

```sql
CREATE SCHEMA module_data;

module_data.tables (
    id UUID PK, workspace_id UUID → workspaces, name TEXT,
    form_token TEXT UNIQUE NULL,          -- public form URL secret
    created_by UUID, created_at, updated_at, archived_at
)

module_data.columns (
    id UUID PK, table_id UUID → tables, name TEXT,
    type TEXT CHECK (text|number|date|select|checkbox),
    options JSONB DEFAULT '[]',           -- select choices
    position INT, created_at
)

module_data.records (
    id UUID PK, table_id UUID → tables, workspace_id UUID,
    data JSONB NOT NULL DEFAULT '{}',     -- {columnId: value}
    source TEXT NOT NULL DEFAULT 'ui',    -- 'ui' | 'form' | 'csv'
    created_by UUID NULL, created_at, updated_at
)
```

Values are keyed by column id in JSONB — schema changes never break rows.

## 3. API

Module-gated like all modules:

```
GET/POST   /v1/workspaces/{id}/tables
GET/PATCH  /v1/workspaces/{id}/tables/{tid}          (rename, archive)
POST       /v1/workspaces/{id}/tables/{tid}/columns   (add column)
PATCH      /v1/workspaces/{id}/columns/{cid}          (rename)
GET/POST   /v1/workspaces/{id}/tables/{tid}/records   (?filter=&sort=&page=)
GET/PATCH/DELETE /v1/workspaces/{id}/records/{rid}
POST       /v1/workspaces/{id}/tables/{tid}/import    (CSV text)
GET        /v1/workspaces/{id}/tables/{tid}/export     (CSV)
POST       /v1/workspaces/{id}/tables/{tid}/form      (mint public form token)
```

Plus the **public, unauthenticated** route (same pattern as webhooks):
`POST /forms/{token}` — any JSON object → validated against columns →
record insert + `data.record.created` event, transactionally.

## 4. Events & the headline loop

`record.created` payload: `{tableId, recordId, source, data}` — workflows
subscribe with the existing event trigger and substitute `{{payload.data.FieldName}}`.
The release demo: a public "Contact us" form → record → workflow → email.

## 5. UI

- **Tables page** (module route): list + create, per-table grid view:
  columns as headers, records as rows, inline cell editing (click to edit,
  Enter commits), filter/sort controls, CSV import (paste) + export link
- **Form builder (beta-honest)**: mint a form → toggle which columns appear
  → the public URL with copy button (reuse the webhook panel pattern)
- **Public form page**: `/forms/{token}` — renders outside the auth wall,
  submits to the public endpoint, shows "Thank you" — this is what makes
  ORBIT feel like a product to a stranger

## 6. Manifest

`orbit.data` v0.3.0, permissions `workspace.read, events.write`, events
`[table.created, record.created]`, install/enable through the existing gallery.

## 7. Implementation order

1. Migration 00008 + manifest + service (tables/columns) + tests
2. Records CRUD + filter/sort/pagination + tests
3. Events on record creation; headline-loop integration test
   (form → record → workflow run completed)
4. CSV import/export + tests
5. Public form endpoint + tests (token semantics like webhooks: 404 no-leak)
6. UI: grid, CSV, form builder, public form page
7. Live e2e of the full chain; README/roadmap; tag `v0.4.0-beta`

## 8. Acceptance criteria

1. Create a table with mixed column types; add records through the grid
2. CSV import appends rows; export round-trips them
3. A public form submits → record created with `source=form` → event →
   a subscribed workflow completes — verified live
4. Unknown form token → 404; disabled module → module routes vanish
5. Cross-workspace isolation holds; suite green

## 9. Risks

| Risk | Mitigation |
|---|---|
| Grid UX rabbit hole | Inline click-edit only; no virtualization, no cell types beyond inputs |
| CSV parsing edge cases | Quote/escape handling in a small focused parser with table-driven tests |
| JSONB schema drift | Values keyed by column id; unknown keys preserved on write |
| Public form abuse | Rate limit + 1 MB cap, same posture as webhooks |
