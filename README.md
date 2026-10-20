# Nurtur

A calm, one-thumb baby tracker for nursing, diapers, and sleep. One Go service,
server-rendered with HTMX, backed by a local [Turso](https://turso.tech) database
that syncs to Turso Cloud when configured.

This repository currently ships **Phase 2 — Diapers, Sleep + At-a-Glance
Dashboard**: one-tap diaper and sleep logging, a live three-card status strip,
wake windows, day/night sleep totals, and the Phase 1 nursing timer. See
[`PLANS.md`](./PLANS.md) for the full roadmap and [`FEATURES.md`](./FEATURES.md)
for the product brief.

---

## Quick start

Requirements: Go 1.24+ (the Turso driver requires it).

```sh
make dev
```

Open <http://localhost:8080>. Local development uses `NURTUR_LOCAL_ONLY=true`
and writes `./data/nurtur.db`. `make dev` rebuilds and restarts on changes to
Go, template, CSS, JS, or migration files.

The default timezone is the system local zone. Set `NURTUR_TZ` (for example
`Asia/Kuala_Lumpur`) so "today" and edit inputs match the phone.

```sh
NURTUR_TZ=Asia/Kuala_Lumpur NURTUR_BABY_NAME=Pip make dev
```

No account, analytics, CDN, or external font request. Everything is served by
the one binary.

---

## Configuration

| Variable | Default | Purpose |
|---|---|---|
| `NURTUR_ADDR` | `:8080` | HTTP listen address |
| `NURTUR_DB` | `./data/nurtur.db` | Local Turso database file |
| `NURTUR_LOCAL_ONLY` | `false` | Skip Turso Cloud sync entirely |
| `TURSO_DATABASE_URL` | — | Sync target, e.g. `libsql://…` |
| `TURSO_AUTH_TOKEN` | — | Turso database token |
| `NURTUR_SYNC_INTERVAL` | `60s` | Periodic pull interval |
| `NURTUR_TZ` | system local | IANA timezone for display and day boundaries |
| `NURTUR_BABY_NAME` | `Baby` | Name in the header |
| `NURTUR_NIGHT_START` | `19:00` | Local night window start (`HH:MM`) |
| `NURTUR_NIGHT_END` | `07:00` | Local night window end (`HH:MM`) |
| `NURTUR_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error` |

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
export NURTUR_BABY_NAME='Pip'

make build
./bin/nurtur
```

The local file is the write path. A feed or diaper log commits locally and
returns immediately; a debounced background `Push` copies changes to Turso
Cloud. Pulls run at startup, every `NURTUR_SYNC_INTERVAL`, and on shutdown the
queue is flushed and the WAL checkpointed. A cloud outage degrades sync, never
logging.

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
  21:00 and ends at 07:00 by default; the header toggle cycles auto → day →
  night and stores `nurtur.theme`.
- The first-paint theme bootstrap is a nonce-protected inline script in
  `layout.html`; CSP allows no other inline script or style.
- Fonts are self-hosted, subsetted OFL WOFF2: Fredoka (display) and Nunito Sans
  (UI and timers). Licenses are in `web/static/fonts/`.
- Pip is inline SVG so the mascot can follow the theme through CSS variables.
- The three status cards fit at 360 px; below 360 px they become three slim
  rows. The dock stays in the bottom third with the feed, diaper, and sleep
  tiers in a fixed order.
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
internal/store/      Turso open/migrations, feed/diaper/sleep queries, sync worker
internal/server/     routes, handlers, middleware, page view models
internal/view/       renderer, formatting, icons, Pip poses
migrations/          0001_init.sql, 0002_phase2_indexes.sql
web/templates/       layout, dashboard, partials
web/static/          CSS, timer.js, vendored htmx, fonts, icons, images
```

---

## Tests

```sh
make test        # unit + integration
make race        # go test -race ./...
make vet
make staticcheck
```

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
  -e NURTUR_BABY_NAME='Pip' \
  nurtur
```

The image is Debian slim (glibc), not Alpine: `tursogo` loads a prebuilt native
library through `purego`. The service runs as a non-root user, keeps the
database on `/data`, and ships a `curl /healthz` healthcheck.

`GET /healthz` reports database health; `GET /healthz?verbose=1` adds the last
push/pull, pending-sync age, CDC operations, and WAL size. Peer at the JSON when
sync looks stuck.

### Docker Compose

```sh
docker compose up -d --build
```

Open <http://localhost:8005>. The compose file runs local-only by default and
keeps the database in the `nurtur-data` volume. To use Turso Cloud instead, set
`NURTUR_LOCAL_ONLY=false` plus `TURSO_DATABASE_URL` and `TURSO_AUTH_TOKEN` in
the environment before `docker compose up`.

### Backups (Phase 1)

Stop the service cleanly (which flushes sync and checkpoints), then copy
`NURTUR_DB` and its `-wal`/`-shm` siblings if present. Full JSON/CSV/SQL export
and a tested restore drill arrive in Phase 3.

---

## Security

There is no authentication in Phase 2. Run Nurtur on a trusted LAN or behind a
private network tunnel; do not expose it to the public internet until the Phase 3
passcode gate and TLS are in place.

The server sets a restrictive CSP, `nosniff`, `Referrer-Policy: same-origin`,
`X-Frame-Options: DENY`, and a permissions policy. There are no third-party
runtime requests.
