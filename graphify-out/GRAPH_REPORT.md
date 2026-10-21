# Graph Report - nurtur  (2026-10-21)

## Corpus Check
- 72 files · ~71,536 words
- Verdict: corpus is large enough that graph structure adds value.
- Unclassified: 11 file(s) not represented in the graph (top: (none) 5, .woff2 2, .example 1)

## Summary
- 978 nodes · 3405 edges · 35 communities (25 shown, 10 thin omitted)
- Extraction: 87% EXTRACTED · 13% INFERRED · 0% AMBIGUOUS · INFERRED: 456 edges (avg confidence: 0.85)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `52b5fc4f`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify update .` after code changes (no API cost).

## Community Hubs (Navigation)
- HTTP Handlers & Middleware
- Entrypoint & Stdlib Deps
- HTMX Library
- testing.T
- time.Time
- Dual-Side Breastfeeding Timer & Logger
- Web Templates & Views
- models.go
- time.Time
- Client Timer Script
- Deployment
- Feed
- App Configuration
- Diaper Domain Model
- Session
- SyncWorker
- App Wiring & Renderer
- Phase 3 — Timeline, Undo + Ship
- Playwright E2E Setup
- Debian-slim (glibc) Docker Runtime for tursogo
- Local-First Turso Topology (local file + async Push/Pull)
- Night Mode & Single-Hand UX
- Local Dev Script
- time.Duration
- Playwright Browser Smoke Suite (390x844)
- Font Licensing (OFL)
- Service Worker Caching
- HTTP Status Recorder
- Apple Touch Icon Branding
- PWA Icon 192 Branding
- Maskable Icon Branding
- Go Module Root
- PWA Icon 512 Branding
- Brand Identity Assets
- AGENTS.md

## God Nodes (most connected - your core abstractions)
1. `Server` - 105 edges
2. `newTestEnv()` - 44 edges
3. `Feed` - 40 edges
4. `Session` - 32 edges
5. `newTestStore()` - 32 edges
6. `Diaper` - 28 edges
7. `encodeTime()` - 28 edges
8. `te()` - 28 edges
9. `ie()` - 27 edges
10. `He()` - 27 edges

## Surprising Connections (you probably didn't know these)
- `5. Updates` --references--> `main()`  [INFERRED]
  DEPLOYMENT.md → cmd/nurtur/main.go
- `Deployment` --references--> `main()`  [INFERRED]
  DEPLOYMENT.md → cmd/nurtur/main.go
- `App Layout Shell` --references--> `App Icon (Pip Face)`  [INFERRED]
  web/templates/layout.html → web/static/img/icon.svg
- `run()` --calls--> `Load()`  [EXTRACTED]
  cmd/nurtur/main.go → internal/config/config.go
- `run()` --calls--> `New()`  [EXTRACTED]
  cmd/nurtur/main.go → internal/server/server.go

## Import Cycles
- None detected.

## Hyperedges (group relationships)
- **HTMX Out-of-Band Fragment Update Pattern** — web_templates_partials_status_statusstripoob, web_templates_partials_last_nursed_lastnursedoob, web_templates_partials_sleep_sleeppilloob, web_templates_partials_timeline_timelinetodayoob, web_templates_partials_toast_toastoob [EXTRACTED 1.00]
- **Nurtur Three-Phase Delivery Plan** — plans_nurtur_build_plan, plans_phase1_nursing_core, plans_phase2_diapers_sleep_dashboard, plans_phase3_timeline_undo_ship [EXTRACTED 1.00]
- **Quick-Logging Dock (Timer, Diapers, Sleep, Last Nursed)** — web_templates_layout_layout, web_templates_partials_timer_timer, web_templates_partials_diapers_diaperrow, web_templates_partials_sleep_sleeppill, web_templates_partials_last_nursed_lastnursed [EXTRACTED 1.00]
- **One-Thumb, 3 a.m. Design Language** — features_zero_friction_rule, features_night_mode_one_handed_ux, plans_thumb_first_principles, plans_night_first_palette, plans_pip_mascot [INFERRED 0.85]
- **Single Action Dialog Serving Multiple Bottom Sheets** — web_templates_layout_sheetdialog, web_templates_partials_status_statuscard, web_templates_partials_nursing_sheet_nursingsheet, web_templates_partials_diapers_diapersheet, web_templates_partials_sleep_sleepsheet [INFERRED 0.95]

## Communities (35 total, 10 thin omitted)

### Community 0 - "HTTP Handlers & Middleware"
Cohesion: 0.07
Nodes (23): NormalizeUsername(), ParseSide(), clientIP(), requiresAuth(), safeNext(), isoPtr(), verbose(), parseLocalDateTime() (+15 more)

### Community 1 - "Entrypoint & Stdlib Deps"
Cohesion: 0.06
Nodes (36): Real, Color, ValidationError, ValidationError, HashPassword(), TestHashPasswordRoundTrip(), TestHashPasswordUsesFreshSalt(), ValidatePassword() (+28 more)

### Community 2 - "HTMX Library"
Cohesion: 0.08
Nodes (101): a(), Ae(), an(), at(), B(), be(), bn(), bt() (+93 more)

### Community 3 - "testing.T"
Cohesion: 0.05
Nodes (85): TestNormalizeUsername(), TestEditValidate(), TestKindCoversBoth(), TestCompletedEditValidate(), TestOpposite(), TestParseSide(), TestOnboardingRejectsWeakCredentials(), TestOnboardingSignupThenLogin() (+77 more)

### Community 4 - "time.Time"
Cohesion: 0.17
Nodes (10): Edit, ValidateNotes(), ValidateTimes(), nullableString(), timePtr(), setSettingsTx(), encodeTime(), isUniqueViolation() (+2 more)

### Community 5 - "Dual-Side Breastfeeding Timer & Logger"
Cohesion: 0.25
Nodes (11): Streamlined Diaper Tracker (Wet/Dirty/Both), Dual-Side Breastfeeding Timer & Logger, Last Nursed Indicator, Resilient Timer State (localStorage + backend sync), Sleep & Nap Tracker with Wake Window Counter, Recommended Minimal SQLite Schema, 'Time Elapsed Since' Dashboard Status Cards, Feed Segments Model (derived per-side durations) (+3 more)

### Community 6 - "Web Templates & Views"
Cohesion: 0.08
Nodes (42): App Icon (Pip Face), Maskable App Icon, Dashboard View, App Layout Shell, Action Dialog Sheet System, Inline Theme Bootstrap Script, Server-Side View Dispatch, Auth Mode Variants (chooser / signup / signin) (+34 more)

### Community 7 - "models.go"
Cohesion: 0.14
Nodes (27): applyDiaperValidation(), applySleepEditError(), applySleepEditValidation(), applySleepSheetError(), applySleepSheetValidation(), DiaperEditView, DiaperSheetView, EditView (+19 more)

### Community 8 - "time.Time"
Cohesion: 0.11
Nodes (22): DayBounds(), Fake, NewFake(), DayBounds(), diaperIcon(), previousLocalDayStart(), NightIntervals(), overlap() (+14 more)

### Community 9 - "Client Timer Script"
Cohesion: 0.14
Nodes (26): activeTimerElapsed(), announce(), applyTheme(), armToast(), clearCache(), closeDialog(), closeOpenDialogs(), formatAgo() (+18 more)

### Community 10 - "Deployment"
Cohesion: 0.17
Nodes (13): main(), 1. Create the runtime config, 2. Add Nurtur to your Compose stack, 3. Point Caddy at it, 4. Start and onboard, 5. Updates, 6. Backups and restore, CI/CD boundary (+5 more)

### Community 11 - "Feed"
Cohesion: 0.12
Nodes (8): Feed, Segment, TestCurrentSegmentAndMostRecentSide(), scanFeed(), scanSegment(), segmentIndex(), queryer, scanner

### Community 12 - "App Configuration"
Cohesion: 0.14
Nodes (22): Config, env(), Load(), parseBool(), parseClock(), parseLevel(), clearEnv(), TestLoadDefaults() (+14 more)

### Community 13 - "Diaper Domain Model"
Cohesion: 0.10
Nodes (10): Consistency, Diaper, Edit, Kind, ParseConsistency(), TestSummary(), scanDiaper(), decodeNullTime() (+2 more)

### Community 14 - "Session"
Cohesion: 0.11
Nodes (13): csrfFrom(), Session, TestWakeWindow(), TestWakeWindowAcrossMidnight(), WakeWindow(), Build(), TestBuildEmpty(), TestBuildPartialAndWakeWindow() (+5 more)

### Community 15 - "SyncWorker"
Cohesion: 0.06
Nodes (29): run(), Store, Open(), splitSQL(), discardLogger(), DefaultSyncConfig(), SyncStats, SyncStatus (+21 more)

### Community 16 - "App Wiring & Renderer"
Cohesion: 0.12
Nodes (14): Clock, newAuth(), newLoginLimiter(), migrationVersions(), sqlNullable(), sqlText(), sqlTimePtr(), writeSchema() (+6 more)

### Community 17 - "Phase 3 — Timeline, Undo + Ship"
Cohesion: 0.33
Nodes (8): Nurtur Feature Brief, Nurtur Build Plan, Phase 1 — Foundation + Nursing Core, Phase 2 — Diapers, Sleep + At-a-Glance Dashboard, Phase 3 — Timeline, Undo + Ship, Local Account Onboarding with Signup Key, Nurtur README, PWA Shell, Service Worker and Offline Page

### Community 18 - "Playwright E2E Setup"
Cohesion: 0.14
Nodes (11): devDependencies, @playwright/test, name, private, scripts, test, test:headed, version (+3 more)

### Community 20 - "Local-First Turso Topology (local file + async Push/Pull)"
Cohesion: 0.32
Nodes (8): Manual Release Gate (soak, sync outage, restore, 3 a.m. test), Non-Goals for v1 (single user, single baby, no notifications), Risks and Mitigations Register, Local-First Turso Topology (local file + async Push/Pull), JSON/CSV/SQL Export and Restore Drill, /healthz Health and Sync Status Endpoint, Local-First Write Path with Debounced Push/Pull, One Server Instance per Database Constraint

### Community 21 - "Night Mode & Single-Hand UX"
Cohesion: 0.25
Nodes (6): Night Mode & Single-Hand UX, HTMX Out-of-Band Update Convention, Night-First Day/Night Token Palette, Pip the Sprout Mascot, Progressive Enhancement Contract, Progressive Enhancement (README)

### Community 22 - "Local Dev Script"
Cohesion: 0.48
Nodes (6): cleanup(), fingerprint(), NURTUR_LOCAL_ONLY, dev.sh script, start_server(), stop_server()

### Community 23 - "time.Duration"
Cohesion: 0.08
Nodes (23): BuildSegments(), CompletedEdit, Side, Opposite(), TestBuildSegmentsOrder(), TestBuildSegmentsSingleSide(), TestOpenSegmentCountsToNowAndClampsNegative(), TestTotalsDeriveFromSegments() (+15 more)

### Community 24 - "Playwright Browser Smoke Suite (390x844)"
Cohesion: 0.40
Nodes (6): Playwright Browser Smoke Suite (390x844), Chronological Feed with Quick Undo, Injected Clock Interface (elapsed time in Go), Soft Delete (deleted_at) for Real Undo, Layered Test Strategy (unit/integration/sync/concurrency/browser), History and In-Place Undo

### Community 25 - "Font Licensing (OFL)"
Cohesion: 0.83
Nodes (4): Self-Hosted OFL Fonts (Fredoka + Nunito Sans), Fredoka OFL License, Nunito Sans OFL License, SIL Open Font License 1.1

## Knowledge Gaps
- **27 isolated node(s):** `nurtur`, `NURTUR_LOCAL_ONLY`, `Debian-slim (glibc) Docker Runtime for tursogo`, `Apple Touch Icon (PWA Home-Screen Icon)`, `Nurtur Brand Mascot: Smiling Cream Fruit with Leaves` (+22 more)
  These have ≤1 connection - possible missing edges or undocumented components. (Counts symbols only; 101 node(s) total have ≤1 connection when file, concept and rationale nodes are included.)
- **10 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `Server` connect `HTTP Handlers & Middleware` to `Entrypoint & Stdlib Deps`, `time.Time`, `SyncWorker`, `App Wiring & Renderer`, `time.Duration`?**
  _High betweenness centrality (0.093) - this node is a cross-community bridge._
- **What connects `nurtur`, `NURTUR_LOCAL_ONLY`, `Debian-slim (glibc) Docker Runtime for tursogo` to the rest of the system?**
  _27 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `HTTP Handlers & Middleware` be split into smaller, more focused modules?**
  _Cohesion score 0.06666666666666667 - nodes in this community are weakly interconnected._
- **Why does `main()` connect `Deployment` to `Entrypoint & Stdlib Deps`, `SyncWorker`?**
  _High betweenness centrality (0.080) - this node is a cross-community bridge._
- **Should `Entrypoint & Stdlib Deps` be split into smaller, more focused modules?**
  _Cohesion score 0.06408345752608048 - nodes in this community are weakly interconnected._
- **Why does `Deployment` connect `Deployment` to `Phase 3 — Timeline, Undo + Ship`?**
  _High betweenness centrality (0.078) - this node is a cross-community bridge._
- **Should `HTMX Library` be split into smaller, more focused modules?**
  _Cohesion score 0.07897684839432412 - nodes in this community are weakly interconnected._