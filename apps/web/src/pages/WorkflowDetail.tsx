import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  ApiError,
  getWorkflowRun,
  listWorkflowRuns,
  runWorkflow,
  validateWorkflow,
  type WorkflowRun,
  type WorkflowStep,
} from '../lib/api'

function statusColor(status: string): string {
  switch (status) {
    case 'completed':
      return 'text-emerald-400'
    case 'failed':
      return 'text-red-400'
    case 'running':
    case 'queued':
      return 'text-amber-400'
    default:
      return 'text-neutral-400'
  }
}

export default function WorkflowDetail() {
  const { id, wid } = useParams<{ id: string; wid: string }>()
  const [runs, setRuns] = useState<WorkflowRun[] | null>(null)
  const [selected, setSelected] = useState<{ run: WorkflowRun; steps: WorkflowStep[] } | null>(null)
  const [error, setError] = useState<string | null>(null)

  function load() {
    if (!id || !wid) return
    listWorkflowRuns(id, wid).then(setRuns).catch(showError)
  }

  function showError(err: unknown) {
    setError(err instanceof ApiError ? err.message : 'Something went wrong')
  }

  useEffect(load, [id, wid])

  async function openRun(rid: string) {
    if (!id || !wid) return
    try {
      setSelected(await getWorkflowRun(id, wid, rid))
    } catch (err) {
      showError(err)
    }
  }

  return (
    <div className="space-y-6">
      <div>
        <Link to={`/workspaces/${id}/workflows`} className="text-sm text-neutral-400 hover:text-neutral-200">
          ← Workflows
        </Link>
        <h1 className="mt-2 text-2xl font-semibold">Execution history</h1>
      </div>

      <div className="flex gap-3">
        <button
          onClick={() => runWorkflow(id!, wid!).then(load).catch(showError)}
          className="rounded bg-neutral-100 px-3 py-1.5 text-sm font-medium text-neutral-900 hover:bg-white"
        >
          Run now
        </button>
        <button
          onClick={() => validateWorkflow(id!, wid!).then(load).catch(showError)}
          className="rounded border border-neutral-700 px-3 py-1.5 text-sm hover:border-neutral-500"
        >
          Validate
        </button>
      </div>

      {error && <p className="text-sm text-red-400">{error}</p>}

      {runs === null ? (
        <p className="text-sm text-neutral-500">Loading runs…</p>
      ) : runs.length === 0 ? (
        <p className="text-sm text-neutral-500">No runs yet.</p>
      ) : (
        <ul className="max-w-3xl space-y-2">
          {runs.map((r) => (
            <li key={r.id}>
              <button
                onClick={() => openRun(r.id)}
                className={
                  'w-full rounded-lg border border-neutral-800 bg-neutral-900/50 px-4 py-3 text-left hover:border-neutral-600 ' +
                  (selected?.run.id === r.id ? 'border-neutral-600' : '')
                }
              >
                <div className="flex items-center justify-between text-sm">
                  <span className="font-mono text-xs text-neutral-500">{r.id.slice(0, 8)}</span>
                  <span className={statusColor(r.status)}>{r.status}</span>
                </div>
                <div className="mt-1 text-xs text-neutral-500">
                  {r.triggerType} · {new Date(r.createdAt).toLocaleString()}
                  {r.error && <span className="ml-2 text-red-400">{r.error}</span>}
                </div>
              </button>
            </li>
          ))}
        </ul>
      )}

      {selected && (
        <div className="max-w-3xl space-y-3">
          <h2 className="text-lg font-medium">
            Run <span className="font-mono text-sm text-neutral-500">{selected.run.id.slice(0, 8)}</span>
          </h2>
          {selected.steps.map((st) => (
            <div key={st.seq} className="rounded-lg border border-neutral-800 p-4 text-sm">
              <div className="flex items-center justify-between">
                <span className="font-medium">
                  {st.seq}. {st.nodeId}{' '}
                  <span className="text-xs text-neutral-500">({st.nodeType})</span>
                </span>
                <span className={statusColor(st.status)}>{st.status}</span>
              </div>
              {st.error && <p className="mt-2 text-red-400">{st.error}</p>}
              {Object.keys(st.output).length > 0 && (
                <pre className="mt-2 max-h-40 overflow-auto rounded bg-neutral-950 p-2 font-mono text-xs text-neutral-400">
                  {JSON.stringify(st.output, null, 2)}
                </pre>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}