package feed

import (
	"errors"
	"testing"
	"time"
)

func TestParseSide(t *testing.T) {
	tests := []struct {
		in      string
		want    Side
		wantErr bool
	}{
		{"left", SideLeft, false},
		{"right", SideRight, false},
		{"", "", true},
		{"LEFT", "", true},
		{"both", "", true},
	}
	for _, tt := range tests {
		got, err := ParseSide(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseSide(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseSide(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestOpposite(t *testing.T) {
	if got := Opposite(SideLeft); got != SideRight {
		t.Fatalf("Opposite(left) = %q", got)
	}
	if got := Opposite(SideRight); got != SideLeft {
		t.Fatalf("Opposite(right) = %q", got)
	}
}

func TestTotalsDeriveFromSegments(t *testing.T) {
	now := time.Date(2026, 10, 20, 15, 0, 0, 0, time.UTC)
	end := now.Add(-3 * time.Minute)
	f := Feed{Segments: []Segment{
		{Side: SideLeft, StartedAt: now.Add(-23 * time.Minute), EndedAt: timePtr(now.Add(-15 * time.Minute))},
		{Side: SideRight, StartedAt: now.Add(-15 * time.Minute), EndedAt: timePtr(end)},
	}}
	left, right := f.Totals(now)
	if left != 8*time.Minute || right != 12*time.Minute {
		t.Fatalf("totals = %v/%v, want 8m/12m", left, right)
	}
	if f.Total(now) != 20*time.Minute {
		t.Fatalf("total = %v, want 20m", f.Total(now))
	}
}

func TestOpenSegmentCountsToNowAndClampsNegative(t *testing.T) {
	now := time.Date(2026, 10, 20, 15, 0, 0, 0, time.UTC)
	f := Feed{Segments: []Segment{
		{Side: SideLeft, StartedAt: now.Add(-90 * time.Second)},
		{Side: SideRight, StartedAt: now.Add(5 * time.Minute)},
	}}
	left, right := f.Totals(now)
	if left != 90*time.Second {
		t.Fatalf("left = %v, want 90s", left)
	}
	if right != 0 {
		t.Fatalf("right = %v, want 0 (future segment)", right)
	}
}

func TestCurrentSegmentAndMostRecentSide(t *testing.T) {
	end := time.Now()
	f := Feed{LastSide: SideLeft, Segments: []Segment{
		{Side: SideLeft, StartedAt: end.Add(-10 * time.Minute), EndedAt: &end},
		{Side: SideRight, StartedAt: end},
	}}
	cur, ok := f.CurrentSegment()
	if !ok || cur.Side != SideRight {
		t.Fatalf("CurrentSegment = %+v, %v", cur, ok)
	}
	if f.MostRecentSide() != SideRight {
		t.Fatalf("MostRecentSide = %q", f.MostRecentSide())
	}
	if !f.Active() {
		t.Fatal("feed with no EndedAt should be active")
	}
}

func TestCompletedEditValidate(t *testing.T) {
	start := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		edit    CompletedEdit
		field   string
		wantErr bool
	}{
		{"valid", CompletedEdit{start, start.Add(20 * time.Minute), 8 * time.Minute, 12 * time.Minute}, "", false},
		{"left only", CompletedEdit{start, start.Add(20 * time.Minute), 8 * time.Minute, 0}, "", false},
		{"right only", CompletedEdit{start, start.Add(20 * time.Minute), 0, 12 * time.Minute}, "", false},
		{"no totals", CompletedEdit{start, start.Add(20 * time.Minute), 0, 0}, "totals", true},
		{"negative", CompletedEdit{start, start.Add(20 * time.Minute), -1 * time.Minute, 12 * time.Minute}, "left_minutes", true},
		{"end before start", CompletedEdit{start, start.Add(-time.Minute), 8 * time.Minute, 0}, "ended_at", true},
		{"end equals start", CompletedEdit{start, start, 8 * time.Minute, 0}, "ended_at", true},
		{"too long", CompletedEdit{start, start.Add(20 * time.Minute), 8 * time.Minute, 14 * time.Minute}, "totals", true},
		{"within tolerance", CompletedEdit{start, start.Add(20 * time.Minute), 10 * time.Minute, 10*time.Minute + 30*time.Second}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.edit.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil {
				return
			}
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("error is not a ValidationError: %v", err)
			}
			if ve.Field != tt.field {
				t.Fatalf("field = %q, want %q", ve.Field, tt.field)
			}
		})
	}
}

func TestBuildSegmentsOrder(t *testing.T) {
	start := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	segs := BuildSegments(7, CompletedEdit{
		StartedAt: start,
		EndedAt:   start.Add(30 * time.Minute),
		Left:      8 * time.Minute,
		Right:     12 * time.Minute,
	})
	if len(segs) != 2 {
		t.Fatalf("len = %d, want 2", len(segs))
	}
	if segs[0].Side != SideLeft || segs[1].Side != SideRight {
		t.Fatalf("order = %q, %q", segs[0].Side, segs[1].Side)
	}
	if !segs[0].StartedAt.Equal(start) || !segs[0].EndedAt.Equal(start.Add(8*time.Minute)) {
		t.Fatalf("left segment times = %v -> %v", segs[0].StartedAt, segs[0].EndedAt)
	}
	if !segs[1].StartedAt.Equal(start.Add(8*time.Minute)) || !segs[1].EndedAt.Equal(start.Add(20*time.Minute)) {
		t.Fatalf("right segment times = %v -> %v", segs[1].StartedAt, segs[1].EndedAt)
	}
	if segs[0].FeedID != 7 || segs[1].FeedID != 7 {
		t.Fatal("feed id not propagated")
	}
}

func TestBuildSegmentsSingleSide(t *testing.T) {
	start := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	segs := BuildSegments(1, CompletedEdit{StartedAt: start, EndedAt: start.Add(10 * time.Minute), Right: 5 * time.Minute})
	if len(segs) != 1 || segs[0].Side != SideRight {
		t.Fatalf("segments = %+v", segs)
	}
	if !segs[0].StartedAt.Equal(start) {
		t.Fatalf("right-only feed should start at the feed start")
	}
}

func TestPausedOnlyWhileActiveWithoutAnOpenSegment(t *testing.T) {
	now := time.Date(2026, 10, 20, 15, 0, 0, 0, time.UTC)
	end := now.Add(-2 * time.Minute)

	paused := Feed{Segments: []Segment{
		{Side: SideLeft, StartedAt: now.Add(-20 * time.Minute), EndedAt: &end},
	}}
	if !paused.Paused() {
		t.Fatal("active feed without an open segment should be paused")
	}
	left, right := paused.Totals(now)
	if left != 18*time.Minute || right != 0 {
		t.Fatalf("totals = %v/%v, want 18m/0m", left, right)
	}

	running := Feed{Segments: []Segment{
		{Side: SideRight, StartedAt: now.Add(-5 * time.Minute)},
	}}
	if running.Paused() {
		t.Fatal("feed with an open segment should not be paused")
	}

	ended := Feed{EndedAt: &now, Segments: paused.Segments}
	if ended.Paused() {
		t.Fatal("completed feed should never report paused")
	}
}
