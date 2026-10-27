package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"nurtur/internal/diaper"
	"nurtur/internal/feed"
)

func TestTimelinePageGroupsByLocalDay(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	f, err := env.store.CreateFeed(ctx, feed.SideLeft, testNow.Add(-2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.store.StopFeed(ctx, f.ID, testNow.Add(-100*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := env.store.CreateDiaper(ctx, diaper.Wet, testNow.Add(-90*time.Minute)); err != nil {
		t.Fatal(err)
	}
	session, err := env.store.StartSleep(ctx, testNow.Add(-80*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.store.StopSleep(ctx, session.ID, testNow.Add(-40*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := env.store.CreateDiaper(ctx, diaper.Dirty, testNow.Add(-30*time.Hour)); err != nil {
		t.Fatal(err)
	}

	rec := env.do(t, http.MethodGet, "/timeline", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"History", "Today", "Yesterday", "Nursed", "Wet", "Slept", "timeline-list",
		"1 feed", `class="feed-summary"`, "<dt>Total</dt>", "<dt>Left</dt>", "<dt>Right</dt>", "20m",
		`<details class="timeline-day"`, `<summary id="day-head-`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("timeline missing %q\n%s", want, body)
		}
	}

	// A cursor at local midnight returns only the previous day.
	rec = env.do(t, http.MethodGet, "/timeline/partial?before=2026-10-20T00:00:00Z", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("partial status = %d", rec.Code)
	}
	partial := rec.Body.String()
	if !strings.Contains(partial, "Yesterday") || !strings.Contains(partial, "Dirty") {
		t.Fatalf("partial missing yesterday's row:\n%s", partial)
	}
	if strings.Contains(partial, "Today") {
		t.Fatalf("partial leaked today's events:\n%s", partial)
	}
}

func TestHistoryDaySummaryRefreshesAfterTimerMutation(t *testing.T) {
	env := newTestEnv(t)

	form := url.Values{"side": {"left"}}.Encode()
	req := httptest.NewRequest(http.MethodPost, "/feeds", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Current-URL", "http://example.test/timeline")
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`id="day-head-2026-10-20"`, `class="feed-summary"`, `hx-swap-oob="outerHTML"`} {
		if !strings.Contains(body, want) {
			t.Errorf("timer response missing %q\n%s", want, body)
		}
	}
}

func TestDiaperQuickLogFromHistoryInsertsTodayRow(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	// The history page already has a today group, so the new row can be
	// prepended to it instead of replacing the whole list.
	if _, _, err := env.store.CreateDiaper(ctx, diaper.Wet, testNow.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}

	form := url.Values{"type": {"dirty"}, "timeline_today": {"1"}}
	req := httptest.NewRequest(http.MethodPost, "/diapers", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Current-URL", "http://example.test/timeline")
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("HX-Retarget"); got != "#timeline-events-2026-10-20" {
		t.Errorf("HX-Retarget = %q", got)
	}
	if got := rec.Header().Get("HX-Reswap"); got != "afterbegin" {
		t.Errorf("HX-Reswap = %q", got)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`id="row-diaper-2"`,
		`id="day-head-2026-10-20"`,
		`id="toast-region"`,
		"Dirty diaper logged.",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("history quick log missing %q\n%s", want, body)
		}
	}
	if strings.Contains(body, `id="status-strip"`) {
		t.Errorf("history quick log should not target the dashboard status strip:\n%s", body)
	}
}

func TestDiaperQuickLogFromEmptyHistoryRefreshesList(t *testing.T) {
	env := newTestEnv(t)

	form := url.Values{"type": {"wet"}}
	req := httptest.NewRequest(http.MethodPost, "/diapers", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Current-URL", "http://example.test/timeline")
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("HX-Retarget"); got != "#timeline-list" {
		t.Errorf("HX-Retarget = %q", got)
	}
	if got := rec.Header().Get("HX-Reswap"); got != "innerHTML" {
		t.Errorf("HX-Reswap = %q", got)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`id="timeline-events-2026-10-20"`,
		`id="toast-region"`,
		"Wet diaper logged.",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("empty-history quick log missing %q\n%s", want, body)
		}
	}
}

func TestTimelineDeleteAndUndoRestoresInPlace(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	d, _, err := env.store.CreateDiaper(ctx, diaper.Wet, testNow.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	rec := env.do(t, http.MethodDelete, "/timeline/diaper/"+itoa(d.ID), nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`id="deleted-diaper-1"`,
		`class="timeline-placeholder"`,
		"Diaper deleted.",
		"Undo",
		`id="status-strip"`,
		`id="day-head-2026-10-20"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("delete response missing %q\n%s", want, body)
		}
	}
	if _, err := env.store.GetDiaper(ctx, d.ID); err == nil {
		t.Fatal("diaper should be soft-deleted")
	}

	rec = env.do(t, http.MethodPost, "/timeline/diaper/"+itoa(d.ID)+"/restore", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("restore status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body = rec.Body.String()
	for _, want := range []string{`id="row-diaper-1"`, "Diaper restored.", "Wet"} {
		if !strings.Contains(body, want) {
			t.Errorf("restore response missing %q\n%s", want, body)
		}
	}
	if _, err := env.store.GetDiaper(ctx, d.ID); err != nil {
		t.Fatalf("diaper was not restored: %v", err)
	}
}

func TestTimelineDeleteActiveFeedClearsTimer(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	f, err := env.store.CreateFeed(ctx, feed.SideRight, testNow.Add(-20*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	rec := env.do(t, http.MethodDelete, "/timeline/feed/"+itoa(f.ID), nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`id="deleted-feed-1"`, `id="timer-card"`, "Feed deleted."} {
		if !strings.Contains(body, want) {
			t.Errorf("delete response missing %q\n%s", want, body)
		}
	}

	if rec.Header().Get("HX-Trigger") != "timer-cleared" {
		t.Errorf("feed delete HX-Trigger = %q", rec.Header().Get("HX-Trigger"))
	}

	rec = env.do(t, http.MethodPost, "/timeline/feed/"+itoa(f.ID)+"/restore", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("restore status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Feed restored.") {
		t.Fatalf("restore toast missing:\n%s", rec.Body.String())
	}
}

func TestSettingsNightWindowPersists(t *testing.T) {
	env := newTestEnv(t)
	rec := env.do(t, http.MethodPost, "/settings", url.Values{
		"night_start": {"20:00"},
		"night_end":   {"06:00"},
	}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"Night window saved.", `value="20:00"`, `value="06:00"`} {
		if !strings.Contains(body, want) {
			t.Errorf("settings response missing %q\n%s", want, body)
		}
	}
	start, ok, err := env.store.Setting(context.Background(), settingNightStart)
	if err != nil || !ok || start != "20:00" {
		t.Fatalf("stored night start = %q ok=%v err=%v", start, ok, err)
	}

	rec = env.do(t, http.MethodPost, "/settings", url.Values{
		"night_start": {"20:00"},
		"night_end":   {"20:00"},
	}, true)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("same window status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "start and end at the same time") {
		t.Fatalf("missing validation message:\n%s", rec.Body.String())
	}
}

func TestExportEndpoints(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	f, err := env.store.CreateFeed(ctx, feed.SideLeft, testNow.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.store.StopFeed(ctx, f.ID, testNow.Add(-50*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := env.store.CreateDiaper(ctx, diaper.Both, testNow.Add(-40*time.Minute)); err != nil {
		t.Fatal(err)
	}

	rec := env.do(t, http.MethodGet, "/export.csv", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("csv status = %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.Contains(got, "attachment") {
		t.Fatalf("csv content-disposition = %q", got)
	}
	csvBody := rec.Body.String()
	for _, want := range []string{"kind,id,started_at", "feed,1,", "diaper,1,"} {
		if !strings.Contains(csvBody, want) {
			t.Errorf("csv missing %q\n%s", want, csvBody)
		}
	}

	rec = env.do(t, http.MethodGet, "/export.json", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("json status = %d", rec.Code)
	}
	jsonBody := rec.Body.String()
	for _, want := range []string{`"feeds"`, `"diapers"`, `"sleeps"`, `"baby_name": "Pip"`, `"left_minutes": 10`} {
		if !strings.Contains(jsonBody, want) {
			t.Errorf("json missing %q\n%s", want, jsonBody)
		}
	}

	rec = env.do(t, http.MethodGet, "/export.sql", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("sql status = %d", rec.Code)
	}
	sqlBody := rec.Body.String()
	for _, want := range []string{"BEGIN TRANSACTION", "CREATE TABLE feeds", "CREATE TABLE IF NOT EXISTS schema_migrations", "INSERT INTO feeds", "INSERT INTO feed_segments", "INSERT OR IGNORE INTO schema_migrations"} {
		if !strings.Contains(sqlBody, want) {
			t.Errorf("sql missing %q", want)
		}
	}
}

func TestPasscodeGateAndCSRF(t *testing.T) {
	env := newTestEnvConfig(t, func(cfg *Config) {
		cfg.Passcode = "secret"
	})

	// A signed-out visitor sees the landing page rather than the passcode form.
	rec := env.do(t, http.MethodGet, "/", nil, false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "One less thing to remember.") {
		t.Fatalf("root = %d %q", rec.Code, rec.Header().Get("Location"))
	}

	rec = env.do(t, http.MethodGet, "/login", nil, false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Passcode") {
		t.Fatalf("login page = %d\n%s", rec.Code, rec.Body.String())
	}

	rec = env.do(t, http.MethodPost, "/login", url.Values{"passcode": {"nope"}}, false)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong passcode = %d", rec.Code)
	}

	rec = env.do(t, http.MethodPost, "/login", url.Values{"passcode": {"secret"}}, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("right passcode = %d", rec.Code)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("no session cookie")
	}

	rec = doWith(t, env, http.MethodGet, "/", nil, map[string]string{
		"Cookie": cookies[0].Name + "=" + cookies[0].Value,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("authed root = %d", rec.Code)
	}
	token := csrfPattern.FindStringSubmatch(rec.Body.String())
	if len(token) != 2 {
		t.Fatalf("csrf token missing:\n%s", rec.Body.String())
	}

	// HTMX mutations require the token.
	rec = doWith(t, env, http.MethodPost, "/diapers", url.Values{"type": {"wet"}}, map[string]string{
		"Cookie":     cookies[0].Name + "=" + cookies[0].Value,
		"HX-Request": "true",
		"Origin":     "http://example.com",
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("missing csrf = %d", rec.Code)
	}

	rec = doWith(t, env, http.MethodPost, "/diapers", url.Values{"type": {"wet"}}, map[string]string{
		"Cookie":       cookies[0].Name + "=" + cookies[0].Value,
		"HX-Request":   "true",
		"X-CSRF-Token": token[1],
		"Origin":       "http://example.com",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("valid csrf = %d, body = %s", rec.Code, rec.Body.String())
	}
}

var csrfPattern = regexp.MustCompile(`X-CSRF-Token":"([^"]+)"`)

func doWith(t *testing.T, env *testEnv, method, path string, form url.Values, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req := httptest.NewRequest(method, path, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)
	return rec
}

func itoa(id int64) string {
	return strconv.FormatInt(id, 10)
}
