package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"nurtur/internal/clock"
	"nurtur/internal/feed"
	"nurtur/internal/store"
)

var testNow = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

type testEnv struct {
	handler http.Handler
	store   *store.Store
	clock   *clock.Fake
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	return newTestEnvConfig(t, nil)
}

func newTestEnvConfig(t *testing.T, mutate func(*Config)) *testEnv {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	st, err := store.Open(context.Background(), store.Options{
		Path:      filepath.Join(t.TempDir(), "nurtur.db"),
		LocalOnly: true,
		Logger:    logger,
	})
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(context.Background(), os.DirFS("../../migrations"), testNow); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	fakeClock := clock.NewFake(testNow)
	cfg := Config{
		Store:      st,
		Clock:      fakeClock,
		Location:   time.UTC,
		BabyName:   "Pip",
		Version:    "test",
		Assets:     os.DirFS("../.."),
		Migrations: os.DirFS("../../migrations"),
		Logger:     logger,
		LocalOnly:  true,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	srv, err := New(cfg)
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	return &testEnv{handler: srv.Handler(), store: st, clock: fakeClock}
}

func (e *testEnv) do(t *testing.T, method, path string, form url.Values, htmx bool) *httptest.ResponseRecorder {
	t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req := httptest.NewRequest(method, path, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if htmx {
		req.Header.Set("HX-Request", "true")
	}
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	return rec
}

func TestDashboardShellAndSecurityHeaders(t *testing.T) {
	env := newTestEnv(t)
	rec := env.do(t, http.MethodGet, "/", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`id="timer-card"`,
		`data-theme="day"`,
		"Tap a side to start.",
		"Nothing logged yet today.",
		`id="last-nursed"`,
		`id="status-strip"`,
		`id="timeline-today"`,
		`id="feed-edit-dialog"`,
		`hx-post="/feeds"`,
		`/static/js/timer.js`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("dashboard missing %q", want)
		}
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q", got)
	}
	if got := rec.Header().Get("Referrer-Policy"); got != "same-origin" {
		t.Errorf("Referrer-Policy = %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q", got)
	}

	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'self'") || !strings.Contains(csp, "'nonce-") {
		t.Fatalf("CSP = %q", csp)
	}
	nonceMatch := regexp.MustCompile(`'nonce-([^']+)'`).FindStringSubmatch(csp)
	if len(nonceMatch) != 2 {
		t.Fatalf("could not parse nonce from %q", csp)
	}
	if !strings.Contains(body, `nonce="`+nonceMatch[1]+`"`) {
		t.Fatal("inline theme script nonce does not match CSP header")
	}
}

func TestStartSwitchStopFlow(t *testing.T) {
	env := newTestEnv(t)

	rec := env.do(t, http.MethodPost, "/feeds", url.Values{"side": {"left"}}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("start status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`data-feed-id="1"`, `data-side="left"`, "Nursing <strong>Left</strong>", "Nursing left."} {
		if !strings.Contains(body, want) {
			t.Errorf("start response missing %q\n%s", want, body)
		}
	}
	if !strings.Contains(body, `hx-swap-oob="outerHTML"`) {
		t.Error("start response has no OOB fragments")
	}
	for _, id := range []string{`id="last-nursed"`, `id="status-strip"`, `id="timeline-today"`, `id="toast-region"`} {
		if !strings.Contains(body, id) {
			t.Errorf("start response missing OOB target %s", id)
		}
	}

	env.clock.Advance(8 * time.Minute)
	rec = env.do(t, http.MethodPost, "/feeds/1/switch", url.Values{"side": {"right"}}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("switch status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body = rec.Body.String()
	if !strings.Contains(body, "Nursing <strong>Right</strong>") || !strings.Contains(body, `data-side="right"`) {
		t.Fatalf("switch response did not switch side:\n%s", body)
	}
	if !strings.Contains(body, "Switched to right.") {
		t.Fatalf("switch toast missing:\n%s", body)
	}

	env.clock.Advance(12 * time.Minute)
	rec = env.do(t, http.MethodPost, "/feeds/1/stop", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("stop status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body = rec.Body.String()
	if strings.Contains(body, "data-feed-id=") {
		t.Fatalf("timer still active after stop:\n%s", body)
	}
	for _, want := range []string{"Saved. Last side: Right.", "Last: <strong>Right</strong>", "just now", "L 8m · R 12m", "20m"} {
		if !strings.Contains(body, want) {
			t.Errorf("stop response missing %q\n%s", want, body)
		}
	}

	rec = env.do(t, http.MethodGet, "/", nil, false)
	body = rec.Body.String()
	for _, want := range []string{"12:00 PM", "20m", "L 8m · R 12m", "1 feed", `class="feed-summary"`} {
		if !strings.Contains(body, want) {
			t.Errorf("dashboard after stop missing %q", want)
		}
	}
}

func TestSubMinuteFeedReadsClearly(t *testing.T) {
	env := newTestEnv(t)
	env.do(t, http.MethodPost, "/feeds", url.Values{"side": {"left"}}, true)
	env.clock.Advance(5 * time.Second)
	rec := env.do(t, http.MethodPost, "/feeds/1/stop", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("stop status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Under a minute") {
		t.Errorf("sub-minute feed detail missing:\n%s", body)
	}
	if strings.Contains(body, "0m · 0m") {
		t.Errorf("sub-minute feed detail still shows duplicate zeros:\n%s", body)
	}
}

func TestNonHTMXStartRedirects(t *testing.T) {
	env := newTestEnv(t)
	rec := env.do(t, http.MethodPost, "/feeds", url.Values{"side": {"left"}}, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/" {
		t.Fatalf("Location = %q", got)
	}
	if _, err := env.store.ActiveFeed(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestDoubleStartIsFriendly(t *testing.T) {
	env := newTestEnv(t)
	env.do(t, http.MethodPost, "/feeds", url.Values{"side": {"left"}}, true)
	rec := env.do(t, http.MethodPost, "/feeds", url.Values{"side": {"right"}}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "already running") {
		t.Fatalf("missing already-running toast:\n%s", body)
	}
	if !strings.Contains(body, `data-side="left"`) {
		t.Fatal("double start changed the active side")
	}
	feeds, err := env.store.RecentFeeds(context.Background(), testNow.Add(-time.Hour), testNow.Add(time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(feeds) != 1 {
		t.Fatalf("feeds = %d, want 1", len(feeds))
	}
}

func TestSameSideSwitchIsNoOp(t *testing.T) {
	env := newTestEnv(t)
	env.do(t, http.MethodPost, "/feeds", url.Values{"side": {"left"}}, true)
	rec := env.do(t, http.MethodPost, "/feeds/1/switch", url.Values{"side": {"left"}}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Already nursing left.") {
		t.Fatalf("missing no-op toast:\n%s", body)
	}
	active, err := env.store.ActiveFeed(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(active.Segments) != 1 {
		t.Fatalf("segments = %d, want 1", len(active.Segments))
	}
}

func TestEditCompletedFeed(t *testing.T) {
	env := newTestEnv(t)
	env.do(t, http.MethodPost, "/feeds", url.Values{"side": {"left"}}, true)
	env.clock.Advance(20 * time.Minute)
	env.do(t, http.MethodPost, "/feeds/1/stop", nil, true)

	rec := env.do(t, http.MethodPatch, "/feeds/1", url.Values{
		"started_at":    {"2026-10-20T11:50:00"},
		"ended_at":      {"2026-10-20T12:15:00"},
		"left_minutes":  {"10"},
		"right_minutes": {"5"},
	}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("HX-Trigger-After-Swap"); got != "feed-saved" {
		t.Fatalf("HX-Trigger-After-Swap = %q", got)
	}
	if got := rec.Header().Get("HX-Retarget"); got != "#timer-card" {
		t.Fatalf("HX-Retarget = %q", got)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "15m") || !strings.Contains(body, "L 10m · R 5m") {
		t.Fatalf("edited totals missing:\n%s", body)
	}
	if !strings.Contains(body, "Feed updated.") {
		t.Fatalf("edited toast missing:\n%s", body)
	}

	rec = env.do(t, http.MethodGet, "/", nil, false)
	body = rec.Body.String()
	if !strings.Contains(body, "L 10m · R 5m") || !strings.Contains(body, "11:50 AM") {
		t.Fatalf("dashboard did not reflect edit:\n%s", body)
	}
}

func TestEditValidationStaysInDialog(t *testing.T) {
	env := newTestEnv(t)
	env.do(t, http.MethodPost, "/feeds", url.Values{"side": {"left"}}, true)
	env.clock.Advance(20 * time.Minute)
	env.do(t, http.MethodPost, "/feeds/1/stop", nil, true)

	rec := env.do(t, http.MethodPatch, "/feeds/1", url.Values{
		"started_at":    {"2026-10-20T12:00:00"},
		"ended_at":      {"2026-10-20T11:00:00"},
		"left_minutes":  {"10"},
		"right_minutes": {"0"},
	}, true)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("HX-Retarget"); got != "#feed-edit-dialog" {
		t.Fatalf("HX-Retarget = %q", got)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "That end time is before the start.") {
		t.Fatalf("inline error missing:\n%s", body)
	}
	if strings.Contains(body, "hx-swap-oob") {
		t.Fatal("validation response should not update other fragments")
	}
}

func TestCompletedEditRejectsFutureTimes(t *testing.T) {
	env := newTestEnv(t)
	env.do(t, http.MethodPost, "/feeds", url.Values{"side": {"left"}}, true)
	env.clock.Advance(20 * time.Minute)
	env.do(t, http.MethodPost, "/feeds/1/stop", nil, true)

	rec := env.do(t, http.MethodPatch, "/feeds/1", url.Values{
		"started_at":    {"2026-10-20T12:30:00"},
		"ended_at":      {"2026-10-20T12:40:00"},
		"left_minutes":  {"5"},
		"right_minutes": {"0"},
	}, true)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "That start is in the future.") || !strings.Contains(body, "That end time is in the future.") {
		t.Fatalf("future errors missing:\n%s", body)
	}
}

func TestActiveAdjustAndFutureRejection(t *testing.T) {
	env := newTestEnv(t)
	env.do(t, http.MethodPost, "/feeds", url.Values{"side": {"left"}}, true)
	env.clock.Advance(10 * time.Minute)

	rec := env.do(t, http.MethodPatch, "/feeds/1", url.Values{
		"segment_started_at": {"2026-10-20T12:00:00"},
		"adjust":             {"-5"},
	}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `value="2026-10-20T11:55:00"`) {
		t.Fatalf("adjust did not update dialog value:\n%s", body)
	}
	if !strings.Contains(body, "15:00") {
		t.Fatalf("adjust did not update the OOB timer:\n%s", body)
	}
	if !strings.Contains(body, `hx-swap-oob="outerHTML"`) {
		t.Fatal("adjust response missing OOB timer")
	}

	active, err := env.store.ActiveFeed(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !active.StartedAt.Equal(time.Date(2026, 10, 20, 11, 55, 0, 0, time.UTC)) {
		t.Fatalf("feed start = %v", active.StartedAt)
	}

	rec = env.do(t, http.MethodPatch, "/feeds/1", url.Values{
		"segment_started_at": {"2026-10-20T12:30:00"},
	}, true)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("future start status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "That start is in the future.") {
		t.Fatalf("future error missing:\n%s", rec.Body.String())
	}
}

func TestNonHTMXEditValidationRendersOpenDialog(t *testing.T) {
	env := newTestEnv(t)
	env.do(t, http.MethodPost, "/feeds", url.Values{"side": {"left"}}, true)
	env.clock.Advance(20 * time.Minute)
	env.do(t, http.MethodPost, "/feeds/1/stop", nil, true)

	rec := env.do(t, http.MethodPost, "/feeds/1", url.Values{
		"started_at":    {"2026-10-20T12:00:00"},
		"ended_at":      {"2026-10-20T11:00:00"},
		"left_minutes":  {"10"},
		"right_minutes": {"0"},
	}, false)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `id="feed-edit-dialog"`) || !strings.Contains(body, " open>") {
		t.Fatalf("dialog was not rendered open:\n%s", body)
	}
	if !strings.Contains(body, "That end time is before the start.") {
		t.Fatalf("inline error missing from full page")
	}
}

func TestDeleteFeed(t *testing.T) {
	env := newTestEnv(t)
	env.do(t, http.MethodPost, "/feeds", url.Values{"side": {"left"}}, true)
	env.clock.Advance(5 * time.Minute)
	env.do(t, http.MethodPost, "/feeds/1/stop", nil, true)

	rec := env.do(t, http.MethodDelete, "/feeds/1", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Feed deleted.") {
		t.Fatalf("delete toast missing:\n%s", rec.Body.String())
	}

	rec = env.do(t, http.MethodGet, "/", nil, false)
	if !strings.Contains(rec.Body.String(), "Nothing logged yet today.") {
		t.Fatal("deleted feed still visible")
	}

	rec = env.do(t, http.MethodDelete, "/feeds/1", nil, true)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "already deleted") {
		t.Fatalf("double delete = %d %s", rec.Code, rec.Body.String())
	}
}

func TestDeleteActiveFeedStopsTimer(t *testing.T) {
	env := newTestEnv(t)
	env.do(t, http.MethodPost, "/feeds", url.Values{"side": {"right"}}, true)
	rec := env.do(t, http.MethodDelete, "/feeds/1", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "data-feed-id=") {
		t.Fatal("timer still active after deleting the feed")
	}
	active, err := env.store.ActiveFeed(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if active != nil {
		t.Fatalf("active = %+v", active)
	}
}

func TestTimerFragment(t *testing.T) {
	env := newTestEnv(t)
	env.do(t, http.MethodPost, "/feeds", url.Values{"side": {"left"}}, true)

	rec := env.do(t, http.MethodGet, "/partials/feed/timer", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{`id="timer-card"`, `data-feed-id="1"`, `id="last-nursed"`, `id="status-strip"`, `id="timeline-today"`} {
		if !strings.Contains(body, want) {
			t.Errorf("timer fragment missing %q", want)
		}
	}
	if strings.Contains(body, `id="toast-region"`) {
		t.Fatal("reconcile fragment should not clear the toast")
	}
}

func TestHealth(t *testing.T) {
	env := newTestEnv(t)
	rec := env.do(t, http.MethodGet, "/healthz", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if payload["status"] != "ok" || payload["db"] != "ok" || payload["mode"] != "local" || payload["version"] != "test" {
		t.Fatalf("health = %+v", payload)
	}

	rec = env.do(t, http.MethodGet, "/healthz?verbose=1", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("verbose status = %d", rec.Code)
	}
}

func TestStaticAssets(t *testing.T) {
	env := newTestEnv(t)
	rec := env.do(t, http.MethodGet, "/static/app.css", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("css status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/css") {
		t.Fatalf("css Content-Type = %q", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "max-age=300") {
		t.Fatalf("css Cache-Control = %q", cc)
	}

	rec = env.do(t, http.MethodGet, "/static/fonts/fredoka-var.woff2", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("font status = %d", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Fatalf("font Cache-Control = %q", cc)
	}

	rec = env.do(t, http.MethodGet, "/static/", nil, false)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("directory listing status = %d, want 404", rec.Code)
	}
}

func TestDayBoundsAcrossDST(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		now  time.Time
		want time.Duration
	}{
		{"normal", time.Date(2026, 10, 20, 12, 0, 0, 0, loc), 24 * time.Hour},
		{"spring forward", time.Date(2026, 3, 8, 12, 0, 0, 0, loc), 23 * time.Hour},
		{"fall back", time.Date(2026, 11, 1, 12, 0, 0, 0, loc), 25 * time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end := DayBounds(tt.now, loc)
			if got := end.Sub(start); got != tt.want {
				t.Fatalf("day length = %v, want %v", got, tt.want)
			}
			local := start.In(loc)
			if local.Hour() != 0 || local.Minute() != 0 {
				t.Fatalf("day start = %v, want local midnight", local)
			}
		})
	}

	// A 23:30 local feed belongs to that local day, not the UTC day.
	moment := time.Date(2026, 10, 20, 23, 30, 0, 0, loc)
	start, end := DayBounds(moment, loc)
	if moment.Before(start) || !moment.Before(end) {
		t.Fatalf("23:30 local is outside its own day")
	}
}

func TestParseLocalDateTime(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	got, err := parseLocalDateTime("2026-10-20T14:32", loc)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 10, 20, 18, 32, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("parsed = %v, want %v", got, want)
	}
	if _, err := parseLocalDateTime("nope", loc); err == nil {
		t.Fatal("invalid input accepted")
	}
}

func TestInvalidSideRejected(t *testing.T) {
	env := newTestEnv(t)
	rec := env.do(t, http.MethodPost, "/feeds", url.Values{"side": {"middle"}}, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	feeds, err := env.store.RecentFeeds(context.Background(), testNow.Add(-time.Hour), testNow.Add(time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(feeds) != 0 {
		t.Fatalf("feeds = %d, want 0", len(feeds))
	}
}

func TestCompletedFeedSegmentsUseSubmittedTotals(t *testing.T) {
	env := newTestEnv(t)
	env.do(t, http.MethodPost, "/feeds", url.Values{"side": {"right"}}, true)
	env.clock.Advance(30 * time.Minute)
	env.do(t, http.MethodPost, "/feeds/1/stop", nil, true)

	rec := env.do(t, http.MethodPatch, "/feeds/1", url.Values{
		"started_at":    {"2026-10-20T12:00:00"},
		"ended_at":      {"2026-10-20T12:30:00"},
		"left_minutes":  {"4"},
		"right_minutes": {"6"},
	}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	f, err := env.store.GetFeed(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Segments) != 2 {
		t.Fatalf("segments = %d, want 2", len(f.Segments))
	}
	if f.Segments[0].Side != feed.SideLeft || f.Segments[1].Side != feed.SideRight {
		t.Fatalf("segments not ordered left then right: %+v", f.Segments)
	}
	left, right := f.Totals(testNow)
	if left != 4*time.Minute || right != 6*time.Minute {
		t.Fatalf("totals = %v/%v", left, right)
	}
}
