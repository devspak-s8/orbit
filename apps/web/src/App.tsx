import { useEffect, useState } from 'react'
import { Link, Navigate, Route, Routes, useLocation, useParams } from 'react-router-dom'
import { logout as apiLogout, me as fetchMe, listNotifications, markAllNotificationsRead, type AppNotification, type User } from './lib/api'
import Login from './pages/Login'
import Signup from './pages/Signup'
import Join from './pages/Join'
import Landing from './pages/Landing'
import Features from './pages/Features'
import Deploy from './pages/Deploy'
import About from './pages/About'
import Workspaces from './pages/Workspaces'
import WorkspaceHome from './pages/WorkspaceHome'
import ModuleDemo from './pages/ModuleDemo'
import Workflows from './pages/Workflows'
import WorkflowDetail from './pages/WorkflowDetail'
import WorkModule from './pages/WorkModule'
import ProjectBoard from './pages/ProjectBoard'
import TaskDetail from './pages/TaskDetail'
import DataModule from './pages/DataModule'
import PublicForm from './pages/PublicForm'
import APITokens from './pages/APITokens'

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

function NotificationBell() {
  const [unread, setUnread] = useState(0)
  const [items, setItems] = useState<AppNotification[] | null>(null)
  const [open, setOpen] = useState(false)

  useEffect(() => {
    let cancelled = false
    const poll = () =>
      listNotifications()
        .then((r) => {
          if (cancelled) return
          setUnread(r.unread)
          setItems(r.notifications)
        })
        .catch(() => {})
    poll()
    const timer = setInterval(poll, 30000)
    return () => {
      cancelled = true
      clearInterval(timer)
    }
  }, [])

  async function readAll() {
    await markAllNotificationsRead()
    setUnread(0)
    setItems((prev) => prev?.map((n) => ({ ...n, readAt: n.readAt ?? new Date().toISOString() })) ?? null)
  }

  const describe = (n: AppNotification): string => {
    const title = (n.payload['title'] as string) ?? ''
    switch (n.type) {
      case 'task.assigned':
        return `You were assigned "${title}"`
      case 'task.completed':
        return `"${title}" was completed`
      case 'member.joined':
        return 'Someone joined your workspace'
      default:
        return n.type
    }
  }

  return (
    <div className="relative">
      <button
        onClick={() => setOpen(!open)}
        aria-label={`Notifications (${unread} unread)`}
        className="relative rounded px-2 py-1 text-neutral-400 hover:bg-neutral-900 hover:text-neutral-100"
      >
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
          <path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9" />
          <path d="M13.73 21a2 2 0 0 1-3.46 0" />
        </svg>
        {unread > 0 && (
          <span className="absolute -right-0.5 -top-0.5 flex h-4 min-w-4 items-center justify-center rounded-full bg-emerald-500 px-1 text-[10px] font-semibold text-black">
            {unread > 9 ? '9+' : unread}
          </span>
        )}
      </button>
      {open && (
        <div className="absolute right-0 top-9 z-10 w-80 rounded-lg border border-neutral-800 bg-neutral-900 p-3 shadow-xl">
          <div className="mb-2 flex items-center justify-between">
            <span className="text-sm font-medium">Notifications</span>
            {unread > 0 && (
              <button onClick={readAll} className="text-xs text-neutral-400 hover:text-neutral-100">
                Mark all read
              </button>
            )}
          </div>
          {items === null || items.length === 0 ? (
            <p className="py-2 text-sm text-neutral-500">Nothing yet.</p>
          ) : (
            <ul className="max-h-72 space-y-1 overflow-auto">
              {items.map((n) => (
                <li
                  key={n.id}
                  className={
                    'rounded px-2 py-1.5 text-sm ' +
                    (n.readAt ? 'text-neutral-500' : 'bg-neutral-800/60 text-neutral-200')
                  }
                >
                  {describe(n)}
                  <span className="ml-1 text-xs text-neutral-600">
                    {new Date(n.createdAt).toLocaleDateString()}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
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
          <Link to="/tokens" className="rounded px-2 py-1.5 hover:bg-neutral-900 hover:text-neutral-100">
            API tokens
          </Link>
        </nav>
        <div className="mt-auto space-y-2 text-xs">
          <NotificationBell />
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

function ModuleRoute() {
  const { id, moduleId } = useParams<{ id: string; moduleId: string }>()
  switch (moduleId) {
    case 'orbit.work':
      return <WorkModule />
    case 'orbit.data':
      return <DataModule />
    case 'orbit.demo':
      return <ModuleDemo />
    default:
      return (
        <div className="space-y-4">
          <p className="text-sm text-neutral-500">Unknown or uninstalled module.</p>
          <Link to={`/workspaces/${id}`} className="text-sm text-neutral-400 underline">
            ← Back to workspace
          </Link>
        </div>
      )
  }
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
        <Route path="/" element={<Landing />} />
        <Route path="/features" element={<Features />} />
        <Route path="/deploy" element={<Deploy />} />
        <Route path="/about" element={<About />} />
        <Route path="/login" element={<Login onSuccess={setMe} />} />
        <Route path="/signup" element={<Signup onSuccess={setMe} />} />
        <Route path="/join/:code" element={<Join />} />
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
        <Route path="/workspaces/:id/modules/:moduleId" element={<ModuleRoute />} />
        <Route path="/workspaces/:id/work" element={<WorkModule />} />
        <Route path="/workspaces/:id/work/:pid" element={<ProjectBoard />} />
        <Route path="/workspaces/:id/work/:pid/task/:tid" element={<TaskDetail />} />
        <Route path="/workspaces/:id/data" element={<DataModule />} />
        <Route path="/workspaces/:id/data/:tid" element={<DataModule />} />
        <Route path="/forms/:token" element={<PublicForm />} />
        <Route path="/tokens" element={<APITokens />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </Shell>
  )
}