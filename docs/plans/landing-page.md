# Landing Page — Implementation Plan

**Milestone:** public face
**Scope:** the page a stranger sees before they sign up
**Status:** planned — no code yet

---

## 1. Purpose

ORBIT has a public repo, releases, and a working product — but the app's
front door is a login form. The landing page is the pitch: what ORBIT is,
why it's free, and one click into signup. It ships **inside the web app**
(no separate site to build or deploy): signed-out visitors get `/`,
signed-in users go straight to their workspace.

## 2. Structure (single page, dark, consistent with the app shell)

1. **Hero** — "One platform for your work, your data, and your automations."
   Subline: "Free and self-hosted. Install only what you need." CTAs:
   **Get started** (→ /signup) and **View source** (→ GitHub)
2. **The loop strip** — the product thesis as a diagram:
   `form → record → event → workflow → email` — "no glue code, no
   third-party automator, one platform"
3. **Three pillars** — Modules (install like apps), Automation (four
   triggers, three nodes), Self-hosted (your data, Apache 2.0, one
   `docker compose up`)
4. **Modules grid** — Work Management, Automation Studio (cards with real
   feature lists); Demo module (proving the pipeline); Data module gets a
   "beta" chip
5. **Open source pitch** — Apache 2.0, no lock-in, cloud optional;
   three stat chips (releases, tests, ADRs — filled from reality)
6. **Footer** — GitHub, roadmap, license, "built in the open"

## 3. Implementation

- Route change only: unauthenticated `/` renders `<Landing/>` instead of
  redirecting to /login; nav links swap (Sign in / Create account)
- One new component, pure Tailwind, zero new dependencies, no data
  fetching — all content static
- Deployable free anywhere the app runs (dev, Compose, and later
  Cloudflare Pages if we split it out — the content is designed to lift
  and shift)
- Accessibility: semantic sections, keyboard-reachable CTAs, reduced-motion
  safe (no animations beyond a subtle gradient)

## 4. Acceptance criteria

1. Signed-out `/` shows the landing page; **Get started** reaches the
   working signup form
2. Signed-in `/` behaves exactly as today (no regression)
3. Lighthouse-clean semantics: one h1, labeled sections, contrast ≥ AA
4. Built and shipped in the same image; visible at `:8081`

## 5. Build order

Last of the three planned milestones — it advertises the Data module, so
the modules grid should be real before the page ships. ~Half a day of work;
done after the data module lands (or first if you want the storefront up
early — it's independent).
