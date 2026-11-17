# Feature Expansion — Broadening the Existing Modules

**Milestone:** v0.5.0-beta (post-Data)
**Scope:** deepen Work Management, Automation, Data, and the platform core — no new modules
**Status:** planned — no code yet

---

## 1. Work Management: task detail, comments, labels

The board is thin — tasks have no home. This round gives them one.

- **Task detail page** (`/work/:pid/task/:tid`): full description editor, all fields,
  activity feed (from the audit-style history), delete — replaces the cramped drawer
- **Comments** (`module_work.comments`): per-task, member-only, ordered; UI thread in
  the detail page; `task.commented` event (feeds automation + notifications)
- **Labels**: per-project colored labels, multi-assign to tasks, board card chips;
  filter the board by label
- **Board filter bar**: by assignee, label, priority, overdue-only

## 2. Automation: branching, editing, presets

- **Branch node** (`"type": "branch"`): config carries `condition` —
  `{"field": "{{payload.score}}", "op": ">", "value": 50}` with ops `>, >=, <, <=, ==, !=`;
  validator allows the chain to fork into a yes-edge and no-edge (bounded: one branch
  node per workflow for beta). The engine already executes linear chains; a branch is
  a chain-selector at one step
- **Workflow editing**: `PATCH /workflows/{wid}` replaces the draft definition
  (validated; re-validation required before it can run again — ready workflows lock
  until re-validated). UI: edit-in-place in the JSON editor
- **Connector presets**: the HTTP node's UI gains preset buttons (Slack/Discord/Telegram
  webhook layouts) — server unchanged; the API grows a small presets doc
- **Run cancellation**: `POST /runs/{rid}/cancel` marks queued/running runs cancelled;
  worker checks status between steps

## 3. Data: column edit, record detail, form builder

- **Column rename** (`PATCH .../columns/{cid}`) and **column delete with data wipe**
  (explicit confirm in UI; values removed from every record JSONB in one UPDATE)
- **Record detail page**: key-value editor for a single record — for wide tables
  where the grid is cramped
- **Form builder (real)**: `GET /forms/{token}/shape` public endpoint returns the
  table's column spec (name/type/options only); the public form renders from it
  — replacing the hardcoded name/email/message trio
- **Record count + pagination** on the grid (limit/offset)

## 4. Platform core

- **API tokens**: `POST /v1/tokens` (member) → hashed token; `Authorization: Bearer`
  accepted by the auth middleware next to cookies — unlocks CLI/script usage
- **Workspace settings page**: rename workspace, danger zone (delete workspace —
  owner only, cascades everything)
- **List pagination**: workspaces, tasks, records, runs grow `?limit=&before=` cursors
  where row counts warrant it (runs first: they grow forever)

## 5. Schema (migration 00009)

```sql
CREATE TABLE module_work.comments (
    id UUID PK, task_id UUID → tasks, workspace_id UUID,
    body TEXT NOT NULL, author_id UUID → users, created_at
);

CREATE TABLE module_work.labels (
    id UUID PK, project_id UUID → projects, name TEXT, color TEXT
);
CREATE TABLE module_work.task_labels (task_id, label_id, PK both);

ALTER TABLE workflows ADD COLUMN edited_at TIMESTAMPTZ;  -- definition updates

CREATE TABLE api_tokens (
    id UUID PK, user_id UUID → users, name TEXT,
    token_hash TEXT UNIQUE, created_at, last_used_at, revoked_at
);
```

## 6. Implementation order

1. Migration 00009 + API tokens (core) + tests
2. Task detail page + comments + labels + board filters (work)
3. Branch node + validator + engine + tests; workflow editing; run cancel
4. Column edit/delete, record detail, form shape endpoint, pagination (data)
5. Workspace settings; UI for all of the above
6. Full suite; live e2e (branch workflow on a webhook payload); tag `v0.5.0-beta`

## 7. Acceptance criteria

1. Bearer token from `POST /v1/tokens` authenticates API calls; revocation works
2. Task detail shows comments; commenting publishes `task.commented` and notifies
3. Labels render on cards and filter the board
4. A branch workflow routes a webhook payload down two different chains in tests
5. A ready workflow can be edited but stays locked until re-validated
6. Public forms render from the table's real columns (no more hardcoded trio)
7. Runs list paginates; suite green

## 8. Risks

| Risk | Mitigation |
|---|---|
| Branch scope creep into a full expression engine | One condition, one branch node, six ops — written here |
| Column delete wipes data silently | Confirm dialog + the event log records the action |
| API token leakage | Hashed at rest, shown once, revocable, last_used tracking |
| Editing breaks running workflows | Definition immutable while ready; edit → revalidate → run |
