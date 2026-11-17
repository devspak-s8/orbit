import { useEffect, useState, type FormEvent } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  addColumn,
  ApiError,
  createRecord,
  createTable,
  deleteRecord,
  enableForm,
  getTable,
  importCSV,
  listRecords,
  listTables,
  updateRecord,
  type DataColumn,
  type DataRecord,
  type DataTable,
} from '../lib/api'

function TablesList({ workspaceId }: { workspaceId: string }) {
  const [tables, setTables] = useState<DataTable[] | null>(null)
  const [name, setName] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  function load() {
    listTables(workspaceId).then(setTables).catch(showError)
  }

  function showError(err: unknown) {
    setError(err instanceof ApiError ? err.message : 'Something went wrong')
  }

  useEffect(load, [workspaceId])

  async function handleCreate(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      // New tables start with a Name column; add more from the grid.
      await createTable(workspaceId, name, [{ name: 'Name', type: 'text' }])
      setName('')
      load()
    } catch (err) {
      showError(err)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="space-y-6">
      <div>
        <Link to={`/workspaces/${workspaceId}`} className="text-sm text-neutral-400 hover:text-neutral-200">
          ← Workspace
        </Link>
        <h1 className="mt-2 text-2xl font-semibold">Data</h1>
        <p className="mt-1 max-w-prose text-sm text-neutral-400">
          Tables, records, CSV, and public forms. Every new record publishes an event your
          workflows can subscribe to.
        </p>
      </div>

      <form onSubmit={handleCreate} className="flex max-w-lg items-end gap-3">
        <label className="block flex-1 text-sm">
          New table
          <input
            type="text"
            required
            maxLength={120}
            placeholder="e.g. Contacts"
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

      {tables === null ? (
        <p className="text-sm text-neutral-500">Loading tables…</p>
      ) : tables.length === 0 ? (
        <p className="text-sm text-neutral-500">No tables yet — create your first one above.</p>
      ) : (
        <ul className="max-w-3xl space-y-2">
          {tables.map((t) => (
            <li key={t.id}>
              <Link
                to={`/workspaces/${workspaceId}/data/${t.id}`}
                className="block rounded-lg border border-neutral-800 bg-neutral-900/50 px-4 py-3 hover:border-neutral-600"
              >
                <div className="font-medium">{t.name}</div>
                <div className="mt-1 flex gap-3 text-xs text-neutral-500">
                  <span>{t.columns?.length ?? 0} columns</span>
                  {t.formToken && <span className="text-emerald-400">public form enabled</span>}
                </div>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

function renderCell(col: DataColumn, value: unknown): string {
  if (value === null || value === undefined) return ''
  switch (col.type) {
    case 'checkbox':
      return value ? '✓' : '·'
    case 'number':
      return String(value)
    default:
      return String(value)
  }
}

function TableView({ workspaceId, tableId }: { workspaceId: string; tableId: string }) {
  const [table, setTable] = useState<DataTable | null>(null)
  const [records, setRecords] = useState<DataRecord[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [editing, setEditing] = useState<DataRecord | null>(null)
  const [editValues, setEditValues] = useState<Record<string, unknown>>({})
  const [csvOpen, setCsvOpen] = useState(false)
  const [csvText, setCsvText] = useState('')
  const [newColumnName, setNewColumnName] = useState('')
  const [newColumnType, setNewColumnType] = useState('text')

  function showError(err: unknown) {
    setError(err instanceof ApiError ? err.message : 'Something went wrong')
  }

  function load() {
    getTable(workspaceId, tableId).then(setTable).catch(showError)
    listRecords(workspaceId, tableId).then(setRecords).catch(showError)
  }

  useEffect(load, [workspaceId, tableId])

  async function addRow() {
    setBusy(true)
    setError(null)
    try {
      // Empty row via the first text column if present; grid supports
      // fully blank rows because every value is optional.
      await createRecord(workspaceId, tableId, {})
      load()
    } catch (err) {
      showError(err)
    } finally {
      setBusy(false)
    }
  }

  async function saveEdit() {
    if (!editing) return
    setBusy(true)
    try {
      await updateRecord(workspaceId, editing.id, editValues)
      setEditing(null)
      load()
    } catch (err) {
      showError(err)
    } finally {
      setBusy(false)
    }
  }

  async function remove(r: DataRecord) {
    setError(null)
    try {
      await deleteRecord(workspaceId, r.id)
      load()
    } catch (err) {
      showError(err)
    }
  }

  async function handleImport() {
    setBusy(true)
    setError(null)
    try {
      await importCSV(workspaceId, tableId, csvText)
      setCsvOpen(false)
      setCsvText('')
      load()
    } catch (err) {
      showError(err)
    } finally {
      setBusy(false)
    }
  }

  async function handleAddColumn() {
    if (!newColumnName.trim()) return
    setBusy(true)
    setError(null)
    try {
      const body: { name: string; type: string; options?: string[] } = {
        name: newColumnName.trim(),
        type: newColumnType,
      }
      if (newColumnType === 'select') body.options = ['option a', 'option b']
      await addColumn(workspaceId, tableId, body)
      setNewColumnName('')
      load()
    } catch (err) {
      showError(err)
    } finally {
      setBusy(false)
    }
  }

  async function handleForm() {
    setBusy(true)
    setError(null)
    try {
      await enableForm(workspaceId, tableId)
      load()
    } catch (err) {
      showError(err)
    } finally {
      setBusy(false)
    }
  }

  const exportUrl = `/v1/workspaces/${workspaceId}/tables/${tableId}/export`

  if (error && !table) {
    return (
      <div className="space-y-4">
        <p className="text-sm text-red-400">{error}</p>
        <Link to={`/workspaces/${workspaceId}/data`} className="text-sm text-neutral-400 underline">
          ← Data
        </Link>
      </div>
    )
  }
  if (!table || records === null) return <p className="text-sm text-neutral-500">Loading table…</p>
  const cols = table.columns ?? []

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <Link to={`/workspaces/${workspaceId}/data`} className="text-sm text-neutral-400 hover:text-neutral-200">
            ← Data
          </Link>
          <h1 className="mt-1 text-2xl font-semibold">{table.name}</h1>
        </div>
        <div className="flex flex-wrap gap-2 text-sm">
          <button onClick={addRow} disabled={busy} className="rounded border border-neutral-700 px-3 py-1.5 hover:border-neutral-500 disabled:opacity-50">
            + Row
          </button>
          <button onClick={() => setCsvOpen(!csvOpen)} className="rounded border border-neutral-700 px-3 py-1.5 hover:border-neutral-500">
            CSV
          </button>
          <a href={exportUrl} className="rounded border border-neutral-700 px-3 py-1.5 hover:border-neutral-500">
            Export
          </a>
          {!table.formToken && (
            <button onClick={handleForm} disabled={busy} className="rounded bg-neutral-100 px-3 py-1.5 font-medium text-neutral-900 hover:bg-white disabled:opacity-50">
              Enable public form
            </button>
          )}
        </div>
      </div>
      {error && <p className="text-sm text-red-400">{error}</p>}

      {table.formToken && (
        <div className="flex items-center gap-2 rounded border border-emerald-800/60 bg-neutral-900/60 p-2">
          <span className="ml-2 text-xs text-neutral-500">Public form:</span>
          <code className="flex-1 overflow-x-auto font-mono text-xs text-emerald-400">
            {window.location.origin}/forms/{table.formToken}
          </code>
          <button
            onClick={() => void navigator.clipboard.writeText(`${window.location.origin}/forms/${table.formToken}`)}
            className="rounded border border-neutral-700 px-2 py-1 text-xs hover:border-neutral-500"
          >
            Copy
          </button>
        </div>
      )}

      {csvOpen && (
        <div className="rounded-lg border border-neutral-800 bg-neutral-900/50 p-4">
          <div className="mb-2 text-sm font-medium">Import CSV</div>
          <p className="mb-2 text-xs text-neutral-500">
            First row must be a header with exact column names (order doesn't matter). Max 1000 rows.
          </p>
          <textarea
            rows={5}
            spellCheck={false}
            value={csvText}
            onChange={(e) => setCsvText(e.target.value)}
            placeholder={`Name,${cols.map((c) => c.name).join(',')}`}
            className="w-full rounded border border-neutral-700 bg-neutral-950 px-3 py-2 font-mono text-xs focus:border-neutral-500 focus:outline-none"
          />
          <button
            onClick={handleImport}
            disabled={busy || !csvText.trim()}
            className="mt-2 rounded bg-neutral-100 px-3 py-1.5 text-sm font-medium text-neutral-900 hover:bg-white disabled:opacity-50"
          >
            Import
          </button>
        </div>
      )}

      {/* Grid */}
      <div className="overflow-x-auto rounded-lg border border-neutral-800">
        <table className="min-w-full text-sm">
          <thead>
            <tr className="border-b border-neutral-800 bg-neutral-900/60 text-left text-xs text-neutral-400">
              {cols.map((c) => (
                <th key={c.id} className="px-3 py-2 font-medium">
                  {c.name}
                  <span className="ml-1 text-[10px] text-neutral-600">{c.type}</span>
                </th>
              ))}
              <th className="px-3 py-2" aria-label="Actions" />
            </tr>
          </thead>
          <tbody>
            {records.map((r) => (
              <tr key={r.id} className="border-b border-neutral-800/50 hover:bg-neutral-900/40">
                {cols.map((c) => (
                  <td key={c.id} className="px-3 py-1.5">
                    {editing?.id === r.id ? (
                      c.type === 'checkbox' ? (
                        <input
                          type="checkbox"
                          checked={Boolean(editValues[c.id])}
                          onChange={(e) => setEditValues({ ...editValues, [c.id]: e.target.checked })}
                        />
                      ) : (
                        <input
                          type={c.type === 'number' ? 'number' : c.type === 'date' ? 'date' : 'text'}
                          value={String(editValues[c.id] ?? '')}
                          onChange={(e) =>
                            setEditValues({
                              ...editValues,
                              [c.id]: c.type === 'number' ? Number(e.target.value) : e.target.value,
                            })
                          }
                          className="w-full rounded border border-neutral-700 bg-neutral-950 px-2 py-1 text-sm focus:border-neutral-500 focus:outline-none"
                        />
                      )
                    ) : (
                      <span className={c.type === 'checkbox' && r.values[c.id] ? 'text-emerald-400' : ''}>
                        {renderCell(c, r.values[c.id])}
                      </span>
                    )}
                  </td>
                ))}
                <td className="px-3 py-1.5 text-right">
                  {editing?.id === r.id ? (
                    <button onClick={saveEdit} disabled={busy} className="text-xs text-emerald-400 hover:text-emerald-300">
                      Save
                    </button>
                  ) : (
                    <>
                      <button
                        onClick={() => {
                          setEditing(r)
                          setEditValues({ ...r.values })
                        }}
                        className="text-xs text-neutral-400 hover:text-neutral-100"
                      >
                        Edit
                      </button>
                      <button onClick={() => remove(r)} className="ml-2 text-xs text-neutral-500 hover:text-red-400">
                        Delete
                      </button>
                    </>
                  )}
                </td>
              </tr>
            ))}
            {records.length === 0 && (
              <tr>
                <td colSpan={cols.length + 1} className="px-3 py-6 text-center text-sm text-neutral-500">
                  No records yet — add a row or import CSV.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {/* Add column */}
      <div className="flex items-end gap-2 border-t border-neutral-800 pt-4">
        <label className="block text-xs">
          Add column
          <input
            type="text"
            placeholder="e.g. Priority"
            value={newColumnName}
            onChange={(e) => setNewColumnName(e.target.value)}
            className="mt-1 block w-40 rounded border border-neutral-700 bg-neutral-950 px-2 py-1.5 text-sm focus:border-neutral-500 focus:outline-none"
          />
        </label>
        <select
          value={newColumnType}
          onChange={(e) => setNewColumnType(e.target.value)}
          className="rounded border border-neutral-700 bg-neutral-950 px-2 py-1.5 text-sm"
        >
          <option value="text">text</option>
          <option value="number">number</option>
          <option value="date">date</option>
          <option value="select">select</option>
          <option value="checkbox">checkbox</option>
        </select>
        <button
          onClick={handleAddColumn}
          disabled={busy || !newColumnName.trim()}
          className="rounded border border-neutral-700 px-3 py-1.5 text-sm hover:border-neutral-500 disabled:opacity-50"
        >
          Add
        </button>
      </div>
    </div>
  )
}

export default function DataModule() {
  const { id, tid } = useParams<{ id: string; tid: string }>()
  if (tid) return <TableView workspaceId={id!} tableId={tid} />
  return <TablesList workspaceId={id!} />
}