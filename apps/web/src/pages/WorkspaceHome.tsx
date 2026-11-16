import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  ApiError,
  createInvite,
  disableModule,
  enableModule,
  getWorkspace,
  installModule,
  listMembers,
  listWorkspaceModules,
  uninstallModule,
  type Member,
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

function MembersSection({ workspaceId, isOwner }: { workspaceId: string; isOwner: boolean }) {
  const [members, setMembers] = useState<Member[] | null>(null)
  const [inviteUrl, setInviteUrl] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    listMembers(workspaceId).then(setMembers).catch(() => {})
  }, [workspaceId])

  async function handleInvite() {
    setError(null)
    try {
      const invite = await createInvite(workspaceId)
      setInviteUrl(`${window.location.origin}/join/${invite.code}`)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not create invite')
    }
  }

  return (
    <div>
      <div className="mb-3 flex items-center justify-between">
        <h2 className="text-lg font-medium">Members</h2>
        {isOwner && (
          <button
            onClick={handleInvite}
            className="rounded border border-neutral-700 px-3 py-1.5 text-sm hover:border-neutral-500"
          >
            Invite people
          </button>
        )}
      </div>
      {error && <p className="mb-2 text-sm text-red-400">{error}</p>}
      {inviteUrl && (
        <div className="mb-3 flex items-center gap-2 rounded border border-emerald-800/60 bg-neutral-900/60 p-2">
          <code className="flex-1 overflow-x-auto font-mono text-xs text-emerald-400">{inviteUrl}</code>
          <button
            onClick={() => {
              void navigator.clipboard.writeText(inviteUrl).then(() => {
                setCopied(true)
                setTimeout(() => setCopied(false), 1500)
              })
            }}
            className="rounded border border-neutral-700 px-2 py-1 text-xs hover:border-neutral-500"
          >
            {copied ? 'Copied!' : 'Copy'}
          </button>
        </div>
      )}
      {members === null ? (
        <p className="text-sm text-neutral-500">Loading members…</p>
      ) : (
        <ul className="max-w-3xl divide-y divide-neutral-800 rounded-lg border border-neutral-800">
          {members.map((m) => (
            <li key={m.userId} className="flex items-center justify-between px-4 py-2 text-sm">
              <div>
                <div>{m.displayName || m.email}</div>
                <div className="text-xs text-neutral-500">{m.email}</div>
              </div>
              <span
                className={
                  m.role === 'owner'
                    ? 'rounded-full border border-emerald-700 px-2 py-0.5 text-xs text-emerald-400'
                    : 'rounded-full border border-neutral-700 px-2 py-0.5 text-xs text-neutral-400'
                }
              >
                {m.role}
              </span>
            </li>
          ))}
        </ul>
      )}
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

      <MembersSection workspaceId={workspace.id} isOwner={workspace.role === 'owner'} />
    </div>
  )
}