// Package timeline defines the unified, cross-domain event feed shown on the
// history page. It has no database or HTTP dependencies.
package timeline

import (
	"fmt"
	"time"

	"nurtur/internal/diaper"
	"nurtur/internal/feed"
	"nurtur/internal/intake"
	"nurtur/internal/sleep"
)

// Kind is one of the event domains.
type Kind string

const (
	Feed   Kind = "feed"
	Diaper Kind = "diaper"
	Sleep  Kind = "sleep"
	Bottle Kind = "bottle"
	Pump   Kind = "pump"
	Solid  Kind = "solid"
)

// Valid reports whether k is a known event kind.
func (k Kind) Valid() bool {
	switch k {
	case Feed, Diaper, Sleep, Bottle, Pump, Solid:
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
	case Bottle:
		return "Bottle"
	case Pump:
		return "Pump"
	case Solid:
		return "Solid"
	default:
		return "Event"
	}
}

// Entry is one event in the unified feed. Exactly one domain pointer is set;
// Anchor is the timestamp the row is ordered by.
type Entry struct {
	Kind   Kind
	ID     int64
	Anchor time.Time
	Feed   *feed.Feed
	Diaper *diaper.Diaper
	Sleep  *sleep.Session
	Bottle *intake.Bottle
	Pump   *intake.Pump
	Solid  *intake.Solid
}

// DaySummary is the per-day aggregate shown under a timeline date header.
// FeedLeft and FeedRight sum segments of feeds that started in the day.
type DaySummary struct {
	Feeds     int
	Diapers   int
	Sleep     time.Duration
	FeedLeft  time.Duration
	FeedRight time.Duration
	Bottles   int
	BottleML  int
	Pumps     int
	PumpML    int
	Solids    int
}
