import { Link } from 'react-router-dom'
import { Eyebrow, PageHero, SiteFooter, SiteNav } from '../components/SiteChrome'

function CodeBlock({ title, children }: { title: string; children: string }) {
  return (
    <div className="overflow-hidden rounded-xl border border-neutral-800 bg-neutral-900/70">
      <div className="border-b border-neutral-800 px-4 py-2 text-[11px] uppercase tracking-wider text-neutral-500">
        {title}
      </div>
      <pre className="code-block overflow-x-auto px-4 py-3 text-neutral-300">{children}</pre>
    </div>
  )
}

function FeatureBlock({
  name,
  tag,
  desc,
  points,
  visual,
}: {
  name: string
  tag?: string
  desc: string
  points: string[]
  visual?: React.ReactNode
}) {
  return (
    <section className="grid items-center gap-10 border-b border-neutral-800/60 py-14 last:border-0 lg:grid-cols-2">
      <div>
        <div className="flex items-center gap-3">
          <h3 className="text-xl font-semibold">{name}</h3>
          {tag && (
            <span className="rounded-full border border-amber-700/70 px-2 py-0.5 text-[11px] text-amber-400">
              {tag}
            </span>
          )}
        </div>
        <p className="mt-3 text-sm leading-relaxed text-neutral-400">{desc}</p>
        <ul className="mt-5 space-y-2 text-sm text-neutral-300">
          {points.map((p) => (
            <li key={p} className="flex gap-2.5">
              <span aria-hidden="true" className="mt-1 text-emerald-400">
                ✓
              </span>
              {p}
            </li>
          ))}
        </ul>
      </div>
      <div className="lg:justify-self-end">{visual}</div>
    </section>
  )
}

function BoardMock() {
  const cols = [
    ['To Do', ['Draft announcement', 'Backup prod DB', 2]],
    ['In Progress', ['Test the invite flow', 1]],
    ['Done', ['Ship v0.3.1', 1]],
  ] as Array<[string, Array<string | number>]>
  return (
    <div className="w-full max-w-md rounded-xl border border-neutral-800 bg-neutral-900/50 p-4 shadow-xl">
      <div className="grid grid-cols-3 gap-3">
        {cols.map(([label, tasks]) => (
          <div key={label} className="rounded-lg bg-neutral-950/60 p-2.5">
            <div className="mb-2 flex items-center justify-between text-[11px] font-medium text-neutral-400">
              {label}
              <span className="text-neutral-600">{tasks[tasks.length - 1]}</span>
            </div>
            <div className="space-y-1.5">
              {tasks
                .slice(0, -1)
                .map((t, i) => (
                  <div
                    key={t as string}
                    className={
                      'rounded border px-2 py-1.5 text-[11px] ' +
                      (i === 0 && label === 'Done'
                        ? 'border-emerald-800/60 bg-emerald-900/20 text-emerald-300'
                        : 'border-neutral-800 bg-neutral-900 text-neutral-300')
                    }
                  >
                    {t}
                  </div>
                ))}
            </div>
          </div>
        ))}
      </div>
      <div className="mt-3 flex items-center justify-between border-t border-neutral-800 pt-2.5 text-[10px] text-neutral-500">
        <span>UI Test Studio · 2 members</span>
        <span className="flex items-center gap-1 text-emerald-400">
          <span className="anim-dot h-1 w-1 rounded-full bg-emerald-400" />1 workflow watching this board
        </span>
      </div>
    </div>
  )
}

function TriggerTable() {
  const triggers: Array<[string, string]> = [
    ['Platform event', 'module events, transactional with the change that caused them'],
    ['Webhook', 'secret URL; any JSON POST from anywhere runs the workflow'],
    ['Schedule', 'intervals from 60s to 7d; restart-safe cursor, no double-fires'],
    ['Manual', 'one click from the UI or one call from the API'],
  ]
  const nodes: Array<[string, string]> = [
    ['HTTP', 'method, URL, headers, JSON body — {{payload.*}} substitution everywhere'],
    ['Email', 'plain-text over your SMTP relay; unconfigured fails loudly, never silently'],
    ['Log', 'checkpoint a message with substituted values into the run history'],
  ]
  return (
    <div className="w-full max-w-md space-y-4">
      <div className="rounded-xl border border-neutral-800 bg-neutral-900/50 p-4">
        <div className="mb-3 text-[11px] font-semibold uppercase tracking-wider text-emerald-400/80">
          Triggers
        </div>
        <ul className="space-y-2.5">
          {triggers.map(([t, d]) => (
            <li key={t} className="text-sm">
              <span className="font-medium text-neutral-200">{t}</span>
              <span className="block text-xs leading-relaxed text-neutral-500">{d}</span>
            </li>
          ))}
        </ul>
      </div>
      <div className="rounded-xl border border-neutral-800 bg-neutral-900/50 p-4">
        <div className="mb-3 text-[11px] font-semibold uppercase tracking-wider text-emerald-400/80">
          Nodes
        </div>
        <ul className="space-y-2.5">
          {nodes.map(([t, d]) => (
            <li key={t} className="text-sm">
              <span className="font-medium text-neutral-200">{t}</span>
              <span className="block text-xs leading-relaxed text-neutral-500">{d}</span>
            </li>
          ))}
        </ul>
      </div>
    </div>
  )
}

function TableMock() {
  return (
    <div className="w-full max-w-md overflow-hidden rounded-xl border border-neutral-800 bg-neutral-900/50 shadow-xl">
      <div className="flex items-center justify-between border-b border-neutral-800 px-4 py-2.5 text-[11px] text-neutral-500">
        <span>Contacts — 128 records</span>
        <span className="rounded-full border border-amber-700/60 px-2 py-0.5 text-[10px] text-amber-400">
          in development
        </span>
      </div>
      <table className="w-full text-left text-[11px]">
        <thead>
          <tr className="border-b border-neutral-800 text-neutral-500">
            <th className="px-4 py-2 font-medium">Name</th>
            <th className="px-4 py-2 font-medium">Email</th>
            <th className="px-4 py-2 font-medium">Source</th>
          </tr>
        </thead>
        <tbody className="text-neutral-300">
          {[
            ['Ada Lovelace', 'ada@example.com', 'form'],
            ['Alan Turing', 'alan@example.com', 'csv'],
            ['Grace Hopper', 'grace@example.com', 'form'],
          ].map((r) => (
            <tr key={r[0]} className="border-b border-neutral-800/50 last:border-0">
              <td className="px-4 py-2">{r[0]}</td>
              <td className="px-4 py-2">{r[1]}</td>
              <td className="px-4 py-2">
                <span
                  className={
                    'rounded px-1.5 py-0.5 text-[10px] ' +
                    (r[2] === 'form' ? 'bg-emerald-900/40 text-emerald-300' : 'bg-neutral-800 text-neutral-400')
                  }
                >
                  {r[2]}
                </span>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

export default function Features() {
  return (
    <div className="min-h-screen bg-neutral-950 text-neutral-100">
      <SiteNav />
      <PageHero
        eyebrow="Features"
        title="Everything ships as a module — and every module speaks to the engine."
        lead="ORBIT is not a suite of bundled apps; it's one platform where independently installable modules share identity, permissions, events, and automation. Here's what's running today."
      />

      <main className="mx-auto max-w-6xl px-6 py-8">
        <FeatureBlock
          name="Work Management"
          desc="The day-to-day surface: projects, task boards, assignments, and due dates. Its real power is underneath — every task lifecycle change publishes a transactional event the automation engine can subscribe to."
          points={[
            'Projects with task boards: To Do / In Progress / Done',
            'Drag-and-drop plus keyboard-accessible move controls',
            'Assignments notify the assignee in-app, in the same transaction',
            'Events: task.created, task.completed, task.reopened, task.assigned',
            'Shareable invite links; role chips; member management',
          ]}
          visual={<BoardMock />}
        />

        <FeatureBlock
          name="Automation Studio"
          desc="The flagship. Workflows are versioned JSON documents: a trigger, a linear chain of nodes, per-node retries and timeouts. Only validated workflows ever run — editing and execution are strictly separated."
          points={[
            'Durable execution: per-step checkpoints, restart resumes where it stopped',
            'Retries with backoff; failures are inspectable down to the node output',
            'Webhook triggers: secret URLs, rate-limited, no existence leak on 404',
            'Schedules: persisted next-due cursor — restarts never double-fire',
            'Full execution history with inputs, outputs, and timings',
          ]}
          visual={<TriggerTable />}
        />

        <FeatureBlock
          name="Data"
          tag="in development"
          desc="Tables with typed columns, records with CSV import/export, and public forms on secret URLs. The chain that sells the whole platform: form → record → event → workflow → email."
          points={[
            'Text, number, date, select, and checkbox columns',
            'Inline grid editing; filter and sort',
            'Public forms — the webhooks pattern applied to data collection',
            'data.record.created events feed existing workflows, zero new engine code',
          ]}
          visual={<TableMock />}
        />

        <FeatureBlock
          name="The platform core"
          desc="What every module inherits for free: accounts, tenant-isolated workspaces, a module registry with install/enable lifecycle, a transactional event log, and an audit trail."
          points={[
            'Argon2id passwords, revocable server-side sessions, rate limiting',
            'Workspace isolation enforced at the data layer — cross-tenant reads are impossible by construction',
            'Module manifests declare permissions and events; uninstall cleans up after itself',
            'Every change writes an audit row; every event commits with its change',
          ]}
          visual={
            <CodeBlock title="wire a webhook from the terminal">{`# any JSON POST runs your workflow
curl -X POST -H "Content-Type: application/json" \\
  -d '{"name":"Ada","message":"hello"}' \\
  http://your-orbit/hooks/<secret-token>

# -> 202 {"runId":"…"} — watch it complete
#    in Execution History, payload substituted`}</CodeBlock>
          }
        />
      </main>

      <section className="border-t border-neutral-800/60 py-16">
        <div className="mx-auto max-w-3xl px-6 text-center">
          <Eyebrow>Try it</Eyebrow>
          <h2 className="text-2xl font-semibold">See the loop on your own server.</h2>
          <div className="mt-6 flex flex-wrap items-center justify-center gap-3">
            <Link
              to="/deploy"
              className="rounded-lg bg-emerald-500 px-6 py-3 text-sm font-semibold text-black hover:bg-emerald-400"
            >
              Deploy in minutes
            </Link>
            <Link
              to="/signup"
              className="rounded-lg border border-neutral-700 px-6 py-3 text-sm font-medium hover:border-neutral-400"
            >
              Create a workspace
            </Link>
          </div>
        </div>
      </section>

      <SiteFooter />
    </div>
  )
}