package server

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestGrowthLifecycleAndExports(t *testing.T) {
	env := newTestEnv(t)

	// Manual sheet lists Growth.
	res := env.do(t, http.MethodGet, "/manual/sheet", nil, true)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "/growth/sheet") {
		t.Fatalf("GET /manual/sheet = %d\n%s", res.Code, res.Body.String())
	}

	// Growth sheet renders metric and imperial.
	res = env.do(t, http.MethodGet, "/growth/sheet", nil, true)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "Weight (kg)") {
		t.Fatalf("GET /growth/sheet = %d", res.Code)
	}
	res = env.do(t, http.MethodGet, "/growth/sheet?unit=imperial", nil, true)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "Weight (lb)") {
		t.Fatalf("GET /growth/sheet?unit=imperial = %d", res.Code)
	}

	// Empty submission returns 422.
	res = env.do(t, http.MethodPost, "/growth", url.Values{
		"unit": {"metric"},
	}, true)
	if res.Code != http.StatusUnprocessableEntity {
		t.Fatalf("empty POST /growth = %d, want 422", res.Code)
	}

	// Create measurement.
	res = env.do(t, http.MethodPost, "/growth", url.Values{
		"unit":   {"metric"},
		"weight": {"4.25"},
		"height": {"54.5"},
		"head":   {"37.2"},
		"notes":  {"6-week checkup"},
	}, true)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "Growth logged.") {
		t.Fatalf("POST /growth = %d\n%s", res.Code, res.Body.String())
	}

	// Timeline shows the measurement.
	res = env.do(t, http.MethodGet, "/timeline", nil, false)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "4.25 kg · 54.5 cm · head 37.2 cm · 6-week checkup") {
		t.Fatalf("GET /timeline = %d\n%s", res.Code, res.Body.String())
	}

	// Edit sheet and update measurement.
	res = env.do(t, http.MethodGet, "/growth/1/sheet", nil, true)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `value="4.25"`) {
		t.Fatalf("GET /growth/1/sheet = %d", res.Code)
	}
	res = env.do(t, http.MethodPatch, "/growth/1", url.Values{
		"unit":   {"metric"},
		"weight": {"4.40"},
		"height": {"55.0"},
		"notes":  {"Updated checkup"},
	}, true)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "Growth updated.") {
		t.Fatalf("PATCH /growth/1 = %d\n%s", res.Code, res.Body.String())
	}

	// Delete & restore via timeline.
	res = env.do(t, http.MethodDelete, "/timeline/growth/1", nil, true)
	if res.Code != http.StatusOK {
		t.Fatalf("DELETE /timeline/growth/1 = %d", res.Code)
	}
	res = env.do(t, http.MethodPost, "/timeline/growth/1/restore", nil, true)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "4.40 kg") {
		t.Fatalf("POST /timeline/growth/1/restore = %d\n%s", res.Code, res.Body.String())
	}

	// Exports include growth.
	for _, path := range []string{"/export.json", "/export.csv", "/export.sql"} {
		rec := env.do(t, http.MethodGet, path, nil, false)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d", path, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "Updated checkup") {
			t.Fatalf("expected %s to include growth measurement", path)
		}
	}
}
