import { useEffect, useState } from 'react'
import { Link, Navigate, Route, Routes, useLocation } from 'react-router-dom'
import { logout as apiLogout, me as fetchMe, type User } from './lib/api'
import Login from './pages/Login'
import Signup from './pages/Signup'
import Workspaces from './pages/Workspaces'
import WorkspaceHome from './pages/WorkspaceHome'
import ModuleDemo from './pages/ModuleDemo'
import Workflows from './pages/Workflows'
import WorkflowDetail from './pages/WorkflowDetail'

function Home({ user }: { user: User }) {
  return (
    <div className="space-y-4">
      <h1 className="text-2xl font-semibold">
        Welcome, {user.displayName || user.email}
      </h1>
      <p className="max-w-prose text-neutral-400">
        The universal digital operations platform. This alpha build contains
        identity, workspaces, and the module system shell — more modules are
        arriving over the coming weeks.
      </p>
      <p className="text-sm text-neutral-500">Signed in as {user.email}</p>
    </div>
  )
}

function Shell({
  me,
  onLogout,
  children,
}: {
  me: User
  onLogout: () => void
  children: React.ReactNode
}) {
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
        <div className="mt-auto space-y-2 text-xs">
          <div className="truncate text-neutral-400">{me.email}</div>
          <button
            onClick={onLogout}
            className="rounded px-2 py-1.5 text-left text-neutral-500 hover:bg-neutral-900 hover:text-neutral-100"
          >
            Sign out
          </button>
          <div className="text-neutral-600">v0.1.0-alpha</div>
        </div>
      </aside>
      <main className="flex-1 overflow-auto p-8">{children}</main>
    </div>
  )
}

export default function App() {
  // undefined = session check in flight, null = signed out, User = signed in.
  const [me, setMe] = useState<User | null | undefined>(undefined)

  useEffect(() => {
    fetchMe().then(setMe)
  }, [])

  const handleLogout = () => {
    void apiLogout()
    setMe(null)
  }

  const location = useLocation()

  if (me === undefined) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-neutral-950 text-neutral-400">
        Loading…
      </div>
    )
  }

  if (me === null) {
    // Public routes only when signed out.
    return (
      <Routes>
        <Route path="/login" element={<Login onSuccess={setMe} />} />
        <Route path="/signup" element={<Signup onSuccess={setMe} />} />
        <Route path="*" element={<Navigate to="/login" replace state={{ from: location }} />} />
      </Routes>
    )
  }

  return (
    <Shell me={me} onLogout={handleLogout}>
      <Routes>
        <Route path="/" element={<Home user={me} />} />
        <Route path="/workspaces" element={<Workspaces />} />
        <Route path="/workspaces/:id" element={<WorkspaceHome />} />
        <Route path="/workspaces/:id/workflows" element={<Workflows />} />
        <Route path="/workspaces/:id/workflows/:wid" element={<WorkflowDetail />} />
        <Route path="/workspaces/:id/modules/:moduleId" element={<ModuleDemo />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </Shell>
  )
}