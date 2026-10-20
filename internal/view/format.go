package view

import (
	"fmt"
	"math"
	"strings"
	"time"

	"nurtur/internal/feed"
)

// FormatDuration renders a duration for summaries: 0m, 45m, 2h 10m, 1h 05m.
func FormatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	minutes := int(d / time.Minute)
	hours := minutes / 60
	mins := minutes % 60
	if hours > 0 {
		return fmt.Sprintf("%dh %02dm", hours, mins)
	}
	return fmt.Sprintf("%dm", mins)
}

// FormatTimer renders a running clock: 05:34 or 1:05:09.
func FormatTimer(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := int(d / time.Second)
	hours := total / 3600
	mins := (total % 3600) / 60
	secs := total % 60
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, mins, secs)
	}
	return fmt.Sprintf("%02d:%02d", mins, secs)
}

// FormatAgo renders a relative timestamp: just now, 12m ago, 1h 45m ago.
func FormatAgo(now, then time.Time) string {
	d := now.Sub(then)
	if d < 0 {
		d = 0
	}
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d/time.Minute))
	}
	if d < 24*time.Hour {
		hours := int(d / time.Hour)
		mins := int(d/time.Minute) % 60
		if mins == 0 {
			return fmt.Sprintf("%dh ago", hours)
		}
		return fmt.Sprintf("%dh %02dm ago", hours, mins)
	}
	days := int(d / (24 * time.Hour))
	hours := int(d/time.Hour) % 24
	if hours == 0 {
		return fmt.Sprintf("%dd ago", days)
	}
	return fmt.Sprintf("%dd %dh ago", days, hours)
}

// SplitLabel renders the side breakdown, e.g. "L 8m · R 12m". When both sides
// are zero, fallback names the side that is actually running.
func SplitLabel(left, right time.Duration, fallback feed.Side) string {
	switch {
	case left > 0 && right > 0:
		return fmt.Sprintf("L %s · R %s", FormatDuration(left), FormatDuration(right))
	case right > 0:
		return fmt.Sprintf("R %s", FormatDuration(right))
	case left > 0:
		return fmt.Sprintf("L %s", FormatDuration(left))
	default:
		if fallback == feed.SideRight {
			return "R 0m"
		}
		return "L 0m"
	}
}

// TodaySplitLabel is used on the summary card.
func TodaySplitLabel(left, right time.Duration) string {
	if left <= 0 && right <= 0 {
		return "No nursing yet"
	}
	if left+right < time.Minute {
		return "Under a minute"
	}
	parts := make([]string, 0, 3)
	if left > 0 {
		parts = append(parts, "L "+FormatDuration(left))
	}
	if right > 0 {
		parts = append(parts, "R "+FormatDuration(right))
	}
	parts = append(parts, FormatDuration(left+right)+" total")
	return strings.Join(parts, " · ")
}

// FeedCountLabel is the count line on the summary card.
func FeedCountLabel(n int) string {
	switch n {
	case 0:
		return "No feeds"
	case 1:
		return "1 feed"
	default:
		return fmt.Sprintf("%d feeds", n)
	}
}

// Minutes rounds a duration to whole minutes for edit inputs.
func Minutes(d time.Duration) int {
	if d <= 0 {
		return 0
	}
	return int(math.Round(d.Minutes()))
}

// Greeting is a warm, short hello based on the local hour.
func Greeting(hour int) string {
	switch {
	case hour < 5:
		return "Still awake?"
	case hour < 12:
		return "Good morning"
	case hour < 17:
		return "Good afternoon"
	case hour < 21:
		return "Good evening"
	default:
		return "Good night"
	}
}
