import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { ApiError, getWorkspace, type Workspace } from '../lib/api'

export default function WorkspaceHome() {
  const { id } = useParams<{ id: string }>()
  const [workspace, setWorkspace] = useState<Workspace | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!id) return
    getWorkspace(id)
      .then(setWorkspace)
      .catch((err) => setError(err instanceof ApiError ? err.message : 'Failed to load workspace'))
  }, [id])

  if (error) {
    return (
      <div className="space-y-4">
        <p className="text-sm text-red-400">{error}</p>
        <Link to="/workspaces" className="text-sm text-neutral-400 underline">
          Back to workspaces
        </Link>
      </div>
    )
  }

  if (!workspace) {
    return <p className="text-sm text-neutral-500">Loading workspace…</p>
  }

  return (
    <div className="space-y-6">
      <div>
        <Link to="/workspaces" className="text-sm text-neutral-400 hover:text-neutral-200">
          ← Workspaces
        </Link>
        <h1 className="mt-2 text-2xl font-semibold">{workspace.name}</h1>
        <p className="mt-1 text-sm text-neutral-400">
          Slug <span className="text-neutral-200">{workspace.slug}</span> · your role{' '}
          <span className="text-neutral-200">{workspace.role}</span>
        </p>
      </div>

      <div className="rounded-lg border border-dashed border-neutral-800 p-8 text-sm text-neutral-500">
        Modules arrive in the next milestone — the first is the Tasks module.
        This is where installed modules will appear.
      </div>
    </div>
  )
}