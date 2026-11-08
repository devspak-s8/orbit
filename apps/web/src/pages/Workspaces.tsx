import { useEffect, useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { ApiError, createWorkspace, listWorkspaces, type Workspace } from '../lib/api'

export default function Workspaces() {
  const [workspaces, setWorkspaces] = useState<Workspace[] | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [name, setName] = useState('')
  const [createError, setCreateError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    listWorkspaces()
      .then(setWorkspaces)
      .catch((err) => setLoadError(err instanceof ApiError ? err.message : 'Failed to load workspaces'))
  }, [])

  async function handleCreate(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setCreateError(null)
    try {
      const ws = await createWorkspace(name)
      setWorkspaces((prev) => [ws, ...(prev ?? [])])
      setName('')
    } catch (err) {
      setCreateError(err instanceof ApiError ? err.message : 'Something went wrong')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold">Workspaces</h1>
        <p className="mt-1 text-sm text-neutral-400">
          Each workspace is a self-contained environment: its own modules, data, and members.
        </p>
      </div>

      <form onSubmit={handleCreate} className="flex max-w-lg items-end gap-3">
        <label className="block flex-1 text-sm">
          New workspace
          <input
            type="text"
            required
            maxLength={80}
            placeholder="e.g. Design Team"
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="mt-1 w-full rounded border border-neutral-700 bg-neutral-950 px-3 py-2 text-sm focus:border-neutral-500 focus:outline-none"
          />
        </label>
        <button
          type="submit"
          disabled={busy}
          className="rounded bg-neutral-100 px-4 py-2 text-sm font-medium text-neutral-900 hover:bg-white disabled:opacity-50"
        >
          {busy ? 'Creating…' : 'Create'}
        </button>
      </form>
      {createError && <p className="text-sm text-red-400">{createError}</p>}

      {loadError && <p className="text-sm text-red-400">{loadError}</p>}

      {workspaces === null ? (
        <p className="text-sm text-neutral-500">Loading workspaces…</p>
      ) : workspaces.length === 0 ? (
        <p className="text-sm text-neutral-500">
          No workspaces yet — create your first one above.
        </p>
      ) : (
        <ul className="max-w-2xl space-y-2">
          {workspaces.map((ws) => (
            <li key={ws.id}>
              <Link
                to={`/workspaces/${ws.id}`}
                className="flex items-center justify-between rounded-lg border border-neutral-800 bg-neutral-900/50 px-4 py-3 hover:border-neutral-600"
              >
                <div>
                  <div className="font-medium">{ws.name}</div>
                  <div className="text-xs text-neutral-500">{ws.slug}</div>
                </div>
                <span className="rounded-full border border-neutral-700 px-2 py-0.5 text-xs text-neutral-400">
                  {ws.role}
                </span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}