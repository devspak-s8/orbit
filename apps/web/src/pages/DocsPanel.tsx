import { useEffect, useState, type FormEvent } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  ApiError,
  createPage,
  deletePage,
  getProject,
  listPages,
  updatePage,
  type Page,
  type Project,
} from '../lib/api'

/**
 * DocsPanel renders the docs tab on a project: a list of pages with an
 * editor for the selected page. Markdown stored as plain text.
 */
export default function DocsPanel() {
  const { id, pid } = useParams<{ id: string; pid: string }>()
  const [project, setProject] = useState<Project | null>(null)
  const [pages, setPages] = useState<Page[] | null>(null)
  const [selected, setSelected] = useState<Page | null>(null)
  const [title, setTitle] = useState('')
  const [body, setBody] = useState('')
  const [editing, setEditing] = useState(false)
  const [newTitle, setNewTitle] = useState('')
  const [error, setError] = useState<string | null>(null)

  function showError(err: unknown) {
    setError(err instanceof ApiError ? err.message : 'Something went wrong')
  }

  function load() {
    if (!id || !pid) return
    getProject(id, pid).then((r) => setProject(r.project)).catch(() => {})
    listPages(id, pid).then(setPages).catch(() => setPages([]))
  }

  useEffect(load, [id, pid])

  function open(p: Page) {
    setSelected(p)
    setTitle(p.title)
    setBody(p.body)
    setEditing(false)
  }

  async function handleCreate(e: FormEvent) {
    e.preventDefault()
    if (!id || !pid || !newTitle.trim()) return
    try {
      const p = await createPage(id, pid, newTitle.trim(), '')
      setNewTitle('')
      load()
      open(p)
    } catch (err) {
      showError(err)
    }
  }

  async function handleSave() {
    if (!id || !selected) return
    try {
      const updated = await updatePage(id, selected.id, title, body)
      setSelected(updated)
      setEditing(false)
      load()
    } catch (err) {
      showError(err)
    }
  }

  async function handleDelete() {
    if (!id || !selected || !window.confirm('Delete this page?')) return
    try {
      await deletePage(id, selected.id)
      setSelected(null)
      load()
    } catch (err) {
      showError(err)
    }
  }

  if (!project) return <p className="text-sm text-neutral-500">Loading…</p>

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <Link to={`/workspaces/${id}/work/${pid}`} className="text-sm text-neutral-400 hover:text-neutral-200">
            ← {project.name}
          </Link>
          <h2 className="text-xl font-semibold">Docs</h2>
        </div>
      </div>
      {error && <p className="text-sm text-red-400">{error}</p>}

      <div className="grid gap-4 lg:grid-cols-[200px_1fr]">
        {/* Page list */}
        <div className="space-y-1">
          <form onSubmit={handleCreate} className="mb-3">
            <input
              type="text"
              placeholder="New page…"
              maxLength={200}
              value={newTitle}
              onChange={(e) => setNewTitle(e.target.value)}
              className="w-full rounded border border-neutral-700 bg-neutral-950 px-2 py-1.5 text-xs focus:border-neutral-500 focus:outline-none"
            />
          </form>
          {pages === null ? (
            <p className="text-xs text-neutral-500">Loading…</p>
          ) : (
            pages.map((p) => (
              <button
                key={p.id}
                onClick={() => open(p)}
                className={
                  'block w-full truncate rounded px-2 py-1.5 text-left text-sm ' +
                  (selected?.id === p.id
                    ? 'bg-neutral-800 text-neutral-100'
                    : 'text-neutral-400 hover:bg-neutral-900 hover:text-neutral-200')
                }
              >
                {p.title}
              </button>
            ))
          )}
          {pages?.length === 0 && <p className="text-xs text-neutral-600">No pages yet.</p>}
        </div>

        {/* Page editor */}
        <div className="min-h-[400px] rounded-lg border border-neutral-800 bg-neutral-900/40 p-5">
          {!selected ? (
            <p className="pt-16 text-center text-sm text-neutral-500">
              Select a page or create a new one.
            </p>
          ) : editing ? (
            <>
              <input
                type="text"
                value={title}
                maxLength={200}
                onChange={(e) => setTitle(e.target.value)}
                className="mb-3 w-full rounded border border-neutral-700 bg-neutral-950 px-3 py-2 text-lg font-semibold focus:border-neutral-500 focus:outline-none"
              />
              <textarea
                rows={16}
                value={body}
                onChange={(e) => setBody(e.target.value)}
                placeholder="Write in markdown…"
                className="w-full resize-y rounded border border-neutral-700 bg-neutral-950 px-3 py-2 font-mono text-sm focus:border-neutral-500 focus:outline-none"
              />
              <div className="mt-3 flex gap-2">
                <button
                  onClick={handleSave}
                  className="rounded bg-neutral-100 px-4 py-1.5 text-sm font-medium text-neutral-900 hover:bg-white"
                >
                  Save
                </button>
                <button
                  onClick={() => { setEditing(false); open(selected); }}
                  className="rounded border border-neutral-700 px-4 py-1.5 text-sm"
                >
                  Cancel
                </button>
              </div>
            </>
          ) : (
            <>
              <div className="mb-2 flex items-center justify-between">
                <h3 className="text-lg font-semibold">{selected.title}</h3>
                <div className="flex gap-2">
                  <button
                    onClick={() => setEditing(true)}
                    className="rounded border border-neutral-700 px-3 py-1 text-xs hover:border-neutral-500"
                  >
                    Edit
                  </button>
                  <button
                    onClick={handleDelete}
                    className="rounded px-3 py-1 text-xs text-neutral-500 hover:text-red-400"
                  >
                    Delete
                  </button>
                </div>
              </div>
              <p className="text-xs text-neutral-600">
                updated {new Date(selected.updatedAt).toLocaleString()}
              </p>
              <div className="prose prose-invert prose-sm mt-4 max-w-none whitespace-pre-wrap">
                {selected.body || <span className="text-neutral-600">Empty page — click Edit to write.</span>}
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  )
}