import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  ApiError,
  disableModule,
  enableModule,
  getWorkspace,
  installModule,
  listWorkspaceModules,
  uninstallModule,
  type Workspace,
  type WorkspaceModule,
} from '../lib/api'

function ModuleCard({
  workspaceId,
  module,
  isOwner,
  onChanged,
}: {
  workspaceId: string
  module: WorkspaceModule
  isOwner: boolean
  onChanged: () => void
}) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function run(fn: () => Promise<unknown>) {
    setBusy(true)
    setError(null)
    try {
      await fn()
      onChanged()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Something went wrong')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="rounded-lg border border-neutral-800 bg-neutral-900/50 p-4">
      <div className="flex items-start justify-between gap-4">
        <div>
          <div className="font-medium">{module.name}</div>
          <div className="text-xs text-neutral-500">
            {module.id} · v{module.version}
          </div>
          <p className="mt-2 text-sm text-neutral-400">{module.description}</p>
          {module.events && module.events.length > 0 && (
            <p className="mt-2 text-xs text-neutral-500">
              emits: {module.events.join(', ')}
            </p>
          )}
        </div>
        <div className="flex shrink-0 items-center gap-2 text-sm">
          {!module.installed && isOwner && (
            <button
              disabled={busy}
              onClick={() => run(() => installModule(workspaceId, module.id))}
              className="rounded border border-neutral-700 px-3 py-1.5 hover:border-neutral-500 disabled:opacity-50"
            >
              Install
            </button>
          )}
          {module.installed && isOwner && (
            <>
              {module.enabled ? (
                <button
                  disabled={busy}
                  onClick={() => run(() => disableModule(workspaceId, module.id))}
                  className="rounded border border-neutral-700 px-3 py-1.5 hover:border-neutral-500 disabled:opacity-50"
                >
                  Disable
                </button>
              ) : (
                <button
                  disabled={busy}
                  onClick={() => run(() => enableModule(workspaceId, module.id))}
                  className="rounded border border-neutral-700 px-3 py-1.5 hover:border-neutral-500 disabled:opacity-50"
                >
                  Enable
                </button>
              )}
              <button
                disabled={busy}
                onClick={() => run(() => uninstallModule(workspaceId, module.id))}
                className="rounded px-3 py-1.5 text-neutral-500 hover:text-red-400 disabled:opacity-50"
              >
                Uninstall
              </button>
            </>
          )}
          {module.installed && module.enabled && (
            <Link
              to={`/workspaces/${workspaceId}/modules/${module.id}`}
              className="rounded bg-neutral-100 px-3 py-1.5 font-medium text-neutral-900 hover:bg-white"
            >
              Open
            </Link>
          )}
          {module.installed && !module.enabled && !isOwner && (
            <span className="text-xs text-neutral-600">disabled</span>
          )}
        </div>
      </div>
      {error && <p className="mt-2 text-xs text-red-400">{error}</p>}
    </div>
  )
}

export default function WorkspaceHome() {
  const { id } = useParams<{ id: string }>()
  const [workspace, setWorkspace] = useState<Workspace | null>(null)
  const [modules, setModules] = useState<WorkspaceModule[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  function load() {
    if (!id) return
    getWorkspace(id).then(setWorkspace).catch(showError)
    listWorkspaceModules(id).then(setModules).catch(showError)
  }

  function showError(err: unknown) {
    setError(err instanceof ApiError ? err.message : 'Failed to load')
  }

  useEffect(load, [id])

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
        <div className="mt-3 flex gap-3 text-sm">
          <Link
            to={`/workspaces/${workspace.id}/workflows`}
            className="rounded border border-neutral-700 px-3 py-1.5 hover:border-neutral-500"
          >
            Workflows →
          </Link>
        </div>
      </div>

      <div>
        <h2 className="mb-3 text-lg font-medium">Modules</h2>
        {modules === null ? (
          <p className="text-sm text-neutral-500">Loading modules…</p>
        ) : (
          <div className="max-w-3xl space-y-3">
            {modules.map((m) => (
              <ModuleCard
                key={m.id}
                workspaceId={workspace.id}
                module={m}
                isOwner={workspace.role === 'owner'}
                onChanged={load}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  )
}