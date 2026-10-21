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

// FeedDetail renders the secondary line on a completed feed row. Sub-minute
// feeds read as "Under a minute" rather than the redundant "L 0m · 0m". The
// total is only repeated when both sides contributed to it.
func FeedDetail(left, right time.Duration) string {
	if left+right < time.Minute {
		return "Under a minute"
	}
	label := SplitLabel(left, right, feed.SideLeft)
	if left > 0 && right > 0 {
		label += " · " + FormatDuration(left+right)
	}
	return label
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

// DiaperCountLabel is the diaper count in today's summary line.
func DiaperCountLabel(n int) string {
	switch n {
	case 0:
		return "No diapers"
	case 1:
		return "1 diaper"
	default:
		return fmt.Sprintf("%d diapers", n)
	}
}

// SleepTotalLabel is the sleep count in today's summary line.
func SleepTotalLabel(d time.Duration) string {
	if d <= 0 {
		return "No sleep"
	}
	if d < time.Minute {
		return "Under a minute sleep"
	}
	return FormatDuration(d) + " sleep"
}

// FormatClock renders a timestamp in the configured timezone using a 12-hour
// clock with an AM/PM suffix.
func FormatClock(t time.Time, loc *time.Location) string {
	if loc == nil {
		loc = time.Local
	}
	return t.In(loc).Format("3:04 PM")
}

// FormatRange renders a local start–end pair.
func FormatRange(start, end time.Time, loc *time.Location) string {
	return FormatClock(start, loc) + "–" + FormatClock(end, loc)
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

// DayLabelRelative names a local day: Today, Yesterday, or a written date.
func DayLabelRelative(day, now time.Time, loc *time.Location) string {
	if loc == nil {
		loc = time.Local
	}
	local := day.In(loc)
	nowLocal := now.In(loc)
	key := local.Format("2006-01-02")
	todayKey := nowLocal.Format("2006-01-02")
	yesterdayKey := nowLocal.AddDate(0, 0, -1).Format("2006-01-02")
	switch key {
	case todayKey:
		return "Today"
	case yesterdayKey:
		return "Yesterday"
	}
	if local.Year() == nowLocal.Year() {
		return local.Format("Monday, January 2")
	}
	return local.Format("Monday, January 2, 2006")
}

// DateKey is a stable DOM id fragment for a local date.
func DateKey(day time.Time, loc *time.Location) string {
	if loc == nil {
		loc = time.Local
	}
	return day.In(loc).Format("2006-01-02")
}

// TimelineSummary is the count line under a history date header.
func TimelineSummary(feeds, diapers int, sleep time.Duration) string {
	parts := make([]string, 0, 3)
	if feeds > 0 {
		parts = append(parts, FeedCountLabel(feeds))
	}
	if diapers > 0 {
		parts = append(parts, DiaperCountLabel(diapers))
	}
	if sleep >= time.Minute {
		parts = append(parts, FormatDuration(sleep)+" sleep")
	}
	if len(parts) == 0 {
		return "No events"
	}
	return strings.Join(parts, " · ")
}

// FormatBytes renders a compact file size.
func FormatBytes(n int64) string {
	switch {
	case n <= 0:
		return "0 KB"
	case n < 1<<20:
		return fmt.Sprintf("%d KB", (n+1023)/1024)
	case n < 1<<30:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	default:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	}
}
