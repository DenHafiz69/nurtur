package server

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
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

func TestBabyNameFallsBackToEnvironment(t *testing.T) {
	env := newTestEnv(t)

	rec := env.do(t, http.MethodGet, "/", nil, false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Pip") {
		t.Fatalf("environment name not used:\n%s", rec.Body.String())
	}
}
