package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nurtur/internal/diaper"
	"nurtur/internal/growth"
	"nurtur/internal/intake"
)

func TestInsightsPageAndPartial(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	now := env.clock.Now().UTC()

	if err := env.store.SetSettings(ctx, map[string]string{
		settingBabyName:      "Aisyah",
		settingBabyBirthDate: now.AddDate(0, -3, 0).Format("2006-01-02"),
		settingBabyGender:    babyGenderGirl,
		settingFeedingMode:   "hybrid",
		settingTrackSolids:   "1",
	}); err != nil {
		t.Fatalf("set settings: %v", err)
	}

	if _, err := env.store.CreateGrowth(ctx, growth.Measurement{
		LoggedAt: now.AddDate(0, -1, 0),
		WeightG:  5200,
		HeightMM: 575,
		HeadMM:   386,
	}, now); err != nil {
		t.Fatalf("create growth 1: %v", err)
	}
	if _, err := env.store.CreateGrowth(ctx, growth.Measurement{
		LoggedAt: now.AddDate(0, 0, -2),
		WeightG:  6000,
		HeightMM: 605,
		HeadMM:   401,
	}, now); err != nil {
		t.Fatalf("create growth 2: %v", err)
	}
	if _, _, err := env.store.CreateBottle(ctx, intake.MilkBreast, 140, now.Add(-2*time.Hour), now, "", false); err != nil {
		t.Fatalf("create bottle: %v", err)
	}
	if _, _, err := env.store.CreateDiaper(ctx, diaper.Both, now.Add(-1*time.Hour)); err != nil {
		t.Fatalf("create diaper: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/insights", nil)
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /insights status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Trends",
		"6.00 kg",
		"WHO",
		"20 ml / day",
		"Checkup snapshot",
		`href="/insights"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("GET /insights missing %q", want)
		}
	}

	// Partial swap with 14d range and height metric.
	req2 := httptest.NewRequest(http.MethodGet, "/insights/partial?range=14d&growth=height", nil)
	req2.Header.Set("HX-Request", "true")
	rec2 := httptest.NewRecorder()
	env.handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("GET /insights/partial status = %d, want 200", rec2.Code)
	}
	body2 := rec2.Body.String()
	if !strings.Contains(body2, "Past 14 days") || !strings.Contains(body2, "60.5 cm") {
		t.Errorf("partial missing 14d or height data")
	}
}
