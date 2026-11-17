import { useEffect, useState, type FormEvent } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import {
  ApiError,
  createTask,
  deleteTask,
  getProject,
  listMembers,
  listLabels,
  setTaskStatus,
  updateTask,
  type Member,
  type Project,
  type Task,
  type TaskLabel,
  type TaskStatus,
} from '../lib/api'

const COLUMNS: { status: TaskStatus; label: string }[] = [
  { status: 'todo', label: 'To Do' },
  { status: 'in_progress', label: 'In Progress' },
  { status: 'done', label: 'Done' },
]

const PRIORITY_COLORS: Record<string, string> = {
  low: 'text-neutral-500',
  medium: 'text-neutral-400',
  high: 'text-amber-400',
  urgent: 'text-red-400',
}

function isOverdue(t: Task): boolean {
  if (!t.dueDate || t.status === 'done') return false
  return t.dueDate < new Date().toISOString().slice(0, 10)
}

const LABEL_STYLES: Record<string, string> = {
  emerald: 'bg-emerald-900/40 text-emerald-300 border-emerald-800/60',
  sky: 'bg-sky-900/40 text-sky-300 border-sky-800/60',
  amber: 'bg-amber-900/40 text-amber-300 border-amber-800/60',
  rose: 'bg-rose-900/40 text-rose-300 border-rose-800/60',
  violet: 'bg-violet-900/40 text-violet-300 border-violet-800/60',
  lime: 'bg-lime-900/40 text-lime-300 border-lime-800/60',
  neutral: 'bg-neutral-800 text-neutral-300 border-neutral-700',
}

function initials(name: string): string {
  return name
    .split(' ')
    .map((w) => w[0])
    .filter(Boolean)
    .slice(0, 2)
    .join('')
    .toUpperCase() || '?'
}

function TaskCard({
  task,
  onOpen,
  onMove,
  onDragStart,
}: {
  task: Task
  onOpen: () => void
  onMove: (status: TaskStatus) => void
  onDragStart: () => void
}) {
  const moves: { to: TaskStatus; label: string }[] = COLUMNS.filter((c) => c.status !== task.status).map(
    (c) => ({ to: c.status, label: c.label }),
  )
  return (
    <li
      draggable
      onDragStart={(e) => {
        e.dataTransfer.setData('text/plain', task.id)
        e.dataTransfer.effectAllowed = 'move'
        onDragStart()
      }}
      className="cursor-grab rounded-lg border border-neutral-800 bg-neutral-900/60 p-3 active:cursor-grabbing"
    >
      <button onClick={onOpen} className="block w-full text-left">
        <div className="text-sm font-medium">{task.title}</div>
        {task.labels.length > 0 && (
          <div className="mt-1.5 flex flex-wrap gap-1">
            {task.labels.map((l) => (
              <span
                key={l.id}
                className={
                  'rounded-full border px-1.5 py-px text-[10px] ' + (LABEL_STYLES[l.color] ?? LABEL_STYLES.neutral)
                }
              >
                {l.name}
              </span>
            ))}
          </div>
        )}
        <div className="mt-1.5 flex flex-wrap items-center gap-2 text-xs">
          {task.priority !== 'medium' && (
            <span className={PRIORITY_COLORS[task.priority]}>{task.priority}</span>
          )}
          {task.dueDate && (
            <span className={isOverdue(task) ? 'text-red-400' : 'text-neutral-500'}>{task.dueDate}</span>
          )}
          {task.assigneeName && (
            <span
              title={task.assigneeName}
              className="flex h-5 w-5 items-center justify-center rounded-full bg-neutral-700 text-[10px] font-medium text-neutral-200"
            >
              {initials(task.assigneeName)}
            </span>
          )}
        </div>
      </button>
      {moves.length > 0 && (
        <div className="mt-2 flex gap-1 border-t border-neutral-800 pt-2">
          {moves.map((m) => (
            <button
              key={m.to}
              onClick={() => onMove(m.to)}
              className="rounded border border-neutral-800 px-2 py-0.5 text-[11px] text-neutral-400 hover:border-neutral-600 hover:text-neutral-200"
            >
              → {m.label}
            </button>
          ))}
        </div>
      )}
    </li>
  )
}

export default function ProjectBoard() {
  const { id, pid } = useParams<{ id: string; pid: string }>()
  const navigate = useNavigate()
  const [project, setProject] = useState<Project | null>(null)
  const [tasks, setTasks] = useState<Task[] | null>(null)
  const [members, setMembers] = useState<Member[]>([])
  const [labels, setLabels] = useState<TaskLabel[]>([])
  const [filterAssignee, setFilterAssignee] = useState('')
  const [filterLabel, setFilterLabel] = useState('')
  const [filterPriority, setFilterPriority] = useState('')
  const [overdueOnly, setOverdueOnly] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [selected, setSelected] = useState<Task | null>(null)
  const [quickAdd, setQuickAdd] = useState('')
  const [busy, setBusy] = useState(false)
  const [dragOver, setDragOver] = useState<TaskStatus | null>(null)

  function showError(err: unknown) {
    setError(err instanceof ApiError ? err.message : 'Something went wrong')
  }

  function load() {
    if (!id || !pid) return
    getProject(id, pid)
      .then((r) => {
        setProject(r.project)
        setTasks(r.tasks)
      })
      .catch(showError)
    listMembers(id).then(setMembers).catch(() => {})
    listLabels(id, pid).then(setLabels).catch(() => {})
  }

  useEffect(load, [id, pid])

  async function handleQuickAdd(e: FormEvent) {
    e.preventDefault()
    if (!id || !pid || !quickAdd.trim()) return
    setBusy(true)
    setError(null)
    try {
      await createTask(id, pid, { title: quickAdd.trim() })
      setQuickAdd('')
      load()
    } catch (err) {
      showError(err)
    } finally {
      setBusy(false)
    }
  }

  async function move(task: Task, status: TaskStatus) {
    if (!id) return
    setError(null)
    try {
      const updated = await setTaskStatus(id, task.id, status)
      setTasks((prev) => prev?.map((t) => (t.id === updated.id ? updated : t)) ?? null)
      if (selected?.id === updated.id) setSelected(updated)
    } catch (err) {
      showError(err)
    }
  }

  async function saveSelected(patch: Record<string, string>) {
    if (!id || !selected) return
    setError(null)
    try {
      const updated = await updateTask(id, selected.id, patch)
      setTasks((prev) => prev?.map((t) => (t.id === updated.id ? updated : t)) ?? null)
      setSelected(updated)
    } catch (err) {
      showError(err)
    }
  }

  async function removeSelected() {
    if (!id || !selected) return
    setError(null)
    try {
      await deleteTask(id, selected.id)
      setTasks((prev) => prev?.filter((t) => t.id !== selected.id) ?? null)
      setSelected(null)
    } catch (err) {
      showError(err)
    }
  }

  if (error && !project) {
    return (
      <div className="space-y-4">
        <p className="text-sm text-red-400">{error}</p>
        <Link to={`/workspaces/${id}/work`} className="text-sm text-neutral-400 underline">
          ← Work Management
        </Link>
      </div>
    )
  }

  if (!project || tasks === null) {
    return <p className="text-sm text-neutral-500">Loading project…</p>
  }

  // Client-side filters (the board loads whole projects at alpha scale).
  const visible = (t: Task) =>
    (!filterAssignee || t.assigneeId === filterAssignee) &&
    (!filterLabel || t.labels.some((l) => l.id === filterLabel)) &&
    (!filterPriority || t.priority === filterPriority) &&
    (!overdueOnly || isOverdue(t))
  const hasFilters = !!(filterAssignee || filterLabel || filterPriority || overdueOnly)

  return (
    <div className="space-y-6">
      <div>
        <Link to={`/workspaces/${id}/work`} className="text-sm text-neutral-400 hover:text-neutral-200">
          ← Work Management
        </Link>
        <h1 className="mt-2 text-2xl font-semibold">{project.name}</h1>
      </div>
      {error && <p className="text-sm text-red-400">{error}</p>}

      {/* Filter bar */}
      <div className="flex flex-wrap items-center gap-2 text-xs">
        <span className="text-neutral-600">Filter:</span>
        <select
          value={filterAssignee}
          onChange={(e) => setFilterAssignee(e.target.value)}
          className="rounded border border-neutral-800 bg-neutral-950 px-2 py-1"
        >
          <option value="">Everyone</option>
          {members.map((m) => (
            <option key={m.userId} value={m.userId}>
              {m.displayName || m.email}
            </option>
          ))}
        </select>
        <select
          value={filterLabel}
          onChange={(e) => setFilterLabel(e.target.value)}
          className="rounded border border-neutral-800 bg-neutral-950 px-2 py-1"
        >
          <option value="">Any label</option>
          {labels.map((l) => (
            <option key={l.id} value={l.id}>
              {l.name}
            </option>
          ))}
        </select>
        <select
          value={filterPriority}
          onChange={(e) => setFilterPriority(e.target.value)}
          className="rounded border border-neutral-800 bg-neutral-950 px-2 py-1"
        >
          <option value="">Any priority</option>
          <option value="low">low</option>
          <option value="medium">medium</option>
          <option value="high">high</option>
          <option value="urgent">urgent</option>
        </select>
        <label className="flex items-center gap-1.5">
          <input type="checkbox" checked={overdueOnly} onChange={(e) => setOverdueOnly(e.target.checked)} />
          Overdue only
        </label>
        {hasFilters && (
          <button
            onClick={() => {
              setFilterAssignee('')
              setFilterLabel('')
              setFilterPriority('')
              setOverdueOnly(false)
            }}
            className="text-neutral-500 underline hover:text-neutral-300"
          >
            clear
          </button>
        )}
      </div>

      <div className="flex gap-4">
        <div className="flex flex-1 gap-4">
          {COLUMNS.map((col) => {
            const columnTasks = tasks.filter((t) => t.status === col.status && visible(t))
            return (
              <section
                key={col.status}
                onDragOver={(e) => {
                  e.preventDefault()
                  e.dataTransfer.dropEffect = 'move'
                  setDragOver(col.status)
                }}
                onDragLeave={() => setDragOver(null)}
                onDrop={(e) => {
                  e.preventDefault()
                  setDragOver(null)
                  const id = e.dataTransfer.getData('text/plain')
                  const task = tasks.find((t) => t.id === id)
                  if (task && task.status !== col.status) void move(task, col.status)
                }}
                className={
                  'flex-1 rounded-lg p-3 transition-colors ' +
                  (dragOver === col.status ? 'bg-neutral-800/40 ring-1 ring-emerald-700' : 'bg-neutral-900/30')
                }
              >
                <h2 className="mb-3 flex items-center justify-between text-sm font-medium">
                  {col.label}
                  <span className="text-xs font-normal text-neutral-500">{columnTasks.length}</span>
                </h2>
                <ul className="space-y-2">
                  {columnTasks.map((t) => (
                    <TaskCard
                      key={t.id}
                      task={t}
                      onOpen={() => navigate(`/workspaces/${id}/work/${pid}/task/${t.id}`)}
                      onMove={(s) => move(t, s)}
                      onDragStart={() => setSelected(null)}
                    />
                  ))}
                </ul>
                {col.status === 'todo' && (
                  <form onSubmit={(e) => handleQuickAdd(e)} className="mt-2">
                    <input
                      type="text"
                      placeholder="+ Add task"
                      value={quickAdd}
                      disabled={busy}
                      onChange={(e) => setQuickAdd(e.target.value)}
                      className="w-full rounded border border-neutral-800 bg-neutral-950 px-2 py-1.5 text-xs focus:border-neutral-600 focus:outline-none"
                    />
                  </form>
                )}
              </section>
            )
          })}
        </div>

        {selected && (
          <aside className="w-72 shrink-0 space-y-4 rounded-lg border border-neutral-800 bg-neutral-900/60 p-4">
            <div className="flex items-start justify-between">
              <h3 className="text-sm font-medium">{selected.title}</h3>
              <button onClick={() => setSelected(null)} className="text-neutral-500 hover:text-neutral-200">
                ×
              </button>
            </div>

            <label className="block text-xs">
              Title
              <input
                type="text"
                defaultValue={selected.title}
                onBlur={(e) => e.target.value !== selected.title && saveSelected({ title: e.target.value })}
                className="mt-1 w-full rounded border border-neutral-700 bg-neutral-950 px-2 py-1.5 text-sm focus:border-neutral-500 focus:outline-none"
              />
            </label>

            <label className="block text-xs">
              Description
              <textarea
                rows={3}
                defaultValue={selected.description}
                onBlur={(e) => e.target.value !== selected.description && saveSelected({ description: e.target.value })}
                className="mt-1 w-full rounded border border-neutral-700 bg-neutral-950 px-2 py-1.5 text-sm focus:border-neutral-500 focus:outline-none"
              />
            </label>

            <label className="block text-xs">
              Assignee
              <select
                value={selected.assigneeId}
                onChange={(e) => saveSelected({ assigneeId: e.target.value })}
                className="mt-1 w-full rounded border border-neutral-700 bg-neutral-950 px-2 py-1.5 text-sm focus:border-neutral-500 focus:outline-none"
              >
                <option value="">Unassigned</option>
                {members.map((m) => (
                  <option key={m.userId} value={m.userId}>
                    {m.displayName || m.email}
                  </option>
                ))}
              </select>
            </label>

            <label className="block text-xs">
              Due date
              <input
                type="date"
                defaultValue={selected.dueDate ?? ''}
                onChange={(e) => saveSelected({ dueDate: e.target.value })}
                className="mt-1 w-full rounded border border-neutral-700 bg-neutral-950 px-2 py-1.5 text-sm focus:border-neutral-500 focus:outline-none"
              />
            </label>

            <label className="block text-xs">
              Priority
              <select
                value={selected.priority}
                onChange={(e) => saveSelected({ priority: e.target.value })}
                className="mt-1 w-full rounded border border-neutral-700 bg-neutral-950 px-2 py-1.5 text-sm focus:border-neutral-500 focus:outline-none"
              >
                <option value="low">low</option>
                <option value="medium">medium</option>
                <option value="high">high</option>
                <option value="urgent">urgent</option>
              </select>
            </label>

            <div className="flex items-center justify-between border-t border-neutral-800 pt-3">
              <select
                value={selected.status}
                onChange={(e) => move(selected, e.target.value as TaskStatus)}
                className="rounded border border-neutral-700 bg-neutral-950 px-2 py-1 text-xs"
              >
                {COLUMNS.map((c) => (
                  <option key={c.status} value={c.status}>
                    {c.label}
                  </option>
                ))}
              </select>
              <button onClick={removeSelected} className="text-xs text-neutral-500 hover:text-red-400">
                Delete
              </button>
            </div>
          </aside>
        )}
      </div>
    </div>
  )
}