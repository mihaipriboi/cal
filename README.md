<p align="center">
  <img src="./logo.svg" alt="Cal" width="120">
</p>

<h1 align="center">Cal</h1>

<p align="center">
  The self-hosted daily toolkit people actually enjoy opening.<br>
  Calendar, tasks, notes, links, files, kanban, time tracking and a GitHub inbox — one quiet place, on your own hardware.
</p>

<p align="center">
  <a href="#quick-start">Quick Start</a> •
  <a href="#screenshots">Screenshots</a> •
  <a href="#features">Features</a> •
  <a href="ROADMAP.md">Roadmap</a> •
  <a href="CONTRIBUTING.md">Contributing</a>
</p>

<p align="center">
  <a href="https://github.com/Dvorinka/cal/actions/workflows/ci.yml"><img src="https://github.com/Dvorinka/cal/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/Dvorinka/cal/releases"><img src="https://img.shields.io/github/v/release/Dvorinka/cal" alt="Release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-green" alt="License: MIT"></a>
</p>

<p align="center">
  <a href="https://calhq.vercel.app">calhq.vercel.app</a> — landing page
</p>

Cal is an open-source personal planner that replaces a handful of SaaS tabs.
Month, week and day views with drag-and-drop; tasks with recurrence and
natural-language quick add; markdown notes with wikilinks; kanban boards whose
cards are real calendar entries; a focus timer and billable timesheets; a
private relationship manager whose birthdays surface on the calendar. No
accounts on someone else's server — one container and it's yours.

## Screenshots

| Today — agenda, streaks, deadlines | Month view |
| --- | --- |
| ![Today view with schedule, habit streaks, upcoming dates and deadlines](docs/screenshots/today.png) | ![Month view with events, tasks and birthdays](docs/screenshots/month.png) |

| Week view — timed blocks | Week view, dark theme |
| --- | --- |
| ![Week view with timed event blocks](docs/screenshots/week.png) | ![Week view in dark theme](docs/screenshots/week-dark.png) |

| Kanban board — cards are real tasks | People — relationship manager |
| --- | --- |
| ![Kanban board with due dates and checklists](docs/screenshots/board.png) | ![People page with birthdays and relations](docs/screenshots/people.png) |

| Notes — markdown, wikilinks, tags | Time — timesheets and billables |
| --- | --- |
| ![Notes page with tag filters](docs/screenshots/notes.png) | ![Time page with billable sessions](docs/screenshots/time.png) |

| Shopping — sections, quantities, can't-find flags |
| --- |
| ![Shopping list with sections, quantities and checked items](docs/screenshots/shopping.png) |

## Features

- **Planner core** — month, week and day views with drag-and-drop, recurring
  tasks, natural-language quick add (`dentist fri 5pm #health`), reminders,
  per-entry version history, soft-delete trash
- **Beyond the calendar** — notes with templates and `[[wikilinks]]`, a link
  library that unfurls titles/thumbnails and searches YouTube via your own
  Invidious instance, file uploads with public share links, kanban boards
  with WIP limits and read-only public sharing
- **Time & people** — focus timer with pomodoro mode, billable sessions with
  hourly rates and CSV/JSON export; a private relationship manager whose
  birthdays, anniversaries and namedays surface on the calendar
- **Shopping lists** — multiple lists with sections (Dairy, Produce…),
  quantities, a "can't find it" flag, bulk check-off, clear-purchased, and
  autocomplete from your own item history
- **Admin & multi-user** — the first registered account administers the
  instance: open/close sign-ups (closed by default once it exists), promote
  admins, remove accounts, all from Settings → Users & access
- **Portable** — full JSON export one click away, or a zip that packs every
  upload binary too; restores merge additively, and the server writes
  nightly snapshots for 14 days
- **Sync & feeds** — two-way CalDAV, read-only Google Calendar, iCalendar feed
  subscriptions, CardDAV birthdays, RSS/Atom items on their publish date,
  holidays for 40+ regions
- **Automation** — GitHub inbox (issues/PRs become cards, status flows both
  ways), HMAC-signed webhooks out, email-to-task intake, `.ics` import/export,
  and a token-gated MCP endpoint so AI assistants can drive your planner
- **Everywhere** — offline-first PWA with a write queue, Android shell with a
  home-screen widget, Wails desktop app, share-target integration, global
  `⌘K` palette, keyboard-first navigation. The Android app can also run
  **serverless**: local mode keeps only Mail and dials your IMAP/SMTP
  provider directly from the device — accounts stay encrypted on-device and
  can be imported to a server if you set one up later
- **Contained** — cookie sessions, bcrypt passwords, login rate limiting,
  AES-256-GCM for stored credentials, SSRF-guarded outbound URLs, zero
  external services required

## Quick Start

Requires Docker:

```bash
curl -fsSL https://raw.githubusercontent.com/Dvorinka/cal/main/install.sh | sh
```

Downloads `docker-compose.yml` into `./cal`, generates a `.env` with a
random Postgres password, pulls the published images, and starts the app
plus its database — UI and API at `http://localhost:8080`. Re-running pulls
new images and restarts; `.env` and the named volumes are never touched.

Override with env vars:

```bash
curl -fsSL https://raw.githubusercontent.com/Dvorinka/cal/main/install.sh | \
  PORT=9090 CAL_DIR=/opt/cal CAL_VERSION=v1.2.3 sh
```

Or run the compose stack by hand:

```bash
echo "POSTGRES_PASSWORD=$(openssl rand -hex 24)" > .env
docker compose up -d
```

Open http://localhost:8080 and create your account. Back up two volumes:
`cal-pg` (the database) and `cal-data` (uploads, exports). Rather run a
single container? A bare `docker run` without `DATABASE_URL` starts the
image's embedded Postgres instead — then `cal-data` is the only thing to
back up.

Desktop apps (Windows/Linux/macOS), headless `cal-server` binaries and an
Android APK attach to every
[release](https://github.com/Dvorinka/cal/releases). On Windows grab
`Cal-Setup-Windows.exe` — a real installer with the database engine bundled,
so first launch works offline. The desktop app bundles its own server, or
can sign in to a server URL to share data between devices.

## Architecture

```
apps/web         React PWA + Capacitor android/ios shells
apps/api         Go API (Gin) + migrations + MCP endpoint
apps/desktop     Wails app; `-tags headless` is the all-in-one server
packages/api-client   shared typed client, generated from openapi.yaml
```

React 19 + Vite + TypeScript + Tailwind 4 + Zustand · Go + Gin + PostgreSQL
(pgx, Goose migrations) · Wails 2 desktop · Capacitor shells · one Docker
image for everything. The API contract lives in `openapi.yaml`; the typed TS
client in `packages/api-client` is kept in sync with it.

## Configuration

All configuration is via environment variables:

| Variable | Default | Purpose |
| --- | --- | --- |
| `PORT` | `8080` | listen port |
| `DATA_DIR` | `/data` (container) | files, backups, embedded PG data |
| `DATABASE_URL` | unset → embedded PG | external Postgres DSN |
| `SESSION_SECURE` | `false` | set `true` behind HTTPS |
| `WEB_ORIGIN` | unset | extra CORS origin for the web app |
| `TRUSTED_PROXIES` | unset | comma-separated IPs/CIDRs of reverse proxies whose `X-Forwarded-For` is honored (e.g. `10.0.0.5`). Unset trusts none, so the client IP is the connecting peer |
| `MAIL_ALLOWED_NETS` | unset | comma-separated IPs/CIDRs allowed to use the Mail API (e.g. `192.168.1.0/24,10.6.0.0/24`); other clients get 403. Uses the same client IP as rate limiting, so set `TRUSTED_PROXIES` when behind a proxy. Unset allows all |
| `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` | unset | enable Google Calendar sync |
| `CAL_ALLOW_PRIVATE_FEEDS` / `CAL_ALLOW_PRIVATE_WEBHOOKS` | unset | allow private/LAN URLs for feeds / webhooks (SSRF guard off for that feature; link previews stay guarded) |

Never commit `.env` — if a secret was ever committed, rotate it.

## Development

```bash
npm install
cp apps/api/.env.example apps/api/.env

# Postgres (or use the db service in docker-compose.yml)
docker run -d --name cal-db -e POSTGRES_DB=cal -e POSTGRES_USER=cal \
  -e POSTGRES_PASSWORD=cal -p 5432:5432 postgres:16-alpine

npm run dev -w @cal/web              # vite on :5173, proxies /api
cd apps/api && go run ./cmd/server   # api on :8080
```

Verify before opening a PR:

```bash
npm run typecheck && npm run test && npm run build
cd apps/api && go vet ./... && go test ./...
```

E2E: `cd apps/web && npx playwright test` (Playwright + axe).
Load: `k6 run perf/k6.js -e EMAIL=… -e PASS=…`.

## Documentation

- [`openapi.yaml`](openapi.yaml) — full API schema; session-cookie auth, bearer
  tokens for MCP/intake/widget/feed endpoints
- [`docs/releasing.md`](docs/releasing.md) — how releases build, code-signing
  secrets
- [`ROADMAP.md`](ROADMAP.md) — what shipped, what's next, what's out of scope

---

[Contributing](CONTRIBUTING.md) · [Security](SECURITY.md) · [MIT License](LICENSE) © 2026 Tomas Dvorak
