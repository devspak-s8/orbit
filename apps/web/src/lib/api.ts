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