import type { Task } from '../lib/api'

/* Shared helpers for the project views */

export function toISODate(d: Date): string {
  return d.toISOString().slice(0, 10)
}

/* --- List view --- */
export function ListView({ tasks, onOpen }: { tasks: Task[]; onOpen: (t: Task) => void }) {
  const sorted = [...tasks].sort((a, b) => {
    if (a.status !== b.status) return a.status === 'done' ? 1 : -1
    return (a.dueDate ?? '9999') < (b.dueDate ?? '9999') ? -1 : 1
  })
  return (
    <div className="overflow-x-auto rounded-lg border border-neutral-800">
      <table className="min-w-full text-sm">
        <thead>
          <tr className="border-b border-neutral-800 bg-neutral-900/60 text-left text-xs text-neutral-400">
            <th className="px-4 py-2 font-medium">Task</th>
            <th className="px-4 py-2 font-medium">Assignee</th>
            <th className="px-4 py-2 font-medium">Status</th>
            <th className="px-4 py-2 font-medium">Priority</th>
            <th className="px-4 py-2 font-medium">Due</th>
            <th className="px-4 py-2 font-medium">Labels</th>
          </tr>
        </thead>
        <tbody>
          {sorted.map((t) => (
            <tr
              key={t.id}
              onClick={() => onOpen(t)}
              className="cursor-pointer border-b border-neutral-800/50 hover:bg-neutral-900/50"
            >
              <td className="px-4 py-2 font-medium">{t.title}</td>
              <td className="px-4 py-2 text-neutral-400">{t.assigneeName || '—'}</td>
              <td className="px-4 py-2">
                <span
                  className={
                    t.status === 'done'
                      ? 'text-emerald-400'
                      : t.status === 'in_progress'
                        ? 'text-amber-400'
                        : 'text-neutral-400'
                  }
                >
                  {t.status === 'in_progress' ? 'In Progress' : t.status === 'done' ? 'Done' : 'To Do'}
                </span>
              </td>
              <td className="px-4 py-2 text-neutral-400">{t.priority}</td>
              <td className="px-4 py-2">
                <span className={t.dueDate && t.dueDate < toISODate(new Date()) && t.status !== 'done' ? 'text-red-400' : 'text-neutral-400'}>
                  {t.dueDate || '—'}
                </span>
              </td>
              <td className="px-4 py-2">
                <span className="flex gap-1">
                  {t.labels.map((l) => (
                    <span key={l.id} className="rounded-full border border-neutral-700 px-1.5 py-px text-[10px] text-neutral-300">
                      {l.name}
                    </span>
                  ))}
                </span>
              </td>
            </tr>
          ))}
          {sorted.length === 0 && (
            <tr>
              <td colSpan={6} className="px-4 py-8 text-center text-neutral-500">
                No tasks match the current filters.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  )
}

/* --- Calendar view --- */
export function CalendarView({ tasks, onOpen }: { tasks: Task[]; onOpen: (t: Task) => void }) {
  const now = new Date()
  const year = now.getFullYear()
  const month = now.getMonth()
  const first = new Date(year, month, 1)
  const startDay = (first.getDay() + 6) % 7 // Monday first
  const daysInMonth = new Date(year, month + 1, 0).getDate()

  const byDate = new Map<string, Task[]>()
  for (const t of tasks) {
    if (!t.dueDate) continue
    const list = byDate.get(t.dueDate) ?? []
    list.push(t)
    byDate.set(t.dueDate, list)
  }

  const cells: Array<{ date: Date | null }> = []
  for (let i = 0; i < startDay; i++) cells.push({ date: null })
  for (let d = 1; d <= daysInMonth; d++) cells.push({ date: new Date(year, month, d) })

  const today = toISODate(now)

  return (
    <div className="rounded-lg border border-neutral-800">
      <div className="border-b border-neutral-800 bg-neutral-900/60 px-4 py-2 text-sm font-medium">
        {first.toLocaleString('en-US', { month: 'long', year: 'numeric' })}
      </div>
      <div className="grid grid-cols-7 border-b border-neutral-800 text-center text-[10px] uppercase text-neutral-500">
        {['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'].map((d) => (
          <div key={d} className="py-1.5">
            {d}
          </div>
        ))}
      </div>
      <div className="grid grid-cols-7">
        {cells.map((cell, i) => (
          <div
            key={i}
            className={
              'min-h-20 border-b border-r border-neutral-800/50 p-1.5 ' +
              (cell.date && toISODate(cell.date) === today ? 'bg-emerald-900/10' : '')
            }
          >
            {cell.date && (
              <>
                <div
                  className={
                    'mb-1 text-[10px] ' +
                    (toISODate(cell.date) === today ? 'font-bold text-emerald-400' : 'text-neutral-500')
                  }
                >
                  {cell.date.getDate()}
                </div>
                <div className="space-y-1">
                  {(byDate.get(toISODate(cell.date)) ?? []).map((t) => (
                    <button
                      key={t.id}
                      onClick={() => onOpen(t)}
                      className={
                        'block w-full truncate rounded px-1.5 py-0.5 text-left text-[11px] ' +
                        (t.status === 'done'
                          ? 'bg-emerald-900/40 text-emerald-300 line-through'
                          : 'bg-neutral-800 text-neutral-300 hover:bg-neutral-700')
                      }
                      title={t.title}
                    >
                      {t.title}
                    </button>
                  ))}
                </div>
              </>
            )}
          </div>
        ))}
      </div>
    </div>
  )
}

/* --- Timeline view (Gantt-lite) --- */
export function TimelineView({ tasks, onOpen }: { tasks: Task[]; onOpen: (t: Task) => void }) {
  const dated = tasks.filter((t) => t.dueDate)
  if (dated.length === 0) {
    return (
      <div className="rounded-lg border border-dashed border-neutral-800 p-8 text-center text-sm text-neutral-500">
        Timeline shows tasks with due dates — add some to see them here.
      </div>
    )
  }

  const today = new Date()
  const dates = dated.flatMap((t) => [new Date(t.createdAt), new Date(t.dueDate! + 'T23:59:59')])
  const min = new Date(Math.min(...dates.map((d) => d.getTime())))
  const max = new Date(Math.max(...dates.map((d) => d.getTime())))
  min.setDate(min.getDate() - 1)
  max.setDate(max.getDate() + 1)
  const span = Math.max(1, (max.getTime() - min.getTime()) / 86400000)

  const pct = (d: Date) => ((d.getTime() - min.getTime()) / 86400000 / span) * 100

  const months: Array<{ label: string; left: number }> = []
  const cur = new Date(min)
  while (cur <= max) {
    months.push({
      label: cur.toLocaleString('en-US', { month: 'short' }),
      left: pct(cur),
    })
    cur.setMonth(cur.getMonth() + 1)
    cur.setDate(1)
  }

  return (
    <div className="rounded-lg border border-neutral-800 p-4">
      {/* Month ruler */}
      <div className="relative mb-3 h-5 border-b border-neutral-800 text-[10px] text-neutral-500">
        {months.map((m, i) => (
          <span key={i} className="absolute top-0" style={{ left: `${m.left}%` }}>
            {m.label}
          </span>
        ))}
        <span className="absolute top-0 h-full w-px bg-emerald-600" style={{ left: `${pct(today)}%` }} />
      </div>
      <div className="space-y-1.5">
        {dated.map((t) => {
          const start = new Date(t.createdAt)
          const end = new Date(t.dueDate! + 'T23:59:59')
          const left = Math.min(pct(start), pct(end))
          const width = Math.max(2, pct(end) - pct(start))
          const overdue = t.dueDate! < toISODate(today) && t.status !== 'done'
          return (
            <div key={t.id} className="flex items-center gap-3 text-sm">
              <button
                onClick={() => onOpen(t)}
                className="w-40 shrink-0 truncate text-left text-xs text-neutral-300 hover:text-neutral-100"
                title={t.title}
              >
                {t.title}
              </button>
              <div className="relative h-5 flex-1 rounded bg-neutral-900">
                <div
                  className={
                    'absolute top-0.5 h-4 rounded ' +
                    (t.status === 'done'
                      ? 'bg-emerald-700/70'
                      : overdue
                        ? 'bg-red-600/80'
                        : 'bg-sky-700/70')
                  }
                  style={{ left: `${left}%`, width: `${width}%` }}
                />
              </div>
              <span className="w-20 shrink-0 text-right text-[10px] text-neutral-500">{t.dueDate}</span>
            </div>
          )
        })}
      </div>
    </div>
  )
}