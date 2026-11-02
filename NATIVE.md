# Nurtur — Android First

**Status:** proposal, not a build log. This is the decision document for turning
Nurtur into a native Android app, with iOS deliberately deferred. Updated
against the v0.5.0 web app (November 2, 2026).

**Current product:** a self-hosted Go 1.24 + Turso + HTMX web app with an
optional local-account layer, a four-tab mobile shell, and a private photo
album. One binary, one database, and either one shared family or one independent
profile per account. The phone is still a thin client; the server is the source
of truth. Logging needs the server process reachable, and the nursing alert only
fires while the page is open, as the README says plainly.

**What changes with native:** the phone stops being a thin client. It owns the
data, writes offline, wakes you when the feed passes your threshold, and lives
on the home screen. That is a different product with a different support burden
and a real reason to charge money.

---

## The short version

| Decision | Recommendation |
|---|---|
| Product shape | **Standalone, local-first Android app** with optional Nurtur Cloud. Self-hosted server is a free power-user mode. |
| Rewrite approach | **Kotlin + Jetpack Compose + Room**, with a pure-Kotlin domain module and app-private files for photo bytes. No Android imports in `core`, ready to become KMP for iOS later. |
| Free tier | **The whole tracker, forever, offline, no account, no ads.** Nursing, diapers, sleep, timeline, album, night mode, background alert, widgets, export, import, local backup. |
| Paid tier | **Nurtur Plus:** cloud backup (database and photos) + multi-device sync, caregiver sharing, unlimited babies, long-range insights, Wear OS, priority support. |
| Price | **$2.99/month, $19.99/year, $69.99 founder lifetime** (capped). Cosmetic supporter pack one-time. |
| The line we never cross | Never paywall logging, the nursing alert, album capture or local photo storage, night mode, export, import, undo, accessibility, or security updates. |
| Launch path | Closed test with 12+ testers for 14 days (new personal Play accounts), then staged production rollout. |
| iOS | Not now. Share the domain layer, not the UI. Revisit only after Android retention and paid conversion are proven. |
| Do not build | An ad-supported app, a Trusted Web Activity wrapper, or a per-seat family plan. |

---

## 1. The product decision hiding inside "native"

There are two different apps you could ship, and they have different business
models:

### A. Nurtur Companion — a native client for your own server

Free. Connects to the self-hosted Go server over LAN, Tailscale, or a public
URL. The server still owns the data. This is the most faithful extension of the
current README. It is also a niche product: normal Play Store users will not run
Docker, configure TLS, or keep a VPS.

### B. Nurtur — a standalone local-first app

The phone owns a local SQLite database. Logging works on a plane, in a basement,
and at 3 a.m. with no server. Cloud is optional: backup, sync, and sharing for
people who want it.

**Recommendation: build B, and keep A as a free mode inside the same app.**

Why:

1. **The mass market will not self-host.** A Play Store listing that requires a
   server is a listing that gets one-star reviews from people who expected an
   app.
2. **The native features only matter if the app is self-sufficient.** A
   locked-screen alert and an offline log are worthless if the app needs a
   server on the same Wi-Fi.
3. **The current product promise survives.** "No account, no ads, no cloud you
   didn't ask for" becomes the free tier, not a compromise. The paid tier is
   opt-in cloud.
4. **The self-hosted community is not abandoned.** Connecting to your own server
   stays free. You already paid for the server; you should not pay again for the
   client.

**What happens to the web app?** It stays. It becomes the self-hosted server UI
and a desktop/tablet view. Do not try to keep two UIs at feature parity forever.
Make the **schema and export format the contract**, not the pixels.

**What the native app must not become:** a second product with a different data
model. If the web app can't read a native export, the promise of "your data
stays yours" is broken.

### What the web app already ships (v0.5.0)

Use this as the parity baseline. The native app must not regress any of it.

- **Nursing:** left/right switch, pause/resume, manual ±5-minute and start/end
  corrections, last side, last nursed, and a threshold alert with ping,
  vibration, and an optional system notification while the page is open.
- **Diapers:** wet/dirty/both with optional colour, consistency, and notes, plus
  edit, delete, and undo.
- **Sleep:** start/wake, retroactive naps, day/night split using the night
  window, wake windows, and overlap refusal.
- **Dashboard and timeline:** status cards, baby age, today totals, one merged
  timeline with day grouping, day totals, whole-day paging, undo, and restore.
- **Quick logging:** a Home-only bottom sheet that logs a feed, diaper, or sleep
  without leaving the timeline.
- **Album:** a fourth tab with camera-first capture and multi-select, browser
  EXIF/resize/WebP encoding, private local or R2 storage, day-grouped browsing,
  a full-screen viewer, date/caption edits, soft delete with undo, and a 30-day
  retention sweep.
- **Accounts:** optional local accounts with PBKDF2 passwords, a shared signup
  key, single-use admin magic-link invites, and per-account data scoping.
- **Settings and data:** baby name and birth date, night window, nursing alert
  threshold, mascot, night mode, JSON/CSV/SQL export (JSON and SQL include
  album metadata, not photo bytes), and an in-app changelog with per-account
  seen-version dismissal.
- **Delivery:** one embeddable binary, Docker Compose or systemd, local Turso
  with optional Cloud sync, PWA install, and an offline shell that never caches
  HTML.

What it still does not have: import/restore, multi-baby, cloud accounts or sync
for the native app, PDF reports, and notifications while the phone is locked.

---

## 2. What native actually buys you

This is the justification for the rewrite. If these do not matter, keep the PWA.

| Capability | Web app today | Native app |
|---|---|---|
| Nursing alert while the phone is locked | No. Documented limit. | Yes. Exact alarm plus a high-importance notification. |
| Timer survives the app being closed | Server keeps counting, but the page must be open for the alert. | Yes. The start time is in the database; a notification chronometer keeps counting with zero wakeups. |
| Logging with no network | Writes need the local server process, even when it runs on the same machine. | Yes. Room is the write path. |
| Photo album | Fourth tab, but every photo uploads to the server or R2 and every read is proxied back. | Camera-first local capture, app-private files, instant browsing, optional encrypted cloud backup. |
| Multiple profiles | Optional local accounts and invites on the server, one baby per account. | Local profiles on the device; no password or signup key needed. |
| Home screen widget | No. | Yes. Glance widget with quick-log actions. |
| Quick Settings tile and long-press shortcuts | No. | Yes. |
| Notification actions (switch side, pause, done) | No. | Yes. |
| Share sheet for export and doctor report | Clunky download links. | Yes. Native share sheet. |
| Haptics, edge-to-edge, predictive back | Partial. | Yes. |
| Background sync | Turso Cloud sync when configured; no multi-device app sync. | Optional, batched, battery-aware. |
| Cloud backup for people without a server | No. | Yes, paid, including album photos. |

The rewrite is justified mainly by **the locked-screen alert, offline writes,
local photos, and widgets**. Everything else is polish. Build those first and
put them in front of a real parent before building anything else.

---

## 3. Monetization principles

These are the rules that keep the brand coherent. Write them down before the
first paywall.

1. **The 3 a.m. test.** If a feature is needed while holding a baby in the dark,
   it is free. Logging, pausing, switching sides, waking, the alert, the night
   palette, and undo are free.
2. **No ads. Ever.** Ads at 3 a.m. are hostile. Ads also break the "no
   third-party requests" promise and attract the wrong incentives.
3. **No data hostage.** Export is free and complete. A lapsed subscription
   stops sync; it does not lock, delete, or degrade local data.
4. **No per-seat pricing for a household.** One Plus subscription covers the
   caregivers in one household. Charging a partner to see their own baby's log
   is a bad look.
5. **No engagement mechanics.** No streaks, no guilt, no "your baby hasn't been
   logged today" push. The app should interrupt you less, not more.
6. **Charge for ongoing cost and genuine extra, not for safety.** Cloud storage
   and sharing cost money to run. Long-range analysis and Wear OS cost
   development. Basic caregiving does not.
7. **Upgrade prompts never interrupt a feed.** Upgrade surfaces live in
   Settings, the caregiver invite flow, and after a trial ends. Never in the
   timer card, never in a modal over the dock, never at night.
8. **A downgrade is graceful.** When Plus ends: sync pauses, a local backup
   reminder appears, cloud data stays readable for 90 days, and export remains
   one tap away.

---

## 4. Free vs paid

### 4.1 Free forever (the complete tracker)

| Area | What is free | Why |
|---|---|---|
| Nursing | Left/right switch, pause/resume, manual corrections, last side, last nursed, history | This is the product. Never gate it. |
| Diapers | Wet, dirty, both, colour, consistency, notes, history | Same. |
| Sleep | Start/wake, retroactive naps, day/night split, wake windows, overlap validation | Same. |
| Dashboard | Status cards, baby age, today totals | Context, not a feature. |
| Timeline | Full history, day grouping, day totals, paging, search/filter | Your data. Free. |
| Corrections | Edit, delete, undo, restore | Trust. Free. |
| Album | Camera-first capture, multi-select, day grouping, full-screen viewer, date/caption edits, delete with undo, 30-day recovery | Photos are your data. Free. |
| Night mode | Auto night window, manual theme, no white flash | A safety/comfort feature. Free. |
| Nursing alert | Threshold 0–120 min, sound, vibration, notification, repeats until Done | The single best reason to go native. Free. |
| Ongoing notification | Chronometer, switch, pause, done actions, survives reboot | Free. |
| Widgets | 2×2 status/quick log, 4×1 status, Quick Settings tile, app shortcuts | Native basics. Free. |
| Baby profiles | Up to **two** babies (twins are not an upsell) | Ethical call. See 4.4. |
| History retention | Unlimited, local | Data hostage is unacceptable. |
| Export | JSON, CSV, SQL, and a formatted PDF doctor report. JSON and SQL include album metadata and object keys; photo bytes travel with the encrypted backup. | Portability is a right, not a feature. |
| Import | Nurtur JSON/CSV/SQL, plus common CSV formats from other trackers | Switching friction should be zero. |
| Local backup | Encrypted file to a location you choose, restore from file | Free safety net. |
| Privacy | No account, no ads, no analytics, no third-party SDKs | The promise. |
| Accessibility | TalkBack, font scaling, contrast, reduced motion | Never a paid tier. |
| Self-hosted mode | Connect to your own Nurtur server; sync and sharing included | You pay for the server. |
| Updates | Security and bug fixes | Always free. |

### 4.2 Nurtur Plus (paid)

| Feature | Why it is paid | Notes |
|---|---|---|
| Automatic encrypted cloud backup | Ongoing storage and bandwidth; album photos are the largest part | Database and photo bytes restore to a new phone in one flow. Store display and thumbnail WebPs only, never camera originals. |
| Multi-device sync | Ongoing sync infrastructure | Phone + tablet; same household. Album metadata syncs; photo bytes arrive on Wi-Fi by default. |
| Caregiver sharing | The strongest recurring value; requires accounts, invites, presence, attribution | Up to 5 caregivers, roles: owner, editor, viewer. One payer. |
| Unlimited baby profiles | Extra data modelling and sharing complexity | Two free; more with Plus. |
| Long-range insights | Development cost; 30/90-day trends, averages, wake-window patterns | Factual only. No predictions, no "your baby is behind" language. |
| Weekly digest | Optional, opt-in, local or email | No push marketing. |
| Wear OS companion | Separate app module and testing | Quick log from the wrist. |
| Priority support and beta channel | Support time | Not a gate on fixes. |

### 4.3 One-time purchases (cosmetic, optional)

| Item | Price | Why |
|---|---|---|
| Supporter pack: theme colours, app icon alternatives, widget styles | $4.99 | Cosmetic. Available to self-hosters too. No functional advantage. |
| Tip jar | $2.99 / $9.99 | For people who want to support the work without a subscription. |

Cosmetics are a good fit for this brand: they do not affect caregiving, they do
not create a second class of user, and they give self-hosters a way to
contribute without buying cloud they do not need.

### 4.4 The multi-baby question

Charging for twins is ethically uncomfortable. A parent with twins has twice the
logging need and half the sleep. Do not make "baby #2" a separate purchase.

Recommendation: **two baby profiles free**, unlimited with Plus. This covers
twins, fosters, and a second child. The paid line is "unlimited profiles and
per-baby sharing," not "you may not track your second baby."

### 4.5 Explicitly never paid

- Logging any event, including corrections and undo.
- The nursing alert, notification actions, and widget quick-log.
- Album capture, browsing, edits, local photo storage, or the recovery window.
- Night mode or any accessibility setting.
- Export, import, or local backup.
- Deleting your data.
- Security fixes.
- A basic PDF summary for a paediatric visit. (A parent should never pay to show
  a doctor their baby's data. Charge for long-range analysis, not the export.)

---

## 5. Pricing and packaging

### 5.1 Recommended prices

Start in USD and set regional prices manually for the big markets, including
Malaysia.

| Plan | USD | MYR starting point | Notes |
|---|---|---|---|
| Free | $0 | RM 0 | Complete local tracker. |
| Plus monthly | $2.99 | RM 12.90 | For people who want to try before committing. |
| Plus annual | $19.99 | RM 79.90 | Default and highlighted. About one pack of diapers a year. |
| Founder lifetime | $69.99 | RM 249.90 | One-time product, capped at 1,000, launch window only. |
| Supporter pack | $4.99 | RM 19.90 | Cosmetic, permanent. |

Set regional prices with purchasing-power parity in mind. Do not let Google's
automatic conversion produce absurd prices in Malaysia, Indonesia, Brazil, or
India.

### 5.2 Why this shape

- **Annual, not monthly, is the default.** A baby tracker is a 12–36 month
  product. Monthly pricing invites churn and guilt. Annual matches the use case.
- **One tier, not three.** At 3 a.m., a feature-comparison matrix is noise. One
  paid plan is a kindness.
- **Lifetime is capped and honest.** A one-time product for a service with
  ongoing costs is a liability if uncapped. Sell 1,000, define "lifetime of the
  Nurtur service," and reserve the revenue. Do not quietly withdraw it.
- **No Play free trial is required.** Offer a **30-day Cloud welcome** instead:
  full Plus for the first month, no card, no auto-charge. The first month is
  when parents are most desperate for backup. At the end, sync pauses; data
  stays. This converts better than a payment-method-gated trial and feels
  generous.
- **Google takes 15%** on the first $1M per year (30% after). On a $19.99 annual
  plan, net is about $17. Hosting per family is cents per month. The real cost
  is support.
- **Grandfather early adopters.** If you raise prices later, existing
  subscribers keep their price. Say so in the listing.

### 5.3 The alternative if you hate subscriptions

A defensible fallback:

- **Pro, one-time $14.99:** unlimited babies, long-range insights, Wear OS,
  cosmetic packs.
- **Cloud, $1.99/month or $14.99/year:** backup, sync, sharing.

This is more honest about cost structure but harder to explain and harder to
sustain. The single Plus plan is simpler. Pick one and do not run both.

### 5.4 What not to do

- No ads, no ad-supported tier, no "remove ads" purchase.
- No charging for export, alerts, night mode, or accessibility.
- No per-caregiver seats.
- No fake scarcity, countdown timers, or "only 3 spots left."
- No price A/B tests in the first six months. Test paywall placement and copy
  instead. Price experiments burn trust in a small community.
- No linking to an external checkout from inside the Android app. Digital
  goods must use Play Billing (with narrow regional exceptions). Verify the
  current policy before launch.

---

## 6. Android architecture

### 6.1 Options considered

| Option | Speed | 3 a.m. quality | Verdict |
|---|---|---|---|
| Trusted Web Activity / PWA wrapper | Fastest | Cannot do locked-screen alerts, widgets, or reliable offline writes | **No.** It is the current app with a Play listing. |
| WebView + native plugins | Medium | Fragile; two runtimes; poor native feel | **No.** |
| Kotlin + Compose, local-first | Slow | Best | **Yes.** |
| Kotlin Multiplatform now | Slower | Good, but adds iOS scaffolding you do not need yet | **Not yet.** Structure for it; do not pay for it. |
| Flutter / React Native | Medium | Good, but duplicates the Go domain and adds a third stack | **No.** |

### 6.2 Recommended structure

```text
android/
  app/        Compose UI, navigation, DI, notifications, widgets, billing
  core/       Pure Kotlin: feed state machine, sleep rules, diaper rules,
              timeline grouping, day/night split, album date rules,
              formatting, export models. No Android imports. This is the future
              KMP module.
  data/       Room database, DAOs, repositories, migrations, app-private photo
              file store, import/export, encrypted backup.
  sync/       Optional API client, change queue, conflict resolution,
              WorkManager jobs, entitlement cache.
  wear/       Later. Separate module, same core.
```

Dependency direction: `app -> data -> core`, `app -> sync -> data`, `wear ->
core`. `core` depends on nothing Android.

### 6.3 Port the Go domain, not just the screens

The Go code is the specification. Port these packages and their tests before
writing UI:

- `internal/feed` — segment state machine, pause semantics, duration maths,
  validation, side switching, edit bounds.
- `internal/sleep` — wake windows, overlap refusal, day/night split.
- `internal/diaper` — type rules, "both counts as wet and dirty."
- `internal/status` — last event, elapsed time, current state.
- `internal/timeline` — merged ordering, day grouping, paging, soft delete and
  restore.
- `internal/album` and `internal/server/album_sweep.go` —
  upload-idempotency, taken-at source precedence (EXIF, file, upload, manual),
  plausibility bounds, content-type allowlist, caption limits, soft delete, and
  the 30-day retention rule.
- `internal/changelog` — version normalization, unseen-release calculation, and
  the per-account seen marker.
- `internal/view/format.go` — durations, relative time, age, date headers.

Do not port `internal/account` password hashing, sessions, signup keys, or
invites into the local app. Port only its scoping intent: a profile's data must
never leak into another profile. Cloud accounts reuse those tables server-side
(see 6.5).

Use the same test cases and edge cases: DST, midnight boundaries, a feed that
crosses the night window, a paused segment, a restored deleted row that would
overlap, a double tap that must not create two open feeds, a photo whose EXIF
date is in the future, and a deleted photo restored after the sweep ran.

The UI may diverge. The rules must not.

### 6.4 Data model

Keep the existing SQLite schema as the base. As of v0.5.0 it has six
migrations: events and settings (0001–0002), local accounts (0003), per-account
scoping (0004), invites (0005), and album metadata (0006). The event tables and
`album_photos` are a good foundation: UTC RFC3339 text timestamps, durations
derived from `feed_segments`, soft delete, and settings as key/value.

Add, do not rename:

```sql
-- Identity and sync (additive; nullable during migration)
ALTER TABLE feeds    ADD COLUMN uuid TEXT;
ALTER TABLE feeds    ADD COLUMN updated_at TEXT;
ALTER TABLE feeds    ADD COLUMN device_id TEXT;
-- ...same for feed_segments, diapers, sleeps

-- Multi-baby support
CREATE TABLE babies (
    id          INTEGER PRIMARY KEY,
    uuid        TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    birth_date  TEXT,
    created_at  TEXT NOT NULL,
    deleted_at  TEXT
);
ALTER TABLE feeds ADD COLUMN baby_id INTEGER;
-- ...same for diapers and sleeps

-- Album metadata and sync (bytes stay outside the database)
ALTER TABLE album_photos ADD COLUMN uuid TEXT;
ALTER TABLE album_photos ADD COLUMN updated_at TEXT;
ALTER TABLE album_photos ADD COLUMN device_id TEXT;

-- Cloud metadata (only used in cloud mode)
CREATE TABLE households (...);
CREATE TABLE devices (...);
CREATE TABLE sync_state (cursor TEXT, last_pull_at TEXT, last_push_at TEXT);
```

Do not port the server's account tables as-is. A local-first app has one
implicit profile, or a small set of local profiles, and no passwords, sessions,
signup keys, or invites until cloud sharing exists. Keep `account_id` only as a
migration seam if you support profiles; otherwise stamp `0`.

Album photos need one extra decision the web app does not have: where the bytes
live. Keep `album_photos` metadata in Room (including `upload_id` for idempotent
retries and `full_key`/`thumb_key` for backups), but store the WebP files in
app-private storage with a stable relative path. Never put blobs in Room. On
delete, keep the row as a tombstone for 30 days and sweep only after the sync
cursor has passed it. Add `uuid`, `updated_at`, and `device_id` to
`album_photos` like the event tables so metadata can sync; photo bytes sync
separately (Plus) or restore from the encrypted backup.

Room specifics:

- `exportSchema = true`, checked-in schemas, migration tests.
- WAL mode, foreign keys on, one transaction per mutation.
- Store timestamps as UTC RFC3339 text, exactly like the server, so export and
  import do not need a conversion layer.
- Keep `deleted_at` tombstones. Never hard-delete on the device until a sync
  cursor has passed them.
- Put device-local preferences (onboarding done, theme override) in DataStore,
  not the database. Anything exportable belongs in the database.

### 6.5 Sync API

The current server is server-rendered HTML with cookie sessions. Do not scrape
it or reuse session cookies. Add a small JSON API to the Go server:

```text
POST /api/v1/auth/device          exchange a magic-link token for a device token
GET  /api/v1/bootstrap            household, babies, entitlements, server_time
GET  /api/v1/changes?since=<cursor>
POST /api/v1/changes              batch upsert, idempotency key per mutation
POST /api/v1/billing/verify       Play purchase token
POST /api/v1/households/{id}/invites
POST /api/v1/album/presign        short-lived upload or download URL for photo bytes
```

The web app already has local accounts, a signup key, single-use magic-link
invites, and an admin screen. Reuse the `accounts` and `invites` tables and the
same per-account scoping for cloud, but replace cookie sessions with device
tokens for the app. Do not scrape the HTML routes or reuse session cookies.

Rules:

- Bearer device tokens, stored in the Android Keystore, revocable per device.
- Cursor-based changes, not full-table pulls.
- Idempotency keys on every mutation; retries must be safe.
- The server returns its time. Clamp device timestamps that are implausibly
  future or past, but never silently rewrite a real start time.
- Entitlements are checked server-side. The local app never blocks logging on a
  billing check. Cloud features simply stop when the entitlement lapses.
- Do not ship Turso database credentials to the client. The Go service is the
  authority and the only holder of database credentials.
- Photo bytes never travel through the JSON change feed. Use short-lived
  pre-signed URLs, keep the bucket private, and scope every key to the
  household.

### 6.6 Cloud topology

Two viable shapes:

| Shape | Pros | Cons |
|---|---|---|
| One database per household | Strong isolation; trivial export and delete; the existing single-writer rules hold per household | Many databases; more ops; per-tenant migrations |
| One database, `household_id` on every row | Simple ops; cheap; reuses current `account_id` scoping | Tenant isolation is a code discipline; delete is a cascade; noisy neighbours |

**Recommendation for the first Plus launch: one database per region with
`household_id` scoping, plus a tenant-isolation test suite.** Move to
per-household databases only if a customer, a regulator, or a scaling problem
demands it. Do not build a control plane you do not need.

If you keep the current Go + Turso code, the existing `account_id` scoping is
most of the work. Since migration 0004 the active-row unique indexes
(`one_active_feed`, `one_open_sleep`) are already **per account**, and settings,
invites, and album rows are scoped the same way. Map or rename that seam to
`household_id` and keep the existing isolation tests. Even then a database index
cannot prevent two offline devices from each opening a feed; that needs the
merge rules in 7.1. The same applies to `album_photos`: scope rows and object
keys by household, and make every signed URL short-lived and entitlement-checked.

### 6.7 Background execution: the part that decides whether this app is good

The nursing alert is the reason to go native. Get it right.

The web app's alert semantics are the specification: threshold 0–120 minutes,
ping, vibration, an optional system notification, repeat until **Done**, and no
reset when switching sides or pausing. Port those rules first, then add
locked-screen delivery.

- **Do not run a countdown.** Store `started_at`; render the elapsed time from
  the clock. A timer that ticks in a service is a battery bug and a drift bug.
- **Ongoing notification with `setUsesChronometer(true)` and `setWhen(start)`**
  gives an accurate, zero-wakeup display. Actions: switch side, pause, done.
- **Schedule the threshold alert with `AlarmManager.setExactAndAllowWhileIdle`.**
  `SCHEDULE_EXACT_ALARM` is user-grantable on Android 12+. Ask with a clear
  rationale the first time an alert is enabled, and degrade to an inexact alarm
  plus a notification if permission is denied. Do not use `USE_EXACT_ALARM`; it
  is reserved for alarm-clock and calendar apps.
- **Prefer no foreground service for the MVP.** An exact alarm plus a
  notification survives process death. If testing shows OEMs kill the alarm,
  add a `specialUse` foreground service with a manifest declaration and a Play
  Console justification. A nursing session can run 40+ minutes, so `shortService`
  is not enough, and `dataSync` is the wrong type.
- **Restore on boot.** `RECEIVE_BOOT_COMPLETED` re-posts the ongoing
  notification for an open feed or sleep.
- **Handle time changes.** `ACTION_TIME_CHANGED` and
  `ACTION_TIMEZONE_CHANGED` re-render, but never change the stored UTC instant.
- **Respect Do Not Disturb** by default. Offer an explicit "override DND for the
  nursing alert" only if the user grants notification policy access.
- **Channels:** `nursing_alert` (high, sound and vibration), `active_feed` (low,
  silent, ongoing), `cloud` (low, sync failures), `updates` (default, opt-in).
  No marketing channel.
- **Battery optimisation:** do not nag on first launch. Show a one-time help
  card after a missed alert, with OEM-specific steps. Test Xiaomi, Redmi, Oppo,
  Vivo, Samsung, Huawei, and a stock Pixel.
- **No full-screen intents.** A feed alert is not an alarm clock. A heads-up
  notification is enough.

### 6.8 Widgets, tiles, and shortcuts

- **Glance widget, 2×2:** "Last fed 1h 20m ago · Left", with Left, Right,
  Diaper, and Sleep buttons. Tap targets at least 48 dp.
- **Glance widget, 4×1:** status strip for nursing, diaper, and sleep.
- **Quick Settings tile:** "Log diaper" or "Start feed." Long-press to choose.
- **Static shortcuts:** Start feed left, start feed right, log wet, log dirty,
  start sleep, take photo.
- Widgets write through the same repository and transaction path as the app.
  Never open a second database connection or duplicate the validation rules.
- Update on database change, on `ACTION_APPWIDGET_UPDATE`, and via WorkManager
  for time-based text. Widgets are not a sync trigger.

### 6.9 Security and local data

- Room database encrypted with SQLCipher if the added complexity is acceptable;
  otherwise rely on Android file-based encryption and be explicit about it.
  Cloud backup must be encrypted regardless.
- Optional biometric app lock. Off by default. Do not block the app at 3 a.m.
  unless the user asked for it.
- `android:allowBackup="false"` and explicit `dataExtractionRules`. Android Auto
  Backup can send the database and album photos to Google Drive, which
  contradicts "your data stays yours." Offer a free, explicit encrypted backup
  file instead.
- Album photos are app-private files, not MediaStore entries. Strip EXIF during
  re-encode, exactly as the web app does, so GPS and camera metadata never enter
  a backup. Store only the WebP display copy and thumbnail, not the camera
  original.
- A deleted photo keeps its metadata as a tombstone for 30 days, then the bytes
  and row are removed by a sweep on launch and once a day. Restore must verify
  the file still exists before it re-links the row.
- If cloud photo backup is enabled, encrypt the bytes as well as the database.
  The bucket stays private, and every read uses a short-lived signed URL.
- No `FLAG_SECURE` by default. Parents share screenshots. Offer it as an
  optional privacy setting.
- No analytics SDK, no advertising ID, no Firebase by default. Play Console
  vitals are enough for the MVP. If you add crash reporting, use a
  privacy-respecting, self-hosted option and redact notes.

### 6.10 Performance budgets

| Metric | Target |
|---|---|
| Cold start to interactive | under 1.5 s on a mid-range phone |
| Log a diaper, tap to confirmation | under 150 ms p95 |
| Timer render after resume | under 100 ms |
| Encode one photo (display + thumbnail) | under 400 ms on a mid-range phone, off the main thread |
| Album grid scroll | no dropped frames on a mid-range phone |
| APK/AAB download size | under 25 MB |
| Crash-free sessions | at least 99.5% |
| ANR rate | under 0.5% |
| Battery during an open feed | no measurable wakeups beyond the notification |

Use Baseline Profiles, R8, App Bundles, and Macrobenchmark. A 3 a.m. app that
takes three seconds to open is a failed app.

---

## 7. Sync and multi-device: the hard problem

The current design assumes one writer per database. Two phones break that.
Treat this as the riskiest engineering work in the project.

### 7.1 Rules

1. **Client-generated UUIDv7 IDs.** Never rely on auto-increment across
   devices. UUIDv7 sorts by time and is index-friendly.
2. **Append-mostly events, last-write-wins per row.** Keep `updated_at`,
   `device_id`, and a monotonically increasing server revision. Merge at the
   row level, not the field level, for v1.
3. **Deletes are tombstones.** `deleted_at` syncs like any other change. Keep
   tombstones for at least 90 days.
4. **One open feed is a household invariant, not a database constraint.** If two
   devices each open a feed, the server picks the earliest `started_at` as the
   real one, closes the other at the same instant with zero duration, flags it,
   and shows a calm review sheet: "We found two feeds running. Which one was
   real?" Never merge silently.
5. **Overlapping sleeps** are trimmed at the later start and surfaced for
   review. Do not discard the row.
6. **Clock skew.** Use server time for the cursor and for plausibility checks.
   Use the device's monotonic clock for live display. A phone whose clock is
   five minutes fast must not create a feed in the future.
7. **Idempotency.** Every mutation carries a client-generated key. A retried
   POST returns the same result, not a duplicate.
8. **Sync cadence.** On write (debounced 2–5 s), on foreground, on manual
   refresh, and via WorkManager every 15–60 minutes. Never every second.
9. **Offline is normal.** The queue is durable. The app never shows a blocking
   spinner for a log.
10. **Album metadata syncs; photo bytes are optional.** `album_photos` follows
    the same row rules and is keyed by `upload_id`, so a retried upload cannot
    create a second photo. Bytes upload on Wi-Fi by default, never inside the
    change feed. A deleted photo's tombstone keeps its file until the 30-day
    retention window passes.

### 7.2 Conflict UI

A conflict is a trust event. Handle it in plain language, like the web app's
validation errors:

- "Two feeds were running on different phones. We kept the one that started
  first. Was that right?" with Keep this / Use the other / Merge.
- "This nap overlaps another nap. We trimmed the later one. You can fix it
  here."
- Never show a raw sync error code to a parent at 3 a.m. Say "Saved on this
  phone. We'll sync when you're online."

### 7.3 Testing

- Two emulators, same household, airplane mode on both, divergent edits, then
  reconnect.
- 1,000 queued mutations, app killed mid-sync, resume.
- Timezone change during an open feed.
- Device clock set five minutes fast, then correct.
- Server rollback and replay.
- Delete on one device, edit on the other, restore, delete again.
- A photo upload retried after a timeout creates one row, not two, and a
  restored photo still has readable bytes.
- Photo delete on one device and restore on the other converges without losing
  the file before the retention window ends.
- Tenant isolation: household A must never see household B, including in error
  messages, signed URLs, and exports.

---

## 8. Play Store and distribution

### 8.1 Before you write code

- **Package name is permanent.** Use a reverse-domain you control, for example
  `app.nurtur.android` if you own `nurtur.app`, otherwise
  `com.yourname.nurtur`. You cannot change it after the first upload.
- **Check the name.** Search trademarks in Malaysia, the EU, and the US for
  "Nurtur." Check the Play Store, the domain, and social handles. A rebrand
  after launch is expensive.
- **Register the Play Console account early.** $25 one-time, identity
  verification. New *personal* developer accounts must run a closed test with at
  least 12 testers for 14 continuous days before production access. Organisation
  accounts are exempt. Recruit the 12 testers before you need them; verify the
  current policy when you register.
- **Decide organisation vs individual.** An organisation account needs a legal
  entity and a D-U-N-S number, but avoids the 12-tester gate and looks more
  trustworthy for a paid health-adjacent app.

### 8.2 Technical requirements

- `minSdk 26` (Android 8.0) and target the latest required SDK (36 in 2026).
  Test Android 10 through 16.
- App Bundle, 64-bit, adaptive icon, monochrome icon for themed icons.
- Play App Signing; keep the upload key in CI secrets, never in the repo.
- `versionCode` strictly increasing; `versionName` from the release tag.
- Edge-to-edge, predictive back, per-app language.
- R8 and resource shrinking; keep the mapping file for crash deobfuscation.
- Request only `POST_NOTIFICATIONS`, `SCHEDULE_EXACT_ALARM`,
  `RECEIVE_BOOT_COMPLETED`, `VIBRATE`, `INTERNET`,
  `ACCESS_NETWORK_STATE`, and optionally `USE_BIOMETRIC`. No location, contacts,
  SMS, or broad storage permissions. Use the Android Photo Picker and the system
  camera intent so you do not need `READ_MEDIA_IMAGES` or a declared `CAMERA`
  permission for the album.

### 8.3 Billing

- Use the current Play Billing library. Subscriptions need base plans, a
  monthly and an annual option, and a 30-day welcome if you implement it
  server-side.
- Verify purchases server-side with Real-time Developer Notifications
  (Pub/Sub). Never trust the client's "I paid" flag.
- Handle grace period, account hold, pause, upgrade/downgrade, and restore
  purchases.
- A "lifetime" product is a one-time, non-consumable product, not a
  subscription. Implement restore and entitlement lookup manually.
- Test with license testers and test cards in internal testing before
  production.
- Google is the merchant of record for Play purchases and handles VAT/GST.
  You still need to account for the income.

### 8.4 Store listing

- **Category:** Parenting. Not Medical, not Health & Fitness. Those categories
  invite scrutiny you do not need for a logbook.
- **Title:** `Nurtur: Baby Tracker` (under 30 characters).
- **Short description:** something like "Nursing, diapers, sleep and photos.
  Private, offline, one-handed."
- **Screenshots:** show the 3 a.m. palette, the one-thumb dock, the widget, the
  timeline, and the album grid and viewer. Use fake data, never a real baby's
  log or a real baby photo. Include a night and a day screenshot.
- **Video:** 20–30 seconds, one hand, dark room, one tap.
- **Privacy policy URL and data safety form** must match reality. If the free
  app is local-only, "no data collected" is a powerful and honest claim.
- **Content rating:** complete the IARC questionnaire. Target audience 18+;
  do not opt into the Families programme. The app is for caregivers, not
  children.
- **Ads:** declare none.
- **Health disclaimer** in the listing and in the app: "Nurtur is a logbook,
  not medical advice."

### 8.5 Release process

- Internal testing → closed testing (12+ for 14 days on a personal account) →
  open testing if useful → production staged rollout at 5%, 20%, 50%, 100%.
- Halt the rollout on a crash spike or a data-loss report. Have a rollback plan
  that does not require a database downgrade.
- Use the Play pre-launch report. It catches layout and API-level issues on
  devices you do not own.
- In-app "What's new" already exists in the web app, with releases kept in the
  binary and a per-account seen-version marker. Keep it, and add a `platforms`
  field to changelog entries so mobile shows mobile notes.
- Ask for a rating after a completed feed in daylight, using the In-App Review
  API. Never at night, never after an error, never after a paywall.
- CI: Gradle lint, unit tests, screenshot tests, managed-device tests, signed
  bundle, upload to internal track. Keep the existing Go CI green as well.

### 8.6 Self-hosted and F-Droid builds

If you publish a build without Play Services, it cannot use Play Billing. That
is fine: self-hosted users get the app free and pay for their own server. Keep
the core free and the cloud optional. Do not let the F-Droid build become a
second-class product with a different data model.

---

## 9. Privacy, data, and trust

This is the brand. Treat it as a feature with an owner.

### 9.1 What you collect

| Mode | Data collected by you | Data safety declaration |
|---|---|---|
| Free, local-only | Nothing leaves the device | No data collected |
| Self-hosted | Nothing leaves the user's server | No data collected by you |
| Plus cloud | Account email, household and baby profile, care events, album metadata and (if enabled) photo bytes, device and purchase metadata | Account info, health and fitness info, photos, app activity, purchase history; encrypted in transit; deletable |

Be conservative. Baby feeding, diaper, sleep, and photo records are
health-adjacent and should be declared as health information, even if the app is
not a medical device.

### 9.2 Health and children

- Nurtur is a logbook. It does not diagnose, predict, or advise. Say this in the
  app, the listing, and the PDF report.
- Avoid percentile charts, "your baby is behind" language, and AI-generated
  advice. If you add growth charts, use WHO/CDC data, show the source, and frame
  it as a record, not an assessment.
- The app is for adults. Do not market to children, do not build a social feed,
  and do not enable user-to-user contact beyond a household invite.
- COPPA, GDPR-K, and Malaysia's PDPA all matter if you store a child's data.
  The caregiver is the account holder and gives consent. Minimise what you
  store, and make deletion real.

### 9.3 Legal and compliance

- Privacy policy, terms of service, and a subprocessor list (hosting, database,
  email, crash reporting if any).
- **Malaysia PDPA**, including the 2024 amendments: notice and choice, security,
  retention, access and correction, breach notification, and a named contact.
- **GDPR/UK GDPR** if you accept EU/UK users: lawful basis, data processing
  agreement, SCCs for transfers, DPIA for health data, erasure and portability.
- **CCPA/CPRA** if you accept California users: do not sell data, honour
  deletion and access.
- **Account deletion in-app and on the web.** Play requires it if accounts
  exist. Delete cloud data within 30 days, keep backups no longer than 90 days,
  and say so.
- **Breach plan.** Who is notified, how, and within what window. Test it once.
- This document is not legal advice. Have a lawyer review the policy and the
  cloud terms before you charge money.

### 9.4 Encryption and backups

- TLS 1.2+ for all cloud traffic. No certificate pinning; it creates
  operational fragility for little gain.
- Encrypt cloud data at rest. Encrypt local backups with a user passphrase or a
  Keystore-wrapped key, and warn that a lost passphrase means a lost backup.
- Nightly cloud backups, tested restore, 30-day retention after cancellation,
  then deletion. Publish the retention window, including how long album photos
  stay after a subscription lapses.
- A backup contains two halves: the database (events, settings, album metadata)
  and the photo bytes. Restore must relink `album_photos` rows to files and
  verify every file before it reports success. A restored row with a missing
  file should stay visible and say so, not disappear silently.
- Do not log care notes, baby names, or email addresses in plaintext. Redact
  diagnostics before the user sends them.

### 9.5 Permissions and transparency

- Ask for notifications when the user enables the nursing alert, not on first
  launch.
- Ask for exact-alarm access with a one-sentence reason: "So the nursing alert
  can wake your phone when the feed passes your threshold."
- A "What Nurtur stores" screen: local database, cloud status, last backup,
  what is shared with whom.
- A "Delete everything" button that actually deletes everything, including the
  local database, album files, and app-owned backups, after a typed
  confirmation. Export first, always.

---

## 10. Licensing, open source, and brand

There is **no LICENSE file** in the repository today. That means "all rights
reserved" by default, which is ambiguous for a public GitHub project. Decide
before the Play listing.

Three coherent options:

| Option | Server/web | Android app | Fit |
|---|---|---|---|
| Open core | AGPL-3.0 | Proprietary | Keeps the self-hosted community, protects the hosted service, lets you charge for cloud. Recommended. |
| Fully open | AGPL-3.0 or MIT | Same licence | Maximum goodwill; anyone can build a free clone, including the paid features. Monetisation must be hosting and support. |
| Fully closed | Proprietary | Proprietary | Simplest commercially, weakest community story, contradicts the current README tone. |

If you choose AGPL for the server, the network-use clause means anyone offering
a modified Nurtur as a service must offer the source. That is usually what you
want.

Other brand work:

- Trademark search and registration for "Nurtur" in the classes that matter.
- Keep the OFL font licences and attributions for Fredoka and Nunito Sans.
- A contributor policy (DCO or CLA) if you accept pull requests, especially if
  you may dual-licence later.
- Decide what happens if someone publishes a paid clone. You cannot stop a
  determined competitor; you can win on trust, support, and cloud quality.

---

## 11. Business operations

### 11.1 Costs to launch

| Item | Estimate |
|---|---|
| Play Console registration | $25 one-time |
| Domain and email | $50–150/year |
| Hosting for cloud beta | $20–100/month |
| Privacy policy and terms review | $500–2,000 one-time |
| Test devices (low-end Android, Samsung, Xiaomi) | $500–1,000 one-time |
| Icon, screenshots, video | $200–1,500 one-time |
| Contingency | 20% |

A lean launch is a few thousand dollars plus your time. Album photo backup adds
object-storage cost, but the WebP display and thumbnail copies keep it small;
price Plus with that growth in mind. The native rewrite is the real cost: expect
3–4 months of focused work for a quality MVP, not 3–4 weeks.

### 11.2 Unit economics

- Play fee: 15% on the first $1M per year.
- Cloud cost per household: cents per month for text rows and a small database.
- Support is the largest marginal cost. Every confused parent is a ticket.
- At $19.99/year and 1,000 paying households, gross is about $20k/year, net
  about $17k before hosting and support. This is a small, sustainable product,
  not a venture-scale one. Price and scope accordingly.

### 11.3 Support

- In-app FAQ and a "Send diagnostics" export that the user can review first.
- Email support with a 48-hour target. No live chat, no 3 a.m. on-call. Say so
  in the listing.
- A status page for cloud sync and backups.
- Macros for the top ten issues: alert not firing, sync stuck, restore purchase,
  move to a new phone, export, delete account, battery optimisation, widget
  stale, timezone wrong, invite expired.
- A generous refund policy. A $20 refund is cheaper than a one-star review from
  a sleep-deprived parent.

### 11.4 Metrics without betraying the promise

Use Play Console and privacy-preserving, opt-in aggregates. No third-party
analytics by default.

| Metric | Why |
|---|---|
| Installs and store conversion | Acquisition |
| Onboarding completion | First-run friction |
| First log within 10 minutes | Activation |
| Days with at least 3 logs | The real engagement signal |
| D7, D30, D90 retention | Whether the app is part of the routine |
| 30-day welcome → Plus conversion | Monetisation |
| Monthly and annual churn | Sustainability |
| Crash-free sessions, ANR rate | Quality |
| Support tickets per 100 users | Hidden complexity |
| Notification opt-out rate | Whether the alert is trusted |

Do not optimise for session length. A good baby tracker is opened for 10
seconds and closed. Success is trust, not time spent.

### 11.5 Marketing

- **Positioning:** "The baby tracker that stays on your phone." Privacy, calm,
  one hand.
- **Channels:** new-parent communities (follow their self-promotion rules),
  breastfeeding and postpartum support groups, doulas and lactation consultants,
  paediatric clinics, Malaysian and Southeast Asian parenting groups, short
  videos showing a real 3 a.m. log.
- **ASO:** "baby tracker," "feeding tracker," "nursing timer," "diaper tracker,"
  "sleep tracker," "newborn log." Put the privacy promise in the short
  description.
- **No fear marketing, no paid reviews, no fake urgency.** This audience is
  exhausted and heavily marketed to; calm is the differentiator.
- **Referrals:** keep them simple. A free month of Plus for both sides is fine
  if it complies with Play policy; do not build a multi-level scheme.
- **Localisation:** English first, then Malay and Indonesian, then Spanish and
  Brazilian Portuguese, then German and French. Date, time, and units matter
  more than perfect prose.

---

## 12. Native UX checklist

The web app already has the right instincts. Preserve them; do not "improve"
them into a dashboard.

- **First run:** baby name, birth date, night window. Skippable. No account, no
  email, no tour. Ask for permissions only when the feature is used.
- **One thumb:** bottom dock, 48 dp minimum touch targets, 56–64 dp primary
  actions, sheets from the bottom.
- **Night:** resolve the theme before the first frame. No white splash. No pure
  black. Keep the lifted plum-charcoal palette and muted amber accents.
- **Timer:** ongoing notification with a chronometer, actions, and a tap that
  opens the app on the timer. Survives reboot and timezone change.
- **Album:** capture in two taps from the tab, multi-select without blocking the
  UI, day-grouped grid with tile times, full-screen viewer with swipe,
  in-place date/caption edits, and the same 8-second undo. Deleted photos stay
  recoverable for 30 days; say so once, not on every delete.
- **Undo:** snackbar for 8 seconds, exactly like the web app. Restores are
  re-validated.
- **Errors:** plain language, no codes, no blame. "That nap overlaps one you
  already logged. Try a different end time?"
- **Empty states:** teach the next action. "No feeds yet. Tap Left or Right when
  you start."
- **Accessibility:** TalkBack labels, 200% font scaling, 4.5:1 contrast, no
  colour-only meaning, reduced motion, switch access.
- **Haptics:** soft and optional. Respect the system setting.
- **Share:** use the Android share sheet for export, backup, and the doctor
  report.
- **Search:** filter the timeline by type and date. Free.
- **Baby switcher:** two free profiles, shown in the header only when there is
  more than one.
- **No badge counts.** No red dots for engagement. A sync dot is fine.

---

## 13. Migration and interoperability

The data promise is the product. Make moving in and out boring.

- **Import the existing web export.** The JSON format is stable and, since
  v0.5.0, includes `album_photos`, a `photo_storage` note, and `settings`.
  Match field names exactly, including `left_minutes`, `right_minutes`,
  `segments`, `deleted_at`, `upload_id`, `full_key`, `thumb_key`, and RFC3339
  timestamps.
- **Import CSV and SQL** as well, for people who have a backup but not a server.
  CSV stays event-only; JSON and SQL are the complete formats.
- **Import from other trackers.** A CSV importer for the common apps
  (Huckleberry, Baby Tracker, Nara) removes the biggest switching barrier. Make
  it free.
- **Export the same formats** plus an encrypted native backup. JSON and SQL
  carry album metadata and object keys; the backup bundles the database and the
  photo bytes so restore is one flow. Never invent a proprietary format that
  the web app cannot read.
- **Photo bytes are a second stream.** Importing a web export without bytes
  restores the timeline and leaves photo tiles marked "missing file" until the
  bytes are supplied. Say that in the import summary rather than dropping rows.
- **Connect to a self-hosted server** with a URL and a device token. The app
  should tell the user clearly which mode it is in: "On this phone" vs
  "Synced with your server" vs "Nurtur Cloud."
- **Uninstall warning.** On first run, explain that local data lives on the
  phone. Offer a backup reminder after the first week. Do not nag.
- **iOS later:** reuse `core` and the same JSON API. Do not fork the schema. A
  SwiftUI client over the shared domain is the lowest-risk path; Compose
  Multiplatform for UI is possible but not required.

---

## 14. Risks and mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Native rewrite takes far longer than the web app | No launch, lost momentum | Ship one vertical slice first: feed timer + locked-screen alert + export. Do not build diapers or sleep until that works on a real phone for a week. |
| OEM battery killers suppress the alert | The core native promise fails | Exact alarm + boot restore + OEM test matrix + a help card after a missed alert. Measure missed alerts in beta. |
| Sync conflicts create duplicate or lost events | Trust is destroyed | UUIDs, idempotency, tombstones, deterministic rules, a review sheet, two-device tests. |
| Play rejects the app or the billing setup | Launch delay | Pre-review against the health, data safety, account deletion, and payments policies. Keep the category Parenting. Avoid medical claims. |
| Subscription churn is high | Revenue below plan | Annual default, 30-day welcome, one household plan, lifetime for founders, no per-seat fees. |
| Lifetime buyers become a permanent cost | Unfunded liability | Cap at 1,000, define the term, reserve the revenue, and do not repeat the offer. |
| A user loses data on uninstall | One-star reviews and real harm | Free local backup, backup reminder, 30-day cloud welcome, export always free. |
| Album bytes are lost, orphaned, or duplicated | Photos are irreplaceable | App-private files, upload-idempotency, verified restore, sweep only after retention, and one backup flow for database plus bytes. |
| A privacy breach destroys the brand | Existential | Collect little, encrypt, no ad SDKs, tenant isolation tests, incident plan, breach notification. |
| Someone clones the app or bypasses billing | Revenue loss | AGPL server, proprietary app, trademark, and compete on cloud quality and support. Do not rely on DRM. |
| Web and native drift | Incompatible data | Schema and export are the contract. Shared test vectors. Feature freeze on the web UI. |
| Feature creep (charts, AI, social) | Never ships | The non-goals below. Review monthly. Say no in writing. |
| Support load exceeds one person | Burnout | FAQ, diagnostics export, no live chat, 48-hour SLA, generous refunds. |
| Health claim or medical advice complaint | Regulatory and store risk | Logbook framing, no diagnosis, no predictions, source growth data, disclaimer everywhere. |

---

## 15. Roadmap and gates

### N0 — Decide and prepare (1–2 weeks)

- Confirm the product shape: standalone local-first, with self-hosted mode.
- Register the Play account, check the name, reserve the package.
- Recruit 12+ closed testers.
- Port the Go domain tests to Kotlin as a specification.
- Write the schema delta, the album byte-storage plan, and the import/export
  contract (metadata and bytes as separate streams).
- Build a clickable prototype of the timer and the widget.

**Gate:** a signed debug build runs on three real phones, and the domain tests
pass against the same fixtures as Go.

### N1 — The vertical slice (5–6 weeks)

- Feed timer: left/right, pause/resume, switch, edit, last side.
- Locked-screen alert with exact alarm and ongoing notification.
- Home screen widget with Left/Right/Done.
- Local database, export/import of the existing event JSON (album metadata and
  bytes follow in N2).
- Night palette, one-thumb layout.

**Gate:** 10 real feeds logged by a real parent, the alert fires within one
minute on a locked phone, no data loss after reboot, and export round-trips
through the web app.

Do not add diapers, sleep, accounts, or billing until this gate passes.

### N2 — The rest of the tracker (4 weeks)

- Diapers, sleep, timeline, album, undo, settings.
- Album capture, WebP encode, app-private files, viewer, edits, delete/restore,
  and the 30-day sweep, at parity with the v0.5.0 web app.
- Onboarding, accessibility, font scaling, timezone tests.
- Local encrypted backup (database plus photo bytes), import from CSV.
- Quick Settings tile, shortcuts, notification actions.
- Closed beta with 12+ testers for 14 days.

**Gate:** crash-free 99.5%, cold start under 1.5 s, no data-loss reports, and a
4.5+ rating from beta testers.

### N3 — Free production launch (3–4 weeks)

- Store listing, screenshots, video, privacy policy, data safety.
- Staged rollout, pre-launch report, support FAQ.
- No paid tier yet. Build trust first.

**Gate:** 30 days of production with no data-loss incident, ANR under 0.5%, and
a support load you can handle alone.

### N4 — Nurtur Plus (4–6 weeks)

- Cloud accounts, device tokens, encrypted backup (database and album photos),
  sync.
- Caregiver invites, roles, attribution, presence.
- Play Billing, server-side verification, entitlements, restore.
- 30-day Cloud welcome, then Plus.
- Multi-baby up to two free, unlimited Plus.
- Long-range insights and the opt-in weekly digest.

**Gate:** 1,000 queued mutations converge, two-device conflict tests pass,
billing sandbox and restore work, tenant isolation is tested, and the 30-day
welcome converts at a rate you are willing to publish.

### N5 — Polish and expansion (ongoing)

- Wear OS, cosmetic packs, more localisation.
- Tablet and foldable layouts.
- Growth charts, pumping/bottle logs, medication logs, milestones, if users ask
  and the trust cost is acceptable.
- Home Assistant or a public API for self-hosters.

### iOS gate

Do not start iOS until:

- Android has been in production for at least three months.
- D30 retention is at least 35%.
- Paid conversion is at least 3% of active households.
- There is a support process that does not depend on you answering every email.

Then move `core` to KMP, keep the Go API, and build SwiftUI. Share logic, not
UI. Do not promise a date before the gate.

---

## 16. Open decisions

Answer these before N1. Each one changes the build.

1. **Standalone or companion?** Recommendation: standalone, with a free
   self-hosted mode. If you only want a companion, the business case is much
   weaker and the Play listing should say so.
2. **One Plus tier or Pro + Cloud?** Recommendation: one Plus tier. Simpler to
   explain and to support.
3. **Is the Android app open source?** Recommendation: AGPL server, proprietary
   app. Decide before the first public release.
4. **Who hosts the cloud, and in which region?** Recommendation: one region
   close to your first users (Singapore for Malaysia/SEA), with a disclosed
   subprocessor list. Expand only when a customer needs it.
5. **Email magic link or passkey for cloud accounts?** Recommendation: email
   magic link first, passkeys later. No passwords, no social login.
6. **Do you keep the web UI at feature parity?** Recommendation: no. Freeze it
   as the self-hosted/desktop view and put new features in native first.
7. **Two free babies or all babies free?** Recommendation: two free, unlimited
   with Plus. It covers twins without an ethical problem.
8. **Lifetime cap and term?** Recommendation: 1,000 units, "lifetime of the
   Nurtur service," with a reserved fund. Write the terms in the listing.
9. **Minimum Android version?** Recommendation: API 26. Verify against your
   actual users; a lower floor costs testing time for little reach.
10. **What is the one metric that decides success?** Recommendation: days with
    at least three logs, not installs or revenue.
11. **Does Plus back up album photos, and for how long?** Recommendation: yes,
    encrypted, with the same retention as the database. Photos are the largest
    storage cost; measure real usage before promising unlimited backups.

---

## 17. Non-goals for native v1

- iOS, iPad, macOS, or a web rewrite.
- Wear OS, Android TV, Android Auto, or ChromeOS certification.
- A social feed, friend graph, or public sharing.
- Ads, ad SDKs, or an advertising ID.
- AI summaries, predictions, or medical advice.
- Growth percentile charts (revisit with WHO/CDC data and a clear disclaimer).
- Pumping, bottle, medication, temperature, or vaccine logs (likely free when
  added, but not in v1).
- Multi-language beyond English.
- A web dashboard at parity with the app.
- A family plan with per-seat pricing.
- Cloud-only features that break the local app when the network is down.

---

## 18. One-page summary

**Free, forever:** the whole tracker on your phone. Nursing, diapers, sleep,
timeline, album, corrections, night mode, the locked-screen alert, widgets,
shortcuts, two babies, unlimited local history, JSON/CSV/SQL/PDF export, import,
encrypted local backup (database and photos), no account, no ads, no analytics.

**Nurtur Plus, $19.99/year:** encrypted cloud backup for the database and album,
multi-device sync, caregiver sharing for the household, unlimited babies,
30/90-day insights, Wear OS, priority support. 30-day welcome, no card. Cancel
and keep your data.

**One-time:** cosmetic themes and icons, tip jar.

**Never paid:** logging, the alert, album capture or local storage, night mode,
export, import, undo, security, accessibility, deleting your data.

**The build:** Kotlin + Compose + Room, a pure-Kotlin `core`, the existing schema
and export formats, app-private photo files, a thin Go JSON API for cloud, exact
alarms for the alert, and a vertical slice before anything else.

**The test:** a real parent, one hand, a dark room, a locked phone, and a feed
that has run long enough to matter. If the alert fires and the log is still
there in the morning, the app is working.
