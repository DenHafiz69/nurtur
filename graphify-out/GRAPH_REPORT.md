# Graph Report - nurtur  (2026-10-21)

## Corpus Check
- 71 files · ~69,919 words
- Verdict: corpus is large enough that graph structure adds value.
- Unclassified: 11 file(s) not represented in the graph (top: (none) 5, .woff2 2, .example 1)

## Summary
- 965 nodes · 3390 edges · 32 communities (22 shown, 10 thin omitted)
- Extraction: 87% EXTRACTED · 13% INFERRED · 0% AMBIGUOUS · INFERRED: 454 edges (avg confidence: 0.85)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `1e502f00`
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
- scanDiaper
- time.Duration
- Client Timer Script
- context.Context
- Side
- App Configuration
- Diaper Domain Model
- Session
- SyncWorker
- App Wiring & Renderer
- Feed
- Playwright E2E Setup
- Debian-slim (glibc) Docker Runtime for tursogo
- Local Dev Script
- .Totals
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
- `Manual Release Gate (soak, sync outage, restore, 3 a.m. test)` --conceptually_related_to--> `Risks and Mitigations Register`  [INFERRED]
  e2e/README.md → PLANS.md
- `run()` --calls--> `Open()`  [EXTRACTED]
  cmd/nurtur/main.go → internal/store/store.go

## Import Cycles
- None detected.

## Hyperedges (group relationships)
- **HTMX Out-of-Band Fragment Update Pattern** — web_templates_partials_status_statusstripoob, web_templates_partials_last_nursed_lastnursedoob, web_templates_partials_sleep_sleeppilloob, web_templates_partials_timeline_timelinetodayoob, web_templates_partials_toast_toastoob [EXTRACTED 1.00]
- **Nurtur Three-Phase Delivery Plan** — plans_nurtur_build_plan, plans_phase1_nursing_core, plans_phase2_diapers_sleep_dashboard, plans_phase3_timeline_undo_ship [EXTRACTED 1.00]
- **Quick-Logging Dock (Timer, Diapers, Sleep, Last Nursed)** — web_templates_layout_layout, web_templates_partials_timer_timer, web_templates_partials_diapers_diaperrow, web_templates_partials_sleep_sleeppill, web_templates_partials_last_nursed_lastnursed [EXTRACTED 1.00]
- **One-Thumb, 3 a.m. Design Language** — features_zero_friction_rule, features_night_mode_one_handed_ux, plans_thumb_first_principles, plans_night_first_palette, plans_pip_mascot [INFERRED 0.85]
- **Single Action Dialog Serving Multiple Bottom Sheets** — web_templates_layout_sheetdialog, web_templates_partials_status_statuscard, web_templates_partials_nursing_sheet_nursingsheet, web_templates_partials_diapers_diapersheet, web_templates_partials_sleep_sleepsheet [INFERRED 0.95]

## Communities (32 total, 10 thin omitted)

### Community 0 - "HTTP Handlers & Middleware"
Cohesion: 0.06
Nodes (28): ParseSide(), clientIP(), requiresAuth(), safeNext(), isoPtr(), verbose(), parseLocalDateTime(), applyValidation() (+20 more)

### Community 1 - "Entrypoint & Stdlib Deps"
Cohesion: 0.07
Nodes (31): ValidationError, ValidationError, HashPassword(), NormalizeUsername(), TestHashPasswordRoundTrip(), TestHashPasswordUsesFreshSalt(), TestNormalizeUsername(), ValidatePassword() (+23 more)

### Community 2 - "HTMX Library"
Cohesion: 0.08
Nodes (101): a(), Ae(), an(), at(), B(), be(), bn(), bt() (+93 more)

### Community 3 - "testing.T"
Cohesion: 0.06
Nodes (84): TestEditValidate(), TestKindCoversBoth(), TestCompletedEditValidate(), TestOpposite(), TestParseSide(), TestOnboardingRejectsWeakCredentials(), TestOnboardingSignupThenLogin(), TestSafeNext() (+76 more)

### Community 4 - "time.Time"
Cohesion: 0.19
Nodes (9): Real, Edit, ValidateNotes(), ValidateTimes(), encodeTime(), isUniqueViolation(), normalizeTime(), loginAttempt (+1 more)

### Community 5 - "Product Spec & Ops Docs"
Cohesion: 0.07
Nodes (39): Manual Release Gate (soak, sync outage, restore, 3 a.m. test), Playwright Browser Smoke Suite (390x844), Nurtur Feature Brief, Chronological Feed with Quick Undo, Streamlined Diaper Tracker (Wet/Dirty/Both), Dual-Side Breastfeeding Timer & Logger, Last Nursed Indicator, Night Mode & Single-Hand UX (+31 more)

### Community 6 - "Web Templates & Views"
Cohesion: 0.08
Nodes (42): App Icon (Pip Face), Maskable App Icon, Dashboard View, App Layout Shell, Action Dialog Sheet System, Inline Theme Bootstrap Script, Server-Side View Dispatch, Auth Mode Variants (chooser / signup / signin) (+34 more)

### Community 7 - "scanDiaper"
Cohesion: 0.16
Nodes (8): scanDiaper(), scanFeed(), scanSleep(), decodeNullTime(), decodeTime(), placeholdersFor(), Entry, scanner

### Community 8 - "time.Duration"
Cohesion: 0.06
Nodes (57): DayBounds(), applyDiaperValidation(), DayBounds(), diaperIcon(), latestDiapers(), applySleepSheetError(), applySleepSheetValidation(), previousLocalDayStart() (+49 more)

### Community 9 - "Client Timer Script"
Cohesion: 0.14
Nodes (26): activeTimerElapsed(), announce(), applyTheme(), armToast(), clearCache(), closeDialog(), closeOpenDialogs(), formatAgo() (+18 more)

### Community 10 - "context.Context"
Cohesion: 0.12
Nodes (4): csrfFrom(), setSettingsTx(), fakeReader, Syncer

### Community 11 - "Side"
Cohesion: 0.17
Nodes (6): Segment, Side, Opposite(), TestCurrentSegmentAndMostRecentSide(), scanSegment(), queryer

### Community 12 - "App Configuration"
Cohesion: 0.14
Nodes (22): Config, env(), Load(), parseBool(), parseClock(), parseLevel(), clearEnv(), TestLoadDefaults() (+14 more)

### Community 13 - "Diaper Domain Model"
Cohesion: 0.11
Nodes (9): Color, Consistency, Diaper, Edit, Kind, ParseColor(), ParseConsistency(), TestSummary() (+1 more)

### Community 14 - "Session"
Cohesion: 0.14
Nodes (10): Session, TestWakeWindow(), TestWakeWindowAcrossMidnight(), WakeWindow(), Build(), Snapshot, TestBuildEmpty(), TestBuildPartialAndWakeWindow() (+2 more)

### Community 15 - "SyncWorker"
Cohesion: 0.05
Nodes (33): main(), run(), Clock, Fake, NewFake(), Store, Open(), splitSQL() (+25 more)

### Community 16 - "App Wiring & Renderer"
Cohesion: 0.14
Nodes (12): newAuth(), newLoginLimiter(), migrationVersions(), sqlNullable(), sqlText(), sqlTimePtr(), writeSchema(), New() (+4 more)

### Community 17 - "Feed"
Cohesion: 0.29
Nodes (3): Feed, segmentIndex(), timePtr()

### Community 18 - "Playwright E2E Setup"
Cohesion: 0.14
Nodes (11): devDependencies, @playwright/test, name, private, scripts, test, test:headed, version (+3 more)

### Community 22 - "Local Dev Script"
Cohesion: 0.48
Nodes (6): cleanup(), fingerprint(), NURTUR_LOCAL_ONLY, dev.sh script, start_server(), stop_server()

### Community 23 - ".Totals"
Cohesion: 0.18
Nodes (7): BuildSegments(), CompletedEdit, TestBuildSegmentsOrder(), TestBuildSegmentsSingleSide(), TestOpenSegmentCountsToNowAndClampsNegative(), TestTotalsDeriveFromSegments(), timePtr()

### Community 25 - "Font Licensing (OFL)"
Cohesion: 0.83
Nodes (4): Self-Hosted OFL Fonts (Fredoka + Nunito Sans), Fredoka OFL License, Nunito Sans OFL License, SIL Open Font License 1.1

## Knowledge Gaps
- **27 isolated node(s):** `nurtur`, `NURTUR_LOCAL_ONLY`, `Apple Touch Icon (PWA Home-Screen Icon)`, `Nurtur Brand Mascot: Smiling Cream Fruit with Leaves`, `Nurtur App Icon (192x192 PWA Icon)` (+22 more)
  These have ≤1 connection - possible missing edges or undocumented components. (Counts symbols only; 92 node(s) total have ≤1 connection when file, concept and rationale nodes are included.)
- **10 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `Server` connect `HTTP Handlers & Middleware` to `Entrypoint & Stdlib Deps`, `time.Duration`, `Side`, `SyncWorker`, `App Wiring & Renderer`?**
  _High betweenness centrality (0.087) - this node is a cross-community bridge._
- **What connects `nurtur`, `NURTUR_LOCAL_ONLY`, `Apple Touch Icon (PWA Home-Screen Icon)` to the rest of the system?**
  _27 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `HTTP Handlers & Middleware` be split into smaller, more focused modules?**
  _Cohesion score 0.0634604754007739 - nodes in this community are weakly interconnected._
- **Why does `SyncWorker` connect `SyncWorker` to `HTTP Handlers & Middleware`, `Entrypoint & Stdlib Deps`, `context.Context`, `time.Time`?**
  _High betweenness centrality (0.021) - this node is a cross-community bridge._
- **Should `Entrypoint & Stdlib Deps` be split into smaller, more focused modules?**
  _Cohesion score 0.07172799254774104 - nodes in this community are weakly interconnected._
- **Why does `Store` connect `SyncWorker` to `HTTP Handlers & Middleware`, `Entrypoint & Stdlib Deps`, `testing.T`, `time.Time`, `context.Context`?**
  _High betweenness centrality (0.018) - this node is a cross-community bridge._
- **Should `HTMX Library` be split into smaller, more focused modules?**
  _Cohesion score 0.07897684839432412 - nodes in this community are weakly interconnected._