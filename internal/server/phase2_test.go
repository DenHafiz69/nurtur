package server

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"nurtur/internal/diaper"
)

func TestDiaperOneTapDedupesRapidTaps(t *testing.T) {
	env := newTestEnv(t)

	rec := env.do(t, http.MethodPost, "/diapers", url.Values{"type": {"wet"}}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"Wet diaper logged.", "Add details", `id="status-strip"`, `id="timeline-today"`, `id="sleep-pill"`, `id="toast-region"`} {
		if !strings.Contains(body, want) {
			t.Errorf("response missing %q\n%s", want, body)
		}
	}
	if got := rec.Header().Get("HX-Trigger-After-Swap"); got != "sheet-saved" {
		t.Fatalf("HX-Trigger = %q", got)
	}

	rec = env.do(t, http.MethodPost, "/diapers", url.Values{"type": {"wet"}}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("second status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "was just logged") {
		t.Fatalf("duplicate tap was not friendly:\n%s", rec.Body.String())
	}

	rows, err := env.store.RecentDiapers(context.Background(), testNow.Add(-time.Hour), testNow.Add(time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("diapers = %d, want 1", len(rows))
	}
}

func TestDiaperBothSatisfiesWetAndDirty(t *testing.T) {
	env := newTestEnv(t)
	rec := env.do(t, http.MethodPost, "/diapers", url.Values{"type": {"both"}}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	wet, err := env.store.LastDiaper(context.Background(), diaper.Wet)
	if err != nil || wet == nil || wet.Kind != diaper.Both {
		t.Fatalf("last wet = %+v, %v", wet, err)
	}
	dirty, err := env.store.LastDiaper(context.Background(), diaper.Dirty)
	if err != nil || dirty == nil || dirty.Kind != diaper.Both {
		t.Fatalf("last dirty = %+v, %v", dirty, err)
	}

	rec = env.do(t, http.MethodGet, "/", nil, false)
	body := rec.Body.String()
	for _, want := range []string{"Both", "Diaper", "Wet", "1 diaper"} {
		if !strings.Contains(body, want) {
			t.Errorf("dashboard missing %q\n%s", want, body)
		}
	}
}

func TestDiaperDetailsSheetAndUpdate(t *testing.T) {
	env := newTestEnv(t)
	env.do(t, http.MethodPost, "/diapers", url.Values{"type": {"dirty"}}, true)

	rec := env.do(t, http.MethodGet, "/diapers/1/sheet", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("sheet status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{`id="action-dialog-title"`, "Dirty diaper", "Colour", "Consistency", "Mustard", "Seedy"} {
		if !strings.Contains(body, want) {
			t.Errorf("diaper edit missing %q\n%s", want, body)
		}
	}

	rec = env.do(t, http.MethodPatch, "/diapers/1", url.Values{
		"logged_at":   {"2026-10-20T11:50:00"},
		"color":       {"mustard"},
		"consistency": {"seedy"},
		"notes":       {"A little seedy."},
	}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("HX-Retarget"); got != "#status-strip" {
		t.Fatalf("HX-Retarget = %q", got)
	}
	if !strings.Contains(rec.Body.String(), "Diaper details saved.") {
		t.Fatalf("toast missing:\n%s", rec.Body.String())
	}
	d, err := env.store.GetDiaper(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if d.Color != diaper.ColorMustard || d.Consistency != diaper.Seedy || d.Notes != "A little seedy." {
		t.Fatalf("diaper = %+v", d)
	}

	rec = env.do(t, http.MethodPatch, "/diapers/1", url.Values{
		"logged_at":   {"2026-10-20T12:30:00"},
		"color":       {"mustard"},
		"consistency": {"seedy"},
	}, true)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("future time status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("HX-Retarget"); got != "#action-dialog" {
		t.Fatalf("error HX-Retarget = %q", got)
	}
	if !strings.Contains(rec.Body.String(), "That time is in the future.") {
		t.Fatalf("inline error missing:\n%s", rec.Body.String())
	}
}

func TestDiaperDeleteAndUndo(t *testing.T) {
	env := newTestEnv(t)
	env.do(t, http.MethodPost, "/diapers", url.Values{"type": {"dirty"}}, true)

	rec := env.do(t, http.MethodDelete, "/diapers/1", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Diaper deleted.", "Undo", "/diapers/1/restore", `data-toast-duration="8000"`} {
		if !strings.Contains(body, want) {
			t.Errorf("delete response missing %q\n%s", want, body)
		}
	}
	last, err := env.store.LastDiaper(context.Background(), diaper.Dirty)
	if err != nil || last != nil {
		t.Fatalf("deleted diaper still last: %+v %v", last, err)
	}

	rec = env.do(t, http.MethodPost, "/diapers/1/restore", nil, true)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Diaper restored.") {
		t.Fatalf("restore = %d %s", rec.Code, rec.Body.String())
	}
	last, err = env.store.LastDiaper(context.Background(), diaper.Dirty)
	if err != nil || last == nil || last.ID != 1 {
		t.Fatalf("restored diaper = %+v, %v", last, err)
	}
}

func TestSleepStartStopFlow(t *testing.T) {
	env := newTestEnv(t)

	rec := env.do(t, http.MethodPost, "/sleeps", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("start status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`id="sleep-pill"`, "Pip is asleep", "asleep. Sleep well.", "data-live-since", `id="status-strip"`, `id="timeline-today"`, "Sleeping"} {
		if !strings.Contains(body, want) {
			t.Errorf("sleep start missing %q\n%s", want, body)
		}
	}

	env.clock.Advance(30 * time.Minute)
	rec = env.do(t, http.MethodPost, "/sleeps/1/stop", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("stop status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body = rec.Body.String()
	for _, want := range []string{"Awake. That was 30m.", "Start sleep", "Awake", "30m"} {
		if !strings.Contains(body, want) {
			t.Errorf("sleep stop missing %q\n%s", want, body)
		}
	}

	active, err := env.store.ActiveSleep(context.Background())
	if err != nil || active != nil {
		t.Fatalf("active sleep = %+v, %v", active, err)
	}
	last, err := env.store.LastEndedSleep(context.Background())
	if err != nil || last == nil || last.Duration(env.clock.Now()) != 30*time.Minute {
		t.Fatalf("last sleep = %+v, %v", last, err)
	}
}

func TestRetroactiveSleepAndOverlap(t *testing.T) {
	env := newTestEnv(t)

	rec := env.do(t, http.MethodPost, "/sleeps", url.Values{
		"started_at": {"2026-10-20T10:30:00"},
		"ended_at":   {"2026-10-20T11:15:00"},
		"notes":      {"Morning nap"},
	}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("retro status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Sleep saved: 45m.") {
		t.Fatalf("retro toast missing:\n%s", rec.Body.String())
	}
	sessions, err := env.store.SleepsBetween(context.Background(), testNow.Add(-24*time.Hour), testNow.Add(time.Hour), testNow, 10)
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions = %+v, %v", sessions, err)
	}
	if sessions[0].Notes != "Morning nap" {
		t.Fatalf("notes = %q", sessions[0].Notes)
	}

	// Start a live sleep, then try to log over it.
	env.do(t, http.MethodPost, "/sleeps", nil, true)
	rec = env.do(t, http.MethodPost, "/sleeps", url.Values{
		"started_at": {"2026-10-20T11:50:00"},
		"ended_at":   {"2026-10-20T12:00:30"},
	}, true)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("overlap status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "overlaps another sleep") {
		t.Fatalf("overlap error missing:\n%s", rec.Body.String())
	}
}

func TestSleepSheetAndEdit(t *testing.T) {
	env := newTestEnv(t)
	env.do(t, http.MethodPost, "/sleeps", nil, true)
	env.clock.Advance(20 * time.Minute)
	env.do(t, http.MethodPost, "/sleeps/1/stop", nil, true)

	rec := env.do(t, http.MethodGet, "/sleeps/sheet", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("sheet status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Sleep", "today", "Day", "Night", "Log an earlier sleep", "Slept"} {
		if !strings.Contains(body, want) {
			t.Errorf("sleep sheet missing %q\n%s", want, body)
		}
	}

	rec = env.do(t, http.MethodGet, "/sleeps/1/sheet", nil, true)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Edit sleep") {
		t.Fatalf("sleep edit = %d %s", rec.Code, rec.Body.String())
	}

	rec = env.do(t, http.MethodPatch, "/sleeps/1", url.Values{
		"started_at": {"2026-10-20T11:30:00"},
		"ended_at":   {"2026-10-20T11:55:00"},
		"notes":      {"Corrected"},
	}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("HX-Retarget"); got != "#sleep-pill" {
		t.Fatalf("HX-Retarget = %q", got)
	}
	updated, err := env.store.GetSleep(context.Background(), 1)
	if err != nil || updated.Notes != "Corrected" || updated.Duration(env.clock.Now()) != 25*time.Minute {
		t.Fatalf("updated sleep = %+v, %v", updated, err)
	}
}

func TestDeleteActiveSleepAndUndo(t *testing.T) {
	env := newTestEnv(t)
	env.do(t, http.MethodPost, "/sleeps", nil, true)

	rec := env.do(t, http.MethodDelete, "/sleeps/1", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Sleep deleted.") || !strings.Contains(rec.Body.String(), "/sleeps/1/restore") {
		t.Fatalf("delete toast missing:\n%s", rec.Body.String())
	}
	active, err := env.store.ActiveSleep(context.Background())
	if err != nil || active != nil {
		t.Fatalf("active sleep = %+v, %v", active, err)
	}

	rec = env.do(t, http.MethodPost, "/sleeps/1/restore", nil, true)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Sleep restored.") {
		t.Fatalf("restore = %d %s", rec.Code, rec.Body.String())
	}
	restored, err := env.store.GetSleep(context.Background(), 1)
	if err != nil || restored.EndedAt == nil {
		t.Fatalf("restored active sleep should be closed: %+v, %v", restored, err)
	}
}

func TestStatusCardsRenderAllThree(t *testing.T) {
	env := newTestEnv(t)
	rec := env.do(t, http.MethodGet, "/", nil, false)
	body := rec.Body.String()
	for _, want := range []string{
		`id="status-strip"`,
		"Nursing", "Diaper", "Sleep",
		"No feeds yet", "No diapers yet", "No sleep yet",
		`href="/feeds/sheet"`, `href="/diapers/sheet"`, `href="/sleeps/sheet"`,
		`id="sleep-pill"`, "Start sleep",
		"Wet", "Dirty", "Both",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("dashboard missing %q\n%s", want, body)
		}
	}

	// Feed + diaper + sleep all present.
	env.do(t, http.MethodPost, "/feeds", url.Values{"side": {"left"}}, true)
	env.clock.Advance(5 * time.Minute)
	env.do(t, http.MethodPost, "/feeds/1/stop", nil, true)
	env.do(t, http.MethodPost, "/diapers", url.Values{"type": {"both"}}, true)
	env.do(t, http.MethodPost, "/sleeps", nil, true)
	env.clock.Advance(20 * time.Minute)

	rec = env.do(t, http.MethodGet, "/", nil, false)
	body = rec.Body.String()
	for _, want := range []string{"1 feed", "1 diaper", "Nursing", "Both", "Asleep", "Nursed", "Sleeping"} {
		if !strings.Contains(body, want) {
			t.Errorf("populated dashboard missing %q\n%s", want, body)
		}
	}
}

func TestNursingSheet(t *testing.T) {
	env := newTestEnv(t)
	rec := env.do(t, http.MethodGet, "/feeds/sheet", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Nursing", "Tap a side to start", `hx-post="/feeds"`, "No feeds yet"} {
		if !strings.Contains(body, want) {
			t.Errorf("nursing sheet missing %q\n%s", want, body)
		}
	}

	env.do(t, http.MethodPost, "/feeds", url.Values{"side": {"right"}}, true)
	rec = env.do(t, http.MethodGet, "/feeds/sheet", nil, true)
	body = rec.Body.String()
	for _, want := range []string{"Nursing Right", "Switch side", "Done", "Adjust the start time"} {
		if !strings.Contains(body, want) {
			t.Errorf("active nursing sheet missing %q\n%s", want, body)
		}
	}
}

func TestNonHTMXSheetsRenderOpenDialog(t *testing.T) {
	env := newTestEnv(t)
	for _, path := range []string{"/feeds/sheet", "/diapers/sheet", "/sleeps/sheet"} {
		rec := env.do(t, http.MethodGet, path, nil, false)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d", path, rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `id="action-dialog"`) || !strings.Contains(body, ` open>`) {
			t.Fatalf("%s did not render an open action dialog", path)
		}
	}
}

func TestNonHTMXSleepStartRedirects(t *testing.T) {
	env := newTestEnv(t)
	rec := env.do(t, http.MethodPost, "/sleeps", nil, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	active, err := env.store.ActiveSleep(context.Background())
	if err != nil || active == nil {
		t.Fatalf("sleep not started: %+v %v", active, err)
	}
}

func TestSleepInvalidTimesStayInSheet(t *testing.T) {
	env := newTestEnv(t)
	rec := env.do(t, http.MethodPost, "/sleeps", url.Values{
		"started_at": {"2026-10-20T12:00:00"},
		"ended_at":   {"2026-10-20T11:00:00"},
	}, true)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("HX-Retarget"); got != "#action-dialog" {
		t.Fatalf("HX-Retarget = %q", got)
	}
	if !strings.Contains(rec.Body.String(), "That end time is before the start.") {
		t.Fatalf("inline error missing:\n%s", rec.Body.String())
	}
}

func TestSleepDeletedRowsAreHidden(t *testing.T) {
	env := newTestEnv(t)
	session, err := env.store.StartSleep(context.Background(), testNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.store.DeleteSleep(context.Background(), session.ID, testNow.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	rec := env.do(t, http.MethodGet, "/", nil, false)
	if strings.Contains(rec.Body.String(), "Sleeping") {
		t.Fatal("deleted sleep still appears in the dashboard")
	}
	if !strings.Contains(rec.Body.String(), "No sleep yet") {
		t.Fatalf("empty sleep card missing:\n%s", rec.Body.String())
	}
}
