# Nurtur

[![CI](https://github.com/DenHafiz69/nurtur/actions/workflows/ci.yml/badge.svg)](https://github.com/DenHafiz69/nurtur/actions/workflows/ci.yml)
[![Container](https://img.shields.io/badge/ghcr.io-nurtur-blue?logo=docker&logoColor=white)](https://github.com/DenHafiz69/nurtur/pkgs/container/nurtur)

> A calm little nursery companion for nursing, diapers, and sleep.

Nurtur is a baby tracker built for the parent holding the baby.

No menus to dig through, no accounts to create, no ads, no cloud you didn't ask
for. Just three big buttons, a timer that refuses to lie, and a quiet screen that
doesn't wake the room at 3 a.m.

It runs as a single self-hosted web app — installable on your phone, usable with
one thumb, and backed by a database you own.

**Built for the 3 a.m. version of you.**

---

## Why Nurtur

Most baby trackers are built like productivity apps: charts, dashboards, upsells,
and a login screen between you and the one thing you needed to record.

Nurtur goes the other way.

- **One thumb, one tap.** Every action lives in the bottom third of the screen,
  sized for a hand that is already holding a baby.
- **The screen stays still.** Logging a feed or a diaper updates the page in
  place. No reload, no jump, no losing your spot.
- **The timer is honest.** Lock the phone, switch apps, refresh, or restart the
  server — the feed keeps counting from the real start time in the database.
- **Nothing to lose.** Local-first storage, automatic cloud sync when you want
  it, and one-click JSON, CSV, and SQL exports.
- **Quiet by design.** A low-glare night palette, no pure black, no flashing, no
  growth-hacking notifications. Just your baby's day.

---

## What's inside

### Nursing that keeps up with you

- **Left / right in one tap.** Tapping the other side switches without stopping
  the feed. Tapping the active side is a friendly no-op, not a duplicate log.
- **A timer that survives real life.** Screen locks, tab switches, refreshes,
  and server restarts don't reset the feed. The server is the source of truth,
  and the browser reconciles against it.
- **Pause when your baby unlatches.** Pause without losing the time already
  nursed. Paused minutes stay out of the total, and Resume picks the same side
  back up where you left it.
- **"Last nursed" at a glance.** Side and elapsed time, right in the header, so
  you never have to guess which side is next.
- **Fix it after the fact.** `−5 min` / `+5 min` and manual start/end edits,
  with validation that keeps totals inside the actual feed window.
- **A nursing alert that actually helps.** A soft ping, vibration, and optional
  system notification when total nursing time crosses your threshold — repeating
  until you tap **Done**, without resetting when you switch sides.

### Diapers in a single tap

- **Wet**, **Dirty**, or **Both** — logged immediately from the dock.
- Colour, consistency, notes, and time are optional and tucked behind
  **Add details**, so the quick path stays quick.
- `Both` counts as the last wet *and* the last dirty change.
- A double tap won't create a duplicate row.
- Every entry is editable and deletable, with Undo on the toast.

### Sleep and wake windows

- **Start sleep** / **Wake up** is one tap.
- Log an earlier nap retroactively with validated start and end times.
- The card shows the live wake window (`Awake · 45m`) or the current sleep
  (`Asleep · 1h 05m`).
- Today's sleep is split into **day and night** using your own night window.
- Overlapping sleeps are refused in plain language, including when you restore a
  deleted one.

### A dashboard that answers "what now?"

Three compact status cards sit at the top of the home screen:

- **Nursing** — last side and how long ago
- **Diaper** — last type and how long ago
- **Sleep** — awake for, or asleep for

The header also shows your baby's current age, derived from their birth date.
Enough context to decide what to do next without reading a log.

The status cards open their detail sheets. When something is running, its row
in Today opens the same quick-log dock as the floating `+`, so pause, switch,
wake, or log a diaper straight from the row.

### A timeline you can actually fix

- **One history, newest first.** Feeds, diapers, and sleeps share a single
  `/timeline`, grouped by your local date.
- **Daily totals in every date header** — feeds, diapers, and sleep for that day.
- **Whole-day paging.** Days never split across pages, so the summary always
  matches the rows underneath.
- **Real undo.** Deleting a row replaces it with a hidden placeholder; Undo puts
  it back in the exact same spot for eight seconds. Restores are re-validated,
  so you can't accidentally create two open feeds or overlapping sleeps.
- **Log without leaving.** The floating `+` dock inserts new events into today's
  group and refreshes the header totals in place.

### Night mode and one-thumb ergonomics

- Auto night mode follows your night window (default **21:00–07:00**), with a
  manual light/dark toggle and no flash of the wrong theme on first paint.
- The night window is adjustable in **Settings**, and sleep totals use the same
  window.
- Touch targets are at least 48 × 48 px; primary actions are 56–64 px.
- The dock is a native drawer that works without JavaScript and remembers
  whether you left it open or closed.
- `prefers-reduced-motion` removes the glow, blink, and transitions.

### Installable, offline-tolerant, local-first

- Install Nurtur to your home screen and it runs app-style, in portrait, without
  browser chrome.
- The service worker caches the shell and static assets, and shows a calm
  offline page instead of a browser error.
- A header chip tells you whether you're **Synced**, **Syncing…**, on a
  **Local copy**, or fully **Local** — separate from whether the network is up.
- The local database is the write path. Logging is instant; syncing happens in
  the background and a cloud outage never blocks a feed.

### Your data stays yours

- **No account required** for a solo parent on a trusted network.
- **No analytics, no CDN, no third-party font or script requests.** Everything is
  served by the same binary.
- **One-click exports:** a structured JSON copy, a one-row-per-event CSV, and a
  replayable SQL dump that includes migrations and soft-deleted rows.
- **A documented restore drill**, because a backup you've never restored is just
  a hope.
- **Optional local accounts** for a shared phone or an internet-facing install:
  each account gets its own private baby profile and care log. No email, OAuth,
  or external identity provider.

### Updates you can read

- After a version change, Nurtur opens a short **What's new** sheet. Dismiss it
  with one tap and it stays dismissed for that account.
- The whole changelog, from **v0.1.0** onward, is kept in the app at
  **Settings → What's new**, so release notes work offline and never disappear.

---

## Under the hood

Nurtur is deliberately small and boring where it matters:

| Piece | Choice |
|---|---|
| Server | Go 1.24+, one binary that embeds templates, CSS, JS, migrations, and fonts |
| UI | Server-rendered HTML with [HTMX](https://htmx.org) and progressive enhancement |
| Storage | Local [Turso](https://turso.tech) / libSQL file, optional sync to Turso Cloud |
| Frontend | Hand-written CSS, a tiny timer script, no build step, no framework |
| Delivery | Docker image on GHCR, Docker Compose, or systemd |
| Quality | Go unit/integration tests, race detector, `go vet`, staticcheck, Playwright |

Every mutation is a real `<form>` with a `method` and `action`. With JavaScript
you get in-place fragments and toasts; without it you get a normal redirect and
a working app. The timer is corrected for phone clock skew. The schema uses
partial unique indexes so a double tap cannot open two feeds or two sleeps, even
under concurrency.

The result is a tracker with the feel of a native app and the durability of a
database file you can copy.

---

## Getting started

The short version:

```sh
cp .env-example .env
make dev
```

Then open <http://localhost:8080>. For Docker, Turso Cloud, backups, systemd,
and the full configuration reference, see [`DEPLOYMENT.md`](./DEPLOYMENT.md).

### Inviting people

Set `NURTUR_ADMIN_PASSWORD` (8+ characters) and restart. Open `/admin`, sign in
as `admin`, and generate a magic link. The link takes one person straight to
signup, without the household signup key, and stops working after it is used or
expires. Set `NURTUR_PUBLIC_URL` to the public origin when Nurtur is behind a
reverse proxy.

---

## Documentation

- [`FEATURES.md`](./FEATURES.md) — the product brief and the thinking behind the UX
- [`PLANS.md`](./PLANS.md) — roadmap, domain rules, design system, and release gates
- [`DEPLOYMENT.md`](./DEPLOYMENT.md) — VPS, Docker Compose, Caddy, and operations

---

## Honest limits

Nurtur is built for one family, not a fleet:

- Run **one server instance per database**. The sync engine and one-active-feed
  rules assume a single writer.
- The nursing alert runs in the open page or installed app. It cannot fire while
  the phone is locked or the browser is fully closed.
- TLS is your responsibility — terminate it at a reverse proxy and set
  `NURTUR_COOKIE_SECURE=true`.
- Charts, multi-baby switching, and engagement mechanics are intentionally out of
  scope. The app should interrupt you less, not more.

---

<p align="center"><em>Made for the long nights, the short naps, and the one free hand.</em></p>
