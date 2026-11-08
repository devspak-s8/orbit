export interface User {
  id: string
  email: string
  displayName: string
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