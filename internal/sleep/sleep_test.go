package sleep

import (
	"errors"
	"testing"
	"time"

	"nurtur/internal/clock"
)

var testNow = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

func TestValidateTimes(t *testing.T) {
	tests := []struct {
		name    string
		start   time.Time
		end     *time.Time
		require bool
		want    bool
	}{
		{"open ok", testNow.Add(-time.Hour), nil, false, true},
		{"open requires end", testNow.Add(-time.Hour), nil, true, false},
		{"completed ok", testNow.Add(-2 * time.Hour), ptr(testNow.Add(-time.Hour)), true, true},
		{"end before start", testNow.Add(-time.Hour), ptr(testNow.Add(-2 * time.Hour)), true, false},
		{"future start", testNow.Add(2 * time.Minute), nil, false, false},
		{"future end", testNow.Add(-time.Hour), ptr(testNow.Add(2 * time.Minute)), true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTimes(tt.start, tt.end, testNow, tt.require)
			if (err == nil) != tt.want {
				t.Fatalf("err = %v, want valid = %v", err, tt.want)
			}
		})
	}
}

func TestOverlaps(t *testing.T) {
	start := testNow.Add(-2 * time.Hour)
	end := testNow.Add(-time.Hour)
	if !Overlaps(start, &end, start.Add(30*time.Minute), ptr(start.Add(90*time.Minute)), testNow) {
		t.Fatal("overlapping sessions not detected")
	}
	if Overlaps(start, &end, end, ptr(end.Add(time.Hour)), testNow) {
		t.Fatal("touching sessions should not overlap")
	}
	// An open session runs until now.
	if !Overlaps(start, nil, start.Add(30*time.Minute), ptr(testNow), testNow) {
		t.Fatal("open session overlap not detected")
	}
}

func TestWakeWindow(t *testing.T) {
	ended := testNow.Add(-45 * time.Minute)
	d, ok := WakeWindow(testNow, &Session{StartedAt: ended.Add(-time.Hour), EndedAt: &ended})
	if !ok || d != 45*time.Minute {
		t.Fatalf("wake window = %v, %v", d, ok)
	}
	if _, ok := WakeWindow(testNow, nil); ok {
		t.Fatal("nil last wake should not produce a window")
	}
	if _, ok := WakeWindow(testNow, &Session{StartedAt: ended}); ok {
		t.Fatal("active sleep should not produce a wake window")
	}
}

func TestSplitRangeNightBoundary(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, loc)
	from, to := clock.DayBounds(now, loc)

	yesterday := time.Date(2026, 10, 19, 23, 0, 0, 0, loc)
	thisMorning := time.Date(2026, 10, 20, 6, 30, 0, 0, loc)
	eveningStart := time.Date(2026, 10, 20, 18, 0, 0, 0, loc)
	eveningEnd := time.Date(2026, 10, 20, 20, 30, 0, 0, loc)

	sessions := []Session{
		{StartedAt: yesterday, EndedAt: &thisMorning},
		{StartedAt: eveningStart, EndedAt: &eveningEnd},
	}
	split := SplitRange(sessions, from, to, now, loc, 19*time.Hour, 7*time.Hour)

	// Overnight: 00:00–06:30 = 6h30 night. Evening: 18:00–20:30 with night
	// from 19:00 = 1h day + 1h30 night.
	wantTotal := 9 * time.Hour
	wantNight := 8 * time.Hour
	wantDay := time.Hour
	if split.Total != wantTotal || split.Night != wantNight || split.Day != wantDay {
		t.Fatalf("split = %+v, want total %v night %v day %v", split, wantTotal, wantNight, wantDay)
	}
}

func TestSplitRangeDST(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		moment    time.Time
		wantTotal time.Duration
		wantNight time.Duration
		wantDay   time.Duration
	}{
		{
			// 25-hour day: night = 00:00–07:00 (8h with the repeated hour)
			// plus 19:00–24:00 (5h).
			name:      "fall back",
			moment:    time.Date(2026, 11, 1, 12, 0, 0, 0, loc),
			wantTotal: 25 * time.Hour,
			wantNight: 13 * time.Hour,
			wantDay:   12 * time.Hour,
		},
		{
			// 23-hour day: night = 00:00–07:00 (6h with the skipped hour)
			// plus 19:00–24:00 (5h).
			name:      "spring forward",
			moment:    time.Date(2026, 3, 8, 12, 0, 0, 0, loc),
			wantTotal: 23 * time.Hour,
			wantNight: 11 * time.Hour,
			wantDay:   12 * time.Hour,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			from, to := clock.DayBounds(tt.moment, loc)
			session := Session{StartedAt: from, EndedAt: &to}
			split := SplitRange([]Session{session}, from, to, tt.moment, loc, 19*time.Hour, 7*time.Hour)
			if split.Total != tt.wantTotal || split.Night != tt.wantNight || split.Day != tt.wantDay {
				t.Fatalf("split = %+v, want total %v night %v day %v", split, tt.wantTotal, tt.wantNight, tt.wantDay)
			}
		})
	}
}

func TestNightWindowThatDoesNotWrap(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, loc)
	from, to := clock.DayBounds(now, loc)
	session := Session{StartedAt: from, EndedAt: &to}
	split := SplitRange([]Session{session}, from, to, now, loc, time.Hour, 6*time.Hour)
	if split.Total != 24*time.Hour || split.Night != 5*time.Hour || split.Day != 19*time.Hour {
		t.Fatalf("split = %+v", split)
	}
}

func TestWakeWindowAcrossMidnight(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	woke := time.Date(2026, 10, 19, 23, 15, 0, 0, loc)
	now := time.Date(2026, 10, 20, 1, 0, 0, 0, loc)
	d, ok := WakeWindow(now, &Session{StartedAt: woke.Add(-time.Hour), EndedAt: &woke})
	if !ok || d != 105*time.Minute {
		t.Fatalf("wake window = %v, %v, want 1h45m", d, ok)
	}
}

func TestValidationErrorType(t *testing.T) {
	err := ValidateTimes(time.Time{}, nil, testNow, false)
	var validation *ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %v, want ValidationError", err)
	}
}

func ptr(t time.Time) *time.Time { return &t }
