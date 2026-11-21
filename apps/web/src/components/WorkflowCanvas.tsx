import { useEffect, useRef, useState } from 'react'
import {
  listConnectors,
  type Connector,
  type WorkflowDefinition,
} from '../lib/api'

/**
 * A minimal, functional workflow graph editor: nodes as positioned cards,
 * SVG edges between them, click-to-connect, drag-to-pan. No external
 * libraries. Saves through the same PATCH endpoint as the JSON editor.
 * HTTP nodes can be connector presets (Slack, Discord, Telegram, generic
 * webhook): the form is driven by the server's catalog.
 */

type NodePos = { x: number; y: number }

const NODE_W = 160
const NODE_H = 60
const NODE_GAP = 80

const NODE_COLORS: Record<string, string> = {
  http: 'border-sky-700 bg-sky-900/20 text-sky-300',
  log: 'border-neutral-600 bg-neutral-800 text-neutral-300',
  email: 'border-amber-700 bg-amber-900/20 text-amber-300',
  branch: 'border-violet-700 bg-violet-900/20 text-violet-300',
}

// The catalog is static server data; cache it for the session.
let connectorCache: Connector[] | null = null

// Layout: auto-arrange nodes left-to-right from the trigger
function autoLayout(def: WorkflowDefinition): Map<string, NodePos> {
  const positions = new Map<string, NodePos>()
  // Build adjacency
  const next = new Map<string, string>()
  for (const e of def.edges) {
    if (e.from === 'trigger' || !next.has(e.from)) {
      next.set(e.from, e.to)
    }
  }
  // Walk from trigger
  let cur = 'trigger'
  let x = 40
  const yBase = 120
  let alt = 0
  const visited = new Set<string>()
  while (cur && !visited.has(cur)) {
    visited.add(cur)
    const n = next.get(cur)
    if (n) {
      const isBranch = def.nodes.find((nd) => nd.id === n)?.type === 'branch'
      positions.set(n, { x, y: yBase + alt * (NODE_H + 40) })
      if (isBranch) {
        // The branch's two children go up/down
        const branchEdges = def.edges.filter((e) => e.from === n)
        if (branchEdges.length === 2) {
          positions.set(branchEdges[0].to, { x: x + NODE_W + NODE_GAP, y: yBase - 80 })
          positions.set(branchEdges[1].to, { x: x + NODE_W + NODE_GAP, y: yBase + 80 })
          alt = 1
        }
      }
      cur = n
      x += NODE_W + NODE_GAP
    } else {
      break
    }
  }
  // Any unpositioned nodes go in a row at the bottom
  for (const nd of def.nodes) {
    if (!positions.has(nd.id)) {
      positions.set(nd.id, { x: 40 + positions.size * (NODE_W + 20), y: 320 })
    }
  }
  return positions
}

export function WorkflowCanvas({
  definition,
  onSave,
}: {
  definition: WorkflowDefinition
  onSave: (def: WorkflowDefinition) => Promise<void>
}) {
  const [def, setDef] = useState<WorkflowDefinition>(definition)
  const [positions, setPositions] = useState<Map<string, NodePos>>(() => autoLayout(definition))
  const [selected, setSelected] = useState<string | null>(null)
  const [connecting, setConnecting] = useState<string | null>(null)
  const [dirty, setDirty] = useState(false)
  const [saving, setSaving] = useState(false)
  const [connectors, setConnectors] = useState<Connector[] | null>(connectorCache)
  const svgRef = useRef<SVGSVGElement>(null)
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    setDef(definition)
    setPositions(autoLayout(definition))
    setDirty(false)
  }, [definition])

  useEffect(() => {
    if (connectorCache) return
    let alive = true
    listConnectors()
      .then((cs) => {
        connectorCache = cs
        if (alive) setConnectors(cs)
      })
      .catch(() => {})
    return () => {
      alive = false
    }
  }, [])

  const handleNodeClick = (id: string) => {
    if (connecting && connecting !== id) {
      // Create an edge from `connecting` to `id`
      const newEdges = def.edges.filter((e) => e.from !== connecting)
      // If source is a branch, alternate labels
      const srcNode = def.nodes.find((n) => n.id === connecting)
      if (srcNode?.type === 'branch') {
        const existingYes = newEdges.some((e) => e.from === connecting && e.label === 'yes')
        newEdges.push({ from: connecting, to: id, label: existingYes ? 'no' : 'yes' })
      } else {
        // Remove any existing outgoing edge from non-branch nodes
        const filtered = newEdges.filter((e) => e.from !== connecting)
        filtered.push({ from: connecting, to: id })
        newEdges.length = 0
        newEdges.push(...filtered)
      }
      setDef({ ...def, edges: newEdges })
      setConnecting(null)
      setDirty(true)
    } else {
      setSelected(id)
      setConnecting(null)
    }
  }

  const handleOutputClick = (id: string, e: React.MouseEvent) => {
    e.stopPropagation()
    setConnecting(id)
  }

  const addNode = (type: 'http' | 'log' | 'email' | 'branch') => {
    const id = `n${Date.now()}`
    const node = {
      id,
      type,
      config: type === 'http'
        ? { method: 'GET', url: 'https://example.com' }
        : type === 'log'
          ? { message: 'hello' }
          : type === 'email'
            ? { to: 'you@example.com', subject: 'Hi' }
            : { condition: { field: '{{payload.value}}', op: '==', value: 'x' } },
    }
    setDef({
      ...def,
      nodes: [...def.nodes, { id, type, config: node.config }],
    })
    const p = positions
    setPositions(new Map([...p, [id, { x: 40 + p.size * (NODE_W + 20), y: 260 }]]))
    setDirty(true)
  }

  const addConnectorNode = (c: Connector) => {
    const id = `n${Date.now()}`
    setDef({
      ...def,
      nodes: [
        ...def.nodes,
        { id, type: 'http', config: { connector: c.id, params: {} } },
      ],
    })
    const p = positions
    setPositions(new Map([...p, [id, { x: 40 + p.size * (NODE_W + 20), y: 260 }]]))
    setSelected(id)
    setDirty(true)
  }

  const deleteNode = (id: string) => {
    setDef({
      ...def,
      nodes: def.nodes.filter((n) => n.id !== id),
      edges: def.edges.filter((e) => e.from !== id && e.to !== id),
    })
    setSelected(null)
    setDirty(true)
  }

  const setParam = (nodeId: string, key: string, value: string) => {
    setDef({
      ...def,
      nodes: def.nodes.map((n) =>
        n.id === nodeId
          ? { ...n, config: { ...n.config, params: { ...n.config.params, [key]: value } } }
          : n,
      ),
    })
    setDirty(true)
  }

  const handleSave = async () => {
    setSaving(true)
    try {
      await onSave(def)
      setDirty(false)
    } finally {
      setSaving(false)
    }
  }

  const selNode = def.nodes.find((n) => n.id === selected)
  const selConnector = selNode?.config.connector
    ? connectors?.find((c) => c.id === selNode.config.connector)
    : undefined
  const nodeTitle = (n: (typeof def.nodes)[number]) =>
    n.config.connector
      ? connectors?.find((c) => c.id === n.config.connector)?.name ?? n.config.connector
      : n.type

  return (
    <div className="space-y-3">
      {/* Toolbar */}
      <div className="flex flex-wrap items-center gap-2 text-xs">
        <select
          onChange={(e) => {
            const c = connectors?.find((x) => x.id === e.target.value)
            if (c) addConnectorNode(c)
            e.target.value = ''
          }}
          className="rounded border border-emerald-700 bg-emerald-900/20 px-2 py-1 text-emerald-300 hover:bg-emerald-900/40"
          aria-label="Add a connector preset node"
        >
          <option value="">+ Connector…</option>
          {connectors?.map((c) => (
            <option key={c.id} value={c.id} className="bg-neutral-900">
              {c.name}
            </option>
          ))}
        </select>
        <button onClick={() => addNode('http')} className="rounded border border-sky-700 bg-sky-900/20 px-2 py-1 text-sky-300 hover:bg-sky-900/40">
          + HTTP
        </button>
        <button onClick={() => addNode('log')} className="rounded border border-neutral-600 bg-neutral-800 px-2 py-1 text-neutral-300 hover:bg-neutral-700">
          + Log
        </button>
        <button onClick={() => addNode('email')} className="rounded border border-amber-700 bg-amber-900/20 px-2 py-1 text-amber-300 hover:bg-amber-900/40">
          + Email
        </button>
        <button onClick={() => addNode('branch')} className="rounded border border-violet-700 bg-violet-900/20 px-2 py-1 text-violet-300 hover:bg-violet-900/40">
          + Branch
        </button>
        {connecting && <span className="text-violet-400">Click a target node to connect from "{connecting}"…</span>}
        {dirty && (
          <button
            onClick={handleSave}
            disabled={saving}
            className="ml-auto rounded bg-emerald-500 px-3 py-1 font-medium text-black hover:bg-emerald-400 disabled:opacity-50"
          >
            {saving ? 'Saving…' : 'Save (returns to draft)'}
          </button>
        )}
      </div>

      <div className="flex gap-4">
        {/* Canvas */}
        <div
          ref={containerRef}
          className="relative flex-1 overflow-auto rounded-lg border border-neutral-800 bg-neutral-950"
          style={{ minHeight: 400 }}
        >
          <svg ref={svgRef} className="absolute inset-0 h-full w-full" style={{ pointerEvents: 'none' }}>
            {def.edges.map((e, i) => {
              const from = positions.get(e.from) ?? { x: 0, y: 0 }
              const to = positions.get(e.to) ?? { x: 0, y: 0 }
              const x1 = from.x + NODE_W
              const y1 = from.y + NODE_H / 2
              const x2 = to.x
              const y2 = to.y + NODE_H / 2
              const mid = (x1 + x2) / 2
              return (
                <g key={i}>
                  <path
                    d={`M ${x1} ${y1} C ${mid} ${y1}, ${mid} ${y2}, ${x2} ${y2}`}
                    fill="none"
                    stroke={e.label === 'yes' ? '#059669' : e.label === 'no' ? '#dc2626' : '#525252'}
                    strokeWidth={2}
                  />
                  {e.label && (
                    <text x={mid} y={(y1 + y2) / 2 - 6} textAnchor="middle" fontSize={10} fill={e.label === 'yes' ? '#34d399' : '#f87171'}>
                      {e.label}
                    </text>
                  )}
                </g>
              )
            })}
          </svg>

          {/* Trigger node */}
          <div
            className="absolute cursor-pointer rounded-lg border border-emerald-700 bg-emerald-900/30 px-3 py-2 text-xs font-medium text-emerald-300"
            style={{ left: 40, top: 120 + NODE_H / 2 - 20, width: 100 }}
            onClick={() => setConnecting('trigger')}
          >
            ▶ Trigger
            <div className="text-[10px] text-emerald-500">{def.trigger.type}</div>
          </div>

          {/* Nodes */}
          {def.nodes.map((n) => {
            const pos = positions.get(n.id) ?? { x: 0, y: 0 }
            return (
              <div key={n.id}>
                <div
                  className={
                    'absolute cursor-pointer rounded-lg border px-3 py-2 text-xs transition-shadow ' +
                    (n.config.connector
                      ? 'border-emerald-700 bg-emerald-900/20 text-emerald-300'
                      : NODE_COLORS[n.type] ?? NODE_COLORS.log) +
                    (selected === n.id ? ' ring-2 ring-white/30' : '') +
                    (connecting === n.id ? ' ring-2 ring-violet-500' : '')
                  }
                  style={{ left: pos.x, top: pos.y, width: NODE_W, minHeight: NODE_H }}
                  onClick={() => handleNodeClick(n.id)}
                >
                  <div className="font-medium">{nodeTitle(n)}</div>
                  <div className="truncate text-[10px] text-neutral-500">{n.id}</div>
                  {/* Output handle */}
                  <button
                    onClick={(e) => handleOutputClick(n.id, e)}
                    className="absolute -right-2 top-1/2 h-4 w-4 -translate-y-1/2 rounded-full border-2 border-neutral-500 bg-neutral-900 hover:border-emerald-400"
                    aria-label="Connect from this node"
                  />
                </div>
              </div>
            )
          })}
        </div>

        {/* Properties panel */}
        {selected && selNode ? (
          <div className="w-72 shrink-0 space-y-3 rounded-lg border border-neutral-800 bg-neutral-900/60 p-3">
            <div className="flex items-center justify-between">
              <span className="text-xs font-semibold uppercase tracking-wider text-neutral-500">
                {selConnector ? `${selConnector.name} preset` : `${selNode.type} node`}
              </span>
              <button onClick={() => deleteNode(selected)} className="text-xs text-red-400 hover:text-red-300">
                Delete
              </button>
            </div>

            {selConnector ? (
              <div className="space-y-3">
                {selConnector.fields.map((f) => (
                  <label key={f.key} className="block space-y-1">
                    <span className="text-xs font-medium text-neutral-300">
                      {f.label}
                      {f.required && <span className="ml-0.5 text-red-400">*</span>}
                    </span>
                    {f.type === 'textarea' ? (
                      <textarea
                        rows={3}
                        value={selNode.config.params?.[f.key] ?? ''}
                        placeholder={f.placeholder}
                        onChange={(e) => setParam(selected, f.key, e.target.value)}
                        className="w-full rounded border border-neutral-700 bg-neutral-950 px-2 py-2 text-xs focus:border-emerald-600 focus:outline-none"
                      />
                    ) : (
                      <input
                        type="text"
                        value={selNode.config.params?.[f.key] ?? ''}
                        placeholder={f.placeholder}
                        onChange={(e) => setParam(selected, f.key, e.target.value)}
                        className="w-full rounded border border-neutral-700 bg-neutral-950 px-2 py-1.5 font-mono text-xs focus:border-emerald-600 focus:outline-none"
                      />
                    )}
                    {f.description && (
                      <span className="block text-[10px] text-neutral-600">{f.description}</span>
                    )}
                  </label>
                ))}
                <p className="text-[10px] text-neutral-600">
                  <span className="text-neutral-400">{'{{payload.field}}'}</span> tokens substitute
                  values from the trigger at run time.
                </p>
              </div>
            ) : (
              <>
                <textarea
                  rows={8}
                  defaultValue={JSON.stringify(selNode.config, null, 2)}
                  onBlur={(e) => {
                    try {
                      const config = JSON.parse(e.target.value)
                      setDef({
                        ...def,
                        nodes: def.nodes.map((n) => (n.id === selected ? { ...n, config } : n)),
                      })
                      setDirty(true)
                    } catch {
                      // invalid JSON — keep the old config
                    }
                  }}
                  className="w-full rounded border border-neutral-700 bg-neutral-950 px-2 py-2 font-mono text-[11px] focus:border-neutral-500 focus:outline-none"
                />
                <p className="text-[10px] text-neutral-600">
                  Edit the node config as JSON. Changes save with the canvas.
                </p>
              </>
            )}
          </div>
        ) : null}
      </div>
    </div>
  )
}
