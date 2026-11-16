# Work Management Polish — Implementation Plan

**Milestone:** post-Alpha-3 polish
**Scope:** invites/membership UI, in-app notifications, board drag-and-drop
**Status:** planned — no code yet

---

## 1. Invites & membership UI

Today memberships exist (owner/member) but the only way to gain one is a
direct database insert. Alpha-polish adds the real flow:

- **Invite codes**: owner generates an invite (optional role: member;
  owner-only). A `invite_codes` row (code, workspace, role, expires_at,
  created_by) backs a shareable URL: `/join/{code}`
- **Redeem**: signed-in user opens the URL → membership created → lands in
  the workspace. Codes expire (7 days default) and are single-purpose
  enough for alpha (reuse allowed until expiry — documented; one-time codes
  come with the roles milestone)
- **UI**: workspace gains a Members panel: list (already have the endpoint),
  role chips, owner sees "Create invite link" with copy button
- Endpoints: `POST /v1/workspaces/{id}/invites` (owner) → `{code, url,
  expiresAt}`; `POST /v1/invites/{code}/redeem` (any signed-in user);
  membership enforcement throughout, no info leak on bad codes

## 2. In-app notifications

Email exists (workflows); what's missing is *ambient* awareness inside the
app. Core service (PRD §3 Layer 3), consumed by modules transactionally:

- `notifications` table: user_id, workspace_id, type, payload (jsonb),
  read_at, created_at
- **Producers (alpha)**: `task.assigned` → notify the assignee ("X assigned
  you a task"); `task.completed` → notify the task creator; `member.joined`
  → notify the workspace owner. All inserted in the same transaction as the
  change that produced them — same discipline as events
- **Consumers**: bell in the shell with unread count; dropdown with the
  latest 20, "mark all read"; polling every 30s (SSE is D7 — noted)
- Endpoints: `GET /v1/notifications` (mine, newest first, unread count),
  `POST /v1/notifications/read-all`

## 3. Board drag-and-drop

Native HTML5 drag-and-drop (no library) on task cards, column-drop and
position-within-column:

- Drag between columns → the existing status-change path (same events fire)
- Reorder within a column → `position` update (drop index → renumber)
- **The keyboard-accessible move buttons stay** — drag-and-drop is
  additive, never the only path (PRD §14 accessibility)
- Drop targets highlight; touch users fall back to buttons (documented)

## 4. Schema (migration 00007)

```sql
CREATE TABLE invite_codes (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    code         TEXT NOT NULL UNIQUE,
    role         TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('member')),
    created_by   UUID NOT NULL REFERENCES users(id),
    expires_at   TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE notifications (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    type         TEXT NOT NULL,
    payload      JSONB NOT NULL DEFAULT '{}',
    read_at      TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX notifications_user_idx ON notifications (user_id, created_at DESC);
```

## 5. Implementation order

1. Migration + invite service/endpoints + tests
2. Notification service + producers in work module + endpoints + tests
3. Bell UI + members panel UI
4. Drag-and-drop on the board (+ tests via service-level position updates)
5. e2e: invite → join → assign task → bell rings; commit, tag `v0.3.1-alpha`

## 6. Non-goals

Email digests, one-time codes, per-role granular permissions, notification
preferences, SSE push, touch drag-and-drop.

## 7. Acceptance criteria

1. Owner creates invite link; a second account joins via URL without DB access
2. Assigning a task notifies the assignee in-app, transactionally
3. Unread badge updates; mark-all-read works
4. Cards drag between columns; the board remains fully usable by keyboard
5. Suite green; live e2e of the invite → assignment → bell loop
