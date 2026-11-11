# Work Management Module — Implementation Plan

**Milestone:** Alpha 2 (the first real product module)
**Module id:** `orbit.work`
**Replaces:** the task-management half of ClickUp/Notion/Asana
**Status:** planned — no code yet

---

## 1. Purpose

Work Management is the first module people install to *do work*, not to evaluate
architecture. It must be usable enough that a small team could run their week on
it — and it must prove the platform promise: **task events trigger workflows
natively** (§19 criterion 10, but for real this time).

The demo moment: *create a task, mark it done, watch a workflow fire.*
That single loop is the product pitch in one screen.

## 2. Scope

### In

- Projects (container for tasks)
- Tasks: title, description, status, priority, assignee, due date
- Board view (three columns by status) and list view
- Quick-add and inline status change (keyboard-accessible — no drag-drop yet)
- Assignment to workspace members
- Platform events on task lifecycle (the automation integration)

### Out (explicitly deferred)

- Comments, attachments, subtasks
- Custom fields, task templates, forms
- Timelines/Gantt, calendars
- Notifications (email/digest)
- Drag-and-drop (buttons/select first — accessible alternative by default, PRD §14)
- Per-project custom status lists (fixed 3-status board for alpha)

## 3. Data model (owned by the module, own schema)

```sql
CREATE SCHEMA module_work;

module_work.projects (
    id UUID PK, workspace_id UUID → workspaces,
    name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '',
    created_by UUID → users, created_at, updated_at,
    archived_at TIMESTAMPTZ NULL
)

module_work.tasks (
    id UUID PK, project_id UUID → projects, workspace_id UUID,
    title TEXT NOT NULL, description TEXT NOT NULL DEFAULT '',
    status TEXT CHECK (status IN ('todo','in_progress','done')) DEFAULT 'todo',
    priority TEXT CHECK (priority IN ('low','medium','high','urgent')) DEFAULT 'medium',
    assignee_id UUID NULL → users, due_date DATE NULL,
    position INT NOT NULL DEFAULT 0,           -- ordering within a status column
    completed_at TIMESTAMPTZ NULL,              -- set when status → done
    created_by UUID → users, created_at, updated_at
)
```

Every table is workspace-scoped; all queries filter by `workspace_id` at the
data layer (same isolation discipline as workspaces/modules).

## 4. Platform events (the integration that matters)

The module declares in its manifest and publishes, transactionally with the
change:

| Event | When | Payload |
|---|---|---|
| `project.created` | project created | projectId, name |
| `task.created` | task created | taskId, projectId, title |
| `task.completed` | status → done | taskId, projectId, title, assigneeId |
| `task.reopened` | done → not-done | taskId, projectId, title |
| `task.assigned` | assignee set/changed | taskId, assigneeId, title |

Deliberate choice: **no `task.updated` firehose.** Editing a description
shouldn't spam the event log. Only lifecycle transitions emit.

These events make workflows like *"when `task.completed` → notify (HTTP)"*
work with zero new engine code — the relay, queue, and history already exist.

## 5. API

First-party module, clean routes (module-scoped, gated on install+enable like
the demo module):

```
GET    /v1/workspaces/{id}/projects                list (member)
POST   /v1/workspaces/{id}/projects                create (member)
GET    /v1/workspaces/{id}/projects/{pid}          detail + tasks (member)
PATCH  /v1/workspaces/{id}/projects/{pid}          rename/archive (member)
GET    /v1/workspaces/{id}/projects/{pid}/tasks     list, ?status= filter
POST   /v1/workspaces/{id}/projects/{pid}/tasks     create
GET    /v1/workspaces/{id}/tasks/{tid}              detail
PATCH  /v1/workspaces/{id}/tasks/{tid}              update (title/desc/priority/
                                                    assignee/due_date)
POST   /v1/workspaces/{id}/tasks/{tid}/status       change status (the event
                                                    source of truth)
DELETE /v1/workspaces/{id}/tasks/{tid}              delete
GET    /v1/workspaces/{id}/members                  list members (core, needed
                                                    for the assignee picker)
```

`GET /members` is a small core addition (memberships already exist; there is
just no read endpoint yet).

## 6. Manifest

```json
{
  "id": "orbit.work",
  "name": "Work Management",
  "version": "0.2.0",
  "apiVersion": "1",
  "description": "Projects and tasks: boards, assignments, due dates — with native automation events.",
  "permissions": ["workspace.read", "events.write"],
  "dependencies": ["orbit.core"],
  "events": ["project.created", "task.created", "task.completed", "task.reopened", "task.assigned"],
  "entrypoints": { "frontend": "frontend" }
}
```

Installs through the existing module gallery — no new lifecycle code needed.

## 7. UI

- **Workspace home**: Work Management appears in the module gallery; "Open"
  goes to `/workspaces/:id/work`
- **Projects page**: list + create
- **Project view**: board (Todo / In Progress / Done) with task cards;
  quick-add input per column; list view toggle
- **Task card**: title, priority chip, due date (overdue in red), assignee
  initial; click → detail drawer: edit fields, status select
- **Members endpoint** feeds the assignee picker
- Consistent with the existing dark shell; no new design system needed yet

## 8. Implementation order

1. Migration 00005 (`module_work` schema) + `orbit.work` manifest + router wiring
2. Projects service + endpoints + tests
3. Tasks service + endpoints + lifecycle events + tests
4. `GET /members` endpoint + tests
5. UI: projects page, board, task detail
6. **Integration proof**: workflow subscribed to `task.completed` fires in the
   browser end to end — plus the same as an automated test
7. README/roadmap update; tag the milestone

## 9. Acceptance criteria

1. A user installs Work Management through the gallery like any module
2. Creates a project, adds tasks, assigns a member, sets due dates
3. Board shows tasks by status; moving a task is keyboard-accessible
4. Marking a task done writes `task.completed` transactionally
5. A workflow on `task.completed` executes and appears in history — the demo
   loop, verified in the browser
6. Cross-workspace isolation holds (second user sees nothing)
7. All tests green; every event has a payload contract test

## 10. Risks

| Risk | Mitigation |
|---|---|
| Scope creep toward "real project management" | §2 list is the contract; everything else is a future milestone |
| Event spam | No `task.updated`; lifecycle events only |
| Board UX rabbit hole | No drag-drop; status controls first |
| Assignment needs membership UI | Minimal `GET /members`; no invites/roles UI yet (that's a core milestone later) |
