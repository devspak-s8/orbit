import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  ApiError,
  cancelWorkflowRun,
  getWorkflow,
  getWorkflowRun,
  listWorkflowRuns,
  runWorkflow,
  updateWorkflow,
  validateWorkflow,
  type Workflow,
  type WorkflowRun,
  type WorkflowStep,
} from '../lib/api'
import { WorkflowCanvas } from '../components/WorkflowCanvas'

function WebhookPanel({ workflow }: { workflow: Workflow }) {
  const [copied, setCopied] = useState(false)
  if (!workflow.webhookToken) return null
  const url = `${window.location.origin}/hooks/${workflow.webhookToken}`
  const curl = `curl -X POST -H "Content-Type: application/json" -d '{"hello":"world"}' ${url}`
  return (
    <div className="max-w-3xl rounded-lg border border-neutral-800 bg-neutral-900/50 p-4">
      <h2 className="text-sm font-medium">Webhook URL</h2>
      <p className="mt-1 text-xs text-neutral-500">
        Any JSON POST to this URL fires the workflow. The secret is the URL — treat it like a password.
      </p>
      <div className="mt-3 flex items-center gap-2">
        <code className="flex-1 overflow-x-auto rounded bg-neutral-950 px-3 py-2 font-mono text-xs text-emerald-400">
          {url}
        </code>
        <button
          onClick={() => {
            void navigator.clipboard.writeText(url).then(() => {
              setCopied(true)
              setTimeout(() => setCopied(false), 1500)
            })
          }}
          className="rounded border border-neutral-700 px-3 py-2 text-xs hover:border-neutral-500"
        >
          {copied ? 'Copied!' : 'Copy'}
        </button>
      </div>
      <pre className="mt-3 overflow-x-auto rounded bg-neutral-950 p-3 font-mono text-xs text-neutral-400">
        {curl}
      </pre>
    </div>
  )
}

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
  const [workflow, setWorkflow] = useState<Workflow | null>(null)
  const [selected, setSelected] = useState<{ run: WorkflowRun; steps: WorkflowStep[] } | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [editing, setEditing] = useState(false)
  const [editJSON, setEditJSON] = useState('')
  const [canvasMode, setCanvasMode] = useState(false)

  function load() {
    if (!id || !wid) return
    listWorkflowRuns(id, wid).then(setRuns).catch(showError)
    getWorkflow(id, wid).then(setWorkflow).catch(() => {})
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

  async function cancelRun(runId: string) {
    if (!id || !wid) return
    setError(null)
    try {
      await cancelWorkflowRun(id, wid, runId)
      load()
    } catch (err) {
      showError(err)
    }
  }

  async function saveEdit() {
    if (!id || !wid || !workflow) return
    setError(null)
    try {
      const def = JSON.parse(editJSON)
      const updated = await updateWorkflow(id, wid, { definition: def })
      setWorkflow(updated)
      setEditing(false)
      load()
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

      <div className="flex flex-wrap gap-3">
        <button
          onClick={() => runWorkflow(id!, wid!).then(load).catch(showError)}
          className="rounded bg-neutral-100 px-3 py-1.5 text-sm font-medium text-neutral-900 hover:bg-white"
        >
          Run now
        </button>
        <button
          onClick={() => validateWorkflow(id!, wid!).then((w) => { setWorkflow(w); load(); }).catch(showError)}
          className="rounded border border-neutral-700 px-3 py-1.5 text-sm hover:border-neutral-500"
        >
          Validate
        </button>
        {!editing && !canvasMode && (
          <>
            <button
              onClick={() => {
                setEditJSON(JSON.stringify(workflow?.definition ?? {}, null, 2))
                setEditing(true)
              }}
              className="rounded border border-neutral-700 px-3 py-1.5 text-sm hover:border-neutral-500"
            >
              Edit definition
            </button>
            {workflow?.definition && (
              <button
                onClick={() => setCanvasMode(!canvasMode)}
                className={
                  'rounded border px-3 py-1.5 text-sm ' +
                  (canvasMode
                    ? 'border-emerald-700 bg-emerald-900/30 text-emerald-300'
                    : 'border-neutral-700 hover:border-neutral-500')
                }
              >
                Canvas
              </button>
            )}
          </>
        )}
      </div>

      {editing && (
        <div className="max-w-3xl rounded-lg border border-neutral-800 bg-neutral-900/50 p-4">
          <p className="mb-2 text-xs text-neutral-500">
            Saving returns the workflow to <span className="text-neutral-300">draft</span> — it must be
            re-validated before it runs again. Existing runs are untouched.
          </p>
          <textarea
            rows={12}
            spellCheck={false}
            value={editJSON}
            onChange={(e) => setEditJSON(e.target.value)}
            className="w-full rounded border border-neutral-700 bg-neutral-950 px-3 py-2 font-mono text-xs focus:border-neutral-500 focus:outline-none"
          />
          <div className="mt-2 flex gap-2">
            <button
              onClick={saveEdit}
              className="rounded bg-neutral-100 px-3 py-1.5 text-sm font-medium text-neutral-900 hover:bg-white"
            >
              Save
            </button>
            <button
              onClick={() => setEditing(false)}
              className="rounded border border-neutral-700 px-3 py-1.5 text-sm hover:border-neutral-500"
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      {error && <p className="text-sm text-red-400">{error}</p>}

      {canvasMode && workflow?.definition && (
        <div className="max-w-5xl">
          <WorkflowCanvas
            definition={workflow.definition}
            onSave={async (def) => {
              try {
                const updated = await updateWorkflow(id!, wid!, { definition: def })
                setWorkflow(updated)
              } catch (err) {
                showError(err)
              }
            }}
          />
        </div>
      )}

      {workflow && <WebhookPanel workflow={workflow} />}

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
                <div className="mt-1 flex items-center justify-between text-xs text-neutral-500">
                  <span>
                    {r.triggerType} · {new Date(r.createdAt).toLocaleString()}
                    {r.error && <span className="ml-2 text-red-400">{r.error}</span>}
                  </span>
                  {(r.status === 'queued' || r.status === 'running') && (
                    <span
                      role="button"
                      tabIndex={0}
                      onClick={(e) => {
                        e.stopPropagation()
                        cancelRun(r.id)
                      }}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter') {
                          e.stopPropagation()
                          cancelRun(r.id)
                        }
                      }}
                      className="text-red-400 hover:text-red-300"
                    >
                      Cancel run
                    </span>
                  )}
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