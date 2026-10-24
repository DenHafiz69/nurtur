package server

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestSaveBabyNameUpdatesDashboardAndSettings(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	rec := env.do(t, http.MethodPost, "/settings/baby", url.Values{
		"baby_name": {"  Aisyah  "},
	}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("save name = %d\n%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Baby name saved.") {
		t.Fatalf("save response missing toast:\n%s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `<div id="baby-heading" class="brand-copy" hx-swap-oob="outerHTML">`) ||
		!strings.Contains(rec.Body.String(), `<h1 class="baby-name">Aisyah</h1>`) {
		t.Fatalf("save response does not refresh the header name:\n%s", rec.Body.String())
	}

	stored, ok, err := env.store.Setting(ctx, settingBabyName)
	if err != nil || !ok || stored != "Aisyah" {
		t.Fatalf("stored name = %q, ok=%v, err=%v", stored, ok, err)
	}

	rec = env.do(t, http.MethodGet, "/", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("dashboard = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Aisyah") {
		t.Fatalf("dashboard does not use the saved name:\n%s", rec.Body.String())
	}

	rec = env.do(t, http.MethodGet, "/settings", nil, false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `value="Aisyah"`) {
		t.Fatalf("settings does not show the saved name:\n%s", rec.Body.String())
	}
}

func TestSaveBabyNameRejectsEmpty(t *testing.T) {
	env := newTestEnv(t)

	rec := env.do(t, http.MethodPost, "/settings/baby", url.Values{
		"baby_name": {"   "},
	}, true)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("empty name = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "name is required") {
		t.Fatalf("empty name response missing error:\n%s", rec.Body.String())
	}
	if _, ok, err := env.store.Setting(context.Background(), settingBabyName); err != nil || ok {
		t.Fatalf("empty name was stored: ok=%v err=%v", ok, err)
	}
}

func TestBabyNameValidationMessages(t *testing.T) {
	if _, err := parseBabyName("   "); err != errBabyNameRequired {
		t.Fatalf("empty name error = %v", err)
	}
	if _, err := parseBabyName(strings.Repeat("a", 41)); err != errBabyNameTooLong {
		t.Fatalf("long name error = %v", err)
	}
	if got := babyNameMessage(errBabyNameRequired); got != "Baby's name is required." {
		t.Errorf("required message = %q", got)
	}
	if got := babyNameMessage(errBabyNameTooLong); got != "Keep the baby's name under 40 characters." {
		t.Errorf("length message = %q", got)
	}
	if got := babyNameMessage(errors.New("other")); got != "That name doesn't look right. Try again?" {
		t.Errorf("fallback message = %q", got)
	}
}

func TestBabyNameFallsBackToEnvironment(t *testing.T) {
	env := newTestEnv(t)

	rec := env.do(t, http.MethodGet, "/", nil, false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Pip") {
		t.Fatalf("environment name not used:\n%s", rec.Body.String())
	}
}

func TestFeedAlertDefaultsToTenMinutes(t *testing.T) {
	env := newTestEnv(t)

	rec := env.do(t, http.MethodGet, "/settings", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("settings = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `name="feed_alert_minutes" value="10"`) {
		t.Fatalf("settings does not show the default alert:\n%s", rec.Body.String())
	}

	rec = env.do(t, http.MethodGet, "/", nil, false)
	if !strings.Contains(rec.Body.String(), `data-alert-minutes="10"`) {
		t.Fatalf("dashboard does not carry the alert threshold:\n%s", rec.Body.String())
	}
}

func TestSaveFeedAlertMinutes(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	rec := env.do(t, http.MethodPost, "/settings/feed-alert", url.Values{
		"feed_alert_minutes": {"15"},
	}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("save alert = %d\n%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Nursing alert saved.") {
		t.Fatalf("save response missing toast:\n%s", rec.Body.String())
	}

	stored, ok, err := env.store.Setting(ctx, settingFeedAlertMinutes)
	if err != nil || !ok || stored != "15" {
		t.Fatalf("stored alert = %q, ok=%v, err=%v", stored, ok, err)
	}

	rec = env.do(t, http.MethodGet, "/settings", nil, false)
	if !strings.Contains(rec.Body.String(), `name="feed_alert_minutes" value="15"`) {
		t.Fatalf("settings does not show the saved alert:\n%s", rec.Body.String())
	}

	if rec = env.do(t, http.MethodPost, "/feeds", url.Values{"side": {"left"}}, true); rec.Code != http.StatusOK {
		t.Fatalf("start feed = %d", rec.Code)
	}
	rec = env.do(t, http.MethodGet, "/", nil, false)
	if !strings.Contains(rec.Body.String(), `data-alert-minutes="15"`) {
		t.Fatalf("timer card does not carry the saved alert:\n%s", rec.Body.String())
	}
}

func TestFeedAlertValidation(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		status  int
		stored  string
		errCopy string
	}{
		{name: "empty", raw: "   ", status: http.StatusUnprocessableEntity, errCopy: "Enter 0 to turn alerts off"},
		{name: "not a number", raw: "soon", status: http.StatusUnprocessableEntity, errCopy: "Enter 0 to turn alerts off"},
		{name: "negative", raw: "-1", status: http.StatusUnprocessableEntity, errCopy: "Enter 0 to turn alerts off"},
		{name: "too high", raw: "121", status: http.StatusUnprocessableEntity, errCopy: "Enter 0 to turn alerts off"},
		{name: "off", raw: "0", status: http.StatusOK, stored: "0"},
		{name: "upper bound", raw: "120", status: http.StatusOK, stored: "120"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newTestEnv(t)
			rec := env.do(t, http.MethodPost, "/settings/feed-alert", url.Values{
				"feed_alert_minutes": {tc.raw},
			}, true)
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d\n%s", rec.Code, tc.status, rec.Body.String())
			}
			if tc.errCopy != "" && !strings.Contains(rec.Body.String(), tc.errCopy) {
				t.Fatalf("response missing %q:\n%s", tc.errCopy, rec.Body.String())
			}
			stored, ok, err := env.store.Setting(context.Background(), settingFeedAlertMinutes)
			if err != nil {
				t.Fatal(err)
			}
			if tc.stored == "" {
				if ok {
					t.Fatalf("invalid value stored: %q", stored)
				}
				return
			}
			if !ok || stored != tc.stored {
				t.Fatalf("stored alert = %q, ok=%v, want %q", stored, ok, tc.stored)
			}
		})
	}
}

func TestParseFeedAlertMinutes(t *testing.T) {
	for _, raw := range []string{"0", "10", "120", " 15 "} {
		if _, err := parseFeedAlertMinutes(raw); err != nil {
			t.Errorf("parseFeedAlertMinutes(%q) = %v", raw, err)
		}
	}
	for _, raw := range []string{"", "ten", "-1", "121", "1.5"} {
		if _, err := parseFeedAlertMinutes(raw); err == nil {
			t.Errorf("parseFeedAlertMinutes(%q) accepted an invalid value", raw)
		}
	}
}

func TestTimerCarriesTotalElapsedAcrossSides(t *testing.T) {
	env := newTestEnv(t)

	if rec := env.do(t, http.MethodPost, "/feeds", url.Values{"side": {"left"}}, true); rec.Code != http.StatusOK {
		t.Fatalf("start feed = %d\n%s", rec.Code, rec.Body.String())
	}
	env.clock.Advance(6 * time.Minute)
	// Switch after six minutes; the alert counts both sides together.
	rec := env.do(t, http.MethodPost, "/feeds/1/switch", url.Values{"side": {"right"}}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("switch = %d\n%s", rec.Code, rec.Body.String())
	}
	env.clock.Advance(4 * time.Minute)

	rec = env.do(t, http.MethodGet, "/partials/feed/timer", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("timer fragment = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `data-feed-total-ms="600000"`) {
		t.Fatalf("timer fragment missing the 10-minute total:\n%s", body)
	}
	if !strings.Contains(body, `data-alert-minutes="10"`) {
		t.Fatalf("timer fragment missing the alert threshold:\n%s", body)
	}
}

func TestSettingsPageCarriesActiveFeedForAlert(t *testing.T) {
	env := newTestEnv(t)

	if rec := env.do(t, http.MethodPost, "/feeds", url.Values{"side": {"left"}}, true); rec.Code != http.StatusOK {
		t.Fatalf("start feed = %d", rec.Code)
	}
	env.clock.Advance(3 * time.Minute)

	rec := env.do(t, http.MethodGet, "/settings", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("settings = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `data-active-feed-id="1"`) || !strings.Contains(body, `data-feed-total-ms="180000"`) {
		t.Fatalf("settings does not expose the active feed for the alert:\n%s", body)
	}
	if !strings.Contains(body, `data-alert-minutes="10"`) {
		t.Fatalf("settings panel does not carry the alert threshold:\n%s", body)
	}
}

func TestSaveMascotUpdatesHeaderAndSettings(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	rec := env.do(t, http.MethodGet, "/settings", nil, false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `value="pip" checked`) {
		t.Fatalf("settings does not start on Pip:\n%s", rec.Body.String())
	}

	rec = env.do(t, http.MethodPost, "/settings/mascot", url.Values{
		"mascot": {"cat"},
	}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("save mascot = %d\n%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Mascot saved.",
		`id="header-mascot" class="pip pip-header" hx-swap-oob="outerHTML"`,
		`data-mascot="cat"`,
		`value="cat" checked`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("save response missing %q\n%s", want, body)
		}
	}

	stored, ok, err := env.store.Setting(ctx, settingMascot)
	if err != nil || !ok || stored != "cat" {
		t.Fatalf("stored mascot = %q, ok=%v, err=%v", stored, ok, err)
	}

	rec = env.do(t, http.MethodGet, "/", nil, false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `data-mascot="cat"`) {
		t.Fatalf("dashboard does not use the saved mascot:\n%s", rec.Body.String())
	}
}

func TestMascotDefaultsToPip(t *testing.T) {
	env := newTestEnv(t)

	rec := env.do(t, http.MethodGet, "/", nil, false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `data-mascot="pip"`) {
		t.Fatalf("dashboard does not default to Pip:\n%s", rec.Body.String())
	}
	if stored, ok, err := env.store.Setting(context.Background(), settingMascot); err != nil || ok {
		t.Fatalf("default mascot was stored: %q ok=%v err=%v", stored, ok, err)
	}
}

func TestMascotValidationRejectsUnknown(t *testing.T) {
	env := newTestEnv(t)

	rec := env.do(t, http.MethodPost, "/settings/mascot", url.Values{
		"mascot": {"dog"},
	}, true)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown mascot = %d\n%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Choose Pip or Mochi.") {
		t.Fatalf("unknown mascot response missing error:\n%s", rec.Body.String())
	}
	if stored, ok, err := env.store.Setting(context.Background(), settingMascot); err != nil || ok {
		t.Fatalf("invalid mascot was stored: %q ok=%v err=%v", stored, ok, err)
	}
}

func TestParseMascot(t *testing.T) {
	cases := map[string]string{
		"":        "pip",
		"  PIP  ": "pip",
		"cat":     "cat",
		" Cat ":   "cat",
	}
	for raw, want := range cases {
		got, err := parseMascot(raw)
		if err != nil || got != want {
			t.Errorf("parseMascot(%q) = %q, %v, want %q", raw, got, err, want)
		}
	}
	if _, err := parseMascot("dog"); err != errMascotInvalid {
		t.Fatalf("parseMascot(dog) = %v, want %v", err, errMascotInvalid)
	}
	if got := mascotMessage(errMascotInvalid); got != "Choose Pip or Mochi." {
		t.Errorf("mascotMessage = %q", got)
	}
}
