package server

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestPauseResumeKeepsTheTimerHonest(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	rec := env.do(t, http.MethodPost, "/feeds", url.Values{"side": {"left"}}, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("start feed = %d\n%s", rec.Code, rec.Body.String())
	}
	f, err := env.store.ActiveFeed(ctx)
	if err != nil || f == nil {
		t.Fatalf("active feed = %+v, err=%v", f, err)
	}
	pauseURL := fmt.Sprintf("/feeds/%d/pause", f.ID)
	resumeURL := fmt.Sprintf("/feeds/%d/resume", f.ID)

	env.clock.Advance(5 * time.Minute)
	rec = env.do(t, http.MethodPost, pauseURL, nil, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("pause = %d\n%s", rec.Code, rec.Body.String())
	}

	paused, err := env.store.ActiveFeed(ctx)
	if err != nil || paused == nil || !paused.Paused() {
		t.Fatalf("feed after pause = %+v, err=%v", paused, err)
	}

	// The dashboard shows a paused card with an obvious way back in.
	rec = env.do(t, http.MethodGet, "/", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("paused dashboard = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`data-paused="true"`,
		"Paused <strong>Left</strong>",
		"Resume",
		resumeURL,
		"so far",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("paused dashboard missing %q\n%s", want, body)
		}
	}

	// Paused minutes are a gap: the totals stay put while the feed is paused.
	left, right := paused.Totals(env.clock.Now())
	if left != 5*time.Minute || right != 0 {
		t.Fatalf("paused totals = %v/%v, want 5m/0m", left, right)
	}
	env.clock.Advance(30 * time.Minute)
	paused, err = env.store.ActiveFeed(ctx)
	if err != nil || paused == nil {
		t.Fatalf("active feed while paused = %+v, err=%v", paused, err)
	}
	left, right = paused.Totals(env.clock.Now())
	if left != 5*time.Minute || right != 0 {
		t.Fatalf("totals advanced during pause = %v/%v, want 5m/0m", left, right)
	}

	// Resume swaps the card back to a live timer through HTMX.
	rec = env.do(t, http.MethodPost, resumeURL, url.Values{"side": {"left"}}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("resume = %d\n%s", rec.Code, rec.Body.String())
	}
	body = rec.Body.String()
	for _, want := range []string{`data-paused="false"`, "Nursing <strong>Left</strong>", "this feed"} {
		if !strings.Contains(body, want) {
			t.Errorf("resumed timer missing %q\n%s", want, body)
		}
	}
	resumed, err := env.store.ActiveFeed(ctx)
	if err != nil || resumed == nil || resumed.Paused() {
		t.Fatalf("feed after resume = %+v, err=%v", resumed, err)
	}
	if len(resumed.Segments) != 2 {
		t.Fatalf("segments after resume = %d, want 2", len(resumed.Segments))
	}

	// A paused feed's edit sheet explains the state instead of offering a
	// start-time form that cannot apply while no side is open.
	env.clock.Advance(2 * time.Minute)
	if rec := env.do(t, http.MethodPost, pauseURL, nil, false); rec.Code != http.StatusSeeOther {
		t.Fatalf("second pause = %d\n%s", rec.Code, rec.Body.String())
	}
	rec = env.do(t, http.MethodGet, fmt.Sprintf("/feeds/%d/edit", f.ID), nil, false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "This feed is paused on Left") {
		t.Fatalf("paused edit sheet = %d\n%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Resume Left") {
		t.Errorf("paused edit sheet has no resume action:\n%s", rec.Body.String())
	}

	rec = env.do(t, http.MethodGet, "/feeds/sheet", nil, false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Paused on Left") {
		t.Fatalf("paused nursing sheet = %d\n%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), resumeURL) {
		t.Errorf("paused nursing sheet has no resume action:\n%s", rec.Body.String())
	}

	// Done from a paused feed still closes it, keeping the time already nursed.
	rec = env.do(t, http.MethodPost, fmt.Sprintf("/feeds/%d/stop", f.ID), nil, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("stop paused feed = %d\n%s", rec.Code, rec.Body.String())
	}
	stopped, err := env.store.GetFeed(ctx, f.ID)
	if err != nil {
		t.Fatalf("GetFeed: %v", err)
	}
	if stopped.EndedAt == nil {
		t.Fatal("paused feed should end when Done is tapped")
	}
	left, right = stopped.Totals(env.clock.Now())
	if left != 7*time.Minute || right != 0 {
		t.Fatalf("final totals = %v/%v, want 7m/0m", left, right)
	}
}
