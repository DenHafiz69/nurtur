# Nurtur — Build Plan

**Product:** A solo-user, mother-focused baby tracker for nursing, diapers, and sleep.
**Stack:** Go + Turso + HTMX, server-rendered, one deployable service.
**Golden rule:** every frequent action must be doable with one thumb, in under two taps, at 3 a.m.

Source requirements: [`FEATURES.md`](./FEATURES.md)

---

## Strategy

Build in three phases. Each phase ends with a usable, deployable app — not a half-finished layer.

| Phase | Theme | Outcome | Estimate |
|---|---|---|---|
| **1** | Foundation + Nursing Core | A reliable dual-side feeding timer with recovery, manual corrections, and "last nursed" status | 4–6 days |
| **2** | Diapers, Sleep + Dashboard | One-tap diaper and sleep logging, live status cards, wake-window counter | 3–4 days |
| **3** | Timeline, Undo + Ship | Full history, one-tap undo, night mode, PWA install, export/backup, deployment | 4–6 days |

**Dependency:** Phase 2 depends on Phase 1's store, layout, and HTMX/OOB conventions. Phase 3 depends on all three event types existing.

**Working rule:** ship Phase 1 to a real phone before starting Phase 2. Timer reliability and thumb reach cannot be validated on a desktop browser.

---

## Architectural Decisions

| Decision | Choice | Why |
|---|---|---|
| Go version | Go 1.23+ | Standard library routing (`GET /feeds/{id}`), `embed`, modern `net/http` |
| Router | `net/http` only | No framework needed for ~20 routes |
| Database | **Turso** via `turso.tech/database/tursogo` | Local-first reads/writes with push/pull sync to Turso Cloud; no CGO |
| DB topology | Local Turso file + `NewTursoSyncDb` | Sub-millisecond local logging at 3 a.m.; cloud copy for durability |
| DB sync | Debounced `Push`, periodic `Pull`, flush on shutdown, `Checkpoint` | Cloud outage never blocks a feed or diaper log |
| Templates | `html/template` + `embed.FS` | One deployable service; no runtime file paths |
| Frontend | HTMX 2.x + tiny vanilla JS | Only JS for the ticking clock, theme persistence, and `<dialog>` behavior |
| CSS | Hand-written CSS with custom properties | No build step; the playful day theme and OLED night theme are first-class token sets |
| Fonts | Self-hosted OFL variable fonts | Cute without a third-party request or layout shift |
| Auth | None by default; optional passcode in Phase 3 | Solo user on LAN. Required before public exposure |
| Multi-user | No | Explicit non-goal |
| Time storage | UTC RFC3339 text in Turso | Sortable, explicit, portable |
| Display time | Configured IANA timezone (`NURTUR_TZ`) | Correct day grouping and wake windows |
| Data deletion | Soft delete (`deleted_at`) | Enables real undo without confirmation dialogs |
| Timer truth | Server is authoritative; `localStorage` is a display cache | Survives refresh, lock, tab switch, and server restart |
| Deployment | Single Go service + Docker/systemd, local Turso DB on a volume, synced to Turso Cloud | Fast local DB plus an off-site copy; no DB server to run |

### Progressive enhancement contract

Every mutation is a real `<form>` with `method` and `action`. HTMX intercepts it when available.

- `HX-Request: true` → return the target fragment plus any OOB fragments.
- No HTMX / JS disabled → return `303 See Other` to `/`.
- Validation error with HTMX → `422` + inline error fragment.
- All timestamps rendered server-side include a machine-readable `datetime`/`data-*` attribute.

### HTMX update convention

One mutation returns:

1. The primary fragment (e.g. the timer card or the new timeline row).
2. Out-of-band fragments for anything else that changed: `#status-strip`, `#last-nursed`, `#timeline-today`, `#toast`.

Use `hx-swap="outerHTML"` for cards and `hx-swap-oob="outerHTML"` for companions. Never replace `#app` or the scroll container on a mutation; that is what causes the page-jump problem.

---

## Project Layout

```text
.
├── cmd/nurtur/main.go            # config, wiring, graceful shutdown
├── internal/
│   ├── config/                   # env parsing + defaults
│   ├── clock/                    # Clock interface, fake clock for tests
│   ├── store/                    # Turso open, migrations, sync worker, transactions
│   ├── feed/                     # domain: state machine, durations, edits
│   ├── diaper/                   # domain
│   ├── sleep/                    # domain + wake window
│   ├── status/                   # dashboard aggregation
│   ├── timeline/                 # unified event feed, paging, delete/restore
│   ├── server/                   # routes, middleware, handlers
│   └── view/                     # renderer, view models, formatting helpers
├── migrations/
│   └── 0001_init.sql
├── web/
│   ├── templates/
│   │   ├── layout.html
│   │   ├── dashboard.html
│   │   └── partials/             # timer, status, timeline, dialogs, toast
│   └── static/
│       ├── app.css
│       ├── timer.js
│       ├── htmx.min.js
│       ├── fonts/                # self-hosted Fredoka + Nunito Sans
│       ├── icons/                # hand-drawn SVG icon set
│       └── img/                  # Pip mascot poses, app icons
├── Dockerfile
├── PLANS.md
├── FEATURES.md
└── go.mod
```

---

## Data Model

The schema in `FEATURES.md` is a good starting point, with three corrections:

1. **Side switches need segments.** A single `feeds` row cannot represent "left 8 min → right 12 min → left 3 min" without losing when each side happened. Add `feed_segments`; derive totals from segments.
2. **Diapers need consistency.** The feature list asks for it but the schema omits it.
3. **Undo needs soft delete.** Add `deleted_at` rather than deleting rows immediately.

The app runs Turso's SQLite-compatible engine locally (`tursogo`) and syncs the same database to Turso Cloud. The SQL below is ordinary SQLite DDL; no `PRAGMA journal_mode` or `busy_timeout` statements are needed. The local engine is configured through its DSN, and its WAL is bounded with `syncDb.Checkpoint()`.

### Migration `0001_init.sql`

```sql
CREATE TABLE feeds (
    id          INTEGER PRIMARY KEY,
    started_at  TEXT NOT NULL,                 -- UTC RFC3339
    ended_at    TEXT,
    last_side   TEXT CHECK (last_side IN ('left', 'right')),
    notes       TEXT,
    created_at  TEXT NOT NULL,
    deleted_at  TEXT
);

-- At most one active feed exists at a time.
CREATE UNIQUE INDEX one_active_feed
    ON feeds ((ended_at IS NULL))
    WHERE ended_at IS NULL AND deleted_at IS NULL;

CREATE TABLE feed_segments (
    id          INTEGER PRIMARY KEY,
    feed_id     INTEGER NOT NULL REFERENCES feeds(id) ON DELETE CASCADE,
    side        TEXT NOT NULL CHECK (side IN ('left', 'right')),
    started_at  TEXT NOT NULL,
    ended_at    TEXT,
    CHECK (ended_at IS NULL OR ended_at >= started_at)
);

-- At most one open segment exists at a time, globally.
CREATE UNIQUE INDEX one_open_feed_segment
    ON feed_segments ((ended_at IS NULL))
    WHERE ended_at IS NULL;

CREATE INDEX idx_feed_segments_feed ON feed_segments (feed_id);

CREATE TABLE diapers (
    id           INTEGER PRIMARY KEY,
    logged_at    TEXT NOT NULL,
    type         TEXT NOT NULL CHECK (type IN ('wet', 'dirty', 'both')),
    color        TEXT CHECK (color IN ('yellow', 'mustard', 'green', 'dark')),
    consistency  TEXT CHECK (consistency IN ('seedy', 'liquid', 'hard')),
    notes        TEXT,
    created_at   TEXT NOT NULL,
    deleted_at   TEXT
);

CREATE TABLE sleeps (
    id          INTEGER PRIMARY KEY,
    started_at  TEXT NOT NULL,
    ended_at    TEXT,
    notes       TEXT,
    created_at  TEXT NOT NULL,
    deleted_at  TEXT
);

-- At most one open sleep session.
CREATE UNIQUE INDEX one_open_sleep
    ON sleeps ((ended_at IS NULL))
    WHERE ended_at IS NULL AND deleted_at IS NULL;

CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE INDEX idx_feeds_started     ON feeds (started_at DESC);
CREATE INDEX idx_diapers_logged    ON diapers (logged_at DESC);
CREATE INDEX idx_sleeps_started    ON sleeps (started_at DESC);
```

### Domain rules

- **Duration is derived**, never duplicated: `SUM(strftime('%s', ended_at) - strftime('%s', started_at))` grouped by side. This prevents the stored totals from drifting after edits.
- **Elapsed time is computed in Go** from a single injected `Clock`, not in SQL, so tests can freeze time.
- **The active feed** is the non-deleted feed with `ended_at IS NULL`. The active segment is its open segment.
- **Overlap is allowed across domains** (a baby can sleep while feeding); it is not allowed within a domain.
- **Deleted active sessions** are closed first (`ended_at = now`), then soft-deleted, in one transaction.
- All multi-row transitions (`start`, `switch`, `stop`, `edit`, `delete`) run in a single `database/sql` transaction using Turso's MVCC. Partial unique indexes back up the application checks so concurrent double-taps cannot create two active sessions.

### Turso topology and sync rules

```text
Phone (HTMX client)
        │  HTTP
        ▼
Go server ──► local Turso DB (authoritative for reads/writes)
                    │
                    │  Push (debounced) / Pull (60s + startup)
                    ▼
              Turso Cloud (durable off-site copy)
```

- **Local DB is the write path.** `INSERT`/`UPDATE` returns as soon as the local transaction commits; logging never waits on Cloud.
- **Push is asynchronous and debounced** (2–5 seconds). A failed push is retried with backoff and shown as "Syncing…" / "Offline copy saved."
- **Pull at startup, on a 60-second interval, and on demand.** Pull is rejected/reconciled if it would conflict with an in-flight local write.
- **One server instance only.** The app is single-user; running two writers against the same sync database is unsupported.
- **Flush on shutdown:** stop accepting requests, drain the push queue, `Push`, `Checkpoint`, then close.
- **Checkpoint periodically** (`syncDb.Checkpoint`) to bound local WAL growth.
- **Clock and data** are local: a Turso Cloud outage degrades sync status, not app functionality.
- Migrations run against the local DB at startup and sync like any other write. `schema_migrations` + a single-instance deployment prevent races.
- For tests and local development, open the same engine with `sql.Open("turso", path)` and skip sync entirely.

### Feeding state machine

```text
IDLE ──tap Left/Right──▶ ACTIVE(side)
ACTIVE(left) ──tap Right──▶ ACTIVE(right)   # close left segment, open right
ACTIVE(right) ──tap Left──▶ ACTIVE(left)
ACTIVE(any) ──tap Done──▶ IDLE              # close segment + feed, compute totals
ACTIVE(any) ──edit start/duration──▶ ACTIVE  # adjust current segment
IDLE ──retroactive/manual log──▶ IDLE        # explicit start + end
```

UI controls:

- **Left** / **Right** — the large primary buttons. Tapping the inactive side starts or switches instantly.
- **Done** — ends the feed. Clearly separated and never adjacent to a switch button.
- The active side is visually dominant (fill, live timer, `aria-live` region).
- Tapping the already-active side is a no-op with a subtle "Already nursing left" toast; it must not accidentally end the session.

---

## UI Design Direction

**Concept:** a soft little nursery companion — playful in daylight, calm at 3 a.m.
**Not:** a clinical form, a generic dashboard, or a baby-themed toy.

The interface should feel like a warm sticker-covered notebook: rounded, tactile, and friendly. Cuteness comes from shape, color, tiny illustrations, and motion — never from clutter or slow interactions. At night, the same app tucks itself in: Pip sleeps, the palette drops to warm plum twilight and amber, and every playful detail becomes quiet.

**Mascot:** **Pip**, a sleepy little sprout. Pip appears in the header and empty states, never on top of a control. Poses:

- `pip-awake.svg` — idle, eyes open, one leaf up.
- `pip-nursing.svg` — curled around a tiny drop, used while a feed is active.
- `pip-sleeping.svg` — eyes closed, used during sleep and in night mode.
- `pip-empty.svg` — a small shrug, used only in empty states.

Pip is decoration with personality, not a status mechanism. The text must always be sufficient on its own.

### Design principles

1. **Thumb first, eyes second.** The three most common actions sit in the bottom third. The screen is readable at a glance, not studied.
2. **Cute but quiet.** Rounded shapes, pastel cards, and a tiny mascot — no confetti storms, no bouncing navigation, no sound.
3. **One primary decision per state.** Idle asks "which side?"; active asks "switch or done?"; asleep asks "wake up?". Everything else is secondary.
4. **Tactile, not flashy.** Presses squish, cards have a soft offset shadow, success is a small star pop. Nothing depends on animation to be understood.
5. **The night theme is a first-class design, not an inverted afterthought.** It keeps the same layout and personality, with less light and less color.
6. **The phone is the product.** Desktop is a comfortable centered view, not a separate information-dense dashboard.

### Visual system

#### Palette

```css
:root {
  /* Daylight — warm cream, peach, butter, sage */
  --bg: #fff8f0;
  --bg-dots: #f2e3d3;
  --surface: #ffffff;
  --surface-soft: #fff0e4;
  --ink: #3d3547;
  --ink-soft: #71697c;
  --coral: #b94a3f;         /* primary action */
  --coral-soft: #f8b8aa;
  --on-coral: #fff9f5;
  --peach: #ffd9c7;
  --butter: #ffe9ae;
  --sage: #c8e8d2;
  --sky: #cbe7f0;
  --lilac: #e0d2f4;
  --line: #eadccf;
  --danger: #a83d35;
  --shadow: #e7d6c6;
}

:root[data-theme="night"] {
  /* Night — warm plum twilight, lifted surfaces, muted amber */
  --bg: #1a1621;
  --bg-dots: #26202e;
  --surface: #2a2334;
  --surface-soft: #342c40;
  --ink: #f5eee9;
  --ink-soft: #c0b5cb;
  --coral: #ff9f87;
  --coral-soft: #74453c;
  --on-coral: #2b1411;
  --peach: #4a2f39;
  --butter: #57431f;
  --sage: #294633;
  --sky: #244252;
  --lilac: #3b3156;
  --line: #4b405c;
  --danger: #ff9d90;
  --shadow: #0d0a11;
}
```

- Day cards use pastel fills with dark ink. Never white text on a pastel.
- Night cards stay neutral and raised; the peach/butter/sage/lilac tints move into icon chips and action pills. Amber marks the active timer and primary emphasis. No pure white.
- Color is never the only signal: active side, success, and danger also use text, icon, or fill weight.
- Verify every text/background pair with a contrast checker; body ≥ 4.5:1, large timer ≥ 3:1.

#### Typography

- **Display:** self-hosted [Fredoka](https://fonts.google.com/specimen/Fredoka) (OFL), 500–700, for the wordmark, greeting, and empty-state headings. Rounded and friendly without being childish.
- **UI/body:** self-hosted [Nunito Sans](https://fonts.google.com/specimen/Nunito+Sans) (OFL), 400–800, for labels, timers, and body copy.
- Subset both to Latin + numerals as WOFF2, `font-display: swap`, and serve from `/static/fonts/`. No Google Fonts request at runtime.
- Base size 16 px. Secondary text never below 14 px. Timers are 44–56 px, weight 800, with `font-variant-numeric: tabular-nums`.
- Headings use slightly tight tracking; UI labels stay normal. Avoid all-caps except tiny status labels.

#### Shape, texture, and icons

- Cards: 24–28 px radius, 2 px `--line` border, `--shadow` offset (sticker-like, not glassy).
- Buttons: fully rounded pills; minimum 48 px tall, primary 56–64 px.
- Background: a very subtle CSS radial-dot / star pattern using `--bg-dots`; no image download, no gradient mesh.
- Icons: custom 24 px SVG set, 2.25 px rounded strokes: left/right drop, diaper cloud, moon, sun, star, undo, pencil, settings. No emoji as interface icons.
- One tiny decorative accent per surface, maximum: a star, a stitch line, or a leaf. Do not fill empty space.
- Illustrations are flat with a soft outline, two or three colors, and no shadows beyond a simple offset.

### Mobile-first layout

Single column, `100dvh`, max-width 520 px centered. The page itself never scrolls horizontally. The timeline is the only scroll region; the header and action dock stay put.

```text
┌──────────────────────────────────┐
│  ☾   Good morning          ⚙     │  header (56–72 px)
│  [ Last: Left · 1h 45m ago ]     │  sticker badge
│                                  │
│  ┌────────┐ ┌────────┐ ┌───────┐ │  3-up status cards
│  │  🍼    │ │  ☁︎    │ │  ☾    │ │  (tap to act)
│  │Nursing │ │ Diaper │ │ Sleep │ │
│  │ 2h 10m │ │ 45m    │ │ 45m   │ │
│  └────────┘ └────────┘ └───────┘ │
│                                  │
│  Today                           │
│  ○ 14:32  Nursed · L 8m · R 12m │  timeline
│  ○ 13:10  Dirty · mustard        │
│  ○ 12:40  Slept 1h 25m           │
│                                  │
├──────────────────────────────────┤
│  [    Left    ] [   Right   ]    │  bottom dock
│  [ Wet ] [ Dirty ] [  Both  ]    │  (thumb zone)
│  [     Start sleep / Wake up    ]│
└──────────────────────────────────┘
```

**Header** — theme toggle, a short greeting, baby name, and the last-nursed sticker. It compresses to a single row once the user scrolls; no large hero on a utility screen.

**Status strip** — three compact cards in one row at ≥360 px. Each card: icon, label, elapsed time, and one line of type/side detail. Whole card is tappable. At <360 px, switch to three slim rows rather than truncating.

**Timeline** — scrolls beneath the status strip. Day header is a small handwritten-feeling label with a summary. Rows are calm and low-contrast until tapped.

**Bottom dock** — always in the bottom 35% of the viewport, with `env(safe-area-inset-bottom)` padding. Three tiers:

1. **Nursing:** giant **Left** / **Right** buttons when idle. When active, the dock becomes `Nursing left · 12:34` + the opposite-side switch + a clearly separated **Done**.
2. **Diaper:** three equal pills — **Wet**, **Dirty**, **Both**.
3. **Sleep:** a slim full-width pill: **Start sleep** / **Pip is asleep · Wake up**.

When the feed is active, the timer is the dock's headline and the diaper row remains available one tier below. When Pip is asleep, the sleep pill becomes a warm "Wake up" card; the dock never jumps position.

**One-handed rule:** the top 55% of the screen is for reading; the bottom 45% is for acting. Settings and edits are the only destinations that may open a sheet.

**Larger screens:** keep the same centered 520 px column up to 900 px. Above 900 px, optionally place the timeline in a second column, but do not add new navigation. The mobile experience is the canonical one.

### Screen states

| State | Visual | Primary action |
|---|---|---|
| **Idle** | Pip awake, soft cream background, "Nothing logged yet today" if empty | Tap Left or Right |
| **Nursing** | Active side is a full coral pill with live digits; the other side is outlined; a slow amber glow behind the timer | Switch side or Done |
| **Asleep** | Cooler tinted card, Pip sleeping, moon icon, sleep duration counting | Wake up |
| **Logged** | Tiny star pop near the dock; toast with optional "Add details" or "Undo" | Continue |
| **Offline** | Small amber "Saved locally · syncing" chip in the header; no blocking modal | Keep logging |
| **Undo** | Toast slides up above the dock, 8-second timer ring | Undo |
| **Error** | Inline card with a plain-language message and one retry action | Retry |

### Component details

| Component | Look | Behavior |
|---|---|---|
| Side button | 56–64 px pill, drop icon + word | Inactive: outline. Active: filled coral, white text, live timer. Press squishes to 0.97. |
| Done button | Butter/sage pill with check icon | Requires no confirmation; toast offers Undo. Never adjacent to the switch target. |
| Timer | Large tabular digits, side label above, subtle breathing glow | Ticks every 250 ms; no reflow; `aria-live` announced every 30 s. |
| Status card | Pastel card, icon in a small circle, label, elapsed | Tap opens the matching action sheet; entire card is the target. |
| Diaper pill | Three equal pills with simple icons | One tap logs immediately. Details live in the toast/sheet, never block the log. |
| Sleep pill | Full-width slim card with moon/sun | One tap start/stop; active state shows elapsed time. |
| Sticker badge | Slightly rotated pill with dashed border | "Last: Left · 1h 45m ago"; rotates only ±1.5°, never enough to hurt readability. |
| Bottom sheet | Rounded top corners, cream/night surface, grab handle | Native `<dialog>`; focus trapped; swipe/backdrop/Esc closes. |
| Toast | Floating pill above dock, icon + text + action | 8 s for Undo, 5 s otherwise; pauses on hover/focus; announced politely. |
| Timeline row | Icon circle, title, detail, time, delete affordance | Tap to edit; delete is a small but ≥44 px target with an accessible label. |
| Theme toggle | Sun/moon in a small sticker circle | Three states: auto, day, night; long-press or settings for auto schedule. |
| Sync chip | Tiny cloud + dot, no spinner unless actively pushing | "Saved locally", "Syncing…", "Synced"; amber when offline, green when caught up. |

### Motion and feedback

- Press: `scale(0.97)` over 90 ms with a gentle ease-out.
- Active timer: a 3-second breathing glow, opacity 0.25 → 0.45; never a flash or strobe.
- Success: a six-point star pops 8 px and fades over 400 ms, near the pressed control.
- Sheet: 180 ms slide-up with a soft spring; backdrop fades at 120 ms.
- Toast: 150 ms slide-up; the Undo ring drains over 8 seconds.
- Pip blinks every 7–12 seconds in idle/day mode. No idle animation at night.
- Optional haptic: `navigator.vibrate(8)` on a successful log; silently skipped where unsupported.
- No sound. No confetti. No full-screen success takeover.
- `prefers-reduced-motion: reduce` removes glow, blink, star pop, and sheet spring; state changes become instant fades.

### Night mode behavior

- Default is **auto**: night from 21:00 to 07:00 local. Manual override is remembered per device (`nurtur.theme`).
- Inline `<head>` script sets `data-theme` before first paint; there is no white flash.
- Night mode keeps the same layout and copy, but:
  - Background is warm plum-charcoal; cards are lifted neutral surfaces with tinted icon chips.
  - Primary actions use muted amber; coral becomes a soft ember, never neon.
  - Pip sleeps, stars dim, the dotted background becomes almost invisible.
  - Success star is suppressed; only a low-opacity glow and the toast remain.
  - Timer digits use warm amber with high contrast.
- Night is the default for the 3 a.m. experience; day mode is the override, not the other way around.

### Voice and microcopy

Warm, brief, specific. Never baby talk. Never a paragraph where four words will do.

| Moment | Copy |
|---|---|
| Empty feeds | "Nothing logged yet today. Tap a side when you're ready." |
| Active feed | "Nursing left" / "12:34" |
| After Done | "Saved. Last side: left." |
| Diaper logged | "Wet diaper logged." / "Dirty diaper logged." |
| Sleep start | "Pip's asleep. Sleep well." |
| Sleep end | "Awake. That was 1h 25m." |
| Offline | "Saved on this phone's server. Sync will catch up." |
| Undo | "Deleted. Undo" |
| Validation | "That end time is before the start. Try again?" |

Avoid: "Oops!", "Yay!", "Good job, mama!", excessive exclamation marks, and clinical phrasing like "Record micturition event."

### Accessibility

- 48×48 px minimum targets; 8 px minimum spacing between destructive and primary actions.
- Visible focus ring (2 px amber + 2 px offset) on all interactive elements.
- Every icon-only control has an `aria-label`; every status has text, not color alone.
- Timer is a polite live region, updated at most every 30 seconds to avoid screen-reader spam.
- Sheets use `<dialog>`, trap focus, restore focus on close, and support Esc.
- 200% text zoom must not clip labels or push controls off-screen.
- Thin or low-contrast text is not allowed for any state that matters.
- No interaction depends on hover, long-press, or precise dragging.

### Responsive and platform details

- Test at 360×640, 390×844, 430×932, 768×1024, and desktop.
- Respect iOS safe areas (`env(safe-area-inset-*)`), especially the dock and sheets.
- `touch-action: manipulation`; `-webkit-tap-highlight-color: transparent` with a designed pressed state instead.
- Inputs use `font-size: 16px` to prevent iOS zoom; time inputs use native pickers.
- PWA icons: a simple Pip face on cream and on near-black; maskable safe zone respected.
- No horizontal scroll at any width; no layout shift when the timer starts or a toast appears.

### UI acceptance criteria

- [ ] A first-time user can start a feed without reading instructions.
- [ ] Every frequent action is in the bottom 35% of the viewport and reachable with one thumb at 390 px width.
- [ ] The three status cards fit at 360 px without truncating the time.
- [ ] No white flash, no bright success flash, and no pure-white surface in night mode.
- [ ] A feed, diaper, or sleep log gives visible feedback in under 100 ms (optimistic or local round trip).
- [ ] Playful details (Pip, star, sticker rotation) never overlap or obscure a control.
- [ ] The app still reads clearly with all motion disabled.
- [ ] Lighthouse accessibility ≥ 95; no contrast failures; no missing labels.
- [ ] Screenshots at 390 px look warm, rounded, and hand-made — not like a default admin template.
- [ ] The "3 a.m. test" passes: dark room, low brightness, no glare, no mis-taps, no confusion.

### UI phase mapping

| Phase | UI deliverable |
|---|---|
| **1** | Tokens, fonts, shell, header, sticker badge, bottom dock, idle/nursing Pip, feed timer, today's feed rows |
| **2** | Three status cards, diaper pills, sleep pill, detail sheets, toasts, wake-window display |
| **3** | Full timeline styling, undo toast, auto night mode, PWA icons/splash, final visual and accessibility pass |

---

## Cross-Cutting Standards

These apply from Phase 1, not bolted on later.

### Time and correctness

- `Clock` interface: `Now() time.Time`; real implementation returns UTC.
- Store UTC; convert to `NURTUR_TZ` only in the view layer.
- Timer renders `data-started-at` and `data-server-now` so the client can compute an offset and avoid phone-clock skew.
- Duration formatting: `0m`, `45m`, `2h 10m`, `1h 05m`; tabular numerals.
- Day boundaries and wake windows are computed in the configured timezone, with tests around midnight and DST transitions.

### Mobile UX

- Minimum 48×48 px tap targets; primary actions 56–64 px tall.
- Fixed bottom action dock inside thumb reach, with `env(safe-area-inset-bottom)`.
- No hover-only affordances, no double-tap zoom (`touch-action: manipulation`), no tiny icon-only actions.
- Scroll position preserved on every HTMX swap.
- Native `<dialog>` for edit/details sheets; focus trapped and restored.
- Respect `prefers-reduced-motion`; animations ≤ 150 ms and never required to understand state.
- Test at 360×640 and 390×844 CSS pixels, one-handed.

### Night-first palette

The full day/night token set lives in **UI Design Direction → Palette**. Define it in Phase 1; refine in Phase 3.

- Night theme is the default experience after 21:00; day theme is the override.
- No pure-white backgrounds at night, no saturated blue light, no flashing success states.
- Text contrast ≥ 4.5:1 for body, ≥ 3:1 for large timer text.
- Apply the theme in an inline head script before first paint; no flash.

### Data safety

- **Local Turso DB is the write target**; async `Push` to Turso Cloud. A cloud outage degrades sync status but never blocks a log.
- Short `database/sql` transactions; Turso MVCC handles concurrent local writes. One server instance only.
- `Checkpoint` on a schedule and on shutdown to bound local WAL growth.
- Every mutation validated server-side; client input is never trusted for durations.
- Soft delete retained; hard purge only via an explicit maintenance path.
- Logical export (`JSON`, `CSV`, `SQL`) plus Turso Cloud backups; restore drill documented and tested.

### Observability

- Structured `log/slog` logs for server start, migrations, mutations, and sync failures.
- `/healthz` checks local DB ping; `/healthz?verbose=1` reports last successful push/pull and pending sync age.
- Surface sync state in the UI as "Saved locally" / "Syncing…" / "Synced" — never a blocking spinner.
- No analytics, no third-party runtime requests. Fonts are self-hosted.

---

# Phase 1 — Foundation + Nursing Core

**Goal:** A mother can start, switch sides, stop, and correct a feeding on her phone. Refresh, screen lock, or server restart loses nothing.

**Demo at the end:** Open the app at 3 a.m., tap Left, lock the phone for 20 minutes, unlock it, see the correct elapsed time, tap Right, tap Done, see "Last side: Right · just now" and a correct entry in today's list.

### 1.1 Project bootstrap

- [ ] `go mod init`, pin Go version, add `turso.tech/database/tursogo`.
- [ ] `cmd/nurtur/main.go`: load config, open local Turso DB, start sync worker, run migrations, mount routes, start server, graceful shutdown on `SIGINT`/`SIGTERM`.
- [ ] Config via env with defaults:
  - `NURTUR_ADDR` (`:8080`)
  - `NURTUR_DB` (`./data/nurtur.db` — local Turso file)
  - `TURSO_DATABASE_URL` (`libsql://…`; sync target)
  - `TURSO_AUTH_TOKEN`
  - `NURTUR_LOCAL_ONLY` (`false`; skips sync for dev/tests)
  - `NURTUR_SYNC_INTERVAL` (`60s`)
  - `NURTUR_TZ` (system local, validated with `time.LoadLocation`)
  - `NURTUR_BABY_NAME` (optional)
- [ ] Embedded `web/` and `migrations/` via `embed.FS`.
- [ ] `GET /healthz` → local DB ping + version; verbose mode adds sync state.
- [ ] Request logging middleware, panic recovery, security headers (`X-Content-Type-Options`, `Referrer-Policy`, CSP allowing only self + inline theme bootstrap).
- [ ] `make dev` (or `justfile`) with live restart; `make build` produces the deployable service. Docker image uses a glibc base because `tursogo` loads a prebuilt native library.

### 1.2 Store, migrations, and Turso sync

- [ ] Migration runner with `schema_migrations` table; idempotent startup; portable SQL.
- [ ] Open the DB:
  - Sync mode: `turso.NewTursoSyncDb(ctx, turso.TursoSyncDbConfig{Path, RemoteUrl, AuthToken})` → `syncDb.Connect(ctx)`.
  - Local-only mode: `sql.Open("turso", path)` for tests/dev.
- [ ] Configure the local DSN busy timeout; use `syncDb.Checkpoint(ctx)` on a schedule and during shutdown instead of SQL `PRAGMA` statements.
- [ ] `WithTx(ctx, fn)` helper using `database/sql` `BeginTx`; never issue raw `BEGIN`/`COMMIT`.
- [ ] Sync worker:
  - Debounced `Push` after mutations (2–5 s), serialized so only one push runs at a time.
  - `Pull` on startup, every `NURTUR_SYNC_INTERVAL`, and on demand.
  - Retry with backoff; record last push/pull/error for `/healthz` and the UI status chip.
  - Flush pending changes and `Checkpoint` during graceful shutdown.
  - Tests can inject a fake syncer; domain code never calls Turso Cloud directly.
- [ ] Feed queries: create feed + first segment, close/open segment, stop feed, list recent feeds, get active feed, update feed, soft delete.
- [ ] Segment totals query grouped by side.
- [ ] Seed test fixtures only in tests, never in production.

### 1.3 Feed domain + endpoints

- [ ] State machine with explicit errors: `already_active`, `no_active_feed`, `invalid_side`, `invalid_time_range`, `already_deleted`.
- [ ] Idempotency guard: a double-tap on Start must not create two feeds (transaction + `one_active_feed` index; map the constraint error to `already_active`).
- [ ] Clamp negative durations to zero; reject `ended_at < started_at`.

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/` | Dashboard shell: active timer, last-nursed badge, today's feeds |
| `GET` | `/partials/feed/timer` | Authoritative timer card (polled/on visibility change) |
| `POST` | `/feeds` | Start a feed on a side (`side=left\|right`) |
| `POST` | `/feeds/{id}/switch` | Close current segment, open the other side |
| `POST` | `/feeds/{id}/stop` | Close segment and feed |
| `PATCH` | `/feeds/{id}` | Adjust start/end times or side durations |
| `DELETE` | `/feeds/{id}` | Soft delete / cancel active feed |

All mutations return the timer fragment and OOB `#last-nursed`, `#status-strip`, `#timeline-today`.

### 1.4 App shell and design system

Implement the Phase 1 subset of **UI Design Direction** above.

- [ ] `layout.html`: header, main scroll region, fixed bottom dock, toast region, `<dialog>` host.
- [ ] CSS tokens for the day/night palettes, spacing, radius, type scale, safe areas, and motion.
- [ ] Self-host Fredoka + Nunito Sans; preload the primary weights; no runtime font request.
- [ ] Header: Pip, baby name/greeting, theme toggle, and the **Last nursed** sticker badge.
- [ ] Bottom dock:
  - Idle: two large **Left** / **Right** buttons.
  - Active: live timer (side + elapsed), the opposite-side switch button, and a distinct **Done** button.
- [ ] Soft offset shadows, rounded cards, dashed sticker badge, and the subtle dot background.
- [ ] Empty state: Pip shrugging + "Nothing logged yet today. Tap a side when you're ready."
- [ ] Today's feed list: side split (e.g. `L 8m · R 12m`), total, start time, delete control.
- [ ] Press squish, reduced-motion fallback, and visible focus rings from the start.

### 1.5 Resilient timer

- [ ] Server renders active segment with `data-feed-id`, `data-side`, `data-started-at`, `data-server-now`, and accessible text.
- [ ] `timer.js`:
  - Tick every 250 ms; compute `elapsed = now - startedAt + clockOffset`.
  - Update only text nodes; no layout thrash.
  - Use `Intl.DateTimeFormat` only where needed; keep the hot path arithmetic.
- [ ] Mirror `{feedId, side, startedAt, serverOffset}` in `localStorage` under `nurtur.timer.v1`.
- [ ] On load: paint immediately from `localStorage`, then reconcile against `GET /partials/feed/timer`; **server wins** on mismatch.
- [ ] Reconcile on `pageshow`, `visibilitychange`, and `focus`.
- [ ] If offline, keep ticking from cached start; show a quiet "Offline — timer still running" hint.
- [ ] Clear `localStorage` when server reports idle or feed ends.
- [ ] `aria-live="polite"` updated at most every 30 seconds to avoid screen-reader spam.

### 1.6 Last-nursed indicator

- [ ] Query the most recent non-deleted feed with at least one segment.
- [ ] Render: `Last side: Left · 1h 45m ago`, or `Nursing left · 12m` while active.
- [ ] High-contrast, single line at 360 px; never truncate the side.
- [ ] Elapsed label ticks client-side; refreshed by OOB after mutations.

### 1.7 Quick-fix / manual edit

- [ ] `<dialog>` opened from the timer card and each history row.
- [ ] **Active feed:** `−5 min` / `+5 min` adjust current segment start; clamp so it cannot start in the future.
- [ ] **Completed feed:** edit `started_at`, `ended_at`, and per-side minute totals.
  - Server rebuilds segments from the submitted totals, preserving order `left → right` when both are non-zero.
  - Reject totals longer than the feed window (within a 60-second tolerance).
- [ ] Time inputs use local timezone and are converted to UTC on submit.
- [ ] Validation errors render inline in the dialog; the dialog stays open.
- [ ] Saving returns updated timer/badge/list via OOB.

### 1.8 Tests and acceptance

**Automated**

- [ ] Table-driven state-machine tests: start, double-start, switch, same-side tap, stop, stop-when-idle, delete-active.
- [ ] Integration tests with `httptest` + a temporary local Turso DB (`t.TempDir()` + `sql.Open("turso", path)`): start → switch → switch → stop produces correct left/right totals and one feed row.
- [ ] One-open-segment and one-active-feed constraints are enforced under concurrent requests.
- [ ] Manual edit rebuilds durations correctly; invalid ranges are rejected.
- [ ] Sync worker tests: push debounce, retry/backoff, pull-on-startup, shutdown flush, and "sync failure does not fail the mutation."
- [ ] Clock-freezing tests for elapsed rendering and timezone conversion.
- [ ] `go test -race ./...`, `go vet ./...`.

**Manual (real phone)**

- [ ] Tap Left, lock screen 10 minutes, unlock → elapsed is correct to the second.
- [ ] Refresh mid-feed → no reset, no duplicate feed.
- [ ] Switch sides 3 times → totals correct; no orphan open segments.
- [ ] Stop → last-nursed badge and today's list update without scroll jump.
- [ ] +5/−5 correction changes the displayed duration and survives reload.
- [ ] Kill and restart the server mid-feed → active feed and elapsed time resume.
- [ ] All primary controls reachable with one thumb at 360×640.
- [ ] No horizontal scroll, no double-tap zoom, no white flash.

**Phase 1 gate:** the app is deployed to a phone and used for one real day of feeds.

---

# Phase 2 — Diapers, Sleep + At-a-Glance Dashboard

**Goal:** Every frequent event can be logged in one tap, and the top of the screen answers "what happened last, and how long ago?"

**Demo at the end:** Wake up, see Nursing / Diaper / Sleep cards with accurate elapsed times, tap **Wet** once, tap **Dirty** once, start sleep, wake and stop sleep, and watch every card and the feed update in place.

### 2.1 Diaper tracking

- [ ] Migration addition if Phase 1 shipped without it: `consistency` column and indexes.
- [ ] Domain: create, list, update details, soft delete.
- [ ] Bottom dock gets a diaper row: **Wet**, **Dirty**, **Both** — 56 px tall, equal width, each with its own pastel tint and a simple SVG icon.
- [ ] One tap logs immediately with `logged_at = now`; no confirmation step.
- [ ] Success toast: `Wet diaper logged · Add details` (action opens dialog).
- [ ] Details `<dialog>` (optional): color chips (`yellow`, `mustard`, `green`, `dark`), consistency chips (`seedy`, `liquid`, `hard`), notes.
- [ ] Detail edits are `PATCH /diapers/{id}` and update the row without a reload.
- [ ] Time badge computes **last wet** and **last dirty** separately; `both` satisfies both categories.
- [ ] Undo control on the toast and row (soft delete; full restore UX in Phase 3).

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/diapers` | Log `wet\|dirty\|both` |
| `PATCH` | `/diapers/{id}` | Update color/consistency/notes/time |
| `DELETE` | `/diapers/{id}` | Soft delete |

### 2.2 Sleep and wake windows

- [ ] One active sleep session, enforced by the partial unique index.
- [ ] **Start sleep** / **Wake up** button, one tap, optimistic UI with server reconciliation.
- [ ] Active sleep timer uses the same offset/tick machinery as the feed timer.
- [ ] Retroactive log form: start and end datetime, validated and timezone-aware.
- [ ] Wake-window counter: time since the last sleep ended; if currently asleep, show sleep duration instead.
- [ ] Daytime vs nighttime totals per local day; night window defaults to `19:00–07:00` and is configurable in settings.
- [ ] Handle "never slept yet" with `—` rather than a fabricated duration.
- [ ] Overlapping sleep entries are rejected; editing a sleep cannot create an overlap.

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/sleeps` | Start sleep (no `ended_at`) or retroactive log |
| `POST` | `/sleeps/{id}/stop` | Wake up |
| `PATCH` | `/sleeps/{id}` | Correct times/notes |
| `DELETE` | `/sleeps/{id}` | Soft delete |

### 2.3 Status dashboard

- [ ] `status` service assembles a single view model: active feed, last feed, last wet, last dirty, active sleep, last wake, wake window.
- [ ] Three compact pastel cards at the top (peach, butter, sage/sky):
  - **Nursing:** `Right side · 2h 10m ago` / `Nursing left · 12m`
  - **Diaper:** `Dirty · 1h 40m ago` (with a secondary `Wet · 45m` line)
  - **Sleep:** `Awake · 45m` / `Asleep · 1h 05m`
- [ ] Each card has a rounded icon chip, label, and elapsed time; at 360 px all three fit without truncation.
- [ ] Cards are high-contrast, glanceable, and tappable to open the relevant action sheet.
- [ ] Relative labels tick locally every 30 seconds; a full refresh occurs on mutation and on `visibilitychange`.
- [ ] No polling faster than 60 seconds; avoid needless battery/network use.
- [ ] First paint is server-rendered — no loading skeleton for the primary status.
- [ ] Empty states are calm and instructive, not error-like.

### 2.4 OOB updates and scroll preservation

- [ ] Standardize a `renderMutation(ctx, w, primary, oob...)` helper.
- [ ] After any event mutation, refresh: `#status-strip`, `#last-nursed`, `#timeline-today`, `#toast`.
- [ ] Confirm the scroll container's `scrollTop` is unchanged after swap (Playwright or manual).
- [ ] Use `hx-preserve` only for the timer node if it contains live client state; prefer re-render + resync.
- [ ] Disable buttons during in-flight requests (`hx-disabled-elt="this"`) to prevent double logs.

### 2.5 Tests and acceptance

**Automated**

- [ ] `both` counts as both wet and dirty in last-event queries.
- [ ] Wake window across midnight, DST spring-forward/fall-back, and a currently active sleep.
- [ ] Daytime/nighttime split totals with sleeps spanning the night boundary.
- [ ] Overlap rejection for sleep; retroactive edit validation.
- [ ] Integration: one-tap diaper creates exactly one row on double-submit protection.
- [ ] Status aggregation with empty database, partial data, and deleted rows.

**Manual (real phone)**

- [ ] Log Wet, Dirty, and Both in one tap each; details remain optional.
- [ ] Start sleep, lock phone 30 minutes, unlock → sleep timer correct.
- [ ] Wake up → wake-window card starts counting; night/day totals update.
- [ ] Retroactive sleep appears in the correct local day.
- [ ] A mutation while scrolled halfway down does not move the page.
- [ ] Cards readable at a glance in a dark room.
- [ ] No accidental duplicate logs from rapid taps.

**Phase 2 gate:** one full day of feeds, diapers, and sleep logged without needing the database or a desktop.

---

# Phase 3 — Timeline, Undo + Ship

**Goal:** Make the app trustworthy over weeks: complete history, real undo, dark-room polish, installability, backups, and a repeatable deployment.

**Demo at the end:** Install the app to the home screen, open the timeline, delete a mistaken diaper, tap **Undo** within 8 seconds, toggle night mode with no flash, export a backup, and restore it into a fresh container.

### 3.1 Unified chronological timeline

- [ ] Query union of feeds, diapers, and sleeps, excluding soft-deleted rows.
- [ ] Reverse chronological, grouped by local date with a day header and daily summary (feeds count, diapers, sleep total).
- [ ] Cursor pagination: `GET /timeline?before=<RFC3339>&limit=50`; "Load earlier" appends via `hx-get`/`afterend`.
- [ ] Stable ordering: primary timestamp DESC, `id` DESC as tiebreaker.
- [ ] Row content:
  - Feed: `Nursed · L 8m · R 12m · 14:32`, side indicator, total.
  - Diaper: `Dirty · mustard · seedy · 13:10`.
  - Sleep: `Slept 1h 25m · 12:40–14:05`.
- [ ] Tapping a row opens the shared edit sheet.
- [ ] Rows use a soft icon circle, dotted connector line, and a small pastel event chip; the date header feels handwritten but stays legible.
- [ ] Day boundaries use the configured timezone, not UTC.
- [ ] Virtualization is not needed at this scale; cap the initial render at 50 rows.
- [ ] Indexes verified with `EXPLAIN QUERY PLAN`; target p95 < 100 ms for 10k events on a phone over LAN.

### 3.2 One-tap delete and real undo

- [ ] `DELETE /timeline/{kind}/{id}` → soft delete, return refreshed timeline + OOB status.
- [ ] Immediate optimistic removal; no confirmation dialog.
- [ ] Toast: `Diaper deleted · Undo` for 8 seconds.
- [ ] `POST /timeline/{kind}/{id}/restore` clears `deleted_at`; re-validates active-session constraints.
- [ ] Undo restores the exact original timestamps and details.
- [ ] Deleting an active feed/sleep closes it and clears the timer/localStorage.
- [ ] Double-delete is idempotent; restoring an already-active restored event is rejected cleanly.
- [ ] Keyboard/screen-reader accessible: toast is announced, undo is focusable.

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/timeline` | Full timeline page |
| `GET` | `/timeline/partial` | Paginated fragment |
| `DELETE` | `/timeline/{kind}/{id}` | Soft delete |
| `POST` | `/timeline/{kind}/{id}/restore` | Undo |

### 3.3 Night mode and visual polish

- [ ] Manual theme toggle in the header; persist in `localStorage` (`nurtur.theme`).
- [ ] Auto mode: night between 21:00 and 07:00 local, or explicit `auto | night | day`.
- [ ] Inline head script applies `data-theme` before first paint — no white flash.
- [ ] Audit every surface: cards, dialogs, inputs, toasts, focus rings, disabled states, sync chip.
- [ ] Replace any pure-white success flashes with muted amber/green.
- [ ] Respect `prefers-color-scheme` for the initial default.
- [ ] Night pass: Pip sleeps, star field dims, active timer turns warm amber, no pastel card is brighter than the text.
- [ ] Contrast check (≥ 4.5:1 body, ≥ 3:1 large timer) and color-blind-safe danger/success cues (icon + text, not color alone).
- [ ] Typography pass: Fredoka headings, Nunito Sans UI, tabular numerals for all timers, no text under 14 px for secondary info.
- [ ] Add subtle, interruptible transitions (90–180 ms); none required for comprehension; reduced motion removes glow/blink/star.
- [ ] Final cute pass: sticker badge rotation, star pop, Pip blink, and dock squish — then remove any detail that competes with a control.

### 3.4 PWA and offline shell

- [ ] `manifest.webmanifest`: name, short name, standalone display, night background, maskable icons.
- [ ] Apple touch icon and `apple-mobile-web-app-capable` metadata.
- [ ] Service worker:
  - Precache app shell, CSS, JS, fonts, icons, Pip illustrations, and offline page.
  - Network-first for HTML/data; never cache mutation responses.
  - Clear offline banner when disconnected.
- [ ] Show the local/cloud sync chip distinctly from network connectivity: the phone can be online while Turso sync is catching up.
- [ ] Keep the Phase 1 `localStorage` timer recovery working with the service worker installed.
- [ ] Optional stretch (only if time allows): queue diaper/sleep mutations in IndexedDB and replay on reconnect, with idempotency keys. **Not required for v1** — do not let it delay shipping.

### 3.5 Settings, export, and backup

- [ ] Settings page: baby name, timezone (read-only from env or selectable), night window, theme mode, sync status.
- [ ] `GET /export.csv` and `GET /export.json` for all events.
- [ ] `GET /export.sql` streams a logical SQL dump (`INSERT` statements) that can be replayed into a fresh Turso database.
- [ ] Enable and document Turso Cloud backups/snapshots in the Turso dashboard; record the retention window.
- [ ] Local snapshot procedure: stop the service cleanly (which flushes `Push` and runs `Checkpoint`), copy the local DB file, then restart.
- [ ] Restore drill: create a fresh Turso database → replay `export.sql` or start with the copied local DB → verify row counts and a sample of durations → re-enable sync with the new credentials.
- [ ] Show local DB path/size, last successful push/pull, and pending sync age (informational only).
- [ ] CSV columns and the restore runbook are documented in `README`.

### 3.6 Security and deployment

- [ ] Optional passcode gate:
  - `NURTUR_PASSCODE` env var; if unset, auth is disabled.
  - Constant-time comparison; signed, `HttpOnly`, `SameSite=Lax` session cookie; 30-day expiry.
  - Login rate limiting (in-memory, per IP).
- [ ] CSRF token on mutations when auth is enabled; verify `Origin`/`Sec-Fetch-Site` for state-changing requests.
- [ ] Secure headers and a restrictive CSP (self only; allow the small inline theme bootstrap via hash or nonce).
- [ ] Multi-stage `Dockerfile`:
  - Build with `CGO_ENABLED=0`.
  - Runtime on Debian slim (glibc) so `tursogo` can load its prebuilt native library; verify `linux/amd64` and `linux/arm64`.
  - Non-root user; `/data` volume for the local Turso DB.
  - Env: `TURSO_DATABASE_URL`, `TURSO_AUTH_TOKEN`, `NURTUR_DB=/data/nurtur.db`.
- [ ] `docker compose` example with a named volume, env file, healthcheck, and restart policy.
- [ ] Systemd unit alternative with `StateDirectory=nurtur` and credentials injected via `EnvironmentFile`.
- [ ] Startup order: open local DB → run migrations → connect/verify sync → start HTTP server. Fail loudly on missing Turso credentials unless `NURTUR_LOCAL_ONLY=true`.
- [ ] README: local dev, Turso setup, env vars, deploy, sync health, backup, restore, upgrade.
- [ ] `/healthz` used by the container healthcheck; degraded if sync is stale but the app is still serving.
- [ ] No external runtime CDNs, font requests, or analytics; all assets are self-hosted.

### 3.7 Final QA and release

- [ ] End-to-end smoke path: feed start/switch/stop, diaper wet/dirty/both, sleep start/stop, edit, delete, undo, export, restore.
- [ ] Add Playwright (or equivalent) smoke test for the above at a 390×844 viewport, including a reload mid-feed.
- [ ] 24-hour soak: server running, repeated mutations, no goroutine/FD growth, local DB/WAL size bounded, sync stays caught up.
- [ ] Turso sync drill: stop network → keep logging → restore network → confirm push catches up and cloud row counts match local.
- [ ] Backup restore drill on a clean machine with a fresh Turso database.
- [ ] Install to iOS Safari and Android Chrome home screen; verify standalone launch and safe areas.
- [ ] "3 a.m. test": use the app in a dark room with the phone at low brightness. No flashes, no glare, no mis-taps.
- [ ] Accessibility pass: keyboard navigation, focus order, labels, live regions, 200% text zoom.
- [ ] Performance: first contentful paint < 1 s on LAN; mutation round trip < 150 ms.
- [ ] Tag `v1.0.0`, write release notes, archive a backup.

**Phase 3 gate:** installable, restorable, and usable for a week without developer intervention.

---

## Test Strategy

| Layer | Tool | What it covers |
|---|---|---|
| Unit | `go test`, table-driven | State machines, durations, formatting, day/night splits, wake windows, timezone/DST |
| Integration | `httptest` + temporary local Turso DB | Routes, transactions, OOB fragments, soft delete/restore, constraints |
| Sync | Fake syncer + optional local Turso sync server | Debounce, retry/backoff, startup pull, shutdown flush, sync failure isolation |
| Concurrency | `go test -race`, parallel requests | One active feed/sleep, double-submit safety |
| Browser smoke | Playwright (Phase 3) | Timer reload, OOB updates, scroll preservation, install/offline shell |
| Manual device | Real phone | Thumb reach, lock-screen timer, dark-room glare, one-handed logging |

**Rules**

- Never test time with `time.Now()` directly; inject the clock.
- Every bug fix gets a regression test.
- Run `go vet`, `staticcheck`, and `go test -race ./...` before each phase gate.
- Keep a `testdata` seed command for realistic multi-day timelines.

---

## Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Timer drifts or resets on lock/refresh | Destroys trust in the core feature | Server-authoritative timestamps, client offset, server reconciliation, real-device lock tests |
| Lost feed due to stale `localStorage` | Wrong "last side" advice | Server always wins; clear cache on idle; reconcile on visibility/focus |
| Double taps create duplicate events | Dirty data, false status | `hx-disabled-elt`, idempotent handlers, unique active-session indexes, transactions |
| Timezone/DST bugs in day grouping | Events on the wrong day; wrong wake window | Store UTC, compute local in Go, DST tests, injectable clock |
| Turso sync lag or cloud outage | Data stale in the cloud, user worries | Local-first writes; visible "Saved locally / Syncing / Synced" chip; retry with backoff; startup pull |
| Two server instances writing to one sync DB | Conflicts and duplicate state | Single-instance design, documented; one process per database |
| Local WAL grows unbounded | Disk exhaustion | Scheduled `Checkpoint`, bounded push queue, soak test |
| Native Turso library / architecture mismatch | Container fails to boot | Debian glibc base, `amd64` + `arm64` builds, smoke test the image |
| Night mode glare or low contrast | Unusable at 3 a.m. | Night-first tokens, real dark-room test, no white flashes |
| HTMX swaps jump the scroll | Loses place in history | Never replace the scroll container; targeted/OOB swaps; explicit scroll test |
| Feature creep (charts, notifications, multi-baby) | Never ships | Non-goals below; Phase 3 is the release gate |
| Public deployment without auth | Private data exposed | Passcode gate + TLS + no public exposure by default |
| No usable backup | Permanent data loss | Logical export + Turso Cloud backups + tested restore drill |

---

## Non-Goals for v1

- Multiple users, accounts, or caregiver invites.
- Multiple babies.
- A native mobile app or direct phone-to-cloud sync. The server syncs to Turso Cloud; the phone is a thin HTMX client.
- Growth charts, milestone tracking, pumping logs, medication logs.
- Push notifications or reminders.
- Full offline write queue on the phone (the shell may be offline-capable; writes require the local server).
- Analytics, ads, third-party runtime scripts, or external font/CDN requests. Fonts and all assets are self-hosted.

These are deliberate. The product wins on speed and reliability for one mother, not on breadth.

---

## Global Definition of Done

- [ ] Every frequent action takes ≤ 2 taps and works one-handed at 360 px width.
- [ ] Timer survives refresh, tab switch, screen lock, and server restart.
- [ ] All mutations are server-validated and update the UI without scroll jumps.
- [ ] Night mode is the default experience and has no white flash or glare.
- [ ] Timeline supports delete + undo with exact restoration.
- [ ] A logical export can be restored on a clean machine; Turso Cloud backup/retention is verified.
- [ ] Local writes keep working through a Turso Cloud outage, and sync catches up cleanly afterward.
- [ ] `go test -race ./...`, `go vet ./...`, and the browser smoke suite pass.
- [ ] One real week of use with no data loss and no manual database intervention.

---

## Suggested Build Order (Checklist)

**Week 1 — Phase 1**
1. Bootstrap, config, Turso local DB + sync wiring, health check, embedded assets.
2. Migrations + store + transaction helper + sync worker.
3. Feed state machine + endpoints + tests.
4. Layout, tokens, fonts, bottom dock.
5. Resilient timer + last-nursed badge.
6. Manual edit dialog.
7. Deploy to phone; one-day trial.

**Week 2 — Phase 2**
1. Diaper schema/domain/endpoints.
2. Diaper one-tap dock + details sheet.
3. Sleep domain + timer + retroactive form.
4. Status aggregation service + three cards.
5. Standardize OOB mutation rendering.
6. Timezone/DST and empty-state tests.
7. One-day trial with all event types.

**Week 3 — Phase 3**
1. Unified timeline + pagination.
2. Soft delete, toast undo, restore.
3. Theme toggle, auto night mode, visual audit.
4. PWA manifest, service worker, offline page.
5. Settings, CSV/JSON/SQL export, Turso Cloud backup + restore runbook.
6. Auth (optional), Docker/systemd, README.
7. Browser smoke tests, 24-hour soak, sync-outage drill, backup restore drill.
8. Tag `v1.0.0`.
