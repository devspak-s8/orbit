# Work Management Completion — Implementation Plan

**Scope:** close every table-stakes gap identified in the 2026 competitive research
**Status:** in progress

## Missing features (from research + review data)

| Feature | Evidence | Priority |
|---|---|---|
| Subtasks | Every tool has them; Trello's checklist-without-assignees is a complaint | 1 |
| Task dependencies | Basecamp's #1 complaint ("no Gantt, no dependencies") | 1 |
| Time tracking | ClickUp differentiates (native, free plan); Basecamp gates it at $99/mo | 1 |
| List view | Only board view exists; 16.2% of complaints are "missing capability" | 2 |
| Calendar view | Due-date visualization is table stakes | 2 |
| Timeline (Gantt-lite) | The most-requested missing view across every review study | 2 |
| Workload view | #1 gap per PM community ("tools manage today, lack forecasting") | 2 |

## Schema (migration 00011)

```sql
module_work.subtasks (id, task_id, workspace_id, title, done, position, created_by, created_at)
module_work.dependencies (task_id, blocks_task_id, PK both)  -- "task_id is blocked by blocks_task_id"
module_work.time_entries (id, task_id, workspace_id, user_id, minutes, note, started_at, ended_at, created_at)
```

## API

- POST/GET/DELETE /tasks/{tid}/subtasks; PATCH /subtasks/{sid} (title, done)
- POST/DELETE /tasks/{tid}/dependencies {blocksTaskId}; GET returns blockers + blocked
- POST /tasks/{tid}/time/start; POST /tasks/{tid}/time/stop; POST /tasks/{tid}/time {minutes, note};
  GET /tasks/{tid}/time; DELETE /time/{eid}
- Task payloads carry: subtaskCounts (total/done), isBlocked, timeMinutes
- task.completed clears active timers; when a task's last blocker completes,
  publish task.unblocked

## Views (web)

- Board (existing) | List | Calendar | Timeline — a view switcher on the project
- List: sortable table
- Calendar: month grid, tasks on due dates
- Timeline: horizontal bars (created→due), grouped by assignee
- Task detail: subtasks checklist, dependency add/remove, timer + manual time log

## Acceptance

1. Subtasks with completion ratio on cards and detail page
2. Dependencies with cycle rejection; completing a blocker unblocks and fires the event
3. Timers and manual entries with per-task totals
4. All four views render the same task set
5. Suite green