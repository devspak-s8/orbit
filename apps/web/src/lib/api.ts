export interface User {
  id: string
  email: string
  displayName: string
  createdAt: string
}

export interface Workspace {
  id: string
  name: string
  slug: string
  role: string
  createdAt: string
}

export class ApiError extends Error {
  code: string

  constructor(code: string, message: string) {
    super(message)
    this.code = code
  }
}

interface ErrorBody {
  code?: string
  message?: string
}

/**
 * Minimal JSON API client. All requests include credentials so the session
 * cookie travels with them; the Vite dev server proxies /v1 to the Go API.
 */
export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      ...(init?.headers ?? {}),
    },
    ...init,
  })

  if (!res.ok) {
    let body: ErrorBody | null = null
    try {
      body = (await res.json()) as ErrorBody
    } catch {
      // non-JSON error body; fall through to generic message
    }
    throw new ApiError(body?.code ?? 'http_error', body?.message ?? `Request failed (${res.status})`)
  }

  return (await res.json()) as T
}

export async function me(): Promise<User | null> {
  try {
    const res = await api<{ user: User }>('/v1/me')
    return res.user
  } catch (err) {
    if (err instanceof ApiError && err.code === 'unauthorized') return null
    // The API is unreachable — treat as signed out so the UI still renders.
    return null
  }
}

export function login(email: string, password: string): Promise<User> {
  return api<{ user: User }>('/v1/auth/login', {
    method: 'POST',
    body: JSON.stringify({ email, password }),
  }).then((r) => r.user)
}

export function register(email: string, password: string, displayName: string): Promise<User> {
  return api<{ user: User }>('/v1/auth/register', {
    method: 'POST',
    body: JSON.stringify({ email, password, displayName }),
  }).then((r) => r.user)
}

export async function logout(): Promise<void> {
  try {
    await api<{ status: string }>('/v1/auth/logout', { method: 'POST' })
  } catch {
    // Logout is best-effort; the local state is cleared regardless.
  }
}

export function listWorkspaces(): Promise<Workspace[]> {
  return api<{ workspaces: Workspace[] }>('/v1/workspaces').then((r) => r.workspaces)
}

export function createWorkspace(name: string): Promise<Workspace> {
  return api<{ workspace: Workspace }>('/v1/workspaces', {
    method: 'POST',
    body: JSON.stringify({ name }),
  }).then((r) => r.workspace)
}

export function getWorkspace(id: string): Promise<Workspace> {
  return api<{ workspace: Workspace }>(`/v1/workspaces/${id}`).then((r) => r.workspace)
}

export interface WorkspaceModule {
  id: string
  name: string
  version: string
  description: string
  installed: boolean
  enabled: boolean
  frontend?: string
  events?: string[]
}

export interface WorkspaceEvent {
  type: string
  schemaVersion: number
  workspaceId: string
  actorUserId?: string
  correlationId?: string
  payload: Record<string, unknown>
  createdAt: string
}

export interface DemoRecord {
  id: string
  title: string
  createdAt: string
}

export function listWorkspaceModules(workspaceId: string): Promise<WorkspaceModule[]> {
  return api<{ modules: WorkspaceModule[] }>(`/v1/workspaces/${workspaceId}/modules`).then((r) => r.modules)
}

export function installModule(workspaceId: string, moduleId: string): Promise<WorkspaceModule> {
  return api<{ module: WorkspaceModule }>(`/v1/workspaces/${workspaceId}/modules/${moduleId}/install`, {
    method: 'POST',
  }).then((r) => r.module)
}

export function enableModule(workspaceId: string, moduleId: string): Promise<WorkspaceModule> {
  return api<{ module: WorkspaceModule }>(`/v1/workspaces/${workspaceId}/modules/${moduleId}/enable`, {
    method: 'POST',
  }).then((r) => r.module)
}

export function disableModule(workspaceId: string, moduleId: string): Promise<WorkspaceModule> {
  return api<{ module: WorkspaceModule }>(`/v1/workspaces/${workspaceId}/modules/${moduleId}/disable`, {
    method: 'POST',
  }).then((r) => r.module)
}

export function uninstallModule(workspaceId: string, moduleId: string): Promise<void> {
  return api<{ status: string }>(`/v1/workspaces/${workspaceId}/modules/${moduleId}`, {
    method: 'DELETE',
  }).then(() => undefined)
}

export function listWorkspaceEvents(workspaceId: string): Promise<WorkspaceEvent[]> {
  return api<{ events: WorkspaceEvent[] }>(`/v1/workspaces/${workspaceId}/events`).then((r) => r.events)
}

export function listDemoRecords(workspaceId: string): Promise<DemoRecord[]> {
  return api<{ records: DemoRecord[] }>(`/v1/workspaces/${workspaceId}/modules/demo/records`).then((r) => r.records)
}

export function addDemoRecord(workspaceId: string, title: string): Promise<DemoRecord> {
  return api<{ record: DemoRecord }>(`/v1/workspaces/${workspaceId}/modules/demo/records`, {
    method: 'POST',
    body: JSON.stringify({ title }),
  }).then((r) => r.record)
}

export interface Workflow {
  id: string
  workspaceId: string
  name: string
  status: 'draft' | 'ready' | 'archived'
  currentVersion: number
  definition?: WorkflowDefinition
  webhookToken?: string
  nextDueAt?: string
  createdAt: string
  updatedAt: string
}

export interface WorkflowDefinition {
  trigger: {
    type: 'event' | 'manual' | 'webhook' | 'schedule'
    eventType?: string
    intervalSec?: number
  }
  nodes: Array<{
    id: string
    type: 'http' | 'log' | 'email' | 'branch'
    config: {
      method?: string
      url?: string
      headers?: Record<string, string>
      body?: Record<string, unknown>
      message?: string
      to?: string
      subject?: string
      text?: string
      condition?: { field: string; op: string; value: unknown }
      retries?: number
      timeoutMs?: number
    }
  }>
  edges: Array<{ from: string; to: string; label?: string }>
  timeoutMs?: number
}

export interface WorkflowRun {
  id: string
  workflowId: string
  status: 'queued' | 'running' | 'completed' | 'failed' | 'cancelled'
  triggerType: string
  triggerPayload: Record<string, unknown>
  error: string
  startedAt: string | null
  finishedAt: string | null
  createdAt: string
}

export interface WorkflowStep {
  seq: number
  nodeId: string
  nodeType: string
  status: 'pending' | 'running' | 'completed' | 'failed' | 'skipped'
  input: Record<string, unknown>
  output: Record<string, unknown>
  error: string
  startedAt: string | null
  finishedAt: string | null
}

export interface APITokenRecord {
  id: string
  name: string
  createdAt: string
  lastUsedAt: string | null
  revokedAt: string | null
}

export function listAPITokens(): Promise<APITokenRecord[]> {
  return api<{ tokens: APITokenRecord[] }>('/v1/tokens').then((r) => r.tokens)
}

export function createAPIToken(name: string): Promise<{ token: APITokenRecord; secret: string }> {
  return api<{ token: APITokenRecord; secret: string }>('/v1/tokens', {
    method: 'POST',
    body: JSON.stringify({ name }),
  })
}

export function revokeAPIToken(tokenId: string): Promise<void> {
  return api(`/v1/tokens/${tokenId}`, { method: 'DELETE' }).then(() => undefined)
}

export function listWorkflows(workspaceId: string): Promise<Workflow[]> {
  return api<{ workflows: Workflow[] }>(`/v1/workspaces/${workspaceId}/workflows`).then((r) => r.workflows)
}

export function createWorkflow(
  workspaceId: string,
  name: string,
  definition: WorkflowDefinition,
): Promise<Workflow> {
  return api<{ workflow: Workflow }>(`/v1/workspaces/${workspaceId}/workflows`, {
    method: 'POST',
    body: JSON.stringify({ name, definition }),
  }).then((r) => r.workflow)
}

export function validateWorkflow(workspaceId: string, workflowId: string): Promise<Workflow> {
  return api<{ workflow: Workflow }>(`/v1/workspaces/${workspaceId}/workflows/${workflowId}/validate`, {
    method: 'POST',
  }).then((r) => r.workflow)
}

export function getWorkflow(workspaceId: string, workflowId: string): Promise<Workflow> {
  return api<{ workflow: Workflow }>(`/v1/workspaces/${workspaceId}/workflows/${workflowId}`).then(
    (r) => r.workflow,
  )
}

export function runWorkflow(workspaceId: string, workflowId: string): Promise<WorkflowRun> {
  return api<{ run: WorkflowRun }>(`/v1/workspaces/${workspaceId}/workflows/${workflowId}/run`, {
    method: 'POST',
  }).then((r) => r.run)
}

export function listWorkflowRuns(workspaceId: string, workflowId: string): Promise<WorkflowRun[]> {
  return api<{ runs: WorkflowRun[] }>(`/v1/workspaces/${workspaceId}/workflows/${workflowId}/runs`).then((r) => r.runs)
}

export function cancelWorkflowRun(
  workspaceId: string,
  workflowId: string,
  runId: string,
): Promise<void> {
  return api(`/v1/workspaces/${workspaceId}/workflows/${workflowId}/runs/${runId}/cancel`, {
    method: 'POST',
  }).then(() => undefined)
}

export function updateWorkflow(
  workspaceId: string,
  workflowId: string,
  patch: { name?: string; definition?: WorkflowDefinition },
): Promise<Workflow> {
  return api<{ workflow: Workflow }>(`/v1/workspaces/${workspaceId}/workflows/${workflowId}`, {
    method: 'PATCH',
    body: JSON.stringify(patch),
  }).then((r) => r.workflow)
}

export function getWorkflowRun(
  workspaceId: string,
  workflowId: string,
  runId: string,
): Promise<{ run: WorkflowRun; steps: WorkflowStep[] }> {
  return api(`/v1/workspaces/${workspaceId}/workflows/${workflowId}/runs/${runId}`)
}

export interface Member {
  userId: string
  email: string
  displayName: string
  role: string
  createdAt: string
}

export interface TaskCounts {
  todo: number
  inProgress: number
  done: number
}

export interface Project {
  id: string
  workspaceId: string
  name: string
  description: string
  createdBy: string
  taskCounts: TaskCounts
  createdAt: string
  updatedAt: string
}

export type TaskStatus = 'todo' | 'in_progress' | 'done'

export interface Task {
  id: string
  projectId: string
  workspaceId: string
  title: string
  description: string
  status: TaskStatus
  priority: 'low' | 'medium' | 'high' | 'urgent'
  assigneeId: string
  assigneeName: string
  dueDate: string | null
  position: number
  completedAt: string | null
  labels: TaskLabel[]
  createdAt: string
  updatedAt: string
}

export interface TaskLabel {
  id: string
  name: string
  color: string
}

export interface TaskComment {
  id: string
  taskId: string
  body: string
  authorId: string
  authorName: string
  createdAt: string
}

export function listMembers(workspaceId: string): Promise<Member[]> {
  return api<{ members: Member[] }>(`/v1/workspaces/${workspaceId}/members`).then((r) => r.members)
}

export function listProjects(workspaceId: string): Promise<Project[]> {
  return api<{ projects: Project[] }>(`/v1/workspaces/${workspaceId}/projects`).then((r) => r.projects)
}

export function createProject(workspaceId: string, name: string, description: string): Promise<Project> {
  return api<{ project: Project }>(`/v1/workspaces/${workspaceId}/projects`, {
    method: 'POST',
    body: JSON.stringify({ name, description }),
  }).then((r) => r.project)
}

export function getProject(
  workspaceId: string,
  projectId: string,
): Promise<{ project: Project; tasks: Task[] }> {
  return api(`/v1/workspaces/${workspaceId}/projects/${projectId}`)
}

export interface TaskInput {
  title: string
  description?: string
  priority?: string
  assigneeId?: string
  dueDate?: string
}

export function createTask(workspaceId: string, projectId: string, input: TaskInput): Promise<Task> {
  return api<{ task: Task }>(`/v1/workspaces/${workspaceId}/projects/${projectId}/tasks`, {
    method: 'POST',
    body: JSON.stringify(input),
  }).then((r) => r.task)
}

export function getTask(workspaceId: string, taskId: string): Promise<Task> {
  return api<{ task: Task }>(`/v1/workspaces/${workspaceId}/tasks/${taskId}`).then((r) => r.task)
}

export function updateTask(
  workspaceId: string,
  taskId: string,
  patch: Partial<Pick<Task, 'title' | 'description' | 'priority' | 'assigneeId'>> & { dueDate?: string },
): Promise<Task> {
  return api<{ task: Task }>(`/v1/workspaces/${workspaceId}/tasks/${taskId}`, {
    method: 'PATCH',
    body: JSON.stringify(patch),
  }).then((r) => r.task)
}

export function setTaskStatus(workspaceId: string, taskId: string, status: TaskStatus): Promise<Task> {
  return api<{ task: Task }>(`/v1/workspaces/${workspaceId}/tasks/${taskId}/status`, {
    method: 'POST',
    body: JSON.stringify({ status }),
  }).then((r) => r.task)
}

export function deleteTask(workspaceId: string, taskId: string): Promise<void> {
  return api<{ status: string }>(`/v1/workspaces/${workspaceId}/tasks/${taskId}`, {
    method: 'DELETE',
  }).then(() => undefined)
}

export function listLabels(workspaceId: string, projectId: string): Promise<TaskLabel[]> {
  return api<{ labels: TaskLabel[] }>(`/v1/workspaces/${workspaceId}/projects/${projectId}/labels`).then(
    (r) => r.labels,
  )
}

export function createLabel(
  workspaceId: string,
  projectId: string,
  name: string,
  color: string,
): Promise<TaskLabel> {
  return api<{ label: TaskLabel }>(`/v1/workspaces/${workspaceId}/projects/${projectId}/labels`, {
    method: 'POST',
    body: JSON.stringify({ name, color }),
  }).then((r) => r.label)
}

export function setTaskLabels(workspaceId: string, taskId: string, labelIds: string[]): Promise<Task> {
  return api<{ task: Task }>(`/v1/workspaces/${workspaceId}/tasks/${taskId}/labels`, {
    method: 'PUT',
    body: JSON.stringify({ labelIds }),
  }).then((r) => r.task)
}

export function listComments(workspaceId: string, taskId: string): Promise<TaskComment[]> {
  return api<{ comments: TaskComment[] }>(`/v1/workspaces/${workspaceId}/tasks/${taskId}/comments`).then(
    (r) => r.comments,
  )
}

export function addComment(workspaceId: string, taskId: string, body: string): Promise<TaskComment> {
  return api<{ comment: TaskComment }>(`/v1/workspaces/${workspaceId}/tasks/${taskId}/comments`, {
    method: 'POST',
    body: JSON.stringify({ body }),
  }).then((r) => r.comment)
}

export interface Subtask {
  id: string
  taskId: string
  title: string
  done: boolean
  position: number
  createdAt: string
}

export interface TaskBrief {
  id: string
  title: string
  status: string
}

export interface DependencyInfo {
  blockedBy: TaskBrief[]
  blocks: TaskBrief[]
}

export interface TimeEntry {
  id: string
  taskId: string
  userId: string
  userName: string
  minutes: number
  note: string
  startedAt: string
  endedAt: string | null
}

export function listSubtasks(workspaceId: string, taskId: string): Promise<Subtask[]> {
  return api<{ subtasks: Subtask[] }>(`/v1/workspaces/${workspaceId}/tasks/${taskId}/subtasks`).then(
    (r) => r.subtasks,
  )
}

export function createSubtask(workspaceId: string, taskId: string, title: string): Promise<Subtask> {
  return api<{ subtask: Subtask }>(`/v1/workspaces/${workspaceId}/tasks/${taskId}/subtasks`, {
    method: 'POST',
    body: JSON.stringify({ title }),
  }).then((r) => r.subtask)
}

export function updateSubtask(
  workspaceId: string,
  subtaskId: string,
  patch: { title?: string; done?: boolean },
): Promise<Subtask> {
  return api<{ subtask: Subtask }>(`/v1/workspaces/${workspaceId}/subtasks/${subtaskId}`, {
    method: 'PATCH',
    body: JSON.stringify(patch),
  }).then((r) => r.subtask)
}

export function deleteSubtask(workspaceId: string, subtaskId: string): Promise<void> {
  return api(`/v1/workspaces/${workspaceId}/subtasks/${subtaskId}`, { method: 'DELETE' }).then(() => undefined)
}

export function getDependencies(workspaceId: string, taskId: string): Promise<DependencyInfo> {
  return api(`/v1/workspaces/${workspaceId}/tasks/${taskId}/dependencies`)
}

export function addDependency(workspaceId: string, taskId: string, blocksTaskId: string): Promise<DependencyInfo> {
  return api(`/v1/workspaces/${workspaceId}/tasks/${taskId}/dependencies`, {
    method: 'POST',
    body: JSON.stringify({ blocksTaskId }),
  })
}

export function removeDependency(workspaceId: string, taskId: string, blocksTaskId: string): Promise<DependencyInfo> {
  return api(`/v1/workspaces/${workspaceId}/tasks/${taskId}/dependencies`, {
    method: 'DELETE',
    body: JSON.stringify({ blocksTaskId }),
  })
}

export function startTime(workspaceId: string, taskId: string): Promise<TimeEntry> {
  return api<{ entry: TimeEntry }>(`/v1/workspaces/${workspaceId}/tasks/${taskId}/time/start`, {
    method: 'POST',
  }).then((r) => r.entry)
}

export function stopTime(workspaceId: string, taskId: string): Promise<TimeEntry> {
  return api<{ entry: TimeEntry }>(`/v1/workspaces/${workspaceId}/tasks/${taskId}/time/stop`, {
    method: 'POST',
  }).then((r) => r.entry)
}

export function logTime(workspaceId: string, taskId: string, minutes: number, note: string): Promise<TimeEntry> {
  return api<{ entry: TimeEntry }>(`/v1/workspaces/${workspaceId}/tasks/${taskId}/time`, {
    method: 'POST',
    body: JSON.stringify({ minutes, note }),
  }).then((r) => r.entry)
}

export function listTime(workspaceId: string, taskId: string): Promise<TimeEntry[]> {
  return api<{ entries: TimeEntry[] }>(`/v1/workspaces/${workspaceId}/tasks/${taskId}/time`).then(
    (r) => r.entries,
  )
}

export function deleteTimeEntry(workspaceId: string, entryId: string): Promise<void> {
  return api(`/v1/workspaces/${workspaceId}/time/${entryId}`, { method: 'DELETE' }).then(() => undefined)
}

export interface Invite {
  code: string
  workspaceId: string
  expiresAt: string
  createdAt: string
}

export function createInvite(workspaceId: string): Promise<Invite> {
  return api<{ invite: Invite }>(`/v1/workspaces/${workspaceId}/invites`, {
    method: 'POST',
  }).then((r) => r.invite)
}

export function redeemInvite(code: string): Promise<Workspace> {
  return api<{ workspace: Workspace }>(`/v1/invites/${code}/redeem`, {
    method: 'POST',
  }).then((r) => r.workspace)
}

export interface AppNotification {
  id: string
  type: string
  workspaceId: string
  payload: Record<string, unknown>
  readAt: string | null
  createdAt: string
}

export function listNotifications(): Promise<{ notifications: AppNotification[]; unread: number }> {
  return api('/v1/notifications')
}

export function markAllNotificationsRead(): Promise<void> {
  return api<{ status: string }>('/v1/notifications/read-all', { method: 'POST' }).then(() => undefined)
}

export interface DataColumn {
  id: string
  name: string
  type: 'text' | 'number' | 'date' | 'select' | 'checkbox'
  options?: string[]
}

export interface DataTable {
  id: string
  workspaceId: string
  name: string
  columns?: DataColumn[]
  formToken?: string
  createdAt: string
  updatedAt: string
}

export interface DataRecord {
  id: string
  tableId: string
  values: Record<string, unknown>
  source: 'ui' | 'form' | 'csv'
  createdBy: string
  createdAt: string
  updatedAt: string
}

export function listTables(workspaceId: string): Promise<DataTable[]> {
  return api<{ tables: DataTable[] }>(`/v1/workspaces/${workspaceId}/tables`).then((r) => r.tables)
}

export function createTable(
  workspaceId: string,
  name: string,
  columns: Array<{ name: string; type: string; options?: string[] }>,
): Promise<DataTable> {
  return api<{ table: DataTable }>(`/v1/workspaces/${workspaceId}/tables`, {
    method: 'POST',
    body: JSON.stringify({ name, columns }),
  }).then((r) => r.table)
}

export function getTable(workspaceId: string, tableId: string): Promise<DataTable> {
  return api<{ table: DataTable }>(`/v1/workspaces/${workspaceId}/tables/${tableId}`).then((r) => r.table)
}

export function addColumn(
  workspaceId: string,
  tableId: string,
  column: { name: string; type: string; options?: string[] },
): Promise<DataColumn> {
  return api<{ column: DataColumn }>(`/v1/workspaces/${workspaceId}/tables/${tableId}/columns`, {
    method: 'POST',
    body: JSON.stringify(column),
  }).then((r) => r.column)
}

export function listRecords(workspaceId: string, tableId: string): Promise<DataRecord[]> {
  return api<{ records: DataRecord[] }>(`/v1/workspaces/${workspaceId}/tables/${tableId}/records`).then(
    (r) => r.records,
  )
}

export function createRecord(
  workspaceId: string,
  tableId: string,
  values: Record<string, unknown>,
): Promise<DataRecord> {
  return api<{ record: DataRecord }>(`/v1/workspaces/${workspaceId}/tables/${tableId}/records`, {
    method: 'POST',
    body: JSON.stringify({ values }),
  }).then((r) => r.record)
}

export function updateRecord(
  workspaceId: string,
  recordId: string,
  values: Record<string, unknown>,
): Promise<DataRecord> {
  return api<{ record: DataRecord }>(`/v1/workspaces/${workspaceId}/records/${recordId}`, {
    method: 'PATCH',
    body: JSON.stringify({ values }),
  }).then((r) => r.record)
}

export function deleteRecord(workspaceId: string, recordId: string): Promise<void> {
  return api(`/v1/workspaces/${workspaceId}/records/${recordId}`, { method: 'DELETE' }).then(() => undefined)
}

export function importCSV(workspaceId: string, tableId: string, csv: string): Promise<number> {
  return api<{ imported: number }>(`/v1/workspaces/${workspaceId}/tables/${tableId}/import`, {
    method: 'POST',
    body: JSON.stringify({ csv }),
  }).then((r) => r.imported)
}

export function enableForm(workspaceId: string, tableId: string): Promise<DataTable> {
  return api<{ table: DataTable }>(`/v1/workspaces/${workspaceId}/tables/${tableId}/form`, {
    method: 'POST',
  }).then((r) => r.table)
}

export function submitPublicForm(token: string, values: Record<string, unknown>): Promise<DataRecord> {
  return api<{ record: DataRecord }>(`/forms/${token}`, {
    method: 'POST',
    body: JSON.stringify(values),
  }).then((r) => r.record)
}