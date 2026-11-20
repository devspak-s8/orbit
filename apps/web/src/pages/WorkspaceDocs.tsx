import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { listProjects, listPages, ApiError, type Project, type Page } from '../lib/api'

/**
 * WorkspaceDocs is the orbit.docs module landing page: lists projects
 * with their page counts, and lets you browse pages per project.
 */
export default function WorkspaceDocs() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [projects, setProjects] = useState<Project[] | null>(null)
  const [pages, setPages] = useState<Page[]>([])
  const [selectedProject, setSelectedProject] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!id) return
    listProjects(id).then(setProjects).catch((err) =>
      setError(err instanceof ApiError ? err.message : 'Failed to load'),
    )
    listPages(id).then(setPages).catch(() => {})
  }, [id])

  const visiblePages = selectedProject
    ? pages.filter((p) => p.projectId === selectedProject)
    : pages

  return (
    <div className="space-y-6">
      <div>
        <Link to={`/workspaces/${id}`} className="text-sm text-neutral-400 hover:text-neutral-200">
          ← Workspace
        </Link>
        <h1 className="mt-2 text-2xl font-semibold">Docs</h1>
        <p className="mt-1 max-w-prose text-sm text-neutral-400">
          Pages and notes on your projects — every edit publishes an event your workflows can
          subscribe to.
        </p>
      </div>
      {error && <p className="text-sm text-red-400">{error}</p>}

      {/* Project filter */}
      <div className="flex flex-wrap gap-2 text-xs">
        <button
          onClick={() => setSelectedProject(null)}
          className={
            'rounded-full border px-3 py-1 ' +
            (!selectedProject ? 'border-emerald-700 bg-emerald-900/30 text-emerald-300' : 'border-neutral-800 text-neutral-400')
          }
        >
          All projects
        </button>
        {(projects ?? []).map((p) => (
          <button
            key={p.id}
            onClick={() => setSelectedProject(p.id)}
            className={
              'rounded-full border px-3 py-1 ' +
              (selectedProject === p.id ? 'border-emerald-700 bg-emerald-900/30 text-emerald-300' : 'border-neutral-800 text-neutral-400')
            }
          >
            {p.name}
          </button>
        ))}
      </div>

      {/* Pages */}
      {visiblePages.length === 0 ? (
        <p className="text-sm text-neutral-500">
          No pages yet — open a project and create one from the Docs tab.
        </p>
      ) : (
        <ul className="max-w-2xl space-y-2">
          {visiblePages.map((p) => (
            <li key={p.id}>
              <button
                onClick={() => navigate(`/workspaces/${id}/work/${p.projectId}/docs`)}
                className="block w-full rounded-lg border border-neutral-800 bg-neutral-900/50 px-4 py-3 text-left hover:border-neutral-600"
              >
                <div className="font-medium">{p.title}</div>
                <div className="mt-1 truncate text-xs text-neutral-500">
                  {(projects ?? []).find((pr) => pr.id === p.projectId)?.name ?? '—'} · updated{' '}
                  {new Date(p.updatedAt).toLocaleString()}
                </div>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}