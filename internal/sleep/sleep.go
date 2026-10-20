// Package sleep holds Nurtur's sleep domain: sessions, wake windows, overlap
// rules, and the local day/night split.
//
// It has no database or HTTP dependencies.
package sleep

import (
	"errors"
	"strings"
	"time"
)

// Domain errors. Handlers map these to friendly copy.
var (
	ErrNotFound       = errors.New("sleep: not found")
	ErrAlreadyActive  = errors.New("sleep: a sleep is already active")
	ErrNoActiveSleep  = errors.New("sleep: no active sleep")
	ErrAlreadyDeleted = errors.New("sleep: already deleted")
	ErrOverlap        = errors.New("sleep: overlaps another sleep")
)

// Session is one sleep, open while EndedAt is nil.
type Session struct {
	ID        int64
	StartedAt time.Time
	EndedAt   *time.Time
	Notes     string
	CreatedAt time.Time
	DeletedAt *time.Time
}

// MaxNotesLength bounds the free-text note.
const MaxNotesLength = 500

// Edit is a correction to a sleep. A nil EndedAt keeps an active sleep open.
type Edit struct {
	StartedAt time.Time
	EndedAt   *time.Time
	Notes     string
}

// Active reports whether the session is still running.
func (s Session) Active() bool { return s.EndedAt == nil && s.DeletedAt == nil }

// EffectiveEnd is the end time, or now for an open session.
func (s Session) EffectiveEnd(now time.Time) time.Time {
	if s.EndedAt != nil {
		return *s.EndedAt
	}
	return now
}

// Duration is the session length at now. Negative values clamp to zero.
func (s Session) Duration(now time.Time) time.Duration {
	d := s.EffectiveEnd(now).Sub(s.StartedAt)
	if d < 0 {
		return 0
	}
	return d
}

// ValidationError is a user-correctable problem with a form field.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

// ValidateTimes checks a start/end pair. requireEnd rejects an open session;
// it is used when editing an already completed sleep.
func ValidateTimes(start time.Time, end *time.Time, now time.Time, requireEnd bool) error {
	if start.IsZero() {
		return &ValidationError{Field: "started_at", Message: "Pick a start time."}
	}
	if start.After(now.Add(time.Minute)) {
		return &ValidationError{Field: "started_at", Message: "That start is in the future. Try again?"}
	}
	if end == nil {
		if requireEnd {
			return &ValidationError{Field: "ended_at", Message: "Pick an end time."}
		}
		return nil
	}
	if !end.After(start) {
		return &ValidationError{Field: "ended_at", Message: "That end time is before the start. Try again?"}
	}
	if end.After(now.Add(time.Minute)) {
		return &ValidationError{Field: "ended_at", Message: "That end time is in the future. Try again?"}
	}
	return nil
}

// ValidateNotes bounds the free-text note.
func ValidateNotes(notes string) error {
	if len([]rune(strings.TrimSpace(notes))) > MaxNotesLength {
		return &ValidationError{Field: "notes", Message: "Keep the note a little shorter."}
	}
	return nil
}

// Overlaps reports whether two sessions share any time. Open sessions are
// treated as running until now.
func Overlaps(aStart time.Time, aEnd *time.Time, bStart time.Time, bEnd *time.Time, now time.Time) bool {
	effectiveA := now
	if aEnd != nil {
		effectiveA = *aEnd
	}
	effectiveB := now
	if bEnd != nil {
		effectiveB = *bEnd
	}
	return aStart.Before(effectiveB) && bStart.Before(effectiveA)
}

// Interval is a half-open time range.
type Interval struct {
	Start time.Time
	End   time.Time
}

// Split is the sleep total for a local day, divided by the night window.
type Split struct {
	Day   time.Duration
	Night time.Duration
	Total time.Duration
}

// SplitRange sums session overlap with [from, to), then divides it into the
// configured night window. from and to are UTC instants that correspond to
// local midnight boundaries.
func SplitRange(sessions []Session, from, to, now time.Time, loc *time.Location, nightStart, nightEnd time.Duration) Split {
	var split Split
	if !to.After(from) {
		return split
	}
	for _, session := range sessions {
		start := session.StartedAt
		if start.Before(from) {
			start = from
		}
		end := session.EffectiveEnd(now)
		if end.After(to) {
			end = to
		}
		if !end.After(start) {
			continue
		}
		split.Total += end.Sub(start)
		for _, night := range NightIntervals(from, to, loc, nightStart, nightEnd) {
			split.Night += overlap(start, end, night.Start, night.End)
		}
	}
	if split.Night > split.Total {
		split.Night = split.Total
	}
	split.Day = split.Total - split.Night
	return split
}

// NightIntervals returns the night portions of one local day in UTC. A night
// window that crosses midnight (19:00–07:00) produces two intervals.
func NightIntervals(from, to time.Time, loc *time.Location, nightStart, nightEnd time.Duration) []Interval {
	if loc == nil {
		loc = time.Local
	}
	if nightStart == nightEnd || !to.After(from) {
		return nil
	}
	local := from.In(loc)
	year, month, day := local.Date()

	at := func(dayOffset int, offset time.Duration) time.Time {
		hour := int(offset / time.Hour)
		minute := int((offset % time.Hour) / time.Minute)
		return time.Date(year, month, day+dayOffset, hour, minute, 0, 0, loc)
	}
	clamp := func(value time.Time) time.Time {
		if value.Before(from) {
			return from
		}
		if value.After(to) {
			return to
		}
		return value
	}

	if nightStart > nightEnd {
		// Night wraps midnight: [00:00, nightEnd] and [nightStart, 24:00].
		return []Interval{
			{Start: from, End: clamp(at(0, nightEnd))},
			{Start: clamp(at(0, nightStart)), End: to},
		}
	}
	return []Interval{
		{Start: clamp(at(0, nightStart)), End: clamp(at(0, nightEnd))},
	}
}

// WakeWindow is the time since the last ended sleep. ok is false when there
// has never been a completed sleep.
func WakeWindow(now time.Time, lastWake *Session) (time.Duration, bool) {
	if lastWake == nil || lastWake.EndedAt == nil {
		return 0, false
	}
	d := now.Sub(*lastWake.EndedAt)
	if d < 0 {
		d = 0
	}
	return d, true
}

func overlap(aStart, aEnd, bStart, bEnd time.Time) time.Duration {
	start := aStart
	if bStart.After(start) {
		start = bStart
	}
	end := aEnd
	if bEnd.Before(end) {
		end = bEnd
	}
	if !end.After(start) {
		return 0
	}
	return end.Sub(start)
}
