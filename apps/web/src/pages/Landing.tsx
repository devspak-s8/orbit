import { Link } from 'react-router-dom'

const GITHUB_URL = 'https://github.com/devspak-s8/orbit'

function LoopDiagram() {
  const steps = ['Form', 'Record', 'Event', 'Workflow', 'Email']
  return (
    <ol className="flex flex-wrap items-center justify-center gap-1" aria-label="The ORBIT automation loop">
      {steps.map((s, i) => (
        <li key={s} className="flex items-center gap-1">
          <span className="rounded-full border border-emerald-800/80 bg-emerald-900/20 px-3 py-1 text-xs font-medium text-emerald-300">
            {s}
          </span>
          {i < steps.length - 1 && (
            <span aria-hidden="true" className="text-neutral-600">
              →
            </span>
          )}
        </li>
      ))}
    </ol>
  )
}

function Pillar({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="rounded-xl border border-neutral-800 bg-neutral-900/40 p-6">
      <h3 className="text-base font-semibold">{title}</h3>
      <p className="mt-2 text-sm leading-relaxed text-neutral-400">{children}</p>
    </div>
  )
}

function ModuleCard({
  name,
  status,
  children,
}: {
  name: string
  status?: string
  children: React.ReactNode
}) {
  return (
    <div className="flex h-full flex-col rounded-xl border border-neutral-800 bg-neutral-900/40 p-6 transition-colors hover:border-neutral-600">
      <div className="flex items-center justify-between">
        <h3 className="text-base font-semibold">{name}</h3>
        {status && (
          <span className="rounded-full border border-amber-700/70 px-2 py-0.5 text-[11px] text-amber-400">
            {status}
          </span>
        )}
      </div>
      <p className="mt-2 flex-1 text-sm leading-relaxed text-neutral-400">{children}</p>
    </div>
  )
}

export default function Landing() {
  return (
    <div className="min-h-screen bg-neutral-950 text-neutral-100">
      {/* Hero */}
      <header className="mx-auto flex max-w-6xl items-center justify-between px-6 py-5">
        <div className="text-lg font-semibold tracking-widest">ORBIT</div>
        <nav className="flex items-center gap-4 text-sm" aria-label="Account">
          <a href={GITHUB_URL} className="text-neutral-400 hover:text-neutral-100">
            GitHub
          </a>
          <Link to="/login" className="text-neutral-300 hover:text-white">
            Sign in
          </Link>
          <Link
            to="/signup"
            className="rounded bg-neutral-100 px-3 py-1.5 font-medium text-neutral-900 hover:bg-white"
          >
            Get started
          </Link>
        </nav>
      </header>

      <main>
        <section className="relative mx-auto max-w-6xl px-6 pb-20 pt-16 text-center sm:pt-24">
          <div
            aria-hidden="true"
            className="pointer-events-none absolute inset-x-0 top-0 h-64 bg-[radial-gradient(ellipse_at_top,rgba(16,185,129,0.10),transparent_65%)]"
          />
          <h1 className="mx-auto max-w-3xl text-4xl font-semibold leading-tight sm:text-5xl">
            One platform for your work, your data, and your automations.
          </h1>
          <p className="mx-auto mt-5 max-w-xl text-lg text-neutral-400">
            Free and self-hosted. Install only the modules you need — they share one login, one
            data model, and one automation engine.
          </p>
          <div className="mt-8 flex flex-wrap items-center justify-center gap-3">
            <Link
              to="/signup"
              className="rounded-lg bg-emerald-500 px-6 py-3 text-sm font-semibold text-black hover:bg-emerald-400"
            >
              Create your workspace
            </Link>
            <a
              href={GITHUB_URL}
              className="rounded-lg border border-neutral-700 px-6 py-3 text-sm font-medium hover:border-neutral-400"
            >
              View source
            </a>
          </div>
          <p className="mt-6 text-xs text-neutral-500">
            Apache 2.0 · no subscriptions · your infrastructure
          </p>
        </section>

        {/* The loop */}
        <section aria-label="The product in one loop" className="border-y border-neutral-800/60 bg-neutral-900/20 py-10">
          <div className="mx-auto max-w-4xl px-6 text-center">
            <LoopDiagram />
            <p className="mx-auto mt-4 max-w-2xl text-sm text-neutral-400">
              A public form creates a record, the record publishes an event, the event fires a
              workflow, the workflow sends the email.{' '}
              <span className="text-neutral-200">No glue code, no third-party automator.</span>
            </p>
          </div>
        </section>

        {/* Pillars */}
        <section aria-label="Why ORBIT" className="mx-auto max-w-6xl px-6 py-20">
          <div className="grid gap-4 sm:grid-cols-3">
            <Pillar title="Modular by design">
              Modules install like apps — through a manifest-driven gallery with per-workspace
              enable switches. Uninstalling cleans up after itself.
            </Pillar>
            <Pillar title="Automation built in">
              Four triggers — platform events, webhooks, schedules, manual — and durable,
              restart-safe execution with per-step history you can actually read.
            </Pillar>
            <Pillar title="Self-hosted, no lock-in">
              One <code className="text-neutral-300">docker compose up</code>. Your PostgreSQL,
              your machine, Apache 2.0 license. The cloud is optional, never required.
            </Pillar>
          </div>
        </section>

        {/* Modules */}
        <section aria-label="Modules" className="border-t border-neutral-800/60 py-20">
          <div className="mx-auto max-w-6xl px-6">
            <h2 className="text-center text-2xl font-semibold">Install what you need</h2>
            <p className="mt-2 text-center text-sm text-neutral-500">
              Every module shares the same login, permissions, and event log.
            </p>
            <div className="mt-10 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
              <ModuleCard name="Work Management">
                Projects, task boards, assignments, due dates. Task events drive your
                automations natively.
              </ModuleCard>
              <ModuleCard name="Automation Studio">
                Webhook triggers with secret URLs, restart-safe schedules, HTTP and email
                actions, full execution history.
              </ModuleCard>
              <ModuleCard name="Data" status="in development">
                Tables, records, CSV, and public forms — the form → record → workflow chain.
              </ModuleCard>
              <ModuleCard name="Demo Module">
                A tiny reference module proving the whole pipeline: manifest, install, events.
              </ModuleCard>
            </div>
          </div>
        </section>

        {/* Open source pitch */}
        <section aria-label="Open source" className="border-t border-neutral-800/60 py-16">
          <div className="mx-auto max-w-4xl px-6 text-center">
            <h2 className="text-2xl font-semibold">Built in the open</h2>
            <p className="mx-auto mt-3 max-w-2xl text-sm leading-relaxed text-neutral-400">
              Every decision is a public architecture decision record, every release is tagged,
              and the roadmap lives in the repository. ORBIT exists to replace a stack of
              disconnected subscriptions with one platform you own.
            </p>
            <div className="mt-8 flex flex-wrap items-center justify-center gap-3 text-xs text-neutral-400">
              {['5 releases', 'Apache 2.0', '40+ automated tests', '5 decision records', 'Self-hosted'].map(
                (chip) => (
                  <span key={chip} className="rounded-full border border-neutral-800 px-3 py-1">
                    {chip}
                  </span>
                ),
              )}
            </div>
          </div>
        </section>
      </main>

      <footer className="border-t border-neutral-800/60 py-8">
        <div className="mx-auto flex max-w-6xl flex-wrap items-center justify-between gap-4 px-6 text-xs text-neutral-500">
          <div className="font-semibold tracking-widest text-neutral-400">ORBIT</div>
          <nav className="flex gap-5" aria-label="Footer">
            <a href={GITHUB_URL} className="hover:text-neutral-200">
              GitHub
            </a>
            <a href={`${GITHUB_URL}/blob/main/docs/ROADMAP.md`} className="hover:text-neutral-200">
              Roadmap
            </a>
            <a href={`${GITHUB_URL}/blob/main/LICENSE`} className="hover:text-neutral-200">
              License
            </a>
          </nav>
        </div>
      </footer>
    </div>
  )
}