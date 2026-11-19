import { useEffect, useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import {
  ApiError,
  createAPIToken,
  listAPITokens,
  revokeAPIToken,
  type APITokenRecord,
} from '../lib/api'

/**
 * APITokens manages the caller's Bearer credentials for CLIs and scripts.
 * Secrets are shown exactly once at creation — the server stores only a
 * SHA-256 digest.
 */
export default function APITokens() {
  const [tokens, setTokens] = useState<APITokenRecord[] | null>(null)
  const [name, setName] = useState('')
  const [freshSecret, setFreshSecret] = useState<{ name: string; secret: string } | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  function load() {
    listAPITokens()
      .then(setTokens)
      .catch((err) => setError(err instanceof ApiError ? err.message : 'Failed to load'))
  }

  useEffect(load, [])

  async function handleCreate(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      const { token, secret } = await createAPIToken(name)
      setFreshSecret({ name: token.name, secret })
      setName('')
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Something went wrong')
    } finally {
      setBusy(false)
    }
  }

  async function handleRevoke(id: string) {
    setError(null)
    try {
      await revokeAPIToken(id)
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Something went wrong')
    }
  }

  return (
    <div className="mx-auto max-w-3xl space-y-6">
      <div>
        <Link to="/" className="text-sm text-neutral-400 hover:text-neutral-200">
          ← Home
        </Link>
        <h1 className="mt-2 text-2xl font-semibold">API tokens</h1>
        <p className="mt-1 max-w-prose text-sm text-neutral-400">
          Bearer credentials for scripts and CLIs. Use them with{' '}
          <code className="rounded bg-neutral-900 px-1.5 py-0.5 text-xs text-emerald-400">
            Authorization: Bearer &lt;token&gt;
          </code>{' '}
          — they authenticate everywhere your login does, on every endpoint.
        </p>
      </div>

      {freshSecret && (
        <div className="rounded-xl border border-emerald-800/60 bg-neutral-900/60 p-5">
          <h2 className="text-sm font-semibold text-emerald-300">
            "{freshSecret.name}" created — copy it now
          </h2>
          <p className="mt-1 text-xs text-neutral-500">
            This is the only time the secret is shown. The server stores a one-way digest.
          </p>
          <div className="mt-3 flex items-center gap-2">
            <code className="flex-1 overflow-x-auto rounded bg-neutral-950 px-3 py-2 font-mono text-xs text-emerald-400">
              {freshSecret.secret}
            </code>
            <button
              onClick={() => {
                void navigator.clipboard.writeText(freshSecret.secret)
              }}
              className="rounded border border-neutral-700 px-3 py-2 text-xs hover:border-neutral-500"
            >
              Copy
            </button>
          </div>
        </div>
      )}

      <form onSubmit={handleCreate} className="flex max-w-lg items-end gap-3">
        <label className="block flex-1 text-sm">
          New token
          <input
            type="text"
            required
            maxLength={80}
            placeholder="e.g. deploy-script"
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

      {tokens === null ? (
        <p className="text-sm text-neutral-500">Loading tokens…</p>
      ) : tokens.length === 0 ? (
        <p className="text-sm text-neutral-500">No tokens yet — create one above.</p>
      ) : (
        <ul className="space-y-2">
          {tokens.map((t) => (
            <li
              key={t.id}
              className="flex items-center justify-between rounded-lg border border-neutral-800 bg-neutral-900/50 px-4 py-3"
            >
              <div>
                <div className="text-sm font-medium">{t.name}</div>
                <div className="text-xs text-neutral-500">
                  created {new Date(t.createdAt).toLocaleDateString()}
                  {t.lastUsedAt && ` · last used ${new Date(t.lastUsedAt).toLocaleString()}`}
                  {t.revokedAt && <span className="text-red-400"> · revoked</span>}
                </div>
              </div>
              {!t.revokedAt && (
                <button
                  onClick={() => handleRevoke(t.id)}
                  className="text-xs text-neutral-500 hover:text-red-400"
                >
                  Revoke
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}