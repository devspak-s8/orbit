import { Link } from 'react-router-dom'
import { PageHero, SiteFooter, SiteNav } from '../components/SiteChrome'

function Code({ children }: { children: string }) {
  return (
    <div className="overflow-hidden rounded-xl border border-neutral-800 bg-neutral-900/70">
      <pre className="code-block overflow-x-auto px-4 py-3 text-neutral-300">{children}</pre>
    </div>
  )
}

function Step({ n, title, children }: { n: number; title: string; children: React.ReactNode }) {
  return (
    <li className="relative flex gap-5 pb-10 last:pb-0">
      <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full border border-emerald-800/70 bg-emerald-900/20 text-sm font-semibold text-emerald-300">
        {n}
      </div>
      <div className="pt-1">
        <h3 className="text-base font-semibold">{title}</h3>
        <div className="mt-3 space-y-3 text-sm leading-relaxed text-neutral-400">{children}</div>
      </div>
    </li>
  )
}

export default function Deploy() {
  return (
    <div className="min-h-screen bg-neutral-950 text-neutral-100">
      <SiteNav />
      <PageHero
        eyebrow="Deploy"
        title="Your server. Your data. One command."
        lead="ORBIT runs as three containers: nginx serving the web app, the Go API, and PostgreSQL. Everything else — migrations, module registry, workers — starts itself."
      />

      <main className="mx-auto max-w-4xl px-6 py-14">
        <ol>
          <Step n={1} title="Clone and run">
            <p>Any Linux box with Docker works. A 1 vCPU / 2 GB VPS comfortably runs a small team.</p>
            <Code>{`git clone https://github.com/devspak-s8/orbit.git
cd orbit
docker compose up -d`}</Code>
          </Step>

          <Step n={2} title="Open the app">
            <p>
              The web UI is on <span className="text-neutral-200">http://your-server:8081</span>.
              Create an account, create a workspace, install a module from the gallery — the
              database schema migrates itself on startup.
            </p>
          </Step>

          <Step n={3} title="Wire automation">
            <p>
              Give a workflow the webhook trigger and validate it — the page hands you a secret
              URL. Anything that can POST can now run that workflow.
            </p>
            <Code>{`curl -X POST -H "Content-Type: application/json" \\
  -d '{"source":"landing page"}' \\
  http://your-server:8081/hooks/<token>`}</Code>
          </Step>

          <Step n={4} title="Back up (and verify) regularly">
            <p>The database is the single source of truth; backing it up backs up everything.</p>
            <Code>{`# backup
docker compose exec -T db pg_dump -U orbit -d orbit > orbit-$(date +%F).sql

# restore
cat orbit-2026-11-16.sql | docker compose exec -T db psql -U orbit -d orbit`}</Code>
            <p>
              The restore path is tested: a dump of the full alpha schema restores cleanly with
              all data intact.
            </p>
          </Step>

          <Step n={5} title="Email (optional)">
            <p>
              Point ORBIT at any SMTP relay to enable email nodes. Unconfigured, email nodes fail
              with a clear error instead of pretending.
            </p>
            <Code>{`# docker-compose override or environment:
ORBIT_SMTP_HOST=smtp.example.com
ORBIT_SMTP_PORT=587
ORBIT_SMTP_USER=orbit@example.com
ORBIT_SMTP_PASS=…
ORBIT_SMTP_FROM=orbit@example.com`}</Code>
          </Step>
        </ol>

        <section aria-label="Reference" className="mt-16 border-t border-neutral-800/60 pt-10">
          <h2 className="text-lg font-semibold">Ports & configuration</h2>
          <div className="mt-5 grid gap-6 sm:grid-cols-2">
            <div className="overflow-hidden rounded-xl border border-neutral-800">
              <table className="w-full text-left text-sm">
                <thead className="bg-neutral-900/70 text-xs text-neutral-500">
                  <tr>
                    <th className="px-4 py-2.5 font-medium">Port</th>
                    <th className="px-4 py-2.5 font-medium">Service</th>
                  </tr>
                </thead>
                <tbody className="text-neutral-300">
                  {[
                    ['8081', 'Web UI + API proxy'],
                    ['8082', 'API (direct, for CLIs)'],
                    ['5434', 'PostgreSQL'],
                  ].map(([p, s]) => (
                    <tr key={p} className="border-t border-neutral-800/60">
                      <td className="px-4 py-2.5 font-mono text-xs text-emerald-400">{p}</td>
                      <td className="px-4 py-2.5">{s}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <div className="rounded-xl border border-neutral-800 p-4 text-sm text-neutral-400">
              <h3 className="mb-2 text-neutral-200">Put it behind TLS</h3>
              <p className="leading-relaxed">
                Put any reverse proxy (Caddy, Traefik, nginx) in front of port 8081 and you have
                HTTPS with automatic certificates. Webhook URLs and public forms work unchanged
                behind the proxy.
              </p>
              <p className="mt-3 leading-relaxed">
                Home server behind NAT? A{' '}
                <a href="https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/" className="text-emerald-400 hover:text-emerald-300">
                  Cloudflare Tunnel
                </a>{' '}
                publishes your instance without opening a single port.
              </p>
            </div>
          </div>
        </section>

        <section aria-label="Next" className="mt-16 border-t border-neutral-800/60 pt-10 text-center">
          <h2 className="text-xl font-semibold">That's the whole ritual.</h2>
          <p className="mx-auto mt-3 max-w-md text-sm text-neutral-400">
            No config files to hand-edit on first run, no external services required, nothing to
            rent. Uninstall the platform and it leaves nothing behind but a folder.
          </p>
          <Link
            to="/signup"
            className="mt-6 inline-block rounded-lg bg-emerald-500 px-6 py-3 text-sm font-semibold text-black hover:bg-emerald-400"
          >
            Create your workspace
          </Link>
        </section>
      </main>

      <SiteFooter />
    </div>
  )
}