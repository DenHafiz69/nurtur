package server

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestChangelogSheetOpensOnVersionChange(t *testing.T) {
	env := newTestEnvConfig(t, func(cfg *Config) {
		cfg.Version = "v0.3.8"
	})

	rec := env.do(t, http.MethodGet, "/", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("dashboard = %d\n%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`id="changelog-dialog"`,
		`id="changelog-title">What's new`,
		`v0.3.8`,
		`Update notes, in the app`,
		`action="/changelog/dismiss"`,
		`href="/changelog"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("first visit missing %q", want)
		}
	}

	rec = env.do(t, http.MethodPost, "/changelog/dismiss", url.Values{}, true)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("dismiss = %d\n%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("HX-Trigger"); got != "changelog-dismissed" {
		t.Errorf("HX-Trigger = %q, want changelog-dismissed", got)
	}
	stored, ok, err := env.store.Setting(context.Background(), settingChangelogSeen)
	if err != nil || !ok || stored != "v0.3.8" {
		t.Fatalf("stored seen version = %q, ok=%v, err=%v", stored, ok, err)
	}

	rec = env.do(t, http.MethodGet, "/", nil, false)
	if strings.Contains(rec.Body.String(), `id="changelog-dialog"`) {
		t.Fatal("changelog sheet reopened after dismissal")
	}
}

func TestChangelogShowsEverySkippedRelease(t *testing.T) {
	env := newTestEnvConfig(t, func(cfg *Config) {
		cfg.Version = "v0.3.8"
	})
	if err := env.store.SetSettings(context.Background(), map[string]string{
		settingChangelogSeen: "v0.3.5",
	}); err != nil {
		t.Fatalf("seed seen version: %v", err)
	}

	rec := env.do(t, http.MethodGet, "/", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("dashboard = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"v0.3.8", "v0.3.7", "v0.3.6"} {
		if !strings.Contains(body, want) {
			t.Errorf("skipped update sheet missing %q", want)
		}
	}
	if strings.Contains(body, "v0.3.5") {
		t.Errorf("skipped update sheet includes already-seen v0.3.5")
	}
}

func TestChangelogPageKeepsEveryRelease(t *testing.T) {
	env := newTestEnvConfig(t, func(cfg *Config) {
		cfg.Version = "v0.3.8"
	})

	rec := env.do(t, http.MethodGet, "/changelog", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("changelog page = %d\n%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"v0.1.0", "v0.2.0", "v0.3.0", "v0.3.8", "Every Nurtur update"} {
		if !strings.Contains(body, want) {
			t.Errorf("changelog page missing %q", want)
		}
	}
	if strings.Contains(body, `id="changelog-dialog"`) {
		t.Error("changelog page also opened the update sheet")
	}
}

func TestChangelogPlainDismissRedirectsBack(t *testing.T) {
	env := newTestEnvConfig(t, func(cfg *Config) {
		cfg.Version = "v0.3.8"
	})

	rec := env.do(t, http.MethodPost, "/changelog/dismiss", url.Values{
		"return_to": {"/settings"},
	}, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("plain dismiss = %d", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/settings" {
		t.Errorf("redirect = %q, want /settings", got)
	}
}

func TestChangelogDevBuildStaysQuiet(t *testing.T) {
	env := newTestEnv(t) // Version is "test", which has no release notes.

	rec := env.do(t, http.MethodGet, "/", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("dashboard = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), `id="changelog-dialog"`) {
		t.Fatal("dev build opened the changelog sheet")
	}
}
