// Package clock provides the single source of wall-clock time for the app.
//
// Domain and view code never call time.Now directly. Tests inject a Fake so
// elapsed times, day boundaries, and timestamps are deterministic.
package clock

import (
	"sync"
	"time"
)

// Clock returns the current time. Implementations must return UTC.
type Clock interface {
	Now() time.Time
}

// DayBounds returns the local day containing now as a UTC half-open interval.
// Building the next midnight with AddDate keeps 23- and 25-hour DST days
// correct.
func DayBounds(now time.Time, loc *time.Location) (time.Time, time.Time) {
	if loc == nil {
		loc = time.Local
	}
	local := now.In(loc)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	return start.UTC(), start.AddDate(0, 0, 1).UTC()
}

// Real is the production clock.
type Real struct{}

// Now returns the current UTC time.
func (Real) Now() time.Time { return time.Now().UTC() }

// Fake is a manually advanced clock for tests.
type Fake struct {
	mu  sync.Mutex
	now time.Time
}

// NewFake returns a fake clock frozen at t.
func NewFake(t time.Time) *Fake {
	return &Fake{now: t.UTC()}
}

// Now returns the current fake time.
func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// Set moves the fake clock to t.
func (f *Fake) Set(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = t.UTC()
}

// Advance moves the fake clock forward by d.
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}
