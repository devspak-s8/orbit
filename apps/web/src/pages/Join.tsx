import { useEffect, useState } from 'react'
import { Link, Navigate, useParams } from 'react-router-dom'
import { ApiError, redeemInvite } from '../lib/api'

export default function Join() {
  const { code } = useParams<{ code: string }>()
  const [state, setState] = useState<'trying' | 'unauthenticated' | 'joined' | 'error'>('trying')
  const [workspaceId, setWorkspaceId] = useState<string | null>(null)
  const [message, setMessage] = useState('')

  useEffect(() => {
    if (!code) return
    redeemInvite(code)
      .then((ws) => {
        setWorkspaceId(ws.id)
        setState('joined')
      })
      .catch((err) => {
        if (err instanceof ApiError && err.code === 'unauthorized') {
          setState('unauthenticated')
        } else {
          setMessage(err instanceof ApiError ? err.message : 'Something went wrong')
          setState('error')
        }
      })
  }, [code])

  if (state === 'joined' && workspaceId) {
    return <Navigate to={`/workspaces/${workspaceId}`} replace />
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-neutral-950 text-neutral-100">
      <div className="w-full max-w-sm rounded-lg border border-neutral-800 bg-neutral-900/60 p-8 text-center">
        <div className="mb-6 text-lg font-semibold tracking-widest">ORBIT</div>
        {state === 'trying' && <p className="text-sm text-neutral-400">Joining workspace…</p>}
        {state === 'unauthenticated' && (
          <div className="space-y-4">
            <p className="text-sm text-neutral-400">
              You need an account before you can join. Sign in, then reopen this link.
            </p>
            <Link
              to="/login"
              className="inline-block rounded bg-neutral-100 px-4 py-2 text-sm font-medium text-neutral-900 hover:bg-white"
            >
              Sign in
            </Link>
          </div>
        )}
        {state === 'error' && (
          <div className="space-y-4">
            <p className="text-sm text-red-400">{message}</p>
            <Link to="/" className="text-sm text-neutral-400 underline">
              Go home
            </Link>
          </div>
        )}
      </div>
    </div>
  )
}