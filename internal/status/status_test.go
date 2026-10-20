package status

import (
	"context"
	"testing"
	"time"

	"nurtur/internal/diaper"
	"nurtur/internal/feed"
	"nurtur/internal/sleep"
)

type fakeReader struct {
	activeFeed  *feed.Feed
	lastFeed    *feed.Feed
	lastWet     *diaper.Diaper
	lastDirty   *diaper.Diaper
	activeSleep *sleep.Session
	lastWake    *sleep.Session
	sleeps      []sleep.Session
}

func (f fakeReader) ActiveFeed(context.Context) (*feed.Feed, error) { return f.activeFeed, nil }
func (f fakeReader) LastFeed(context.Context) (*feed.Feed, error)   { return f.lastFeed, nil }
func (f fakeReader) ActiveSleep(context.Context) (*sleep.Session, error) {
	return f.activeSleep, nil
}
func (f fakeReader) LastEndedSleep(context.Context) (*sleep.Session, error) {
	return f.lastWake, nil
}
func (f fakeReader) LastDiaper(_ context.Context, kind diaper.Kind) (*diaper.Diaper, error) {
	if kind == diaper.Wet {
		return f.lastWet, nil
	}
	return f.lastDirty, nil
}
func (f fakeReader) SleepsBetween(context.Context, time.Time, time.Time, time.Time, int) ([]sleep.Session, error) {
	return f.sleeps, nil
}

func TestBuildEmpty(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	snap, err := Build(context.Background(), fakeReader{}, now, time.UTC, 19*time.Hour, 7*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if snap.ActiveFeed != nil || snap.ActiveSleep != nil || snap.HasWakeWindow {
		t.Fatalf("empty snapshot = %+v", snap)
	}
	if snap.SleepToday.Total != 0 {
		t.Fatalf("sleep total = %v", snap.SleepToday.Total)
	}
}

func TestBuildPartialAndWakeWindow(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	feedEnd := now.Add(-2 * time.Hour)
	wetAt := now.Add(-45 * time.Minute)
	dirtyAt := now.Add(-100 * time.Minute)
	wakeAt := now.Add(-30 * time.Minute)
	sleepStart := now.Add(-25 * time.Minute)
	session := sleep.Session{ID: 4, StartedAt: sleepStart}

	reader := fakeReader{
		lastFeed:  &feed.Feed{ID: 1, StartedAt: feedEnd.Add(-20 * time.Minute), EndedAt: &feedEnd, LastSide: feed.SideRight, Segments: []feed.Segment{{Side: feed.SideRight, StartedAt: feedEnd.Add(-20 * time.Minute), EndedAt: &feedEnd}}},
		lastWet:   &diaper.Diaper{ID: 2, LoggedAt: wetAt, Kind: diaper.Wet},
		lastDirty: &diaper.Diaper{ID: 3, LoggedAt: dirtyAt, Kind: diaper.Dirty},
		lastWake:  &sleep.Session{ID: 5, StartedAt: wakeAt.Add(-time.Hour), EndedAt: &wakeAt},
		sleeps:    []sleep.Session{session},
	}
	snap, err := Build(context.Background(), reader, now, time.UTC, 19*time.Hour, 7*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if snap.LastFeed == nil || snap.LastFeed.ID != 1 {
		t.Fatalf("last feed = %+v", snap.LastFeed)
	}
	if snap.LastWet == nil || snap.LastWet.ID != 2 || snap.LastDirty == nil || snap.LastDirty.ID != 3 {
		t.Fatalf("diapers = %+v %+v", snap.LastWet, snap.LastDirty)
	}
	if !snap.HasWakeWindow || snap.WakeWindow != 30*time.Minute {
		t.Fatalf("wake window = %v, %v", snap.WakeWindow, snap.HasWakeWindow)
	}
	if snap.SleepToday.Total != 25*time.Minute {
		t.Fatalf("sleep total = %v", snap.SleepToday.Total)
	}
}

func TestBuildSleepingUsesActiveDuration(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	start := now.Add(-65 * time.Minute)
	session := sleep.Session{ID: 9, StartedAt: start}
	snap, err := Build(context.Background(), fakeReader{
		activeSleep: &session,
		sleeps:      []sleep.Session{session},
	}, now, time.UTC, 19*time.Hour, 7*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if snap.ActiveSleep == nil || snap.ActiveSleep.Duration(now) != 65*time.Minute {
		t.Fatalf("active sleep = %+v", snap.ActiveSleep)
	}
	if snap.SleepToday.Total != 65*time.Minute {
		t.Fatalf("sleep total = %v", snap.SleepToday.Total)
	}
}
