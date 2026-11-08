import { useEffect, useState } from 'react'
import { Link, Route, Routes } from 'react-router-dom'

type ApiStatus = 'checking' | 'ok' | 'down'

function useApiStatus(): ApiStatus {
  const [status, setStatus] = useState<ApiStatus>('checking')
  useEffect(() => {
    let cancelled = false
    fetch('/v1/ping')
      .then((r) => (cancelled ? undefined : setStatus(r.ok ? 'ok' : 'down')))
      .catch(() => (cancelled ? undefined : setStatus('down')))
    return () => {
      cancelled = true
    }
  }, [])
  return status
}

function StatusBadge({ status }: { status: ApiStatus }) {
  const map = {
    checking: { label: 'checking…', cls: 'text-neutral-400' },
    ok: { label: 'online', cls: 'text-emerald-400' },
    down: { label: 'unreachable', cls: 'text-red-400' },
  } as const
  const { label, cls } = map[status]
  return <span className={cls}>{label}</span>
}

function Home() {
  const status = useApiStatus()
  return (
    <div className="space-y-4">
      <h1 className="text-2xl font-semibold">Welcome to ORBIT</h1>
      <p className="max-w-prose text-neutral-400">
        The universal digital operations platform. This alpha build contains the
        platform shell: workspaces, identity, and the module system are arriving
        over the coming weeks.
      </p>
      <p className="text-sm">
        API status: <StatusBadge status={status} />
      </p>
    </div>
  )
}

export default function App() {
  return (
    <div className="flex h-screen bg-neutral-950 text-neutral-100">
      <aside className="flex w-60 flex-col border-r border-neutral-800 p-4">
        <div className="text-lg font-semibold tracking-widest">ORBIT</div>
        <nav className="mt-8 flex flex-col gap-1 text-sm text-neutral-400">
          <Link to="/" className="rounded px-2 py-1.5 hover:bg-neutral-900 hover:text-neutral-100">
            Home
          </Link>
          <Link to="/workspaces" className="rounded px-2 py-1.5 hover:bg-neutral-900 hover:text-neutral-100">
            Workspaces
          </Link>
        </nav>
        <div className="mt-auto text-xs text-neutral-600">v0.1.0-alpha</div>
      </aside>
      <main className="flex-1 overflow-auto p-8">
        <Routes>
          <Route path="/" element={<Home />} />
          <Route path="/workspaces" element={<div className="text-2xl font-semibold">Workspaces — next milestone</div>} />
        </Routes>
      </main>
    </div>
  )
}