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
