package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestIntakeOptInAndQuickLogFlows(t *testing.T) {
	env := newTestEnv(t)

	// By default, growing-baby dock rows are hidden so existing prod users see no surprise buttons.
	res := env.do(t, http.MethodGet, "/", nil, false)
	if res.Code != http.StatusOK {
		t.Fatalf("GET / = %d", res.Code)
	}
	body := res.Body.String()
	if strings.Contains(body, `action="/bottles"`) || strings.Contains(body, `href="/pumps/sheet`) || strings.Contains(body, `href="/solids/sheet"`) {
		t.Fatal("expected intake dock rows to be hidden by default")
	}

	// Enable all 3 trackers in oz mode via Settings (HTMX request must OOB-update the dock).
	res = env.do(t, http.MethodPost, "/settings/trackers", url.Values{
		"track_bottles": {"1"},
		"track_pumps":   {"1"},
		"track_solids":  {"1"},
		"volume_unit":   {"oz"},
	}, true)
	if res.Code != http.StatusOK {
		t.Fatalf("POST /settings/trackers = %d: %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), `id="intake-dock-rows"`) || !strings.Contains(res.Body.String(), `hx-swap-oob="outerHTML"`) || !strings.Contains(res.Body.String(), "2 oz") {
		t.Fatalf("expected POST /settings/trackers to OOB-swap #intake-dock-rows")
	}

	res = env.do(t, http.MethodGet, "/", nil, false)
	body = res.Body.String()
	if !strings.Contains(body, "2 oz") || !strings.Contains(body, "/pumps/sheet?side=left") || !strings.Contains(body, "/solids/sheet") {
		t.Fatalf("expected enabled intake dock rows in oz mode")
	}

	// Quick-tap bottle
	res = env.do(t, http.MethodPost, "/bottles", url.Values{
		"milk_type": {"breast_milk"},
		"amount_ml": {"89"},
		"quick":     {"1"},
	}, true)
	if res.Code != http.StatusOK {
		t.Fatalf("POST /bottles quick = %d: %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "Bottle") {
		t.Fatalf("expected bottle toast, got: %s", res.Body.String())
	}

	// Pump sheet + create + validation error
	res = env.do(t, http.MethodGet, "/pumps/sheet?side=right", nil, true)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "Log pump") {
		t.Fatalf("GET /pumps/sheet = %d", res.Code)
	}
	res = env.do(t, http.MethodPost, "/pumps", url.Values{
		"side":             {"right"},
		"amount":           {"4"},
		"duration_minutes": {"15"},
	}, true)
	if res.Code != http.StatusOK {
		t.Fatalf("POST /pumps = %d: %s", res.Code, res.Body.String())
	}

	// Solid create (validation error then valid)
	res = env.do(t, http.MethodPost, "/solids", url.Values{
		"food": {"   "},
	}, true)
	if res.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for empty food, got %d", res.Code)
	}
	res = env.do(t, http.MethodPost, "/solids", url.Values{
		"food":     {"Banana mash"},
		"portion":  {"some"},
		"reaction": {"none"},
	}, true)
	if res.Code != http.StatusOK {
		t.Fatalf("POST /solids = %d: %s", res.Code, res.Body.String())
	}

	// Timeline & Today show all three
	res = env.do(t, http.MethodGet, "/", nil, false)
	body = res.Body.String()
	if !strings.Contains(body, "Bottle · 3 oz") || !strings.Contains(body, "Pumped · Right") || !strings.Contains(body, "Solids · Banana mash") {
		t.Fatalf("expected bottle, pump, and solid in Today list")
	}

	res = env.do(t, http.MethodGet, "/timeline", nil, false)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "Solids · Banana mash") {
		t.Fatalf("expected solid on /timeline, status=%d", res.Code)
	}

	// Delete & restore via timeline
	res = env.do(t, http.MethodDelete, "/timeline/solid/1", nil, true)
	if res.Code != http.StatusOK {
		t.Fatalf("DELETE /timeline/solid/1 = %d", res.Code)
	}
	res = env.do(t, http.MethodPost, "/timeline/solid/1/restore", nil, true)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "Banana mash") {
		t.Fatalf("POST /timeline/solid/1/restore = %d", res.Code)
	}

	// Exports include bottles, pumps, solids
	for _, path := range []string{"/export.json", "/export.csv", "/export.sql"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		env.handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d", path, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "Banana mash") {
			t.Fatalf("expected %s to include solid food", path)
		}
	}

	// Edit bottle
	res = env.do(t, http.MethodPatch, fmt.Sprintf("/bottles/%d", 1), url.Values{
		"milk_type": {"formula"},
		"amount":    {"5"},
	}, true)
	if res.Code != http.StatusOK {
		t.Fatalf("PATCH /bottles/1 = %d: %s", res.Code, res.Body.String())
	}
}
