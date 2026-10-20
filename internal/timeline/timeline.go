// Package timeline defines the unified, cross-domain event feed shown on the
// history page. It has no database or HTTP dependencies.
package timeline

import (
	"fmt"
	"time"

	"nurtur/internal/diaper"
	"nurtur/internal/feed"
	"nurtur/internal/sleep"
)

// Kind is one of the three event domains.
type Kind string

const (
	Feed   Kind = "feed"
	Diaper Kind = "diaper"
	Sleep  Kind = "sleep"
)

// Valid reports whether k is a known event kind.
func (k Kind) Valid() bool {
	switch k {
	case Feed, Diaper, Sleep:
		return true
	default:
		return false
	}
}

// ParseKind converts a path segment into a Kind.
func ParseKind(raw string) (Kind, error) {
	k := Kind(raw)
	if !k.Valid() {
		return "", fmt.Errorf("timeline: invalid kind %q", raw)
	}
	return k, nil
}

// Title is the singular noun used in delete and undo copy.
func (k Kind) Title() string {
	switch k {
	case Feed:
		return "Feed"
	case Diaper:
		return "Diaper"
	case Sleep:
		return "Sleep"
	default:
		return "Event"
	}
}

// Entry is one event in the unified feed. Exactly one of Feed, Diaper, or
// Sleep is set; Anchor is the timestamp the row is ordered by.
type Entry struct {
	Kind   Kind
	ID     int64
	Anchor time.Time
	Feed   *feed.Feed
	Diaper *diaper.Diaper
	Sleep  *sleep.Session
}

// DaySummary is the compact count line under a timeline date header.
type DaySummary struct {
	Feeds   int
	Diapers int
	Sleep   time.Duration
}
