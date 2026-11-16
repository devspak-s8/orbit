import { Link } from 'react-router-dom'
import { Eyebrow, GITHUB_URL, SiteFooter, SiteNav } from '../components/SiteChrome'

/* --- Hero orbit visual: pure CSS rings with orbiting module dots --- */
function OrbitVisual() {
  const modules = [
    { label: 'Work', offset: '0deg', ring: 1 },
    { label: 'Data', offset: '140deg', ring: 2 },
    { label: 'Automation', offset: '260deg', ring: 2 },
    { label: 'Demo', offset: '200deg', ring: 1 },
  ]
  return (
    <div className="relative mx-auto aspect-square w-full max-w-sm" role="img" aria-label="Modules orbiting the ORBIT core">
      {/* Core */}
      <div className="absolute left-1/2 top-1/2 flex h-20 w-20 -translate-x-1/2 -translate-y-1/2 items-center justify-center rounded-full border border-emerald-700/50 bg-neutral-900 shadow-[0_0_60px_rgba(16,185,129,0.25)]">
        <span className="text-sm font-semibold tracking-widest text-emerald-400">ORBIT</span>
      </div>
      {/* Rings + orbiting dots */}
      <div className="anim-orbit absolute inset-0 rounded-full border border-neutral-800">
        {modules
          .filter((m) => m.ring === 1)
          .map((m) => (
            <span
              key={m.label}
              className="absolute left-1/2 top-1/2"
              style={{ transform: `rotate(${m.offset}) translateX(50%) translateY(-50%)` }}
            >
              <span
                className="absolute -translate-x-1/2 -translate-y-1/2 rounded-full border border-neutral-700 bg-neutral-900 px-2 py-0.5 text-[10px] text-neutral-300"
                style={{ transform: `translate(-50%,-50%) rotate(-${m.offset})`, marginLeft: '100%' }}
              >
                {m.label}
              </span>
            </span>
          ))}
      </div>
      <div className="anim-orbit-rev absolute inset-8 rounded-full border border-dashed border-neutral-800">
        {modules
          .filter((m) => m.ring === 2)
          .map((m) => (
            <span
              key={m.label}
              className="absolute left-1/2 top-1/2"
              style={{ transform: `rotate(${m.offset})` }}
            >
              <span className="absolute left-1/2 -ml-8 -mt-3 rounded-full border border-neutral-700 bg-neutral-900 px-2 py-0.5 text-[10px] text-neutral-300">
                {m.label}
              </span>
            </span>
          ))}
      </div>
      {/* Fixed stars */}
      <span aria-hidden="true" className="anim-dot absolute left-[18%] top-[22%] h-1 w-1 rounded-full bg-emerald-400" />
      <span aria-hidden="true" className="anim-dot absolute right-[14%] top-[30%] h-1.5 w-1.5 rounded-full bg-neutral-500" />
      <span aria-hidden="true" className="anim-dot absolute bottom-[18%] left-[30%] h-1 w-1 rounded-full bg-emerald-400" />
    </div>
  )
}

/* --- The loop as a live-looking console --- */
function LoopConsole() {
  const lines: Array<[string, string]> = [
    ['POST', '/hooks/xKq9…  {"name":"Ada"}'],
    ['event', 'form.received → data.record.created'],
    ['run', 'workflow #482 started (trigger: event)'],
    ['http', 'GET /v1/ping → 200 in 41ms'],
    ['email', '→ ada@example.com "Thanks for reaching out"'],
  ]
  return (
    <div className="overflow-hidden rounded-xl border border-neutral-800 bg-neutral-900/60 shadow-2xl">
      <div className="flex items-center gap-2 border-b border-neutral-800 bg-neutral-900 px-4 py-2.5">
        <span className="h-2.5 w-2.5 rounded-full bg-neutral-700" />
        <span className="h-2.5 w-2.5 rounded-full bg-neutral-700" />
        <span className="h-2.5 w-2.5 rounded-full bg-neutral-700" />
        <span className="ml-2 text-[11px] text-neutral-500">orbit — execution history</span>
        <span className="ml-auto flex items-center gap-1.5 text-[11px] text-emerald-400">
          <span className="anim-dot h-1.5 w-1.5 rounded-full bg-emerald-400" /> worker online
        </span>
      </div>
      <ol className="code-block space-y-2 px-5 py-4">
        {lines.map(([tag, text], i) => (
          <li key={tag} className="anim-fade-up flex gap-3" style={{ animationDelay: `${0.3 + i * 0.15}s` }}>
            <span
              className={
                'w-14 shrink-0 text-right ' +
                (tag === 'POST' ? 'text-sky-400' : tag === 'http' ? 'text-amber-400' : 'text-emerald-400')
              }
            >
              {tag}
            </span>
            <span className="text-neutral-400">{text}</span>
          </li>
        ))}
        <li className="flex gap-3">
          <span className="w-14 shrink-0 text-right text-neutral-500">done</span>
          <span className="text-neutral-500">completed in 0.4s · restart-safe · audited</span>
        </li>
      </ol>
    </div>
  )
}

function Pillar({
  icon,
  title,
  children,
}: {
  icon: React.ReactNode
  title: string
  children: React.ReactNode
}) {
  return (
    <div className="group rounded-xl border border-neutral-800 bg-neutral-900/40 p-6 transition-all hover:-translate-y-1 hover:border-emerald-800/60 hover:shadow-[0_8px_40px_rgba(16,185,129,0.08)]">
      <div className="flex h-9 w-9 items-center justify-center rounded-lg border border-emerald-900/60 bg-emerald-900/20 text-emerald-400">
        {icon}
      </div>
      <h3 className="mt-4 text-base font-semibold">{title}</h3>
      <p className="mt-2 text-sm leading-relaxed text-neutral-400">{children}</p>
    </div>
  )
}

export default function Landing() {
  return (
    <div className="min-h-screen bg-neutral-950 text-neutral-100">
      <SiteNav />

      {/* Hero */}
      <section className="relative overflow-hidden">
        <div
          aria-hidden="true"
          className="pointer-events-none absolute inset-0 bg-[radial-gradient(ellipse_80%_50%_at_50%_-10%,rgba(16,185,129,0.14),transparent)]"
        />
        <div
          aria-hidden="true"
          className="pointer-events-none absolute inset-0 opacity-[0.35] [background-image:linear-gradient(rgba(255,255,255,0.03)_1px,transparent_1px),linear-gradient(90deg,rgba(255,255,255,0.03)_1px,transparent_1px)] [background-size:64px_64px]"
        />
        <div className="relative mx-auto grid max-w-6xl items-center gap-12 px-6 py-16 sm:py-24 lg:grid-cols-2">
          <div>
            <div className="anim-fade-up inline-flex items-center gap-2 rounded-full border border-emerald-800/60 bg-emerald-900/10 px-3 py-1 text-xs text-emerald-300">
              <span className="anim-dot h-1.5 w-1.5 rounded-full bg-emerald-400" />
              Open source · v0.3.1-alpha · actively building
            </div>
            <h1 className="anim-fade-up delay-1 mt-6 text-4xl font-semibold leading-[1.1] sm:text-5xl lg:text-6xl">
              One platform for your work, your data, and your{' '}
              <span className="flow-text">automations</span>.
            </h1>
            <p className="anim-fade-up delay-2 mt-6 max-w-lg text-lg leading-relaxed text-neutral-400">
              Stop paying for a stack of disconnected subscriptions. Install only the modules you
              need — they share one login, one data model, and one automation engine.
            </p>
            <div className="anim-fade-up delay-3 mt-8 flex flex-wrap items-center gap-3">
              <Link
                to="/signup"
                className="rounded-lg bg-emerald-500 px-6 py-3 text-sm font-semibold text-black shadow-[0_0_30px_rgba(16,185,129,0.35)] transition-all hover:bg-emerald-400"
              >
                Create your workspace
              </Link>
              <Link
                to="/features"
                className="rounded-lg border border-neutral-700 px-6 py-3 text-sm font-medium transition-colors hover:border-neutral-400"
              >
                Explore features
              </Link>
              <a href={GITHUB_URL} className="px-2 py-3 text-sm text-neutral-500 hover:text-neutral-200">
                View source ↗
              </a>
            </div>
            <dl className="anim-fade-up delay-4 mt-10 grid max-w-md grid-cols-3 gap-4 border-t border-neutral-800/60 pt-6">
              {[
                ['4', 'trigger types'],
                ['3', 'modules live'],
                ['1', 'command to run'],
              ].map(([n, l]) => (
                <div key={l}>
                  <dt className="text-2xl font-semibold text-emerald-400">{n}</dt>
                  <dd className="text-xs text-neutral-500">{l}</dd>
                </div>
              ))}
            </dl>
          </div>
          <div className="anim-float">
            <OrbitVisual />
          </div>
        </div>
      </section>

      {/* The loop console */}
      <section aria-label="The product in one loop" className="border-y border-neutral-800/60 bg-neutral-900/20 py-16">
        <div className="mx-auto max-w-6xl px-6 lg:grid lg:grid-cols-2 lg:items-center lg:gap-12">
          <div>
            <Eyebrow>The whole point</Eyebrow>
            <h2 className="text-2xl font-semibold sm:text-3xl">
              A form on your website becomes a workflow in your platform.
            </h2>
            <p className="mt-4 text-sm leading-relaxed text-neutral-400">
              A public form creates a record. The record publishes an event — transactionally, in
              the same database commit. The event fires a workflow. The workflow sends the email.
              No glue code, no third-party automator, no per-seat tax.
            </p>
            <Link
              to="/features"
              className="mt-6 inline-block text-sm text-emerald-400 hover:text-emerald-300"
            >
              See what the engine can do →
            </Link>
          </div>
          <div className="mt-10 lg:mt-0">
            <LoopConsole />
          </div>
        </div>
      </section>

      {/* Pillars */}
      <section aria-label="Why ORBIT" className="mx-auto max-w-6xl px-6 py-20">
        <Eyebrow>Why ORBIT</Eyebrow>
        <h2 className="max-w-xl text-2xl font-semibold sm:text-3xl">
          The platform does the integrating, so your tools stop being islands.
        </h2>
        <div className="mt-10 grid gap-4 sm:grid-cols-3">
          <Pillar
            icon={
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <rect x="3" y="3" width="7" height="7" rx="1" />
                <rect x="14" y="3" width="7" height="7" rx="1" />
                <rect x="3" y="14" width="7" height="7" rx="1" />
                <path d="M14 17.5h7M17.5 14v7" />
              </svg>
            }
            title="Modular by design"
          >
            Modules install like apps, through a manifest-driven gallery with per-workspace
            enable switches. Uninstalling cleans up after itself.
          </Pillar>
          <Pillar
            icon={
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2" />
              </svg>
            }
            title="Automation built in"
          >
            Platform events, inbound webhooks, schedules, and manual runs — on a durable,
            restart-safe engine with per-step history you can actually read.
          </Pillar>
          <Pillar
            icon={
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <rect x="2" y="4" width="20" height="7" rx="2" />
                <rect x="2" y="13" width="20" height="7" rx="2" />
                <path d="M6 7.5h.01M6 16.5h.01" />
              </svg>
            }
            title="Self-hosted, no lock-in"
          >
            One <code className="text-neutral-300">docker compose up</code> on any Linux box.
            Your PostgreSQL, your data, Apache 2.0 license. Cloud is optional — never required.
          </Pillar>
        </div>
      </section>

      {/* Subscriptions replaced */}
      <section aria-label="Subscriptions replaced" className="border-y border-neutral-800/60 bg-neutral-900/20 py-16">
        <div className="mx-auto max-w-6xl px-6">
          <div className="grid items-center gap-10 lg:grid-cols-[1fr_auto]">
            <div>
              <Eyebrow>The math</Eyebrow>
              <h2 className="max-w-lg text-2xl font-semibold sm:text-3xl">
                Replace the stack, keep the budget.
              </h2>
              <p className="mt-4 max-w-xl text-sm leading-relaxed text-neutral-400">
                A five-person team typically pays five separate tools for work management,
                automation, data, and forms — every month, per seat. ORBIT runs all of it on one
                small server for the cost of the server.
              </p>
            </div>
            <div className="mx-auto grid w-full max-w-sm gap-3">
              {[
                ['Task management', 'per-seat, monthly'],
                ['Workflow automation', 'per-task, monthly'],
                ['Data tables & forms', 'per-seat, monthly'],
                ['ORBIT', 'your server, $0'],
              ].map(([name, price], i) => (
                <div
                  key={name}
                  className={
                    'flex items-center justify-between rounded-lg border px-4 py-3 text-sm ' +
                    (i === 3
                      ? 'border-emerald-700/70 bg-emerald-900/15'
                      : 'border-neutral-800 bg-neutral-900/40 text-neutral-400')
                  }
                >
                  <span className={i === 3 ? 'font-medium text-emerald-300' : 'line-through decoration-neutral-600'}>
                    {name}
                  </span>
                  <span className={'text-xs ' + (i === 3 ? 'text-emerald-400' : 'text-neutral-500')}>
                    {price}
                  </span>
                </div>
              ))}
            </div>
          </div>
        </div>
      </section>

      {/* Modules grid */}
      <section aria-label="Modules" className="mx-auto max-w-6xl px-6 py-20">
        <Eyebrow>Modules</Eyebrow>
        <h2 className="text-2xl font-semibold sm:text-3xl">Install what you need</h2>
        <p className="mt-2 text-sm text-neutral-500">
          Every module shares the same login, permissions, and event log.
        </p>
        <div className="mt-10 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          {[
            ['Work Management', undefined, 'Projects, task boards, assignments, due dates — with task events that drive your automations natively.'],
            ['Automation Studio', undefined, 'Webhook triggers with secret URLs, restart-safe schedules, HTTP and email actions, readable history.'],
            ['Data', 'in development', 'Tables, records, CSV, and public forms — the form → record → workflow chain that replaces your data tool.'],
            ['Demo Module', undefined, 'A tiny reference module: manifest, install, events. Read its source to build your own module.'],
          ].map(([name, status, desc]) => (
            <div
              key={name as string}
              className="flex h-full flex-col rounded-xl border border-neutral-800 bg-neutral-900/40 p-6 transition-all hover:-translate-y-1 hover:border-neutral-600"
            >
              <div className="flex items-center justify-between">
                <h3 className="text-base font-semibold">{name}</h3>
                {status && (
                  <span className="rounded-full border border-amber-700/70 px-2 py-0.5 text-[11px] text-amber-400">
                    {status}
                  </span>
                )}
              </div>
              <p className="mt-2 flex-1 text-sm leading-relaxed text-neutral-400">{desc}</p>
            </div>
          ))}
        </div>
      </section>

      {/* Final CTA */}
      <section aria-label="Get started" className="border-t border-neutral-800/60 py-20">
        <div className="mx-auto max-w-3xl px-6 text-center">
          <h2 className="text-3xl font-semibold">Run it on your own server in minutes.</h2>
          <p className="mx-auto mt-4 max-w-xl text-sm leading-relaxed text-neutral-400">
            Clone, compose up, sign up, install a module, wire a workflow. If it doesn't replace
            part of your stack, uninstall it — the platform cleans up after itself.
          </p>
          <div className="mt-8 flex flex-wrap items-center justify-center gap-3">
            <Link
              to="/signup"
              className="rounded-lg bg-emerald-500 px-6 py-3 text-sm font-semibold text-black hover:bg-emerald-400"
            >
              Create your workspace
            </Link>
            <Link
              to="/deploy"
              className="rounded-lg border border-neutral-700 px-6 py-3 text-sm font-medium hover:border-neutral-400"
            >
              Deployment guide
            </Link>
          </div>
        </div>
      </section>

      <SiteFooter />
    </div>
  )
}