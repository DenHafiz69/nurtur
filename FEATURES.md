Ran check graphify and query architecture · Exited with code 127
Analyzed GRAPH_REPORT.md:L1-250
Analyzed .graphify_python:L1
Analyzed changelog.go:L1-150
Analyzed README.md:L1-150
Ran list internal packages and server routes
Ran verify feed and timeline capabilities · Exited with code 1

Since Nurtur is already in prod and built around a calm, one-thumb, 3 a.m. UX, the best additions are ones that cover real parenting workflows as your baby grows **without** cluttering the quick-log dock or breaking her muscle memory.

### 1. High-Impact New Features

1. **Partner / Caregiver Sharing (Shared Baby Scope)**
   - Right now, all data (`feed`, `diaper`, `sleep`, `album`) is scoped to a single `AccountID` ([server.go](file:///home/denhafiz/Coding/nurtur/internal/server/server.go)). If you or another caregiver log in with a separate account, you can't see or log for the same baby; if you share one login, you can't tell who logged what or revoke a device.
   - **Suggestion:** Introduce a `baby_id` (or household/caregiver invite link) so multiple accounts can share one baby's log and album, with optional subtle attribution (`Logged by ...`) in History.
   - **Why it helps in prod:** You can log a diaper or bottle from your phone and her Home status cards update without sharing passwords.

2. **Bottle / Pumped Milk / Solids (As Baby Grows)**
   - [internal/feed](file:///home/denhafiz/Coding/nurtur/internal/feed/feed.go) currently models left/right nursing timers only. As soon as expressed milk, formula, or solids enter the picture, there's nowhere natural to log volume (`ml`/`oz`) or pumping sessions.
   - **Suggestion:** Keep the primary dock clean (Nursing, Diaper, Sleep), and add **Bottle** (breast milk / formula + quick amount presets like `60 / 90 / 120 ml`) and/or **Pump** (L/R/Both + volume) either as an optional toggle in Settings or tucked into the `+` quick-log sheet so it doesn't add clutter if she's exclusively nursing right now.

3. **Offline Mutation Queue (Background Sync)**
   - Currently the service worker caches the shell and static assets and falls back to `/offline` on network errors ([README.md](file:///home/denhafiz/Coding/nurtur/README.md#L139-L146)). If she's in a basement, elevator, or spotty bedroom Wi-Fi at 3 a.m., a tap can fail.
   - **Suggestion:** Queue `POST`/`PATCH` mutations in IndexedDB when offline with a client-generated timestamp + idempotency key, show a subtle `"Saved offline — will sync"` pill, and replay them in order when connectivity returns.

4. **Gentle Live-Timer & Milestone Widgets (Optional)**
   - **Growth / Weight & Height Log:** A minimal log in Settings or a sub-sheet (date, weight, length, head circumference) — super useful right before pediatrician visits, and easy to export in CSV/JSON.
   - **Medicine / Fever / Vitamin D:** A simple timestamped note/dose log (e.g., Vitamin D drops, paracetamol with "last given Xh ago" safety check) — huge peace of mind when a baby is sick.

---

### 2. Low-Friction UX & Quality-of-Life Polish

1. **History Filtering & "Jump to Date" on `/timeline`**
   - `/timeline` ([server.go](file:///home/denhafiz/Coding/nurtur/internal/server/server.go#L197-L201)) currently shows all event kinds newest-first with whole-day paging. Once there are weeks or months of logs, finding "when did she last poop?" or "how did she sleep last Tuesday?" takes a lot of scrolling.
   - **Suggestion:** Add lightweight filter chips at the top of History (`All` · `Feeds` · `Diapers` · `Sleep` · `Dirty only`) and a native date picker to jump straight to a day.

2. **Cross-Tab / Multi-Device Freshness**
   - If she leaves the PWA open in the background overnight and wakes up to tap **Start sleep** or **Left**, stale DOM state can lag behind server state until a refresh or HTMX swap.
   - **Suggestion:** On `visibilitychange` (when the PWA returns to foreground) or `pageshow`, trigger a lightweight HTMX refresh of the status strip + Today section so the active timer, "Last nursed", and wake window are always current.

3. **Pediatrician / Summary View (Read-Only or Print-Friendly)**
   - You already have CSV/JSON/SQL exports ([server.go](file:///home/denhafiz/Coding/nurtur/internal/server/server.go#L211-L213)). A clean **"Last 7 days" summary card** (avg feeds/day, wet/dirty diapers/day, avg day vs. night sleep) on History or Settings gives instant answers at doctor appointments without needing to open a spreadsheet.

4. **Album Enhancements**
   - Since `v0.5.0` ([changelog.go](file:///home/denhafiz/Coding/nurtur/internal/changelog/changelog.go#L48-L77)) just shipped the Album tab:
     - **Milestone / Tag Chips** (e.g., `First smile`, `Monthly`) or **Favorites** filter so special photos don't get buried in daily snaps.
     - **Link Photos to Logs / Monthly Recap:** Surface a quiet "1 month ago today" or age badge (`Day 42`) in the full-screen viewer alongside the date/time.

---

### 3. Prod Safety & Operations (Since Your Wife Uses It Daily)

1. **Pre-Migration / Pre-Update Safety**
   - Make sure your backup script covers both the SQLite/Turso DB and the album photo directory/R2 bucket on a tested restore schedule, and consider a **"Share / Export Backup" reminder** or health indicator in `/admin` showing last successful sync/backup timestamp.
2. **Feature Flags in Settings for New Trackers**
   - Because she's using it in prod, gate any new dock/sheet items (like Bottle, Pumping, or Meds) behind per-account **Settings toggles** (default off for existing accounts) so an app update never surprises her at 3 a.m. with extra buttons she didn't ask for.

---

If you want to tackle one of these first, **History filtering + foreground refresh** is the fastest, zero-risk win for her daily use, while **Partner sharing** or **Bottle/Pumping (behind a Settings toggle)** are the biggest functional upgrades.
