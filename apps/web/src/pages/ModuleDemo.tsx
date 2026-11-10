import { useEffect, useState, type FormEvent } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  addDemoRecord,
  ApiError,
  listDemoRecords,
  listWorkspaceEvents,
  type DemoRecord,
  type WorkspaceEvent,
} from '../lib/api'

export default function ModuleDemo() {
  const { id } = useParams<{ id: string }>()
  const [records, setRecords] = useState<DemoRecord[] | null>(null)
  const [events, setEvents] = useState<WorkspaceEvent[]>([])
  const [title, setTitle] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  function load() {
    if (!id) return
    listDemoRecords(id).then(setRecords).catch(showError)
    listWorkspaceEvents(id).then(setEvents).catch(() => {})
  }

  function showError(err: unknown) {
    setError(err instanceof ApiError ? err.message : 'Failed to load')
  }

  useEffect(load, [id])

  async function handleAdd(e: FormEvent) {
    e.preventDefault()
    if (!id) return
    setBusy(true)
    setError(null)
    try {
      await addDemoRecord(id, title)
      setTitle('')
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Something went wrong')
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
        <h1 className="mt-2 text-2xl font-semibold">Demo Module</h1>
        <p className="mt-1 max-w-prose text-sm text-neutral-400">
          This module exists to prove the pipeline: it was installed through the module
          system, its UI is registered by the manifest, and every record it creates
          publishes a <span className="text-neutral-200">demo.record.created</span> event
          on the platform event log.
        </p>
      </div>

      <div className="grid max-w-4xl grid-cols-1 gap-6 md:grid-cols-2">
        <div>
          <h2 className="mb-3 text-lg font-medium">Records</h2>
          <form onSubmit={handleAdd} className="mb-4 flex gap-2">
            <input
              type="text"
              required
              placeholder="New record title"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              className="flex-1 rounded border border-neutral-700 bg-neutral-950 px-3 py-2 text-sm focus:border-neutral-500 focus:outline-none"
            />
            <button
              type="submit"
              disabled={busy}
              className="rounded bg-neutral-100 px-3 py-2 text-sm font-medium text-neutral-900 hover:bg-white disabled:opacity-50"
            >
              {busy ? 'Adding…' : 'Add'}
            </button>
          </form>
          {error && <p className="mb-3 text-sm text-red-400">{error}</p>}
          {records === null ? (
            <p className="text-sm text-neutral-500">Loading records…</p>
          ) : records.length === 0 ? (
            <p className="text-sm text-neutral-500">No records yet.</p>
          ) : (
            <ul className="space-y-2">
              {records.map((r) => (
                <li key={r.id} className="rounded border border-neutral-800 px-3 py-2 text-sm">
                  {r.title}
                  <span className="ml-2 text-xs text-neutral-600">
                    {new Date(r.createdAt).toLocaleString()}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </div>

        <div>
          <h2 className="mb-3 text-lg font-medium">Workspace events</h2>
          {events.length === 0 ? (
            <p className="text-sm text-neutral-500">No events yet.</p>
          ) : (
            <ul className="space-y-2">
              {events.map((e, i) => (
                <li key={i} className="rounded border border-neutral-800 px-3 py-2 text-sm">
                  <span className="font-mono text-xs text-emerald-400">{e.type}</span>
                  <span className="ml-2 text-xs text-neutral-600">
                    {new Date(e.createdAt).toLocaleTimeString()}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>
    </div>
  )
}