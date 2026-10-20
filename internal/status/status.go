// Package status assembles the at-a-glance dashboard snapshot: the active or
// last event in each domain, the wake window, and today's sleep split.
package status

import (
	"context"
	"fmt"
	"time"

	"nurtur/internal/clock"
	"nurtur/internal/diaper"
	"nurtur/internal/feed"
	"nurtur/internal/sleep"
)

// Reader is the slice of the store the dashboard needs.
type Reader interface {
	ActiveFeed(ctx context.Context) (*feed.Feed, error)
	LastFeed(ctx context.Context) (*feed.Feed, error)
	LastDiaper(ctx context.Context, kind diaper.Kind) (*diaper.Diaper, error)
	ActiveSleep(ctx context.Context) (*sleep.Session, error)
	LastEndedSleep(ctx context.Context) (*sleep.Session, error)
	SleepsBetween(ctx context.Context, from, to, now time.Time, limit int) ([]sleep.Session, error)
}

// Snapshot is the single view model behind the three status cards.
type Snapshot struct {
	Now      time.Time
	DayStart time.Time
	DayEnd   time.Time

	ActiveFeed *feed.Feed
	LastFeed   *feed.Feed

	LastWet   *diaper.Diaper
	LastDirty *diaper.Diaper

	ActiveSleep   *sleep.Session
	LastWake      *sleep.Session
	WakeWindow    time.Duration
	HasWakeWindow bool
	SleepToday    sleep.Split
	SleepsToday   []sleep.Session
}

// Build loads and assembles the dashboard snapshot. The night window is a
// duration since local midnight; nightStart may be later than nightEnd.
func Build(
	ctx context.Context,
	reader Reader,
	now time.Time,
	loc *time.Location,
	nightStart, nightEnd time.Duration,
) (Snapshot, error) {
	now = now.UTC()
	dayStart, dayEnd := clock.DayBounds(now, loc)
	snap := Snapshot{Now: now, DayStart: dayStart, DayEnd: dayEnd}

	activeFeed, err := reader.ActiveFeed(ctx)
	if err != nil {
		return Snapshot{}, fmt.Errorf("status: active feed: %w", err)
	}
	lastFeed, err := reader.LastFeed(ctx)
	if err != nil {
		return Snapshot{}, fmt.Errorf("status: last feed: %w", err)
	}
	lastWet, err := reader.LastDiaper(ctx, diaper.Wet)
	if err != nil {
		return Snapshot{}, fmt.Errorf("status: last wet diaper: %w", err)
	}
	lastDirty, err := reader.LastDiaper(ctx, diaper.Dirty)
	if err != nil {
		return Snapshot{}, fmt.Errorf("status: last dirty diaper: %w", err)
	}
	activeSleep, err := reader.ActiveSleep(ctx)
	if err != nil {
		return Snapshot{}, fmt.Errorf("status: active sleep: %w", err)
	}
	lastWake, err := reader.LastEndedSleep(ctx)
	if err != nil {
		return Snapshot{}, fmt.Errorf("status: last wake: %w", err)
	}
	sleeps, err := reader.SleepsBetween(ctx, dayStart, dayEnd, now, 500)
	if err != nil {
		return Snapshot{}, fmt.Errorf("status: today's sleeps: %w", err)
	}

	snap.ActiveFeed = activeFeed
	snap.LastFeed = lastFeed
	snap.LastWet = lastWet
	snap.LastDirty = lastDirty
	snap.ActiveSleep = activeSleep
	snap.LastWake = lastWake
	snap.SleepsToday = sleeps
	snap.SleepToday = sleep.SplitRange(sleeps, dayStart, dayEnd, now, loc, nightStart, nightEnd)
	snap.WakeWindow, snap.HasWakeWindow = sleep.WakeWindow(now, lastWake)
	return snap, nil
}
