import { Link } from 'react-router-dom'

export const GITHUB_URL = 'https://github.com/devspak-s8/orbit'

export function SiteNav() {
  return (
    <header className="sticky top-0 z-20 border-b border-neutral-800/60 bg-neutral-950/80 backdrop-blur">
      <div className="mx-auto flex max-w-6xl items-center justify-between px-6 py-4">
        <Link to="/" className="text-lg font-semibold tracking-widest">
          ORBIT
        </Link>
        <nav className="flex items-center gap-6 text-sm" aria-label="Site">
          <Link to="/features" className="hidden text-neutral-400 hover:text-neutral-100 sm:block">
            Features
          </Link>
          <Link to="/deploy" className="hidden text-neutral-400 hover:text-neutral-100 sm:block">
            Deploy
          </Link>
          <Link to="/about" className="hidden text-neutral-400 hover:text-neutral-100 sm:block">
            About
          </Link>
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
      </div>
    </header>
  )
}

export function SiteFooter() {
  return (
    <footer className="border-t border-neutral-800/60 py-10">
      <div className="mx-auto max-w-6xl px-6">
        <div className="flex flex-wrap items-start justify-between gap-8">
          <div className="max-w-xs">
            <div className="font-semibold tracking-widest text-neutral-300">ORBIT</div>
            <p className="mt-2 text-xs leading-relaxed text-neutral-500">
              The open-source operations platform: one login, one data model, one automation
              engine — on your infrastructure.
            </p>
          </div>
          <nav className="grid grid-cols-2 gap-x-12 gap-y-2 text-xs text-neutral-400" aria-label="Footer">
            <Link to="/features" className="hover:text-neutral-100">
              Features
            </Link>
            <Link to="/deploy" className="hover:text-neutral-100">
              Deploy
            </Link>
            <Link to="/about" className="hover:text-neutral-100">
              About & roadmap
            </Link>
            <a href={GITHUB_URL} className="hover:text-neutral-100">
              Source code
            </a>
            <a href={`${GITHUB_URL}/releases`} className="hover:text-neutral-100">
              Releases
            </a>
            <a href={`${GITHUB_URL}/blob/main/docs/ROADMAP.md`} className="hover:text-neutral-100">
              Roadmap
            </a>
            <a href={`${GITHUB_URL}/blob/main/LICENSE`} className="hover:text-neutral-100">
              Apache 2.0
            </a>
            <a href={`${GITHUB_URL}/security policy`} className="hover:text-neutral-100">
              Security
            </a>
          </nav>
        </div>
        <p className="mt-8 text-[11px] text-neutral-600">
          © 2026 The ORBIT contributors · Apache License 2.0
        </p>
      </div>
    </footer>
  )
}

export function Eyebrow({ children }: { children: React.ReactNode }) {
  return (
    <div className="mb-3 text-[11px] font-semibold uppercase tracking-[0.2em] text-emerald-400/80">
      {children}
    </div>
  )
}

export function PageHero({
  eyebrow,
  title,
  lead,
}: {
  eyebrow: string
  title: string
  lead: string
}) {
  return (
    <section className="relative overflow-hidden border-b border-neutral-800/60">
      <div
        aria-hidden="true"
        className="pointer-events-none absolute inset-x-0 top-0 h-72 bg-[radial-gradient(ellipse_at_top,rgba(16,185,129,0.12),transparent_70%)]"
      />
      <div className="mx-auto max-w-6xl px-6 py-16 sm:py-20">
        <div className="anim-fade-up">
          <Eyebrow>{eyebrow}</Eyebrow>
          <h1 className="max-w-3xl text-3xl font-semibold leading-tight sm:text-4xl">{title}</h1>
          <p className="mt-4 max-w-2xl text-base leading-relaxed text-neutral-400">{lead}</p>
        </div>
      </div>
    </section>
  )
}
