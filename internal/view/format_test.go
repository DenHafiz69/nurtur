package view

import (
	"testing"
	"time"

	"nurtur/internal/feed"
)

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{-time.Minute, "0m"},
		{0, "0m"},
		{45 * time.Second, "0m"},
		{45 * time.Minute, "45m"},
		{60 * time.Minute, "1h 00m"},
		{65 * time.Minute, "1h 05m"},
		{130 * time.Minute, "2h 10m"},
	}
	for _, tt := range tests {
		if got := FormatDuration(tt.d); got != tt.want {
			t.Errorf("FormatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestFormatTimer(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "00:00"},
		{5*time.Second + 400*time.Millisecond, "00:05"},
		{12*time.Minute + 34*time.Second, "12:34"},
		{time.Hour + 5*time.Minute + 9*time.Second, "1:05:09"},
	}
	for _, tt := range tests {
		if got := FormatTimer(tt.d); got != tt.want {
			t.Errorf("FormatTimer(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestFormatClock(t *testing.T) {
	tests := []struct {
		at   time.Time
		want string
	}{
		{time.Date(2026, 10, 21, 17, 58, 0, 0, time.UTC), "5:58 PM"},
		{time.Date(2026, 10, 21, 9, 7, 0, 0, time.UTC), "9:07 AM"},
		{time.Date(2026, 10, 21, 0, 5, 0, 0, time.UTC), "12:05 AM"},
		{time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC), "12:00 PM"},
	}
	for _, tt := range tests {
		if got := FormatClock(tt.at, time.UTC); got != tt.want {
			t.Errorf("FormatClock(%v) = %q, want %q", tt.at, got, tt.want)
		}
	}

	start := time.Date(2026, 10, 21, 17, 58, 0, 0, time.UTC)
	end := time.Date(2026, 10, 21, 18, 3, 0, 0, time.UTC)
	if got := FormatRange(start, end, time.UTC); got != "5:58 PM–6:03 PM" {
		t.Errorf("FormatRange = %q", got)
	}
}

func TestFormatAgo(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		then time.Time
		want string
	}{
		{now.Add(-10 * time.Second), "just now"},
		{now.Add(-time.Minute), "1m ago"},
		{now.Add(-45 * time.Minute), "45m ago"},
		{now.Add(-105 * time.Minute), "1h 45m ago"},
		{now.Add(-2 * time.Hour), "2h ago"},
		{now.Add(-49 * time.Hour), "2d 1h ago"},
		{now.Add(5 * time.Minute), "just now"},
	}
	for _, tt := range tests {
		if got := FormatAgo(now, tt.then); got != tt.want {
			t.Errorf("FormatAgo(%v) = %q, want %q", tt.then, got, tt.want)
		}
	}
}

func TestSplitLabel(t *testing.T) {
	tests := []struct {
		left, right time.Duration
		fallback    feed.Side
		want        string
	}{
		{8 * time.Minute, 12 * time.Minute, feed.SideLeft, "L 8m · R 12m"},
		{0, 12 * time.Minute, feed.SideRight, "R 12m"},
		{8 * time.Minute, 0, feed.SideLeft, "L 8m"},
		{0, 0, feed.SideRight, "R 0m"},
		{0, 0, feed.SideLeft, "L 0m"},
	}
	for _, tt := range tests {
		if got := SplitLabel(tt.left, tt.right, tt.fallback); got != tt.want {
			t.Errorf("SplitLabel = %q, want %q", got, tt.want)
		}
	}
}

func TestFeedDetail(t *testing.T) {
	tests := []struct {
		left, right time.Duration
		want        string
	}{
		{0, 0, "Under a minute"},
		{45 * time.Second, 0, "Under a minute"},
		{20 * time.Second, 20 * time.Second, "Under a minute"},
		{time.Minute, 0, "L 1m"},
		{8 * time.Minute, 12 * time.Minute, "L 8m · R 12m · 20m"},
	}
	for _, tt := range tests {
		if got := FeedDetail(tt.left, tt.right); got != tt.want {
			t.Errorf("FeedDetail(%v, %v) = %q, want %q", tt.left, tt.right, got, tt.want)
		}
	}
}

func TestGreeting(t *testing.T) {
	tests := []struct {
		hour int
		want string
	}{
		{3, "Still awake?"},
		{8, "Good morning"},
		{14, "Good afternoon"},
		{19, "Good evening"},
		{23, "Good night"},
	}
	for _, tt := range tests {
		if got := Greeting(tt.hour); got != tt.want {
			t.Errorf("Greeting(%d) = %q, want %q", tt.hour, got, tt.want)
		}
	}
}

func TestDayLabelRelative(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, loc)

	today := time.Date(2026, 10, 20, 8, 0, 0, 0, loc)
	if got := DayLabelRelative(today, now, loc); got != "Today" {
		t.Errorf("today = %q", got)
	}
	yesterday := time.Date(2026, 10, 19, 8, 0, 0, 0, loc)
	if got := DayLabelRelative(yesterday, now, loc); got != "Yesterday" {
		t.Errorf("yesterday = %q", got)
	}
	earlier := time.Date(2026, 10, 12, 8, 0, 0, 0, loc)
	if got := DayLabelRelative(earlier, now, loc); got != "Monday, October 12" {
		t.Errorf("earlier = %q", got)
	}
	if got := DateKey(yesterday, loc); got != "2026-10-19" {
		t.Errorf("date key = %q", got)
	}
}

func TestTimelineSummary(t *testing.T) {
	if got := TimelineSummary(0, 0, 0); got != "No events" {
		t.Errorf("empty = %q", got)
	}
	if got := TimelineSummary(2, 1, 3*time.Hour+20*time.Minute); got != "2 feeds · 1 diaper · 3h 20m sleep" {
		t.Errorf("summary = %q", got)
	}
}

func TestFormatBytes(t *testing.T) {
	cases := map[int64]string{
		0:                     "0 KB",
		4 * 1024:              "4 KB",
		2 << 20:               "2.0 MB",
		(3 << 30) + (1 << 29): "3.5 GB",
	}
	for input, want := range cases {
		if got := FormatBytes(input); got != want {
			t.Errorf("FormatBytes(%d) = %q, want %q", input, got, want)
		}
	}
}
