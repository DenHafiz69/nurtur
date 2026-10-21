# Graph Report - nurtur  (2026-10-21)

## Corpus Check
- 75 files · ~74,382 words
- Verdict: corpus is large enough that graph structure adds value.
- Unclassified: 11 file(s) not represented in the graph (top: (none) 5, .woff2 2, .example 1)

## Summary
- 991 nodes · 3518 edges · 46 communities (34 shown, 12 thin omitted)
- Extraction: 86% EXTRACTED · 14% INFERRED · 0% AMBIGUOUS · INFERRED: 509 edges (avg confidence: 0.85)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `5b84c217`
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
- TursoSyncer
- Fake
- Client Timer Script
- Deployment
- Feed
- App Configuration
- Diaper Domain Model
- Session
- SyncWorker
- newTestEnv
- Phase 3 — Timeline, Undo + Ship
- Playwright E2E Setup
- Debian-slim (glibc) Docker Runtime for tursogo
- Local-First Turso Topology (local file + async Push/Pull)
- Night Mode & Single-Hand UX
- Local Dev Script
- Feed
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
- server_test.go
- Build
- AGENTS.md
- account_test.go
- sync_test.go
- Store
- time.Location
- Color
- WakeWindow
- newTestEnvConfig
- SyncStatus
- NewSyncWorker

## God Nodes (most connected - your core abstractions)
1. `Server` - 102 edges
2. `AccountID()` - 46 edges
3. `newTestEnv()` - 44 edges
4. `Feed` - 40 edges
5. `newTestStore()` - 34 edges
6. `Session` - 32 edges
7. `encodeTime()` - 29 edges
8. `Diaper` - 28 edges
9. `te()` - 28 edges
10. `ie()` - 27 edges

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

## Communities (46 total, 12 thin omitted)

### Community 0 - "HTTP Handlers & Middleware"
Cohesion: 0.05
Nodes (38): NormalizeUsername(), ValidatePassword(), ParseSide(), clientIP(), isUnsafeMethod(), newAuth(), newLoginLimiter(), requiresAuth() (+30 more)

### Community 1 - "Entrypoint & Stdlib Deps"
Cohesion: 0.08
Nodes (21): ValidationError, ValidationError, ParseKind(), TestParseKind(), parseMinutesField(), Overlaps(), ptr(), TestOverlaps() (+13 more)

### Community 2 - "HTMX Library"
Cohesion: 0.08
Nodes (101): a(), Ae(), an(), at(), B(), be(), bn(), bt() (+93 more)

### Community 3 - "testing.T"
Cohesion: 0.12
Nodes (34): TestEditValidate(), TestKindCoversBoth(), TestCompletedEditValidate(), TestOpenSegmentCountsToNowAndClampsNegative(), TestOpposite(), TestParseSide(), TestSafeNext(), TestValidationErrorType() (+26 more)

### Community 4 - "time.Time"
Cohesion: 0.15
Nodes (10): Real, Edit, ValidateNotes(), ValidateTimes(), nullableString(), encodeTime(), isUniqueViolation(), normalizeTime() (+2 more)

### Community 5 - "Dual-Side Breastfeeding Timer & Logger"
Cohesion: 0.21
Nodes (11): Streamlined Diaper Tracker (Wet/Dirty/Both), Dual-Side Breastfeeding Timer & Logger, Last Nursed Indicator, Resilient Timer State (localStorage + backend sync), Sleep & Nap Tracker with Wake Window Counter, Recommended Minimal SQLite Schema, 'Time Elapsed Since' Dashboard Status Cards, Feed Segments Model (derived per-side durations) (+3 more)

### Community 6 - "Web Templates & Views"
Cohesion: 0.08
Nodes (42): App Icon (Pip Face), Maskable App Icon, Dashboard View, App Layout Shell, Action Dialog Sheet System, Inline Theme Bootstrap Script, Server-Side View Dispatch, Auth Mode Variants (chooser / signup / signin) (+34 more)

### Community 7 - "TursoSyncer"
Cohesion: 0.22
Nodes (4): SyncStats, NewTursoSyncer(), StatsSyncer, TursoSyncer

### Community 8 - "Fake"
Cohesion: 0.20
Nodes (3): Fake, NewFake(), fakeSyncer

### Community 9 - "Client Timer Script"
Cohesion: 0.14
Nodes (26): activeTimerElapsed(), announce(), applyTheme(), armToast(), clearCache(), closeDialog(), closeOpenDialogs(), formatAgo() (+18 more)

### Community 10 - "Deployment"
Cohesion: 0.15
Nodes (14): main(), 1. Create the runtime config, 2. Add Nurtur to your Compose stack, 3. Point Caddy at it, 4. Start and onboard, 5. Updates, 6. Backups and restore, CI/CD boundary (+6 more)

### Community 11 - "Feed"
Cohesion: 0.11
Nodes (11): Segment, Side, Opposite(), TestCurrentSegmentAndMostRecentSide(), scanFeed(), scanSegment(), decodeNullTime(), decodeTime() (+3 more)

### Community 12 - "App Configuration"
Cohesion: 0.14
Nodes (22): Config, env(), Load(), parseBool(), parseClock(), parseLevel(), clearEnv(), TestLoadDefaults() (+14 more)

### Community 13 - "Diaper Domain Model"
Cohesion: 0.12
Nodes (7): Consistency, Diaper, Edit, Kind, ParseConsistency(), TestSummary(), scanDiaper()

### Community 14 - "Session"
Cohesion: 0.15
Nodes (9): csrfFrom(), Session, claimLegacyDataTx(), setSettingsTx(), scanSleep(), AccountID(), placeholdersFor(), Entry (+1 more)

### Community 16 - "newTestEnv"
Cohesion: 0.15
Nodes (21): TestDeleteActiveSleepAndUndo(), TestDiaperBothSatisfiesWetAndDirty(), TestDiaperDeleteAndUndo(), TestDiaperDetailsSheetAndUpdate(), TestDiaperOneTapDedupesRapidTaps(), TestNonHTMXSheetsRenderOpenDialog(), TestNonHTMXSleepStartRedirects(), TestNursingSheet() (+13 more)

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
Cohesion: 0.33
Nodes (6): Night Mode & Single-Hand UX, HTMX Out-of-Band Update Convention, Night-First Day/Night Token Palette, Pip the Sprout Mascot, Progressive Enhancement Contract, Progressive Enhancement (README)

### Community 22 - "Local Dev Script"
Cohesion: 0.48
Nodes (6): cleanup(), fingerprint(), NURTUR_LOCAL_ONLY, dev.sh script, start_server(), stop_server()

### Community 23 - "Feed"
Cohesion: 0.14
Nodes (9): BuildSegments(), CompletedEdit, Feed, TestBuildSegmentsOrder(), TestBuildSegmentsSingleSide(), TestTotalsDeriveFromSegments(), timePtr(), segmentIndex() (+1 more)

### Community 24 - "Playwright Browser Smoke Suite (390x844)"
Cohesion: 0.40
Nodes (6): Playwright Browser Smoke Suite (390x844), Chronological Feed with Quick Undo, Injected Clock Interface (elapsed time in Go), Soft Delete (deleted_at) for Real Undo, Layered Test Strategy (unit/integration/sync/concurrency/browser), History and In-Place Undo

### Community 25 - "Font Licensing (OFL)"
Cohesion: 0.83
Nodes (4): Self-Hosted OFL Fonts (Fredoka + Nunito Sans), Fredoka OFL License, Nunito Sans OFL License, SIL Open Font License 1.1

### Community 34 - "server_test.go"
Cohesion: 0.13
Nodes (23): TestExportEndpoints(), TestSettingsNightWindowPersists(), TestTimelinePageGroupsByLocalDay(), TestActiveAdjustAndFutureRejection(), TestCompletedEditRejectsFutureTimes(), TestCompletedFeedSegmentsUseSubmittedTotals(), TestDashboardShellAndSecurityHeaders(), TestDayBoundsAcrossDST() (+15 more)

### Community 35 - "Build"
Cohesion: 0.15
Nodes (11): Split, NightIntervals(), overlap(), SplitRange(), Build(), Snapshot, TestBuildEmpty(), TestBuildPartialAndWakeWindow() (+3 more)

### Community 37 - "account_test.go"
Cohesion: 0.40
Nodes (5): HashPassword(), TestHashPasswordRoundTrip(), TestHashPasswordUsesFreshSalt(), TestNormalizeUsername(), VerifyPassword()

### Community 38 - "sync_test.go"
Cohesion: 0.40
Nodes (10): discardLogger(), startWorker(), TestSyncFailureDoesNotFailMutation(), TestSyncPullOnDemand(), TestSyncPullOnStartup(), TestSyncPullsOnInterval(), TestSyncPushIsDebouncedAndCoalesced(), TestSyncRetriesWithBackoffThenSucceeds() (+2 more)

### Community 39 - "Store"
Cohesion: 0.18
Nodes (5): Clock, Store, Open(), Config, Options

### Community 40 - "time.Location"
Cohesion: 0.06
Nodes (59): DayBounds(), applyDiaperValidation(), DayBounds(), diaperIcon(), latestDiapers(), applySleepEditError(), applySleepEditValidation(), applySleepSheetError() (+51 more)

### Community 42 - "WakeWindow"
Cohesion: 0.67
Nodes (3): TestWakeWindow(), TestWakeWindowAcrossMidnight(), WakeWindow()

### Community 43 - "newTestEnvConfig"
Cohesion: 0.13
Nodes (14): TestAccountsHaveSeparateData(), TestOnboardingRejectsWeakCredentials(), TestOnboardingSignupThenLogin(), doWith(), TestPasscodeGateAndCSRF(), newTestEnvConfig(), TestAccountDataIsolation(), TestCreateAccountWithSettingsIsAtomic() (+6 more)

### Community 44 - "SyncStatus"
Cohesion: 0.50
Nodes (3): SyncStatus, healthResponse, syncHealth

### Community 45 - "NewSyncWorker"
Cohesion: 0.27
Nodes (6): run(), DefaultSyncConfig(), NewSyncWorker(), TestNotifyNeverBlocks(), SyncConfig, Syncer

## Knowledge Gaps
- **27 isolated node(s):** `nurtur`, `NURTUR_LOCAL_ONLY`, `Debian-slim (glibc) Docker Runtime for tursogo`, `Non-Goals for v1 (single user, single baby, no notifications)`, `Pip the Sprout Mascot` (+22 more)
  These have ≤1 connection - possible missing edges or undocumented components. (Counts symbols only; 103 node(s) total have ≤1 connection when file, concept and rationale nodes are included.)
- **12 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `Server` connect `HTTP Handlers & Middleware` to `Entrypoint & Stdlib Deps`, `Store`, `time.Location`, `Feed`, `SyncWorker`?**
  _High betweenness centrality (0.089) - this node is a cross-community bridge._
- **What connects `nurtur`, `NURTUR_LOCAL_ONLY`, `Debian-slim (glibc) Docker Runtime for tursogo` to the rest of the system?**
  _27 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `HTTP Handlers & Middleware` be split into smaller, more focused modules?**
  _Cohesion score 0.053691275167785234 - nodes in this community are weakly interconnected._
- **Why does `main()` connect `Deployment` to `Entrypoint & Stdlib Deps`, `NewSyncWorker`?**
  _High betweenness centrality (0.081) - this node is a cross-community bridge._
- **Should `Entrypoint & Stdlib Deps` be split into smaller, more focused modules?**
  _Cohesion score 0.08185840707964602 - nodes in this community are weakly interconnected._
- **Why does `Deployment` connect `Deployment` to `Phase 3 — Timeline, Undo + Ship`?**
  _High betweenness centrality (0.079) - this node is a cross-community bridge._
- **Should `HTMX Library` be split into smaller, more focused modules?**
  _Cohesion score 0.07897684839432412 - nodes in this community are weakly interconnected._