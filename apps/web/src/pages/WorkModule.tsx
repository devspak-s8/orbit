import { useEffect, useState, type FormEvent } from 'react'
import { Link, useParams } from 'react-router-dom'
import { ApiError, createProject, listProjects, type Project } from '../lib/api'

export default function WorkModule() {
  const { id } = useParams<{ id: string }>()
  const [projects, setProjects] = useState<Project[] | null>(null)
  const [name, setName] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  function load() {
    if (!id) return
    listProjects(id).then(setProjects).catch(showError)
  }

  function showError(err: unknown) {
    setError(err instanceof ApiError ? err.message : 'Something went wrong')
  }

  useEffect(load, [id])

  async function handleCreate(e: FormEvent) {
    e.preventDefault()
    if (!id) return
    setBusy(true)
    setError(null)
    try {
      await createProject(id, name, '')
      setName('')
      load()
    } catch (err) {
      showError(err)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="space-y-6">
      <div>
        <Link to={`/workspaces/${id}`} className="text-sm text-neutral-400 hover:text-neutral-200">
          ← Workspace
        </Link>
        <h1 className="mt-2 text-2xl font-semibold">Work Management</h1>
        <p className="mt-1 max-w-prose text-sm text-neutral-400">
          Projects, tasks, and boards. Lifecycle events (created, completed, assigned) publish
          to the platform event log — so workflows can react to your work.
        </p>
      </div>

      <form onSubmit={handleCreate} className="flex max-w-lg items-end gap-3">
        <label className="block flex-1 text-sm">
          New project
          <input
            type="text"
            required
            maxLength={120}
            placeholder="e.g. Launch Website"
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
      {error && <p className="text-sm text-red-400">{error}</p>}

      {projects === null ? (
        <p className="text-sm text-neutral-500">Loading projects…</p>
      ) : projects.length === 0 ? (
        <p className="text-sm text-neutral-500">No projects yet — create your first one above.</p>
      ) : (
        <ul className="max-w-3xl space-y-2">
          {projects.map((p) => (
            <li key={p.id}>
              <Link
                to={`/workspaces/${id}/work/${p.id}`}
                className="block rounded-lg border border-neutral-800 bg-neutral-900/50 px-4 py-3 hover:border-neutral-600"
              >
                <div className="font-medium">{p.name}</div>
                <div className="mt-1 flex gap-3 text-xs text-neutral-500">
                  <span>{p.taskCounts.todo} to do</span>
                  <span>{p.taskCounts.inProgress} in progress</span>
                  <span>{p.taskCounts.done} done</span>
                </div>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}