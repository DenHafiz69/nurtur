# Graph Report - nurtur  (2026-10-21)

## Corpus Check
- 71 files · ~69,894 words
- Verdict: corpus is large enough that graph structure adds value.
- Unclassified: 11 file(s) not represented in the graph (top: (none) 5, .woff2 2, .example 1)

## Summary
- 967 nodes · 3394 edges · 33 communities (23 shown, 10 thin omitted)
- Extraction: 87% EXTRACTED · 13% INFERRED · 0% AMBIGUOUS · INFERRED: 455 edges (avg confidence: 0.85)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `3c09e643`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify update .` after code changes (no API cost).

## Community Hubs (Navigation)
- HTTP Handlers & Middleware
- Entrypoint & Stdlib Deps
- HTMX Library
- testing.T
- time.Time
- Product Spec & Ops Docs
- Web Templates & Views
- Feed
- time.Duration
- Client Timer Script
- context.Context
- App Configuration
- Diaper Domain Model
- Session
- Sync Worker
- App Wiring & Renderer
- Sync Worker Tests
- Playwright E2E Setup
- Turso Sync & Health
- Database Bootstrap
- Local Dev Script
- time.Time
- Main & Sync Config
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
- `App Layout Shell` --references--> `App Icon (Pip Face)`  [INFERRED]
  web/templates/layout.html → web/static/img/icon.svg
- `run()` --calls--> `Load()`  [EXTRACTED]
  cmd/nurtur/main.go → internal/config/config.go
- `run()` --calls--> `New()`  [EXTRACTED]
  cmd/nurtur/main.go → internal/server/server.go
- `run()` --calls--> `Open()`  [EXTRACTED]
  cmd/nurtur/main.go → internal/store/store.go
- `run()` --calls--> `NewTursoSyncer()`  [EXTRACTED]
  cmd/nurtur/main.go → internal/store/sync.go

## Import Cycles
- None detected.

## Hyperedges (group relationships)
- **HTMX Out-of-Band Fragment Update Pattern** — web_templates_partials_status_statusstripoob, web_templates_partials_last_nursed_lastnursedoob, web_templates_partials_sleep_sleeppilloob, web_templates_partials_timeline_timelinetodayoob, web_templates_partials_toast_toastoob [EXTRACTED 1.00]
- **Nurtur Three-Phase Delivery Plan** — plans_nurtur_build_plan, plans_phase1_nursing_core, plans_phase2_diapers_sleep_dashboard, plans_phase3_timeline_undo_ship [EXTRACTED 1.00]
- **Quick-Logging Dock (Timer, Diapers, Sleep, Last Nursed)** — web_templates_layout_layout, web_templates_partials_timer_timer, web_templates_partials_diapers_diaperrow, web_templates_partials_sleep_sleeppill, web_templates_partials_last_nursed_lastnursed [EXTRACTED 1.00]
- **Local-First Turso Sync Architecture** — plans_turso_local_first_topology, readme_local_first_sync, readme_single_instance_sync_constraint, readme_healthz_endpoint, compose_nurtur_app_service [INFERRED 0.85]
- **One-Thumb, 3 a.m. Design Language** — features_zero_friction_rule, features_night_mode_one_handed_ux, plans_thumb_first_principles, plans_night_first_palette, plans_pip_mascot [INFERRED 0.85]
- **Single Action Dialog Serving Multiple Bottom Sheets** — web_templates_layout_sheetdialog, web_templates_partials_status_statuscard, web_templates_partials_nursing_sheet_nursingsheet, web_templates_partials_diapers_diapersheet, web_templates_partials_sleep_sleepsheet [INFERRED 0.95]

## Communities (33 total, 10 thin omitted)

### Community 0 - "HTTP Handlers & Middleware"
Cohesion: 0.06
Nodes (27): NormalizeUsername(), ParseSide(), clientIP(), requiresAuth(), safeNext(), isoPtr(), sqlNullable(), sqlText() (+19 more)

### Community 1 - "Entrypoint & Stdlib Deps"
Cohesion: 0.07
Nodes (29): ValidationError, ValidationError, HashPassword(), TestHashPasswordRoundTrip(), TestHashPasswordUsesFreshSalt(), ValidatePassword(), VerifyPassword(), ParseKind() (+21 more)

### Community 2 - "HTMX Library"
Cohesion: 0.08
Nodes (101): a(), Ae(), an(), at(), B(), be(), bn(), bt() (+93 more)

### Community 3 - "testing.T"
Cohesion: 0.05
Nodes (88): TestNormalizeUsername(), TestEditValidate(), TestKindCoversBoth(), TestCompletedEditValidate(), TestOpenSegmentCountsToNowAndClampsNegative(), TestOpposite(), TestParseSide(), TestOnboardingRejectsWeakCredentials() (+80 more)

### Community 4 - "time.Time"
Cohesion: 0.19
Nodes (9): Edit, ValidateNotes(), ValidateTimes(), nullableString(), timePtr(), setSettingsTx(), encodeTime(), isUniqueViolation() (+1 more)

### Community 5 - "Product Spec & Ops Docs"
Cohesion: 0.06
Nodes (42): compose app service (nurtur:local, port 8005), nurtur-data Named Volume, Manual Release Gate (soak, sync outage, restore, 3 a.m. test), Playwright Browser Smoke Suite (390x844), Nurtur Feature Brief, Chronological Feed with Quick Undo, Streamlined Diaper Tracker (Wet/Dirty/Both), Dual-Side Breastfeeding Timer & Logger (+34 more)

### Community 6 - "Web Templates & Views"
Cohesion: 0.08
Nodes (42): App Icon (Pip Face), Maskable App Icon, Dashboard View, App Layout Shell, Action Dialog Sheet System, Inline Theme Bootstrap Script, Server-Side View Dispatch, Auth Mode Variants (chooser / signup / signin) (+34 more)

### Community 7 - "Feed"
Cohesion: 0.10
Nodes (14): Feed, Segment, Side, Opposite(), TestCurrentSegmentAndMostRecentSide(), scanFeed(), scanSegment(), segmentIndex() (+6 more)

### Community 8 - "time.Duration"
Cohesion: 0.05
Nodes (63): DayBounds(), applyDiaperValidation(), DayBounds(), diaperIcon(), latestDiapers(), TestDayBoundsAcrossDST(), applySleepEditError(), applySleepEditValidation() (+55 more)

### Community 9 - "Client Timer Script"
Cohesion: 0.14
Nodes (26): activeTimerElapsed(), announce(), applyTheme(), armToast(), clearCache(), closeDialog(), closeOpenDialogs(), formatAgo() (+18 more)

### Community 10 - "context.Context"
Cohesion: 0.15
Nodes (3): csrfFrom(), queryer, Syncer

### Community 12 - "App Configuration"
Cohesion: 0.14
Nodes (22): Config, env(), Load(), parseBool(), parseClock(), parseLevel(), clearEnv(), TestLoadDefaults() (+14 more)

### Community 13 - "Diaper Domain Model"
Cohesion: 0.11
Nodes (9): Color, Consistency, Diaper, Edit, Kind, ParseColor(), ParseConsistency(), TestSummary() (+1 more)

### Community 14 - "Session"
Cohesion: 0.13
Nodes (9): Session, Build(), Snapshot, TestBuildEmpty(), TestBuildPartialAndWakeWindow(), TestBuildSleepingUsesActiveDuration(), scanSleep(), fakeReader (+1 more)

### Community 16 - "App Wiring & Renderer"
Cohesion: 0.14
Nodes (10): Clock, newAuth(), newLoginLimiter(), migrationVersions(), writeSchema(), New(), newStaticHandler(), Renderer (+2 more)

### Community 17 - "Sync Worker Tests"
Cohesion: 0.30
Nodes (10): startWorker(), TestSyncFailureDoesNotFailMutation(), TestSyncPullOnDemand(), TestSyncPullOnStartup(), TestSyncPullsOnInterval(), TestSyncPushIsDebouncedAndCoalesced(), TestSyncRetriesWithBackoffThenSucceeds(), TestSyncShutdownFlushesAndCheckpoints() (+2 more)

### Community 18 - "Playwright E2E Setup"
Cohesion: 0.14
Nodes (11): devDependencies, @playwright/test, name, private, scripts, test, test:headed, version (+3 more)

### Community 19 - "Turso Sync & Health"
Cohesion: 0.25
Nodes (5): SyncStats, SyncStatus, healthResponse, syncHealth, StatsSyncer

### Community 20 - "Database Bootstrap"
Cohesion: 0.19
Nodes (5): Store, Open(), NewTursoSyncer(), Options, TursoSyncer

### Community 22 - "Local Dev Script"
Cohesion: 0.48
Nodes (6): cleanup(), fingerprint(), NURTUR_LOCAL_ONLY, dev.sh script, start_server(), stop_server()

### Community 23 - "time.Time"
Cohesion: 0.11
Nodes (11): Real, Fake, NewFake(), BuildSegments(), CompletedEdit, TestBuildSegmentsOrder(), TestBuildSegmentsSingleSide(), TestTotalsDeriveFromSegments() (+3 more)

### Community 24 - "Main & Sync Config"
Cohesion: 0.33
Nodes (7): main(), run(), discardLogger(), DefaultSyncConfig(), NewSyncWorker(), TestNotifyNeverBlocks(), SyncConfig

### Community 25 - "Font Licensing (OFL)"
Cohesion: 0.83
Nodes (4): Self-Hosted OFL Fonts (Fredoka + Nunito Sans), Fredoka OFL License, Nunito Sans OFL License, SIL Open Font License 1.1

## Knowledge Gaps
- **28 isolated node(s):** `nurtur`, `NURTUR_LOCAL_ONLY`, `Apple Touch Icon (PWA Home-Screen Icon)`, `Nurtur Brand Mascot: Smiling Cream Fruit with Leaves`, `Nurtur App Icon (192x192 PWA Icon)` (+23 more)
  These have ≤1 connection - possible missing edges or undocumented components. (Counts symbols only; 93 node(s) total have ≤1 connection when file, concept and rationale nodes are included.)
- **10 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `Server` connect `HTTP Handlers & Middleware` to `Entrypoint & Stdlib Deps`, `Feed`, `time.Duration`, `Sync Worker`, `App Wiring & Renderer`, `Database Bootstrap`, `Main & Sync Config`?**
  _High betweenness centrality (0.087) - this node is a cross-community bridge._
- **What connects `nurtur`, `NURTUR_LOCAL_ONLY`, `Apple Touch Icon (PWA Home-Screen Icon)` to the rest of the system?**
  _28 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `HTTP Handlers & Middleware` be split into smaller, more focused modules?**
  _Cohesion score 0.06285403050108933 - nodes in this community are weakly interconnected._
- **Why does `SyncWorker` connect `Sync Worker` to `HTTP Handlers & Middleware`, `Entrypoint & Stdlib Deps`, `context.Context`, `App Wiring & Renderer`, `Sync Worker Tests`, `Turso Sync & Health`, `time.Time`, `Main & Sync Config`?**
  _High betweenness centrality (0.021) - this node is a cross-community bridge._
- **Should `Entrypoint & Stdlib Deps` be split into smaller, more focused modules?**
  _Cohesion score 0.07181409295352324 - nodes in this community are weakly interconnected._
- **Why does `Store` connect `Database Bootstrap` to `HTTP Handlers & Middleware`, `Entrypoint & Stdlib Deps`, `testing.T`, `time.Time`, `context.Context`, `App Wiring & Renderer`, `Main & Sync Config`?**
  _High betweenness centrality (0.018) - this node is a cross-community bridge._
- **Should `HTMX Library` be split into smaller, more focused modules?**
  _Cohesion score 0.07897684839432412 - nodes in this community are weakly interconnected._