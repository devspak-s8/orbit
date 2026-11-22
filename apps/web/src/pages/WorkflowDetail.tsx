import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  ApiError,
  cancelWorkflowRun,
  createWorkflowEval,
  deleteWorkflowEval,
  getWorkflow,
  getWorkflowRun,
  listWorkflowEvals,
  listWorkflowRuns,
  retryWorkflowRun,
  runWorkflow,
  runWorkflowEval,
  updateWorkflow,
  validateWorkflow,
  type EvalAssertion,
  type Workflow,
  type WorkflowEval,
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

function EvalsPanel({
  workspaceId,
  workflowId,
  onError,
}: {
  workspaceId: string
  workflowId: string
  onError: (msg: string) => void
}) {
  const [evals, setEvals] = useState<WorkflowEval[] | null>(null)
  const [name, setName] = useState('')
  const [payload, setPayload] = useState('{}')
  const [assertions, setAssertions] = useState('[]')
  const [busy, setBusy] = useState(false)

  function load() {
    listWorkflowEvals(workspaceId, workflowId)
      .then(setEvals)
      .catch(() => {})
  }
  useEffect(load, [workspaceId, workflowId])

  async function create() {
    onError('')
    setBusy(true)
    try {
      await createWorkflowEval(workspaceId, workflowId, {
        name,
        payload: JSON.parse(payload),
        assertions: JSON.parse(assertions) as EvalAssertion[],
      })
      setName('')
      setPayload('{}')
      setAssertions('[]')
      load()
    } catch (err) {
      onError(err instanceof ApiError ? err.message : 'Invalid eval — check the JSON')
    } finally {
      setBusy(false)
    }
  }

  async function run(evalId: string) {
    onError('')
    try {
      await runWorkflowEval(workspaceId, workflowId, evalId)
      load()
    } catch (err) {
      onError(err instanceof ApiError ? err.message : 'Eval run failed')
    }
  }

  return (
    <div className="max-w-3xl space-y-4 rounded-lg border border-neutral-800 bg-neutral-900/50 p-4">
      <div>
        <h2 className="text-sm font-medium">Evals</h2>
        <p className="mt-1 text-xs text-neutral-500">
          Dry-run replays: the workflow is simulated against a payload — no requests are sent, no
          emails go out. Assertions check what each node <em>would</em> do.
        </p>
      </div>

      {evals === null ? (
        <p className="text-sm text-neutral-500">Loading evals…</p>
      ) : evals.length === 0 ? (
        <p className="text-sm text-neutral-500">No evals yet.</p>
      ) : (
        <ul className="space-y-2">
          {evals.map((e) => (
            <li key={e.id} className="rounded border border-neutral-800 p-3 text-sm">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <span className="font-medium">{e.name}</span>
                <span className="flex items-center gap-3">
                  {e.lastRun ? (
                    <span className={e.lastRun.passed ? 'text-emerald-400' : 'text-red-400'}>
                      {e.lastRun.passed ? 'passing' : 'failing'}
                    </span>
                  ) : (
                    <span className="text-neutral-500">never run</span>
                  )}
                  <button onClick={() => run(e.id)} className="text-xs text-neutral-300 hover:text-white">
                    Run eval
                  </button>
                  <button
                    onClick={() => deleteWorkflowEval(workspaceId, workflowId, e.id).then(load).catch(() => {})}
                    className="text-xs text-red-400 hover:text-red-300"
                  >
                    Delete
                  </button>
                </span>
              </div>
              {e.lastRun && !e.lastRun.passed && (
                <ul className="mt-2 space-y-1 text-xs text-neutral-500">
                  {e.lastRun.results.map((r, i) => (
                    <li key={i}>
                      <span className={r.passed ? 'text-emerald-400' : 'text-red-400'}>
                        {r.passed ? '✓' : '✗'}
                      </span>{' '}
                      {r.assertion.nodeId}.{r.assertion.property} {r.assertion.op}{' '}
                      {String(r.assertion.value)}
                      {!r.passed && r.reason && <span> — {r.reason}</span>}
                    </li>
                  ))}
                </ul>
              )}
              <pre className="mt-2 max-h-32 overflow-auto rounded bg-neutral-950 p-2 font-mono text-[11px] text-neutral-400">
                {JSON.stringify({ payload: e.payload, assertions: e.assertions }, null, 2)}
              </pre>
            </li>
          ))}
        </ul>
      )}

      <div className="space-y-2 border-t border-neutral-800 pt-3">
        <div className="flex items-center justify-between">
          <h3 className="text-xs font-semibold uppercase tracking-wider text-neutral-500">New eval</h3>
          <p className="text-[10px] text-neutral-600">
            Assertions: {'[{"nodeId":"notify","property":"request.body.text","op":"==","value":"hi"}]'}
          </p>
        </div>
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Eval name, e.g. happy path"
          className="w-full rounded border border-neutral-700 bg-neutral-950 px-3 py-1.5 text-sm focus:border-neutral-500 focus:outline-none"
        />
        <textarea
          rows={3}
          spellCheck={false}
          value={payload}
          onChange={(e) => setPayload(e.target.value)}
          placeholder='Payload JSON, e.g. {"title":"ship it"}'
          className="w-full rounded border border-neutral-700 bg-neutral-950 px-3 py-2 font-mono text-xs focus:border-neutral-500 focus:outline-none"
        />
        <textarea
          rows={3}
          spellCheck={false}
          value={assertions}
          onChange={(e) => setAssertions(e.target.value)}
          placeholder="Assertions JSON"
          className="w-full rounded border border-neutral-700 bg-neutral-950 px-3 py-2 font-mono text-xs focus:border-neutral-500 focus:outline-none"
        />
        <button
          onClick={create}
          disabled={busy || !name}
          className="rounded bg-neutral-100 px-3 py-1.5 text-sm font-medium text-neutral-900 hover:bg-white disabled:opacity-50"
        >
          {busy ? 'Creating…' : 'Create eval'}
        </button>
      </div>
    </div>
  )
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
  const [evalsVersion, setEvalsVersion] = useState(0)

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

  async function retry(run: WorkflowRun) {
    if (!id || !wid) return
    setError(null)
    try {
      await retryWorkflowRun(id, wid, run.id)
      load()
    } catch (err) {
      showError(err)
    }
  }

  async function captureEval(run: WorkflowRun) {
    if (!id || !wid) return
    setError(null)
    try {
      await createWorkflowEval(id, wid, {
        name: `from run ${run.id.slice(0, 8)}`,
        assertions: [],
        fromRunId: run.id,
      })
      setEvalsVersion((v) => v + 1)
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
                // Rethrow so the canvas keeps its dirty state — a failed
                // save must not silently drop the unsaved-edit indication.
                throw err
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
                  <span className="flex items-center gap-3">
                    {(r.status === 'failed' || r.status === 'cancelled') && (
                      <span
                        role="button"
                        tabIndex={0}
                        onClick={(e) => {
                          e.stopPropagation()
                          retry(r)
                        }}
                        onKeyDown={(e) => {
                          if (e.key === 'Enter') {
                            e.stopPropagation()
                            retry(r)
                          }
                        }}
                        className="text-emerald-400 hover:text-emerald-300"
                      >
                        Retry
                      </span>
                    )}
                    <span
                      role="button"
                      tabIndex={0}
                      onClick={(e) => {
                        e.stopPropagation()
                        captureEval(r)
                      }}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter') {
                          e.stopPropagation()
                          captureEval(r)
                        }
                      }}
                      className="text-neutral-400 hover:text-neutral-200"
                      title="Save this run's payload as a dry-run eval"
                    >
                      Capture as eval
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
                  </span>
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

      {id && wid && <EvalsPanel key={evalsVersion} workspaceId={id} workflowId={wid} onError={setError} />}
    </div>
  )
}