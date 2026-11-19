import { useEffect, useState, type FormEvent } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import {
  addComment,
  addDependency,
  ApiError,
  createLabel,
  createSubtask,
  deleteSubtask,
  deleteTask,
  deleteTimeEntry,
  getDependencies,
  getProject,
  getTask,
  listComments,
  listLabels,
  listMembers,
  listSubtasks,
  listTime,
  logTime,
  removeDependency,
  setTaskLabels,
  setTaskStatus,
  startTime,
  stopTime,
  updateSubtask,
  updateTask,
  type DependencyInfo,
  type Member,
  type Subtask,
  type Task,
  type TaskComment,
  type TaskLabel,
  type TimeEntry,
  type TaskStatus,
} from '../lib/api'

const LABEL_COLORS: Record<string, string> = {
  emerald: 'bg-emerald-900/40 text-emerald-300 border-emerald-800/60',
  sky: 'bg-sky-900/40 text-sky-300 border-sky-800/60',
  amber: 'bg-amber-900/40 text-amber-300 border-amber-800/60',
  rose: 'bg-rose-900/40 text-rose-300 border-rose-800/60',
  violet: 'bg-violet-900/40 text-violet-300 border-violet-800/60',
  lime: 'bg-lime-900/40 text-lime-300 border-lime-800/60',
  neutral: 'bg-neutral-800 text-neutral-300 border-neutral-700',
}

function LabelChip({ label, onClick }: { label: TaskLabel; onClick?: () => void }) {
  return (
    <button
      onClick={onClick}
      className={
        'rounded-full border px-2 py-0.5 text-[11px] ' + (LABEL_COLORS[label.color] ?? LABEL_COLORS.neutral) +
        (onClick ? ' cursor-pointer opacity-80 hover:opacity-100' : '')
      }
    >
      {label.name}
    </button>
  )
}

const STATUSES: { value: TaskStatus; label: string }[] = [
  { value: 'todo', label: 'To Do' },
  { value: 'in_progress', label: 'In Progress' },
  { value: 'done', label: 'Done' },
]

/* --- Subtasks --- */
function SubtasksSection({ workspaceId, taskId }: { workspaceId: string; taskId: string }) {
  const [subtasks, setSubtasks] = useState<Subtask[] | null>(null)
  const [title, setTitle] = useState('')
  const [error, setError] = useState<string | null>(null)

  function load() {
    listSubtasks(workspaceId, taskId).then(setSubtasks).catch(() => setSubtasks([]))
  }
  useEffect(load, [workspaceId, taskId])

  async function add() {
    if (!title.trim()) return
    try {
      await createSubtask(workspaceId, taskId, title.trim())
      setTitle('')
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed')
    }
  }

  const done = subtasks?.filter((s) => s.done).length ?? 0
  const total = subtasks?.length ?? 0

  return (
    <div className="rounded-xl border border-neutral-800 bg-neutral-900/40 p-5">
      <div className="mb-3 flex items-center justify-between">
        <h2 className="text-xs font-semibold uppercase tracking-wider text-neutral-500">
          Subtasks {total > 0 && <span className="text-neutral-700">({done}/{total})</span>}
        </h2>
        {total > 0 && (
          <div className="h-1 w-24 overflow-hidden rounded-full bg-neutral-800">
            <div className="h-full bg-emerald-500" style={{ width: `${(done / total) * 100}%` }} />
          </div>
        )}
      </div>
      <ul className="space-y-1.5">
        {subtasks?.map((st) => (
          <li key={st.id} className="group flex items-center gap-2.5 text-sm">
            <input
              type="checkbox"
              checked={st.done}
              onChange={() =>
                updateSubtask(workspaceId, st.id, { done: !st.done }).then(() => load())
              }
              className="h-4 w-4 accent-emerald-500"
            />
            <span className={st.done ? 'flex-1 text-neutral-500 line-through' : 'flex-1'}>{st.title}</span>
            <button
              onClick={() => deleteSubtask(workspaceId, st.id).then(load)}
              className="text-neutral-700 opacity-0 transition-opacity group-hover:opacity-100 hover:text-red-400"
              aria-label="Delete subtask"
            >
              ×
            </button>
          </li>
        ))}
      </ul>
      <div className="mt-3 flex gap-2 border-t border-neutral-800 pt-3">
        <input
          type="text"
          placeholder="Add a subtask…"
          value={title}
          maxLength={500}
          onChange={(e) => setTitle(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && add()}
          className="flex-1 rounded border border-neutral-700 bg-neutral-950 px-3 py-1.5 text-xs focus:border-neutral-500 focus:outline-none"
        />
        <button onClick={add} className="rounded border border-neutral-700 px-3 py-1.5 text-xs hover:border-neutral-500">
          Add
        </button>
      </div>
      {error && <p className="mt-2 text-xs text-red-400">{error}</p>}
    </div>
  )
}

/* --- Dependencies --- */
function DependenciesSection({
  workspaceId,
  projectId,
  taskId,
}: {
  workspaceId: string
  projectId: string
  taskId: string
}) {
  const [info, setInfo] = useState<DependencyInfo | null>(null)
  const [tasks, setTasks] = useState<Task[]>([])
  const [selected, setSelected] = useState('')
  const [error, setError] = useState<string | null>(null)

  function load() {
    getDependencies(workspaceId, taskId).then(setInfo).catch(() => {})
    getProject(workspaceId, projectId)
      .then((r) => setTasks(r.tasks.filter((t) => t.id !== taskId)))
      .catch(() => {})
  }
  useEffect(load, [workspaceId, projectId, taskId])

  async function add() {
    if (!selected) return
    setError(null)
    try {
      const updated = await addDependency(workspaceId, taskId, selected)
      setInfo(updated)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed')
    }
  }

  return (
    <div className="rounded-xl border border-neutral-800 bg-neutral-900/40 p-5">
      <h2 className="mb-3 text-xs font-semibold uppercase tracking-wider text-neutral-500">
        Dependencies
      </h2>
      {info && (
        <div className="space-y-3">
          {info.blockedBy.length > 0 && (
            <div>
              <div className="text-[11px] text-neutral-600">Blocked by</div>
              <ul className="mt-1 space-y-1">
                {info.blockedBy.map((b) => (
                  <li key={b.id} className="flex items-center justify-between rounded border border-neutral-800 px-2.5 py-1.5 text-sm">
                    <span>
                      <Link to={`../task/${b.id}`} className="hover:text-emerald-300">{b.title}</Link>
                      {b.status !== 'done' && <span className="ml-1.5 text-[10px] text-amber-400">incomplete</span>}
                    </span>
                    <button
                      onClick={() => removeDependency(workspaceId, taskId, b.id).then(setInfo)}
                      className="text-xs text-neutral-600 hover:text-red-400"
                    >
                      remove
                    </button>
                  </li>
                ))}
              </ul>
            </div>
          )}
          {info.blocks.length > 0 && (
            <div>
              <div className="text-[11px] text-neutral-600">Blocks</div>
              <ul className="mt-1 space-y-1">
                {info.blocks.map((b) => (
                  <li key={b.id} className="rounded border border-neutral-800 px-2.5 py-1.5 text-sm">
                    <Link to={`../task/${b.id}`} className="hover:text-emerald-300">{b.title}</Link>
                    <span className="ml-1.5 text-[10px] text-neutral-500">{b.status}</span>
                  </li>
                ))}
              </ul>
            </div>
          )}
          {info.blockedBy.length === 0 && info.blocks.length === 0 && (
            <p className="text-xs text-neutral-600">No dependencies — this task is independent.</p>
          )}
        </div>
      )}
      <div className="mt-3 flex gap-2 border-t border-neutral-800 pt-3">
        <select
          value={selected}
          onChange={(e) => setSelected(e.target.value)}
          className="flex-1 rounded border border-neutral-700 bg-neutral-950 px-2 py-1.5 text-xs"
        >
          <option value="">Blocked by… (pick a task)</option>
          {tasks.map((t) => (
            <option key={t.id} value={t.id}>
              {t.title}
            </option>
          ))}
        </select>
        <button onClick={add} disabled={!selected} className="rounded border border-neutral-700 px-3 py-1.5 text-xs hover:border-neutral-500 disabled:opacity-50">
          Add blocker
        </button>
      </div>
      {error && <p className="mt-2 text-xs text-red-400">{error}</p>}
    </div>
  )
}

/* --- Time tracking --- */
function TimeSection({ workspaceId, taskId }: { workspaceId: string; taskId: string }) {
  const [entries, setEntries] = useState<TimeEntry[] | null>(null)
  const [running, setRunning] = useState(false)
  const [minutes, setMinutes] = useState('')
  const [note, setNote] = useState('')
  const [error, setError] = useState<string | null>(null)

  function load() {
    listTime(workspaceId, taskId).then((list) => {
      setEntries(list)
      setRunning(list.some((e) => !e.endedAt))
    }).catch(() => setEntries([]))
  }
  useEffect(load, [workspaceId, taskId])

  const total = entries?.reduce((sum, e) => sum + e.minutes, 0) ?? 0

  async function toggle() {
    setError(null)
    try {
      if (running) {
        await stopTime(workspaceId, taskId)
      } else {
        await startTime(workspaceId, taskId)
      }
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed')
    }
  }

  async function log(e: FormEvent) {
    e.preventDefault()
    const m = parseInt(minutes, 10)
    if (!m || m < 1) return
    setError(null)
    try {
      await logTime(workspaceId, taskId, m, note)
      setMinutes('')
      setNote('')
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed')
    }
  }

  return (
    <div className="rounded-xl border border-neutral-800 bg-neutral-900/40 p-5">
      <div className="mb-3 flex items-center justify-between">
        <h2 className="text-xs font-semibold uppercase tracking-wider text-neutral-500">
          Time {total > 0 && <span className="text-neutral-700">{Math.floor(total / 60)}h {total % 60}m</span>}
        </h2>
        <button
          onClick={toggle}
          className={
            'rounded px-3 py-1.5 text-xs font-medium ' +
            (running ? 'bg-red-500/80 text-black hover:bg-red-400' : 'bg-emerald-500 text-black hover:bg-emerald-400')
          }
        >
          {running ? '■ Stop' : '▶ Start timer'}
        </button>
      </div>
      {entries !== null && entries.length > 0 && (
        <ul className="mb-3 space-y-1">
          {entries.map((e) => (
            <li key={e.id} className="flex items-center justify-between rounded border border-neutral-800/60 px-2.5 py-1.5 text-xs">
              <span>
                <span className="font-medium text-neutral-200">{e.userName || 'Someone'}</span>
                {e.endedAt ? ` · ${e.minutes}m` : ' · running…'}
                {e.note && <span className="text-neutral-500"> · {e.note}</span>}
              </span>
              <button
                onClick={() => deleteTimeEntry(workspaceId, e.id).then(load)}
                className="text-neutral-700 hover:text-red-400"
                aria-label="Delete entry"
              >
                ×
              </button>
            </li>
          ))}
        </ul>
      )}
      <form onSubmit={log} className="flex gap-2 border-t border-neutral-800 pt-3">
        <input
          type="number"
          min={1}
          max={1440}
          placeholder="min"
          value={minutes}
          onChange={(e) => setMinutes(e.target.value)}
          className="w-16 rounded border border-neutral-700 bg-neutral-950 px-2 py-1.5 text-xs focus:border-neutral-500 focus:outline-none"
        />
        <input
          type="text"
          placeholder="Note (optional)"
          maxLength={500}
          value={note}
          onChange={(e) => setNote(e.target.value)}
          className="flex-1 rounded border border-neutral-700 bg-neutral-950 px-2 py-1.5 text-xs focus:border-neutral-500 focus:outline-none"
        />
        <button type="submit" className="rounded border border-neutral-700 px-3 py-1.5 text-xs hover:border-neutral-500">
          Log
        </button>
      </form>
      {error && <p className="mt-2 text-xs text-red-400">{error}</p>}
    </div>
  )
}

export default function TaskDetail() {
  const { id: wsId, pid, tid } = useParams<{ id: string; pid: string; tid: string }>()
  const navigate = useNavigate()
  const [task, setTask] = useState<Task | null>(null)
  const [members, setMembers] = useState<Member[]>([])
  const [labels, setLabels] = useState<TaskLabel[]>([])
  const [comments, setComments] = useState<TaskComment[] | null>(null)
  const [commentBody, setCommentBody] = useState('')
  const [newLabelName, setNewLabelName] = useState('')
  const [newLabelColor, setNewLabelColor] = useState('emerald')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [titleEditing, setTitleEditing] = useState(false)

  function showError(err: unknown) {
    setError(err instanceof ApiError ? err.message : 'Something went wrong')
  }

  function load() {
    if (!wsId || !tid) return
    getTask(wsId, tid).then(setTask).catch(showError)
    listComments(wsId, tid).then(setComments).catch(showError)
    listMembers(wsId).then(setMembers).catch(() => {})
    if (pid) listLabels(wsId, pid).then(setLabels).catch(() => {})
  }

  useEffect(load, [wsId, pid, tid])

  async function patch(fields: Record<string, unknown>) {
    if (!wsId || !task) return
    setError(null)
    try {
      setTask(await updateTask(wsId, task.id, fields))
    } catch (err) {
      showError(err)
    }
  }

  async function move(status: TaskStatus) {
    if (!wsId || !task) return
    setError(null)
    try {
      setTask(await setTaskStatus(wsId, task.id, status))
    } catch (err) {
      showError(err)
    }
  }

  async function toggleLabel(label: TaskLabel) {
    if (!wsId || !task) return
    const current = task.labels.map((l) => l.id)
    const next = current.includes(label.id)
      ? current.filter((id) => id !== label.id)
      : [...current, label.id]
    try {
      setTask(await setTaskLabels(wsId, task.id, next))
    } catch (err) {
      showError(err)
    }
  }

  async function handleAddLabel() {
    if (!wsId || !pid || !newLabelName.trim()) return
    try {
      await createLabel(wsId, pid, newLabelName.trim(), newLabelColor)
      setNewLabelName('')
      setLabels(await listLabels(wsId, pid))
    } catch (err) {
      showError(err)
    }
  }

  async function handleComment(e: FormEvent) {
    e.preventDefault()
    if (!wsId || !task || !commentBody.trim()) return
    setBusy(true)
    setError(null)
    try {
      await addComment(wsId, task.id, commentBody.trim())
      setCommentBody('')
      setComments(await listComments(wsId, task.id))
    } catch (err) {
      showError(err)
    } finally {
      setBusy(false)
    }
  }

  async function handleDelete() {
    if (!wsId || !task || !pid) return
    if (!window.confirm('Delete this task? This cannot be undone.')) return
    try {
      await deleteTask(wsId, task.id)
      navigate(`/workspaces/${wsId}/work/${pid}`)
    } catch (err) {
      showError(err)
    }
  }

  if (error && !task) {
    return (
      <div className="space-y-4">
        <p className="text-sm text-red-400">{error}</p>
        <Link to={`/workspaces/${wsId}/work/${pid}`} className="text-sm text-neutral-400 underline">
          ← Back to board
        </Link>
      </div>
    )
  }
  if (!task || comments === null) return <p className="text-sm text-neutral-500">Loading task…</p>

  const assigned = task.labels.map((l) => l.id)

  return (
    <div className="mx-auto max-w-3xl space-y-6">
      <div className="flex items-center justify-between">
        <Link
          to={`/workspaces/${wsId}/work/${pid}`}
          className="text-sm text-neutral-400 hover:text-neutral-200"
        >
          ← Board
        </Link>
        <button onClick={handleDelete} className="text-xs text-neutral-500 hover:text-red-400">
          Delete task
        </button>
      </div>

      {/* Title + status */}
      <div className="rounded-xl border border-neutral-800 bg-neutral-900/40 p-5">
        {titleEditing ? (
          <input
            autoFocus
            type="text"
            defaultValue={task.title}
            onBlur={(e) => {
              setTitleEditing(false)
              if (e.target.value.trim() && e.target.value !== task.title) {
                void patch({ title: e.target.value.trim() })
              }
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter') (e.target as HTMLInputElement).blur()
              if (e.key === 'Escape') setTitleEditing(false)
            }}
            className="w-full rounded border border-neutral-700 bg-neutral-950 px-3 py-2 text-xl font-semibold focus:border-neutral-500 focus:outline-none"
          />
        ) : (
          <h1
            onClick={() => setTitleEditing(true)}
            className="cursor-text text-xl font-semibold leading-snug hover:text-emerald-300"
            title="Click to edit"
          >
            {task.title}
          </h1>
        )}
        <div className="mt-3 flex flex-wrap items-center gap-2">
          {STATUSES.map((s) => (
            <button
              key={s.value}
              onClick={() => move(s.value)}
              className={
                'rounded-full border px-3 py-1 text-xs transition-colors ' +
                (task.status === s.value
                  ? 'border-emerald-700 bg-emerald-900/40 text-emerald-300'
                  : 'border-neutral-700 text-neutral-400 hover:border-neutral-500')
              }
            >
              {s.label}
            </button>
          ))}
          {task.completedAt && (
            <span className="text-xs text-neutral-500">
              completed {new Date(task.completedAt).toLocaleString()}
            </span>
          )}
        </div>
      </div>

      {/* Description + properties */}
      <div className="grid gap-4 sm:grid-cols-[1fr_220px]">
        <div className="rounded-xl border border-neutral-800 bg-neutral-900/40 p-5">
          <h2 className="mb-2 text-xs font-semibold uppercase tracking-wider text-neutral-500">
            Description
          </h2>
          <textarea
            rows={6}
            defaultValue={task.description}
            placeholder="Add more detail…"
            onBlur={(e) => {
              if (e.target.value !== task.description) void patch({ description: e.target.value })
            }}
            className="w-full resize-y rounded border border-transparent bg-transparent px-2 py-1.5 text-sm leading-relaxed text-neutral-200 focus:border-neutral-700 focus:bg-neutral-950 focus:outline-none"
          />
        </div>
        <div className="space-y-4 rounded-xl border border-neutral-800 bg-neutral-900/40 p-5">
          <h2 className="text-xs font-semibold uppercase tracking-wider text-neutral-500">
            Properties
          </h2>
          <label className="block text-xs">
            Assignee
            <select
              value={task.assigneeId}
              onChange={(e) => patch({ assigneeId: e.target.value })}
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
              defaultValue={task.dueDate ?? ''}
              onChange={(e) => patch({ dueDate: e.target.value })}
              className="mt-1 w-full rounded border border-neutral-700 bg-neutral-950 px-2 py-1.5 text-sm focus:border-neutral-500 focus:outline-none"
            />
          </label>
          <label className="block text-xs">
            Priority
            <select
              value={task.priority}
              onChange={(e) => patch({ priority: e.target.value })}
              className="mt-1 w-full rounded border border-neutral-700 bg-neutral-950 px-2 py-1.5 text-sm focus:border-neutral-500 focus:outline-none"
            >
              <option value="low">low</option>
              <option value="medium">medium</option>
              <option value="high">high</option>
              <option value="urgent">urgent</option>
            </select>
          </label>
        </div>
      </div>

      {/* Labels */}
      <div className="rounded-xl border border-neutral-800 bg-neutral-900/40 p-5">
        <h2 className="mb-3 text-xs font-semibold uppercase tracking-wider text-neutral-500">Labels</h2>
        <div className="flex flex-wrap gap-2">
          {labels.map((l) => (
            <LabelChip
              key={l.id}
              label={l}
              onClick={() => toggleLabel(l)}
            />
          ))}
          {labels.length === 0 && <p className="text-xs text-neutral-600">No labels in this project yet.</p>}
        </div>
        <div className="mt-3 flex items-center gap-2 border-t border-neutral-800 pt-3">
          <input
            type="text"
            placeholder="New label…"
            maxLength={40}
            value={newLabelName}
            onChange={(e) => setNewLabelName(e.target.value)}
            className="w-36 rounded border border-neutral-700 bg-neutral-950 px-2 py-1.5 text-xs focus:border-neutral-500 focus:outline-none"
          />
          <select
            value={newLabelColor}
            onChange={(e) => setNewLabelColor(e.target.value)}
            className="rounded border border-neutral-700 bg-neutral-950 px-2 py-1.5 text-xs"
          >
            {Object.keys(LABEL_COLORS).map((c) => (
              <option key={c} value={c}>
                {c}
              </option>
            ))}
          </select>
          <button
            onClick={handleAddLabel}
            disabled={!newLabelName.trim()}
            className="rounded border border-neutral-700 px-3 py-1.5 text-xs hover:border-neutral-500 disabled:opacity-50"
          >
            Add
          </button>
          <span className="text-[11px] text-neutral-600">
            click a label to {assigned.length > 0 ? 'toggle it on this task' : 'assign it'}
          </span>
        </div>
      </div>

      {/* Subtasks */}
      <SubtasksSection workspaceId={wsId!} taskId={task.id} />

      {/* Dependencies */}
      <DependenciesSection workspaceId={wsId!} projectId={task.projectId} taskId={task.id} />

      {/* Time tracking */}
      <TimeSection workspaceId={wsId!} taskId={task.id} />

      {/* Comments */}
      <div className="rounded-xl border border-neutral-800 bg-neutral-900/40 p-5">
        <h2 className="mb-4 text-xs font-semibold uppercase tracking-wider text-neutral-500">
          Comments {comments.length > 0 && <span className="text-neutral-700">({comments.length})</span>}
        </h2>
        <ul className="space-y-4">
          {comments.map((c) => (
            <li key={c.id} className="flex gap-3">
              <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-neutral-800 text-xs font-medium text-neutral-300">
                {(c.authorName || '?').slice(0, 1).toUpperCase()}
              </div>
              <div className="min-w-0 flex-1">
                <div className="flex items-baseline gap-2">
                  <span className="text-xs font-medium text-neutral-200">{c.authorName || 'Member'}</span>
                  <span className="text-[10px] text-neutral-600">
                    {new Date(c.createdAt).toLocaleString()}
                  </span>
                </div>
                <p className="mt-1 whitespace-pre-wrap break-words text-sm leading-relaxed text-neutral-300">
                  {c.body}
                </p>
              </div>
            </li>
          ))}
          {comments.length === 0 && (
            <p className="text-xs text-neutral-600">No comments yet — start the thread.</p>
          )}
        </ul>
        <form onSubmit={handleComment} className="mt-5 border-t border-neutral-800 pt-4">
          <textarea
            rows={2}
            placeholder="Write a comment… (the assignee gets notified)"
            value={commentBody}
            onChange={(e) => setCommentBody(e.target.value)}
            className="w-full resize-y rounded border border-neutral-700 bg-neutral-950 px-3 py-2 text-sm focus:border-neutral-500 focus:outline-none"
          />
          <div className="mt-2 flex justify-end">
            <button
              type="submit"
              disabled={busy || !commentBody.trim()}
              className="rounded bg-neutral-100 px-4 py-1.5 text-sm font-medium text-neutral-900 hover:bg-white disabled:opacity-50"
            >
              {busy ? 'Posting…' : 'Comment'}
            </button>
          </div>
        </form>
      </div>
      {error && <p className="text-sm text-red-400">{error}</p>}
    </div>
  )
}