package server

import (
	"net/http"
	"strings"
	"testing"
)

func TestAlbumPageShell(t *testing.T) {
	env := newTestEnv(t)
	rec := env.do(t, http.MethodGet, "/album", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`<h2 class="page-title">Album</h2>`,
		`data-view="album"`,
		"Photos of the small moments.",
		"No photos yet.",
		"Snap the first one.",
		`class="tabbar-link is-active"`,
		`aria-current="page"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("album page missing %q:\n%s", want, body)
		}
	}

	start := strings.Index(body, `class="tabbar"`)
	if start < 0 {
		t.Fatal("tabbar not found")
	}
	end := strings.Index(body[start:], "</nav>")
	if end < 0 {
		t.Fatal("tabbar has no closing nav")
	}
	tabbar := body[start : start+end]
	home := strings.Index(tabbar, `href="/"`)
	album := strings.Index(tabbar, `href="/album"`)
	history := strings.Index(tabbar, `href="/timeline"`)
	settings := strings.Index(tabbar, `href="/settings"`)
	if home < 0 || album < 0 || history < 0 || settings < 0 {
		t.Fatalf("tabbar is missing a tab:\n%s", tabbar)
	}
	if !(home < album && album < history && history < settings) {
		t.Fatalf("album tab is not directly right of home:\n%s", tabbar)
	}
}

func TestAlbumPartialReturnsDaysFragment(t *testing.T) {
	env := newTestEnv(t)
	rec := env.do(t, http.MethodGet, "/album/partial", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "<!doctype") || strings.Contains(body, `class="tabbar"`) {
		t.Fatalf("partial rendered the whole shell:\n%s", body)
	}
	for _, want := range []string{`id="album-days"`, "No photos yet.", "Snap the first one."} {
		if !strings.Contains(body, want) {
			t.Fatalf("album partial missing %q:\n%s", want, body)
		}
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
}
