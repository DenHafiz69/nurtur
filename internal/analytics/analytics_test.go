package analytics

import (
	"strings"
	"testing"
	"time"

	"nurtur/internal/diaper"
	"nurtur/internal/feed"
	"nurtur/internal/growth"
	"nurtur/internal/intake"
	"nurtur/internal/sleep"
)

func TestBuildGrowthWithWHOBands(t *testing.T) {
	loc := time.UTC
	birth := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, loc)

	res := Build(Input{
		Now:          now,
		Loc:          loc,
		RangeDays:    7,
		GrowthMetric: "weight",
		GrowthUnit:   growth.UnitMetric,
		VolumeUnit:   intake.UnitML,
		BirthDate:    birth,
		Gender:       "girl",
		BabyName:     "Aisyah",
		Growth: []growth.Measurement{
			{ID: 1, LoggedAt: time.Date(2026, 2, 1, 9, 0, 0, 0, loc), WeightG: 4200, HeightMM: 537, HeadMM: 365},
			{ID: 2, LoggedAt: time.Date(2026, 4, 1, 9, 0, 0, 0, loc), WeightG: 5800, HeightMM: 598, HeadMM: 395},
		},
	})

	if !res.Growth.HasData {
		t.Fatalf("expected growth data")
	}
	if res.Growth.LatestValue != "5.80 kg" {
		t.Fatalf("got latest %q, want 5.80 kg", res.Growth.LatestValue)
	}
	if !strings.Contains(res.Growth.DeltaLabel, "+1.60 kg") {
		t.Fatalf("unexpected delta %q", res.Growth.DeltaLabel)
	}
	if !res.Growth.Chart.HasWHO {
		t.Fatalf("expected WHO bands to be generated")
	}
	if res.Growth.Chart.BandOuterPoints == "" || res.Growth.Chart.BandInnerPoints == "" || res.Growth.Chart.MedianPoints == "" {
		t.Fatalf("expected non-empty WHO polygons and median line")
	}
	if !strings.Contains(res.Growth.PercentileLabel, "WHO") {
		t.Fatalf("expected WHO percentile label, got %q", res.Growth.PercentileLabel)
	}
}

func TestBuildFeedingSleepDiaperAndSolidsAverages(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 5, 10, 18, 0, 0, 0, loc)
	day1 := time.Date(2026, 5, 9, 10, 0, 0, 0, loc)
	day2 := time.Date(2026, 5, 10, 10, 0, 0, 0, loc)

	feedEnd := day2.Add(20 * time.Minute)
	nightStart := time.Date(2026, 5, 9, 21, 0, 0, 0, loc)
	nightEnd := time.Date(2026, 5, 10, 4, 0, 0, 0, loc)
	napStart := time.Date(2026, 5, 10, 7, 0, 0, 0, loc)
	napEnd := time.Date(2026, 5, 10, 8, 30, 0, 0, loc)

	res := Build(Input{
		Now:          now,
		Loc:          loc,
		RangeDays:    7,
		GrowthMetric: "weight",
		GrowthUnit:   growth.UnitMetric,
		VolumeUnit:   intake.UnitML,
		FeedingMode:  "hybrid",
		TrackSolids:  true,
		NightStart:   19 * time.Hour,
		NightEnd:     7 * time.Hour,
		BabyName:     "Aisyah",
		Bottles: []intake.Bottle{
			{ID: 1, LoggedAt: day1, MilkType: intake.MilkBreast, AmountML: 350},
			{ID: 2, LoggedAt: day2, MilkType: intake.MilkFormula, AmountML: 350},
		},
		Pumps: []intake.Pump{
			{ID: 1, LoggedAt: day2, Side: intake.SideBoth, AmountML: 700, DurationMinutes: 20},
		},
		Feeds: []feed.Feed{
			{
				ID:        1,
				StartedAt: day2,
				EndedAt:   &feedEnd,
				Segments: []feed.Segment{
					{Side: feed.SideLeft, StartedAt: day2, EndedAt: timePtr(day2.Add(12 * time.Minute))},
					{Side: feed.SideRight, StartedAt: day2.Add(12 * time.Minute), EndedAt: &feedEnd},
				},
			},
		},
		Sleeps: []sleep.Session{
			{ID: 1, StartedAt: nightStart, EndedAt: &nightEnd},
			{ID: 2, StartedAt: napStart, EndedAt: &napEnd},
		},
		Diapers: []diaper.Diaper{
			{ID: 1, LoggedAt: day1, Kind: diaper.Wet},
			{ID: 2, LoggedAt: day2, Kind: diaper.Both},
		},
		Solids: []intake.Solid{
			{ID: 1, LoggedAt: day1, Food: "Avocado", Portion: intake.PortionMost, Reaction: intake.ReactionNone},
			{ID: 2, LoggedAt: day2, Food: "Egg", Portion: intake.PortionTaste, Reaction: intake.ReactionMild},
		},
	})

	if !res.Feeding.HasBottles || res.Feeding.AvgMilkPerDay != "100 ml / day" {
		t.Fatalf("unexpected avg milk: %+v", res.Feeding)
	}
	if !res.Feeding.HasBalance || res.Feeding.LeftPct != 60 || res.Feeding.RightPct != 40 {
		t.Fatalf("unexpected L/R balance: %d/%d", res.Feeding.LeftPct, res.Feeding.RightPct)
	}
	if !res.Sleep.HasData || res.Sleep.LongestStretch != "7h 00m" {
		t.Fatalf("unexpected sleep stats: %+v", res.Sleep)
	}
	if res.Sleep.AvgWakeWindow != "3h 00m" {
		t.Fatalf("got avg wake window %q, want 3h 00m", res.Sleep.AvgWakeWindow)
	}
	if !res.Pattern.HasData || len(res.Pattern.Days) != 7 {
		t.Fatalf("expected 7 pattern days, got %d", len(res.Pattern.Days))
	}
	if !res.Diapers.HasData {
		t.Fatalf("expected diaper stats")
	}
	if res.Solids == nil || res.Solids.UniqueFoods != 2 || len(res.Solids.Reactions) != 1 {
		t.Fatalf("unexpected solids stats: %+v", res.Solids)
	}
	if !strings.Contains(res.Snapshot.CopyText, "Aisyah") {
		t.Fatalf("expected baby name in snapshot copy text: %q", res.Snapshot.CopyText)
	}
}

func timePtr(t time.Time) *time.Time { return &t }
