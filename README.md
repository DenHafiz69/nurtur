# Nurtur

[![CI](https://github.com/DenHafiz69/nurtur/actions/workflows/ci.yml/badge.svg)](https://github.com/DenHafiz69/nurtur/actions/workflows/ci.yml)
[![Container](https://img.shields.io/badge/ghcr.io-nurtur-blue?logo=docker&logoColor=white)](https://github.com/DenHafiz69/nurtur/pkgs/container/nurtur)

A calm, one-thumb baby tracker for nursing, diapers, and sleep. One Go service,
server-rendered with HTMX, backed by a local [Turso](https://turso.tech) database
that syncs to Turso Cloud when configured.

This repository ships **Phase 3 — Timeline, Undo + Ship**: the Phase 1 nursing
timer, the Phase 2 diaper/sleep dashboard, plus a unified history page, in-place
undo, night-mode polish, a PWA shell, JSON/CSV/SQL export, a documented restore
drill, optional local sign-in, and a repeatable deployment. See
[`PLANS.md`](./PLANS.md) for the full roadmap and [`FEATURES.md`](./FEATURES.md)
for the product brief.

---

## Quick start

Requirements: Go 1.24+ (the Turso driver requires it).

```sh
cp .env-example .env
# edit NURTUR_TZ, NURTUR_SIGNUP_KEY, and NURTUR_SESSION_SECRET
make dev
```

Open <http://localhost:8080>. Local development uses `NURTUR_LOCAL_ONLY=true`
and writes `./data/nurtur.db`. `make dev` rebuilds and restarts on changes to
Go, template, CSS, JS, or migration files.

The default timezone is the system local zone. Set `NURTUR_TZ` (for example
`Asia/Kuala_Lumpur`) so "today" and edit inputs match the phone.

```sh
NURTUR_TZ=Asia/Kuala_Lumpur make dev
```

Variables exported in your shell override the same names in `.env`.

No account, analytics, CDN, or external font request. Everything is served by
the one binary.

---

## Configuration

| Variable | Default | Purpose |
|---|---|---|
| `NURTUR_ENV_FILE` | `.env` | Optional dotenv file to load |
| `NURTUR_ADDR` | `:8080` | HTTP listen address |
| `NURTUR_DB` | `./data/nurtur.db` | Local Turso database file |
| `NURTUR_LOCAL_ONLY` | `false` | Skip Turso Cloud sync entirely |
| `TURSO_DATABASE_URL` | — | Sync target, e.g. `libsql://…` |
| `TURSO_AUTH_TOKEN` | — | Turso database token |
| `NURTUR_SYNC_INTERVAL` | `60s` | Periodic pull interval |
| `NURTUR_TZ` | system local | IANA timezone for display and day boundaries |
| `NURTUR_BABY_NAME` | — | Optional fallback; onboarding normally stores the name |
| `NURTUR_NIGHT_START` | `19:00` | Local night window start (`HH:MM`) |
| `NURTUR_NIGHT_END` | `07:00` | Local night window end (`HH:MM`) |
| `NURTUR_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error` |
| `NURTUR_PASSCODE` | — | Legacy single-user passcode gate |
| `NURTUR_SIGNUP_KEY` | — | Secret key that lets people create local accounts |
| `NURTUR_SESSION_SECRET` | derived | HMAC key for session cookies (set this to survive credential changes) |
| `NURTUR_COOKIE_SECURE` | `false` | Set `true` when serving over HTTPS (also auto-detected per request) |

The night window can also be changed at runtime on the **Settings** page. Values
stored there take precedence over `NURTUR_NIGHT_START` / `NURTUR_NIGHT_END`.

If sync is not configured you must set `NURTUR_LOCAL_ONLY=true`. The app fails
at startup rather than silently pretending to sync.

### Turso Cloud

```sh
turso db create nurtur
turso db show nurtur --url
turso db tokens create nurtur

export TURSO_DATABASE_URL='libsql://…'
export TURSO_AUTH_TOKEN='…'
export NURTUR_TZ='Asia/Kuala_Lumpur'

make build
./bin/nurtur
```

The baby's name, date of birth, and gender are asked for during onboarding and
stored in the database, so they do not need environment variables. The header
shows the baby's current age, derived from the stored birth date.

The local file is the write path. A feed or diaper log commits locally and
returns immediately; a debounced background `Push` copies changes to Turso
Cloud. Pulls run at startup, every `NURTUR_SYNC_INTERVAL`, and on shutdown the
queue is flushed and the WAL checkpointed. A cloud outage degrades sync, never
logging.

A push or pull that cannot make progress for five minutes is treated as a
stalled native sync engine. The process exits so Docker or systemd can restart
it; local writes are already durable in the WAL and the restart resumes the
queued push. `/healthz?verbose=1` reports the operation in flight and its age
while the worker is still waiting on it.

**Run one server instance per database.** The sync engine and the one-active-feed
constraints assume a single writer. This is a solo-user app, not a multi-tenant
service.

---

## How the timer stays honest

- The server stores UTC RFC3339 timestamps and is the only source of truth.
- `timer.js` computes elapsed time from `data-started-at` and
  `data-server-now`, correcting for phone clock skew. It updates only text nodes
  every 250 ms; the polite live region announces at most every 30 seconds.
- The active timer is mirrored in `localStorage` (`nurtur.timer.v1`). On resume,
  on `pageshow`, `focus`, and `visibilitychange`, the client reconciles against
  `GET /partials/feed/timer`. Server state always wins.
- Locking the phone, switching tabs, refreshing, or restarting the server does
  not reset the feed: the start time lives in the database, not the browser.
- A double-tap cannot create two feeds. Transactions plus partial unique indexes
  (`one_active_feed`, `one_open_feed_segment`) back up the application checks.

### Feeding rules

- `feeds` stores the session; `feed_segments` stores every uninterrupted stretch
  on a side. Side totals are always derived from segments, never duplicated.
- Tapping the already-active side is a no-op ("Already nursing left").
- `Done` closes the open segment and the feed in one transaction.
- Deleting an active feed closes it first, then soft-deletes it.
- Manual corrections rebuild segments in `left → right` order. Totals longer
  than the feed window (with a 60-second tolerance) are rejected inline.
- `−5 min` / `+5 min` clamp the current side's start between the previous
  segment and now; it can never be moved into the future.

### Diaper rules

- `Wet`, `Dirty`, and `Both` log in one tap from the dock; details are optional
  and live behind `Add details` on the toast or the diaper sheet.
- A `both` event satisfies the last-wet and last-dirty queries.
- A second identical tap within two seconds is treated as a double tap and does
  not create another row.
- Colour, consistency, notes, and the logged time are editable with
  `PATCH /diapers/{id}`. Deleting is soft, with Undo on the toast.

### Sleep rules

- At most one sleep is active. The partial unique index backs the application
  check, and a double tap returns a friendly "already asleep" message.
- `Start sleep` / `Wake up` is one tap. The sheet also logs an earlier sleep with
  start and end times; times are validated in the configured timezone.
- Sleeps may not overlap each other (a currently open sleep runs until now).
  Editing a sleep re-checks the overlap.
- The sleep card shows the wake window (`Awake · 45m`) or the active sleep
  duration (`Asleep · 1h 05m`). "Never slept" renders `—`, not a fake duration.
- The sleep sheet shows today's total split into day and night using the
  `NURTUR_NIGHT_START` / `NURTUR_NIGHT_END` window.

### History and undo

- `/timeline` unions feeds, diapers, and sleeps, newest first, and groups rows by
  **local** date. Each date header carries that day's feed, diaper, and sleep
  totals.
- Paging is by whole local days: `GET /timeline/partial?before=<RFC3339>` loads
  the seven days before the cursor (up to 50 events, capped at 200). A day header
  never splits across pages.
- Deleting a row is a soft delete and is optimistic: the row is replaced by a
  hidden placeholder so the Undo action can restore it into the exact same spot.
  The toast's Undo lasts 8 seconds.
- Deleting an active feed or sleep closes it first, in one transaction, and the
  timer/localStorage cache clears through the timer OOB fragment or the
  `timer-cleared` client event.
- Restoring re-validates the domain: a sleep that would overlap another sleep is
  refused with a plain-language message, and restoring an open feed while
  another feed is running is refused.
- Undo is also available on the dashboard's today list for all three event types.
- On `/timeline` the quick-log dock starts collapsed as a floating `+` button so
  the day cards get the whole screen. Opening it logs in place: the new row is
  inserted into today's group and the header totals refresh without a reload.

### Night mode, install, and offline

- The inline head script applies `data-theme` before first paint. **Auto** is
  night from 21:00–07:00 local (and follows `prefers-color-scheme` on a first
  visit); the header toggle flips between light and dark, skipping any auto
  step that would look the same, and stores `nurtur.theme`.
- The Settings page can change the night window; sleep totals and the auto theme
  both use it.
- `GET /sw.js` is served from the domain root so it controls the whole app. Static
  assets are cache-first; navigation is network-first and never cached (a cached
  authenticated page could outlive the sign-in gate). Offline navigation falls
  back to `/offline`.
- A header chip shows sync state separately from network connectivity: `Synced`,
  `Syncing…`, `Local copy`, or `Local`.
- The `localStorage` timer recovery keeps working with the service worker
  installed.

### Settings and export

- `/settings` shows the baby name (editable), the timezone (from the
  environment), the editable night window, theme help, sync state, database
  path/size, and backup links. The birth date and gender are deliberately not
  editable.
- `GET /export.json` is a structured copy with feeds (and segments), diapers,
  sleeps, settings, and per-side minute totals.
- `GET /export.csv` is one row per event. Columns:
  `kind,id,started_at,ended_at,left_minutes,right_minutes,last_side,type,color,consistency,notes,created_at,deleted_at`.
  Feed rows use `started_at`/`ended_at`; diaper rows use `started_at` for the
  logged time and `type` for `wet|dirty|both`; sleep rows use the times.
- `GET /export.sql` streams the schema plus `INSERT` statements, including
  `schema_migrations`, so it replays into a **fresh** Turso/SQLite database
  without re-running migrations. Soft-deleted rows are included so a restore is
  faithful.

---

## Progressive enhancement

Every mutation is a real `<form>` with a `method` and `action`:

- HTMX request: a primary fragment (timer, status strip, or sleep pill) plus the
  OOB companions `#last-nursed`, `#status-strip`, `#timeline-today`,
  `#sleep-pill`, and `#toast-region`. The primary is never also sent OOB.
- No HTMX/JS: `303 See Other` back to `/`; edit pages render an open `<dialog>`
  and submit with a normal POST.
- Validation error with HTMX: `422` plus an inline dialog fragment
  (`HX-Retarget: #feed-edit-dialog` or `#action-dialog`).
- The scroll container (`main`) is never replaced on a mutation, so the page
  does not jump. This is checked manually with the scroll position preserved
  across a dock log.
- Every mutation form disables its own submit button while the request is in
  flight (`hx-disabled-elt`).

---

## Design notes

- Day/night tokens live at the top of `web/static/app.css`. Night starts at
  21:00 and ends at 07:00 by default; the header toggle flips between light
  and dark, skipping an auto step that would look the same, and stores
  `nurtur.theme`.
- The first-paint theme bootstrap is a nonce-protected inline script in
  `layout.html`; CSP allows no other inline script or style.
- Fonts are self-hosted, subsetted OFL WOFF2: Fredoka (display) and Nunito Sans
  (UI and timers). Licenses are in `web/static/fonts/`.
- Pip is inline SVG so the mascot can follow the theme through CSS variables.
- The three status cards fit at 360 px; below 360 px they become three slim
  rows. The dock stays in the bottom third with the feed, diaper, and sleep
  tiers in a fixed order.
- The dock is a native `<details>` drawer, so it works without JavaScript. Home
  opens it by default; History and the FAB share the same markup.
- All touch targets are at least 48×48 px; primary buttons are 56–64 px.
- `prefers-reduced-motion` removes the glow, blink, and transitions.

---

## Project layout

```text
cmd/nurtur/          config wiring, startup, graceful shutdown
internal/config/     environment parsing
internal/clock/      Clock interface + fake clock
internal/feed/       sides, segments, derived durations, edit validation
internal/diaper/     kinds, details, validation
internal/sleep/      sessions, wake windows, overlap, day/night split
internal/status/     dashboard aggregation snapshot
internal/timeline/   unified event kinds, entries, day summaries
internal/store/      Turso open/migrations, queries, settings, sync worker
internal/server/     routes, handlers, middleware, auth, export, page view models
internal/view/       renderer, formatting, icons, Pip poses
migrations/          0001_init.sql, 0002_phase2_indexes.sql
web/templates/       layout, dashboard, timeline, settings, login, offline, partials
web/static/          CSS, timer.js, sw.js, manifest, vendored htmx, fonts, icons
deploy/              systemd unit and environment example
```

---

## Tests

```sh
make test        # unit + integration
make race        # go test -race ./...
make vet
make staticcheck

cd e2e && npm install && npx playwright install chromium && npm test
```

The Playwright suite runs at 390×844, reloads mid-feed, logs diapers and sleep,
deletes and undoes a history row in place, and downloads all three exports.

The suites cover:

- the feed state machine, double-start, same-side tap, stop-when-idle,
  delete-active, edits, and clamping;
- HTTP flows with a temp Turso database and `httptest`, including OOB fragments,
  422 dialog errors, no-HTMX redirects, health, static caching, and CSP nonce
  matching;
- concurrent starts (exactly one feed or sleep), the partial unique indexes, and
  idempotent migrations;
- diaper dedupe, `both` satisfying wet and dirty, detail edits, delete/restore,
  and status aggregation with empty and partial data;
- sleep overlap rejection, retroactive logs across midnight, wake windows, and
  day/night splits across the night boundary and both DST transitions;
- sync debounce, coalescing, retry/backoff, pull-on-startup, shutdown flush,
  and "a failed push does not fail the mutation";
- timeline ordering, soft-delete exclusion, whole-day pagination, day summaries
  with sleeps that overlap local midnight, and delete/restore-in-place;
- settings persistence, night-window validation, all three export formats, and
  the sign-in gate (session cookie, rate limit, Origin check, HTMX CSRF token);
- timezone and DST day boundaries (23/25-hour days), and formatting.

The real-phone checklist in `PLANS.md` §2.5 is still the release gate:
lock-screen sleep timers, one-thumb reach, no scroll jump, and the 3 a.m.
dark-room test cannot be automated.

---

## Docker

```sh
docker build -t nurtur .

docker run --rm -p 8080:8080 \
  -v nurtur-data:/data \
  -e TURSO_DATABASE_URL='libsql://…' \
  -e TURSO_AUTH_TOKEN='…' \
  -e NURTUR_TZ='Asia/Kuala_Lumpur' \
  nurtur
```

The image is Debian slim (glibc), not Alpine: `tursogo` loads a prebuilt native
library through `purego`. The service runs as a non-root user, keeps the
database on `/data`, and ships a `curl /healthz` healthcheck.

`GET /healthz` reports database health; `GET /healthz?verbose=1` adds the last
push/pull, pending-sync age, the operation in flight, CDC operations, and WAL
size. Stats are skipped with `sync: engine busy` rather than blocking a health
request behind a slow push.

### Prebuilt images (GHCR)

CI runs formatting, `go vet`, and the Go tests, then builds the `linux/amd64`
image and pushes it to GitHub Container Registry on every push to `main` and
every `v*` tag. Pull requests build the image without pushing.

```sh
docker pull ghcr.io/denhafiz69/nurtur:latest
docker run --rm -p 8080:8080 -v nurtur-data:/data --env-file .env \
  ghcr.io/denhafiz69/nurtur:latest
```

Tag scheme: `latest` follows `main`, release tags produce `X.Y.Z` and `X.Y`,
and every build also gets `<branch>` and `sha-<short>` tags.

The package is private by default. Make it public under the repository's
[package settings](https://github.com/DenHafiz69/nurtur/pkgs/container/nurtur/settings)
if anonymous pulls are wanted, or log in with a token that has `read:packages`
before pulling.

### Docker Compose

```sh
cp .env-example .env   # edit it first
docker compose up -d --build
```

Open <http://localhost:8005>. Docker Compose reads `.env` automatically for
variable substitution. The docker-compose file runs local-only by default and keeps the
database in the `nurtur-data` volume. To use Turso Cloud instead, set
`NURTUR_LOCAL_ONLY=false` plus `TURSO_DATABASE_URL` and `TURSO_AUTH_TOKEN` in
`.env` before `docker compose up`.

### Backups and restore

Nurtur is local-first, so there are two useful copies: the Turso Cloud copy
(configured in the Turso dashboard) and the logical exports from Settings.

**Manual snapshot.** Stop the service cleanly first — this flushes the push queue
and checkpoints the WAL — then copy `NURTUR_DB` plus any `-wal`/`-shm` siblings:

```sh
docker compose stop
docker run --rm -v nurtur-data:/data -v "$PWD":/backup debian:bookworm-slim \
  tar czf /backup/nurtur-$(date +%F).tgz -C /data .
docker compose start
```

**Restore drill** (do this before you need it):

1. Create a fresh Turso database or an empty local directory.
2. Replay the logical dump:

   ```sh
   turso db shell <new-db> < nurtur-2026-10-20.sql
   ```

   or start Nurtur with `NURTUR_LOCAL_ONLY=true` and an empty `NURTUR_DB`, then
   pipe the same file through the local engine.
3. Verify row counts:

   ```sql
   SELECT (SELECT COUNT(*) FROM feeds)   AS feeds,
          (SELECT COUNT(*) FROM diapers) AS diapers,
          (SELECT COUNT(*) FROM sleeps)  AS sleeps;
   ```

4. Open a few durations and confirm they match the export/CSV.
5. Re-enable sync with the new credentials and confirm the header chip reaches
   `Synced`, then check `/healthz?verbose=1` for the last push/pull.

The SQL export includes `schema_migrations`, so the app will not try to recreate
tables on first start. Replaying into a database that already has tables will
fail; restore into a fresh one.

Turso Cloud backup/retention windows are configured in the Turso dashboard rather
than by Nurtur. Record the chosen retention in your deployment notes.

---

## Deployment

For a VPS walkthrough that fits into an existing Docker Compose stack and Caddy
setup, see [`DEPLOYMENT.md`](./DEPLOYMENT.md).

The single binary embeds templates, static assets, migrations, and fonts.

```sh
make build            # bin/nurtur
docker build -t nurtur .
docker run --rm -p 8080:8080 -v nurtur-data:/data --env-file .env nurtur
```

Build both architectures if the target is not the build host (the build host
needs binfmt/QEMU, e.g. `docker run --privileged --rm tonistiigi/binfmt --install arm64`):

```sh
docker buildx build --platform linux/amd64,linux/arm64 -t nurtur .
```

Runtime requirements: glibc (Debian slim, not Alpine), a writable `/data` volume,
and `TZ`/`NURTUR_TZ`. The container runs as UID 10001 and uses `/healthz` as its
healthcheck. Startup order is: open local DB → migrate → connect/verify sync →
serve. Missing Turso credentials fail loudly unless `NURTUR_LOCAL_ONLY=true`.

### systemd

`deploy/nurtur.service` and `deploy/nurtur.env.example` are a starting point:

```sh
sudo cp bin/nurtur /usr/local/bin/nurtur
sudo useradd --system --home /var/lib/nurtur --shell /usr/sbin/nologin nurtur
sudo install -d -o nurtur -g nurtur /var/lib/nurtur
sudo cp deploy/nurtur.service /etc/systemd/system/
sudo cp deploy/nurtur.env.example /etc/nurtur/nurtur.env   # then edit it
sudo systemctl daemon-reload
sudo systemctl enable --now nurtur
```

`StateDirectory=nurtur` creates `/var/lib/nurtur`; point `NURTUR_DB` there.

---

## Security

Authentication is **optional and off by default** for a solo user on a trusted
LAN.

### Local account onboarding

For a shared phone or an install that reaches the internet, set a signup key:

```sh
NURTUR_SIGNUP_KEY='the secret you give your wife' \
NURTUR_SESSION_SECRET='a random 32+ byte value' \
NURTUR_COOKIE_SECURE=true \
nurtur
```

The first browser that opens Nurtur is asked **“Do you already have an
account?”**

- **No, I'm new here** → enter the signup key, choose a username and password,
  and enter the baby's name, date of birth, and gender. The key is only checked
  locally; no email, OAuth, or external service is involved. Nurtur stores a
  PBKDF2-SHA256 password hash for each account.
- **Yes, sign in** → enter your username and password.

Anyone who knows the signup key can create another account. Every account has
its own private baby profile and care log: its own baby name, birth date,
gender, night window, feeds, diapers, sleep sessions, timeline, and exports.
Accounts cannot see or change each other's data. The baby's name is shown in the
header and sleep messages and can be corrected later on **Settings**. The birth
date and gender are written once during onboarding and cannot be changed; the
header shows the age derived from the birth date. `NURTUR_BABY_NAME` remains
only as a fallback for an account that has not saved a name yet.

Accounts created before the birth date and gender existed are sent to a
one-time **One more thing** page after their next page load. It asks for those
two details and then never appears again.

Every other browser/device sees the same question and can sign in. Accounts live
in the `accounts` table in the local database, so removing `NURTUR_SIGNUP_KEY`
after setup closes new signups without disabling the gate. Data created before
accounts were enabled is adopted by the first account created. To start over,
delete the rows from `accounts` (or use a fresh `NURTUR_DB`) and re-run
onboarding with the key.

The older single-passcode flow still works when `NURTUR_PASSCODE` is set and no
account exists:

```sh
NURTUR_PASSCODE='a long passphrase' \
NURTUR_SESSION_SECRET='a random 32+ byte value' \
NURTUR_COOKIE_SECURE=true \
nurtur
```

- Credentials are compared in constant time; passwords use per-account salted
  PBKDF2-SHA256 hashes, never plain text.
- Sessions are signed `HttpOnly`, `SameSite=Lax` cookies valid for 30 days.
  `NURTUR_SESSION_SECRET` keeps sessions alive across credential changes;
  without it the key is derived from the configured credentials, so changing
  either one signs everyone out.
- Login and signup attempts are limited to five per IP per 15 minutes (in
  memory).
- Mutations verify `Origin`/`Sec-Fetch-Site`; HTMX mutations also carry a
  per-session `X-CSRF-Token` (rendered into the page as `hx-headers`). Classic
  no-JS form posts rely on the same-origin check.
- `GET /healthz`, `/login`, `/signup`, `/offline`, `/sw.js`, and `/static/*` stay
  public so container healthchecks and the installable shell work before
  sign-in.
- The server sets a restrictive CSP, `nosniff`, `Referrer-Policy: same-origin`,
  `X-Frame-Options: DENY`, and a permissions policy. There are no third-party
  runtime requests.

TLS is still your responsibility: terminate it at a reverse proxy and set
`NURTUR_COOKIE_SECURE=true`.
