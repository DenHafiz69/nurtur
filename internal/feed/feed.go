// Package feed holds Nurtur's feeding domain: sides, segments, derived
// durations, and the rules for correcting a feed.
//
// It has no database or HTTP dependencies. The store persists these values and
// the server turns them into view models.
package feed

import (
	"errors"
	"fmt"
	"time"
)

// Side is one side of a nursing session.
type Side string

const (
	SideLeft  Side = "left"
	SideRight Side = "right"
)

// WindowTolerance is the grace period allowed when comparing submitted side
// totals with the feed's start/end window. Timers stop within a second or two
// of each other, so a minute of slack avoids false rejections.
const WindowTolerance = 60 * time.Second

// Domain errors. Handlers map these to friendly copy instead of surfacing raw
// error strings.
var (
	ErrAlreadyActive    = errors.New("feed: a feed is already active")
	ErrNoActiveFeed     = errors.New("feed: no active feed")
	ErrSameSide         = errors.New("feed: already nursing that side")
	ErrInvalidSide      = errors.New("feed: invalid side")
	ErrInvalidTimeRange = errors.New("feed: end time is before start time")
	ErrAlreadyDeleted   = errors.New("feed: already deleted")
	ErrNotFound         = errors.New("feed: not found")
	ErrNoSegments       = errors.New("feed: feed has no segments")
	ErrFeedActive       = errors.New("feed: feed is still active")
	ErrFutureStart      = errors.New("feed: start time is in the future")
)

// ValidationError is a user-correctable problem with a form field.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

// ParseSide converts a form value into a Side.
func ParseSide(raw string) (Side, error) {
	switch Side(raw) {
	case SideLeft:
		return SideLeft, nil
	case SideRight:
		return SideRight, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidSide, raw)
	}
}

// Valid reports whether s is a known side.
func (s Side) Valid() bool { return s == SideLeft || s == SideRight }

// Opposite returns the other side. Invalid sides return left as a safe
// fallback; callers validate before using it.
func Opposite(s Side) Side {
	if s == SideLeft {
		return SideRight
	}
	return SideLeft
}

// Title returns the capitalised side label used in copy.
func (s Side) Title() string {
	if s == SideRight {
		return "Right"
	}
	return "Left"
}

// Segment is one uninterrupted stretch on one side.
type Segment struct {
	ID        int64
	FeedID    int64
	Side      Side
	StartedAt time.Time
	EndedAt   *time.Time
}

// Duration is the segment length at now. Open segments count up to now.
// Negative durations (clock skew or bad data) clamp to zero.
func (s Segment) Duration(now time.Time) time.Duration {
	end := now
	if s.EndedAt != nil {
		end = *s.EndedAt
	}
	d := end.Sub(s.StartedAt)
	if d < 0 {
		return 0
	}
	return d
}

// Open reports whether the segment has not been closed.
func (s Segment) Open() bool { return s.EndedAt == nil }

// Feed is a nursing session made of ordered segments.
type Feed struct {
	ID        int64
	StartedAt time.Time
	EndedAt   *time.Time
	LastSide  Side
	Notes     string
	CreatedAt time.Time
	DeletedAt *time.Time
	Segments  []Segment
}

// Active reports whether the feed is still running.
func (f Feed) Active() bool { return f.EndedAt == nil && f.DeletedAt == nil }

// CurrentSegment returns the open segment, if any.
func (f Feed) CurrentSegment() (Segment, bool) {
	for i := len(f.Segments) - 1; i >= 0; i-- {
		if f.Segments[i].Open() {
			return f.Segments[i], true
		}
	}
	return Segment{}, false
}

// MostRecentSide is the side of the last segment, falling back to the stored
// last_side value for defensive rendering of older rows.
func (f Feed) MostRecentSide() Side {
	if n := len(f.Segments); n > 0 {
		return f.Segments[n-1].Side
	}
	if f.LastSide.Valid() {
		return f.LastSide
	}
	return SideLeft
}

// Totals derives time per side from segments. This is the only source of
// truth for durations; totals are never persisted.
func (f Feed) Totals(now time.Time) (left, right time.Duration) {
	for _, seg := range f.Segments {
		d := seg.Duration(now)
		switch seg.Side {
		case SideLeft:
			left += d
		case SideRight:
			right += d
		}
	}
	return left, right
}

// Total is the sum of both sides at now.
func (f Feed) Total(now time.Time) time.Duration {
	left, right := f.Totals(now)
	return left + right
}

// CompletedEdit is a correction to a finished feed. Side totals are rebuilt
// into segments in left -> right order.
type CompletedEdit struct {
	StartedAt time.Time
	EndedAt   time.Time
	Left      time.Duration
	Right     time.Duration
}

// Validate checks the correction against the feed window.
func (e CompletedEdit) Validate() error {
	if e.StartedAt.IsZero() {
		return &ValidationError{Field: "started_at", Message: "Pick a start time."}
	}
	if e.EndedAt.IsZero() {
		return &ValidationError{Field: "ended_at", Message: "Pick an end time."}
	}
	if !e.EndedAt.After(e.StartedAt) {
		return &ValidationError{Field: "ended_at", Message: "That end time is before the start. Try again?"}
	}
	if e.Left < 0 {
		return &ValidationError{Field: "left_minutes", Message: "Left minutes can't be negative."}
	}
	if e.Right < 0 {
		return &ValidationError{Field: "right_minutes", Message: "Right minutes can't be negative."}
	}
	total := e.Left + e.Right
	if total <= 0 {
		return &ValidationError{Field: "totals", Message: "Add at least one minute on a side."}
	}
	if total > e.EndedAt.Sub(e.StartedAt)+WindowTolerance {
		return &ValidationError{Field: "totals", Message: "The side totals are longer than the feed. Check the times?"}
	}
	return nil
}

// BuildSegments rebuilds the segment list for a correction. Segments are
// contiguous from StartedAt, left first then right. Any unused part of the
// window remains as a gap at the end.
func BuildSegments(feedID int64, e CompletedEdit) []Segment {
	segs := make([]Segment, 0, 2)
	cursor := e.StartedAt
	if e.Left > 0 {
		end := cursor.Add(e.Left)
		segs = append(segs, Segment{
			FeedID:    feedID,
			Side:      SideLeft,
			StartedAt: cursor,
			EndedAt:   timePtr(end),
		})
		cursor = end
	}
	if e.Right > 0 {
		end := cursor.Add(e.Right)
		segs = append(segs, Segment{
			FeedID:    feedID,
			Side:      SideRight,
			StartedAt: cursor,
			EndedAt:   timePtr(end),
		})
	}
	return segs
}

// EditSummary describes the side a corrected feed ends on.
func (e CompletedEdit) LastSide() Side {
	if e.Right > 0 {
		return SideRight
	}
	return SideLeft
}

func timePtr(t time.Time) *time.Time { return &t }
