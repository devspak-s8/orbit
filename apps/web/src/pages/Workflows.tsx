import { useEffect, useState, type FormEvent } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  ApiError,
  createWorkflow,
  listWorkflows,
  runWorkflow,
  validateWorkflow,
  type Workflow,
  type WorkflowDefinition,
} from '../lib/api'

const exampleDefinition: WorkflowDefinition = {
  trigger: { type: 'event', eventType: 'demo.record.created' },
  nodes: [
    {
      id: 'notify',
      type: 'http',
      config: {
        method: 'GET',
        url: 'http://localhost:8080/v1/ping',
      },
    },
    { id: 'done', type: 'log', config: { message: 'workflow finished' } },
  ],
  edges: [
    { from: 'trigger', to: 'notify' },
    { from: 'notify', to: 'done' },
  ],
}

export default function Workflows() {
  const { id } = useParams<{ id: string }>()
  const [workflows, setWorkflows] = useState<Workflow[] | null>(null)
  const [name, setName] = useState('')
  const [definition, setDefinition] = useState(JSON.stringify(exampleDefinition, null, 2))
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  function load() {
    if (!id) return
    listWorkflows(id).then(setWorkflows).catch(showError)
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
      const def = JSON.parse(definition) as WorkflowDefinition
      await createWorkflow(id, name, def)
      setName('')
      load()
    } catch (err) {
      showError(err)
    } finally {
      setBusy(false)
    }
  }

  async function handleAction(fn: () => Promise<unknown>) {
    setError(null)
    try {
      await fn()
      load()
    } catch (err) {
      showError(err)
    }
  }

  return (
    <div className="space-y-6">
      <div>
        <Link to={`/workspaces/${id}`} className="text-sm text-neutral-400 hover:text-neutral-200">
          ← Workspace
        </Link>
        <h1 className="mt-2 text-2xl font-semibold">Workflows</h1>
        <p className="mt-1 max-w-prose text-sm text-neutral-400">
          Workflows connect events and actions. A workflow must be validated before it can run —
          drafts are never executed.
        </p>
      </div>

      <form onSubmit={handleCreate} className="max-w-3xl space-y-3">
        <label className="block text-sm">
          Name
          <input
            type="text"
            required
            maxLength={120}
            placeholder="e.g. Notify on new record"
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="mt-1 w-full rounded border border-neutral-700 bg-neutral-950 px-3 py-2 text-sm focus:border-neutral-500 focus:outline-none"
          />
        </label>
        <label className="block text-sm">
          Definition (JSON)
          <textarea
            rows={12}
            spellCheck={false}
            value={definition}
            onChange={(e) => setDefinition(e.target.value)}
            className="mt-1 w-full rounded border border-neutral-700 bg-neutral-950 px-3 py-2 font-mono text-xs focus:border-neutral-500 focus:outline-none"
          />
        </label>
        {error && <p className="text-sm text-red-400">{error}</p>}
        <button
          type="submit"
          disabled={busy}
          className="rounded bg-neutral-100 px-4 py-2 text-sm font-medium text-neutral-900 hover:bg-white disabled:opacity-50"
        >
          {busy ? 'Creating…' : 'Create draft'}
        </button>
      </form>

      {workflows === null ? (
        <p className="text-sm text-neutral-500">Loading workflows…</p>
      ) : workflows.length === 0 ? (
        <p className="text-sm text-neutral-500">No workflows yet — create one above.</p>
      ) : (
        <ul className="max-w-3xl space-y-2">
          {workflows.map((wf) => (
            <li key={wf.id} className="rounded-lg border border-neutral-800 bg-neutral-900/50 p-4">
              <div className="flex items-start justify-between gap-4">
                <div>
                  <Link
                    to={`/workspaces/${id}/workflows/${wf.id}`}
                    className="font-medium hover:underline"
                  >
                    {wf.name}
                  </Link>
                  <div className="mt-1 text-xs text-neutral-500">
                    v{wf.currentVersion} · created {new Date(wf.createdAt).toLocaleString()}
                  </div>
                </div>
                <div className="flex shrink-0 items-center gap-2 text-sm">
                  <span
                    className={
                      'rounded-full border px-2 py-0.5 text-xs ' +
                      (wf.status === 'ready'
                        ? 'border-emerald-700 text-emerald-400'
                        : 'border-neutral-700 text-neutral-400')
                    }
                  >
                    {wf.status}
                  </span>
                  {wf.status === 'draft' && (
                    <button
                      onClick={() => handleAction(() => validateWorkflow(id!, wf.id))}
                      className="rounded border border-neutral-700 px-3 py-1.5 hover:border-neutral-500"
                    >
                      Validate
                    </button>
                  )}
                  {wf.status === 'ready' && (
                    <button
                      onClick={() => handleAction(() => runWorkflow(id!, wf.id))}
                      className="rounded bg-neutral-100 px-3 py-1.5 font-medium text-neutral-900 hover:bg-white"
                    >
                      Run
                    </button>
                  )}
                </div>
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}