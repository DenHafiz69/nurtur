package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"nurtur/internal/diaper"
	"nurtur/internal/sleep"
)

func TestDiaperCreateDedupeAndLastEvents(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	now := testNow

	wet, created, err := st.CreateDiaper(ctx, diaper.Wet, now)
	if err != nil || !created {
		t.Fatalf("CreateDiaper = %+v, %v, %v", wet, created, err)
	}
	duplicate, created, err := st.CreateDiaper(ctx, diaper.Wet, now)
	if err != nil {
		t.Fatal(err)
	}
	if created || duplicate.ID != wet.ID {
		t.Fatalf("duplicate = %+v created=%v, want existing id %d", duplicate, created, wet.ID)
	}

	dirty, created, err := st.CreateDiaper(ctx, diaper.Dirty, now)
	if err != nil || !created {
		t.Fatalf("dirty = %+v, %v, %v", dirty, created, err)
	}
	lastWet, err := st.LastDiaper(ctx, diaper.Wet)
	if err != nil || lastWet == nil || lastWet.ID != wet.ID {
		t.Fatalf("last wet = %+v, %v", lastWet, err)
	}
	lastDirty, err := st.LastDiaper(ctx, diaper.Dirty)
	if err != nil || lastDirty == nil || lastDirty.ID != dirty.ID {
		t.Fatalf("last dirty = %+v, %v", lastDirty, err)
	}

	// A "both" event satisfies both categories and becomes the latest for each.
	both, _, err := st.CreateDiaper(ctx, diaper.Both, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	lastWet, _ = st.LastDiaper(ctx, diaper.Wet)
	lastDirty, _ = st.LastDiaper(ctx, diaper.Dirty)
	if lastWet == nil || lastWet.ID != both.ID || lastDirty == nil || lastDirty.ID != both.ID {
		t.Fatalf("both did not satisfy wet and dirty: wet=%+v dirty=%+v", lastWet, lastDirty)
	}

	rows, err := st.RecentDiapers(ctx, now.Add(-time.Minute), now.Add(time.Minute), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("recent diapers = %d, want 3", len(rows))
	}
}

func TestDiaperUpdateDeleteRestore(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	now := testNow

	d, _, err := st.CreateDiaper(ctx, diaper.Dirty, now)
	if err != nil {
		t.Fatal(err)
	}
	edit := diaper.Edit{
		LoggedAt:    now.Add(-10 * time.Minute),
		Color:       diaper.ColorMustard,
		Consistency: diaper.Seedy,
		Notes:       "A little seedy.",
	}
	updated, err := st.UpdateDiaper(ctx, d.ID, edit, now)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Color != diaper.ColorMustard || updated.Consistency != diaper.Seedy || updated.Notes != "A little seedy." {
		t.Fatalf("updated = %+v", updated)
	}
	if !updated.LoggedAt.Equal(now.Add(-10 * time.Minute)) {
		t.Fatalf("logged at = %v", updated.LoggedAt)
	}

	if err := st.DeleteDiaper(ctx, d.ID, now); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteDiaper(ctx, d.ID, now); !errors.Is(err, diaper.ErrAlreadyDeleted) {
		t.Fatalf("double delete = %v", err)
	}
	last, err := st.LastDiaper(ctx, diaper.Dirty)
	if err != nil || last != nil {
		t.Fatalf("deleted diaper still last: %+v %v", last, err)
	}

	restored, changed, err := st.RestoreDiaper(ctx, d.ID)
	if err != nil || !changed || restored.DeletedAt != nil {
		t.Fatalf("restore = %+v %v %v", restored, changed, err)
	}
	_, changed, err = st.RestoreDiaper(ctx, d.ID)
	if err != nil || changed {
		t.Fatalf("second restore = %v %v", changed, err)
	}
}

func TestSleepStartStopAndWakeWindow(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	now := testNow

	session, err := st.StartSleep(ctx, now)
	if err != nil {
		t.Fatalf("StartSleep: %v", err)
	}
	if !session.Active() {
		t.Fatalf("session not active: %+v", session)
	}
	if _, err := st.StartSleep(ctx, now.Add(time.Minute)); !errors.Is(err, sleep.ErrAlreadyActive) {
		t.Fatalf("double start = %v", err)
	}
	active, err := st.ActiveSleep(ctx)
	if err != nil || active == nil || active.ID != session.ID {
		t.Fatalf("active = %+v, %v", active, err)
	}

	stopped, err := st.StopSleep(ctx, session.ID, now.Add(90*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if stopped.EndedAt == nil || stopped.Duration(now.Add(2*time.Hour)) != 90*time.Minute {
		t.Fatalf("stopped = %+v", stopped)
	}
	if _, err := st.StopSleep(ctx, session.ID, now.Add(2*time.Hour)); !errors.Is(err, sleep.ErrNoActiveSleep) {
		t.Fatalf("double stop = %v", err)
	}

	lastWake, err := st.LastEndedSleep(ctx)
	if err != nil || lastWake == nil || lastWake.ID != session.ID {
		t.Fatalf("last wake = %+v, %v", lastWake, err)
	}
}

func TestSleepOverlapRules(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	now := testNow

	// A completed earlier sleep is fine.
	earlierStart := now.Add(-3 * time.Hour)
	earlierEnd := now.Add(-2 * time.Hour)
	earlier, err := st.CreateSleep(ctx, earlierStart, earlierEnd, "", now)
	if err != nil {
		t.Fatalf("CreateSleep: %v", err)
	}

	active, err := st.StartSleep(ctx, now.Add(-30*time.Minute))
	if err != nil {
		t.Fatalf("StartSleep: %v", err)
	}

	// Overlaps the active session.
	if _, err := st.CreateSleep(ctx, now.Add(-time.Hour), now.Add(-10*time.Minute), "", now); !errors.Is(err, sleep.ErrOverlap) {
		t.Fatalf("overlap with active = %v", err)
	}
	// Overlaps the completed session.
	if _, err := st.CreateSleep(ctx, now.Add(-150*time.Minute), now.Add(-90*time.Minute), "", now); !errors.Is(err, sleep.ErrOverlap) {
		t.Fatalf("overlap with completed = %v", err)
	}
	// Editing the earlier sleep into the active window is rejected.
	end := now.Add(-20 * time.Minute)
	if _, err := st.UpdateSleep(ctx, earlier.ID, sleep.Edit{StartedAt: earlierStart, EndedAt: &end}, now); !errors.Is(err, sleep.ErrOverlap) {
		t.Fatalf("edit overlap = %v", err)
	}

	// A non-overlapping correction is accepted.
	newEnd := now.Add(-150 * time.Minute)
	updated, err := st.UpdateSleep(ctx, earlier.ID, sleep.Edit{StartedAt: earlierStart, EndedAt: &newEnd}, now)
	if err != nil {
		t.Fatalf("valid edit: %v", err)
	}
	if updated.EndedAt == nil || !updated.EndedAt.Equal(newEnd) {
		t.Fatalf("updated = %+v", updated)
	}

	if _, err := st.StopSleep(ctx, active.ID, now); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteSleep(ctx, active.ID, now); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteSleep(ctx, active.ID, now); !errors.Is(err, sleep.ErrAlreadyDeleted) {
		t.Fatalf("double delete = %v", err)
	}
}

func TestConcurrentStartSleepCreatesOneSession(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	const workers = 10

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		success int
		other   []error
	)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := st.StartSleep(ctx, testNow)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				success++
			case errors.Is(err, sleep.ErrAlreadyActive):
			default:
				other = append(other, err)
			}
		}()
	}
	wg.Wait()

	if len(other) > 0 {
		t.Fatalf("unexpected errors: %v", other)
	}
	if success != 1 {
		t.Fatalf("successful starts = %d, want 1", success)
	}
	active, err := st.ActiveSleep(ctx)
	if err != nil || active == nil {
		t.Fatalf("active sleep = %+v, %v", active, err)
	}
}

func TestDeleteActiveSleepThenRestore(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	now := testNow

	session, err := st.StartSleep(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteSleep(ctx, session.ID, now.Add(20*time.Minute)); err != nil {
		t.Fatal(err)
	}
	active, err := st.ActiveSleep(ctx)
	if err != nil || active != nil {
		t.Fatalf("active after delete = %+v, %v", active, err)
	}

	restored, changed, err := st.RestoreSleep(ctx, session.ID, now.Add(25*time.Minute))
	if err != nil || !changed {
		t.Fatalf("restore = %+v %v %v", restored, changed, err)
	}
	if restored.EndedAt == nil {
		t.Fatal("restored active sleep was not closed before deletion")
	}
}

func TestSleepRetroactivePastMidnight(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	now := testNow

	start := time.Date(2026, 10, 19, 22, 0, 0, 0, time.UTC)
	end := time.Date(2026, 10, 20, 6, 30, 0, 0, time.UTC)
	session, err := st.CreateSleep(ctx, start, end, "Overnight", now)
	if err != nil {
		t.Fatal(err)
	}
	if session.Duration(now) != 8*time.Hour+30*time.Minute {
		t.Fatalf("duration = %v", session.Duration(now))
	}
	sessions, err := st.SleepsBetween(ctx, time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC), now, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions overlapping today = %d, want 1", len(sessions))
	}
}
