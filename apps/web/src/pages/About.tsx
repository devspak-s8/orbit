import { Link } from 'react-router-dom'
import { Eyebrow, GITHUB_URL, PageHero, SiteFooter, SiteNav } from '../components/SiteChrome'

const milestones: Array<{ v: string; date: string; title: string; done: boolean }> = [
  { v: 'v0.1.0-alpha', date: 'Nov 2026', title: 'Platform skeleton: identity, workspaces, module system, event log, durable engine, full-stack Compose', done: true },
  { v: 'v0.2.0-alpha', date: 'Nov 2026', title: 'Work Management: projects, boards, assignments — task events drive workflows', done: true },
  { v: 'v0.3.0-alpha', date: 'Nov 2026', title: 'Automation hardening: inbound webhooks, restart-safe schedules, email node', done: true },
  { v: 'v0.3.1-alpha', date: 'Nov 2026', title: 'Polish: shareable invites, in-app notifications, board drag-and-drop', done: true },
  { v: 'v0.4.0-beta', date: 'next', title: 'Data module: tables, records, CSV, public forms — the beta arc begins', done: false },
  { v: 'v1.0.0', date: '2027', title: 'Docs module, security audit, install polish — the first version we call finished', done: false },
]

const principles = [
  ['Open by default', 'Apache 2.0, public repo, public roadmap, decisions as ADRs.'],
  ['Self-hosting is first-class', 'Nothing in the core requires a cloud, an account system, or a credit card.'],
  ['Modular, not monolithic UX', 'Install what you need; uninstall removes it. The core stays small.'],
  ['API-first', 'Every module capability is a documented endpoint — the UI is a client, not the product.'],
  ['Automation is the flagship', 'Events, workflows, and durability get disproportionate engineering attention.'],
  ['AI-assisted, never AI-dependent', 'AI arrives later as an optional gateway; core functionality never needs it.'],
]

export default function About() {
  return (
    <div className="min-h-screen bg-neutral-950 text-neutral-100">
      <SiteNav />
      <PageHero
        eyebrow="About"
        title="An open platform that replaces categories of software, one module at a time."
        lead="ORBIT started from a simple observation: teams pay five subscriptions for tools that can't talk to each other, then pay a sixth to glue them together. The glue should be the platform."
      />

      <main className="mx-auto max-w-6xl px-6 py-14">
        {/* The story */}
        <section aria-label="The idea" className="grid gap-10 lg:grid-cols-2">
          <div>
            <Eyebrow>The idea</Eyebrow>
            <h2 className="text-xl font-semibold">One login. One data model. One engine.</h2>
            <p className="mt-4 text-sm leading-relaxed text-neutral-400">
              A form submission should become a database record, trigger a workflow, notify the
              team, and update a dashboard — natively, in one system, with no integration product
              in the middle. That's not a feature list; it's an architecture. ORBIT's modules are
              independent apps that share identity, permissions, events, and automation, so the
              integrations users normally pay for are just… how the platform works.
            </p>
            <p className="mt-3 text-sm leading-relaxed text-neutral-400">
              It's also a bet on self-hosting: the same codebase runs on a €5 VPS or a
              data-center rack, with no artificial limits parked behind a paid tier.
            </p>
          </div>
          <div>
            <Eyebrow>Principles</Eyebrow>
            <ul className="space-y-3">
              {principles.map(([t, d]) => (
                <li key={t} className="rounded-lg border border-neutral-800 bg-neutral-900/40 px-4 py-3">
                  <div className="text-sm font-medium text-neutral-200">{t}</div>
                  <div className="mt-1 text-xs leading-relaxed text-neutral-500">{d}</div>
                </li>
              ))}
            </ul>
          </div>
        </section>

        {/* Roadmap */}
        <section aria-label="Roadmap" className="mt-16 border-t border-neutral-800/60 pt-12">
          <Eyebrow>Roadmap</Eyebrow>
          <h2 className="text-xl font-semibold">Shipped in the open, in order.</h2>
          <ol className="mt-8 space-y-0">
            {milestones.map((m, i) => (
              <li key={m.v} className="relative flex gap-5 pb-8 last:pb-0">
                <div className="flex flex-col items-center">
                  <span
                    className={
                      'flex h-8 w-8 items-center justify-center rounded-full border text-xs font-semibold ' +
                      (m.done
                        ? 'border-emerald-700 bg-emerald-900/30 text-emerald-300'
                        : 'border-neutral-700 bg-neutral-900 text-neutral-500')
                    }
                  >
                    {m.done ? '✓' : i + 1}
                  </span>
                  {i < milestones.length - 1 && (
                    <span aria-hidden="true" className="mt-1 w-px flex-1 bg-neutral-800" />
                  )}
                </div>
                <div className="pb-2 pt-1">
                  <div className="flex flex-wrap items-baseline gap-x-3">
                    <span className="font-mono text-sm text-emerald-400">{m.v}</span>
                    <span className="text-xs text-neutral-500">{m.date}</span>
                  </div>
                  <p className="mt-1 text-sm leading-relaxed text-neutral-400">{m.title}</p>
                </div>
              </li>
            ))}
          </ol>
          <p className="mt-6 text-sm text-neutral-500">
            The full roadmap, with scope rules, lives in the repository:{' '}
            <a href={`${GITHUB_URL}/blob/main/docs/ROADMAP.md`} className="text-emerald-400 hover:text-emerald-300">
              docs/ROADMAP.md
            </a>
            . Every architecture decision has a public record in{' '}
            <a href={`${GITHUB_URL}/tree/main/docs/adr`} className="text-emerald-400 hover:text-emerald-300">
              docs/adr/
            </a>
            .
          </p>
        </section>

        {/* Built in the open */}
        <section aria-label="Contributing" className="mt-16 border-t border-neutral-800/60 pt-12">
          <Eyebrow>Contribute</Eyebrow>
          <h2 className="text-xl font-semibold">The module system is the front door for contributors.</h2>
          <p className="mt-4 max-w-2xl text-sm leading-relaxed text-neutral-400">
            Third-party developers can build modules without permission from the core team — a
            manifest declares permissions and events, the platform handles install, enable, and
            cleanup. The Demo module in the repository is a working reference: read its source,
            copy it, ship your own.
          </p>
          <div className="mt-6 flex flex-wrap gap-3">
            <a
              href={GITHUB_URL}
              className="rounded-lg border border-neutral-700 px-5 py-2.5 text-sm font-medium hover:border-neutral-400"
            >
              Browse the source
            </a>
            <a
              href={`${GITHUB_URL}/blob/main/docs/plans/data-module.md`}
              className="rounded-lg border border-neutral-700 px-5 py-2.5 text-sm font-medium hover:border-neutral-400"
            >
              Read a real implementation plan
            </a>
            <Link
              to="/signup"
              className="rounded-lg bg-emerald-500 px-5 py-2.5 text-sm font-semibold text-black hover:bg-emerald-400"
            >
              Try it first
            </Link>
          </div>
        </section>
      </main>

      <SiteFooter />
    </div>
  )
}