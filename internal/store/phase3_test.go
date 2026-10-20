package store

import (
	"context"
	"testing"
	"time"

	"nurtur/internal/diaper"
	"nurtur/internal/feed"
	"nurtur/internal/timeline"
)

func TestTimelineBetweenOrdersAndExcludesDeleted(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	now := testNow

	f, err := st.CreateFeed(ctx, feed.SideLeft, now.Add(-3*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.StopFeed(ctx, f.ID, now.Add(-3*time.Hour+10*time.Minute)); err != nil {
		t.Fatal(err)
	}

	wet, _, err := st.CreateDiaper(ctx, diaper.Wet, now.Add(-2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CreateDiaper(ctx, diaper.Dirty, now.Add(-90*time.Minute)); err != nil {
		t.Fatal(err)
	}

	session, err := st.StartSleep(ctx, now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.StopSleep(ctx, session.ID, now.Add(-30*time.Minute)); err != nil {
		t.Fatal(err)
	}

	if err := st.DeleteDiaper(ctx, wet.ID, now); err != nil {
		t.Fatal(err)
	}

	entries, err := st.TimelineBetween(ctx, now.Add(-24*time.Hour), now.Add(time.Minute), 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("entries = %d, want 3", len(entries))
	}
	wantKinds := []timeline.Kind{timeline.Sleep, timeline.Diaper, timeline.Feed}
	for i, entry := range entries {
		if entry.Kind != wantKinds[i] {
			t.Errorf("entry %d kind = %s, want %s", i, entry.Kind, wantKinds[i])
		}
	}
	if entries[0].Sleep == nil || entries[0].Sleep.ID != session.ID {
		t.Fatalf("first entry = %+v", entries[0])
	}
	if entries[1].Diaper == nil || entries[1].Diaper.Kind != diaper.Dirty {
		t.Fatalf("second entry = %+v", entries[1])
	}
	if entries[2].Feed == nil || len(entries[2].Feed.Segments) == 0 {
		t.Fatalf("feed entry was not hydrated: %+v", entries[2])
	}

	hasBefore, err := st.HasTimelineBefore(ctx, now.Add(-2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !hasBefore {
		t.Fatal("expected an event before the diaper window")
	}
	hasBefore, err = st.HasTimelineBefore(ctx, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if hasBefore {
		t.Fatal("expected no events before the range")
	}
}

func TestTimelineDaySummaryCountsAndSleepOverlap(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	dayStart := testNow.Add(-12 * time.Hour)
	dayEnd := dayStart.Add(24 * time.Hour)
	now := dayEnd

	// A sleep that starts before the day and overlaps into it.
	previous, err := st.CreateSleep(ctx, dayStart.Add(-2*time.Hour), dayStart.Add(2*time.Hour), "", now)
	if err != nil {
		t.Fatal(err)
	}
	if previous.EndedAt == nil {
		t.Fatal("retroactive sleep should be closed")
	}
	if _, err := st.CreateSleep(ctx, dayStart.Add(4*time.Hour), dayStart.Add(5*time.Hour), "", now); err != nil {
		t.Fatal(err)
	}

	if _, _, err := st.CreateDiaper(ctx, diaper.Wet, dayStart.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CreateDiaper(ctx, diaper.Dirty, dayStart.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	f, err := st.CreateFeed(ctx, feed.SideLeft, dayStart.Add(6*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.StopFeed(ctx, f.ID, dayStart.Add(6*time.Hour+15*time.Minute)); err != nil {
		t.Fatal(err)
	}

	summary, err := st.TimelineDaySummary(ctx, dayStart, dayEnd, now)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Feeds != 1 {
		t.Errorf("feeds = %d, want 1", summary.Feeds)
	}
	if summary.Diapers != 2 {
		t.Errorf("diapers = %d, want 2", summary.Diapers)
	}
	// 2 hours overlap from the first sleep + 1 hour from the second.
	if summary.Sleep != 3*time.Hour {
		t.Errorf("sleep = %s, want 3h", summary.Sleep)
	}
}

func TestRestoreFeedRoundTrip(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	now := testNow

	f, err := st.CreateFeed(ctx, feed.SideRight, now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.StopFeed(ctx, f.ID, now.Add(-50*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteFeed(ctx, f.ID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetFeed(ctx, f.ID); err == nil {
		t.Fatal("deleted feed should not be visible")
	}

	restored, changed, err := st.RestoreFeed(ctx, f.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || restored.DeletedAt != nil {
		t.Fatalf("restore = %+v changed=%v", restored, changed)
	}
	if _, changed, err = st.RestoreFeed(ctx, f.ID); err != nil || changed {
		t.Fatalf("second restore = changed %v err %v", changed, err)
	}

	all, err := st.AllFeeds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].ID != f.ID || len(all[0].Segments) != 1 {
		t.Fatalf("AllFeeds = %+v", all)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	values, err := st.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 0 {
		t.Fatalf("settings = %v, want empty", values)
	}

	if err := st.SetSettings(ctx, map[string]string{"night_start": "20:00", "night_end": "06:00"}); err != nil {
		t.Fatal(err)
	}
	if err := st.SetSettings(ctx, map[string]string{"night_start": "21:30"}); err != nil {
		t.Fatal(err)
	}

	start, ok, err := st.Setting(ctx, "night_start")
	if err != nil || !ok || start != "21:30" {
		t.Fatalf("night_start = %q ok=%v err=%v", start, ok, err)
	}
	end, ok, err := st.Setting(ctx, "night_end")
	if err != nil || !ok || end != "06:00" {
		t.Fatalf("night_end = %q ok=%v err=%v", end, ok, err)
	}
}
