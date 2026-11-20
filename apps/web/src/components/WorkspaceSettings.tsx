import { useState } from 'react'
import { ApiError, deleteWorkspace, renameWorkspace, type Workspace } from '../lib/api'

export function WorkspaceSettings({
  workspace,
  onRenamed,
}: {
  workspace: Workspace
  onRenamed: (ws: Workspace) => void
}) {
  const [name, setName] = useState('')
  const [confirmName, setConfirmName] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  async function handleRename() {
    if (!name.trim()) return
    setBusy(true)
    setError(null)
    try {
      const ws = await renameWorkspace(workspace.id, name.trim())
      onRenamed(ws)
      setName('')
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to rename')
    } finally {
      setBusy(false)
    }
  }

  async function handleDelete() {
    if (confirmName !== workspace.name) {
      setError('Type the exact workspace name to confirm deletion')
      return
    }
    setBusy(true)
    setError(null)
    try {
      await deleteWorkspace(workspace.id, confirmName)
      window.location.href = '/workspaces'
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to delete')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div>
      <h2 className="mb-3 text-lg font-medium">Settings</h2>
      {error && <p className="mb-3 text-sm text-red-400">{error}</p>}

      <div className="mb-4 flex items-end gap-3">
        <label className="block flex-1 text-sm">
          Rename workspace
          <input
            type="text"
            maxLength={80}
            placeholder={workspace.name}
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="mt-1 w-full rounded border border-neutral-700 bg-neutral-950 px-3 py-2 text-sm focus:border-neutral-500 focus:outline-none"
          />
        </label>
        <button
          onClick={handleRename}
          disabled={busy || !name.trim()}
          className="rounded border border-neutral-700 px-3 py-2 text-sm hover:border-neutral-500 disabled:opacity-50"
        >
          Rename
        </button>
      </div>

      <div className="rounded-lg border border-red-800/60 bg-red-950/10 p-4">
        <h3 className="text-sm font-medium text-red-300">Danger zone</h3>
        <p className="mt-1 text-xs text-neutral-500">
          Deleting a workspace permanently removes all projects, tasks, data, docs, workflows, and
          members. This cannot be undone.
        </p>
        <div className="mt-3 flex items-end gap-3">
          <label className="block flex-1 text-xs">
            Type "{workspace.name}" to confirm
            <input
              type="text"
              value={confirmName}
              onChange={(e) => setConfirmName(e.target.value)}
              className="mt-1 w-full rounded border border-red-800 bg-neutral-950 px-3 py-2 text-sm focus:border-red-500 focus:outline-none"
            />
          </label>
          <button
            onClick={handleDelete}
            disabled={busy || confirmName !== workspace.name}
            className="rounded bg-red-600 px-3 py-2 text-sm font-medium text-white hover:bg-red-500 disabled:opacity-50"
          >
            Delete workspace
          </button>
        </div>
      </div>
    </div>
  )
}