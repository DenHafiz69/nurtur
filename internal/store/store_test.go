package store

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"nurtur/internal/feed"
)

var testNow = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nurtur.db")
	st, err := Open(context.Background(), Options{
		Path:      path,
		LocalOnly: true,
		Logger:    discardLogger(),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	if err := st.Migrate(context.Background(), os.DirFS("../../migrations"), testNow); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return st
}

func TestStartSwitchSwitchStopDerivesTotals(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	now := testNow

	f, err := st.CreateFeed(ctx, feed.SideLeft, now)
	if err != nil {
		t.Fatalf("CreateFeed: %v", err)
	}
	if f.ID == 0 || len(f.Segments) != 1 || !f.Active() {
		t.Fatalf("created feed = %+v", f)
	}

	now = now.Add(8 * time.Minute)
	if _, err := st.SwitchFeed(ctx, f.ID, feed.SideRight, now); err != nil {
		t.Fatalf("switch to right: %v", err)
	}

	now = now.Add(12 * time.Minute)
	if _, err := st.SwitchFeed(ctx, f.ID, feed.SideLeft, now); err != nil {
		t.Fatalf("switch back to left: %v", err)
	}

	now = now.Add(3 * time.Minute)
	f, err = st.StopFeed(ctx, f.ID, now)
	if err != nil {
		t.Fatalf("StopFeed: %v", err)
	}
	if f.EndedAt == nil {
		t.Fatal("feed should be ended")
	}
	if f.LastSide != feed.SideLeft {
		t.Fatalf("last side = %q, want left", f.LastSide)
	}
	if len(f.Segments) != 3 {
		t.Fatalf("segments = %d, want 3", len(f.Segments))
	}
	left, right := f.Totals(now)
	if left != 11*time.Minute || right != 12*time.Minute {
		t.Fatalf("totals = %v/%v, want 11m/12m", left, right)
	}

	// SQL-derived totals must agree with the domain calculation.
	sqlLeft, sqlRight, err := st.TotalsForFeed(ctx, f.ID, now)
	if err != nil {
		t.Fatalf("TotalsForFeed: %v", err)
	}
	if sqlLeft != left || sqlRight != right {
		t.Fatalf("sql totals = %v/%v, want %v/%v", sqlLeft, sqlRight, left, right)
	}

	active, err := st.ActiveFeed(ctx)
	if err != nil {
		t.Fatalf("ActiveFeed: %v", err)
	}
	if active != nil {
		t.Fatalf("active feed = %+v, want nil", active)
	}

	last, err := st.LastFeed(ctx)
	if err != nil {
		t.Fatalf("LastFeed: %v", err)
	}
	if last == nil || last.ID != f.ID || last.MostRecentSide() != feed.SideLeft {
		t.Fatalf("last feed = %+v", last)
	}

	// The stored timestamps are UTC RFC3339 text.
	var raw string
	if err := st.db.QueryRow(`SELECT started_at FROM feeds WHERE id = ?`, f.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if _, err := time.Parse(time.RFC3339, raw); err != nil {
		t.Fatalf("started_at %q is not RFC3339: %v", raw, err)
	}
}

func TestOneActiveFeedAndIdempotentStart(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	if _, err := st.CreateFeed(ctx, feed.SideLeft, testNow); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := st.CreateFeed(ctx, feed.SideRight, testNow.Add(time.Second)); !errors.Is(err, feed.ErrAlreadyActive) {
			t.Fatalf("start %d error = %v, want ErrAlreadyActive", i, err)
		}
	}

	var feeds, segments int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM feeds`).Scan(&feeds); err != nil {
		t.Fatal(err)
	}
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM feed_segments WHERE ended_at IS NULL`).Scan(&segments); err != nil {
		t.Fatal(err)
	}
	if feeds != 1 || segments != 1 {
		t.Fatalf("feeds=%d open segments=%d, want 1/1", feeds, segments)
	}
}

func TestSameSideTapIsNoOp(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	f, err := st.CreateFeed(ctx, feed.SideLeft, testNow)
	if err != nil {
		t.Fatal(err)
	}

	_, err = st.SwitchFeed(ctx, f.ID, feed.SideLeft, testNow.Add(2*time.Minute))
	if !errors.Is(err, feed.ErrSameSide) {
		t.Fatalf("error = %v, want ErrSameSide", err)
	}

	active, err := st.ActiveFeed(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(active.Segments) != 1 {
		t.Fatalf("segments = %d, want 1 after same-side tap", len(active.Segments))
	}
	if !active.Segments[0].StartedAt.Equal(testNow) {
		t.Fatalf("segment start changed to %v", active.Segments[0].StartedAt)
	}
}

func TestStopWhenIdleAndDoubleStop(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	if _, err := st.StopFeed(ctx, 1, testNow); !errors.Is(err, feed.ErrNotFound) {
		t.Fatalf("stop with no feed = %v, want ErrNotFound", err)
	}

	f, err := st.CreateFeed(ctx, feed.SideRight, testNow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.StopFeed(ctx, f.ID, testNow.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := st.StopFeed(ctx, f.ID, testNow.Add(5*time.Minute)); !errors.Is(err, feed.ErrNoActiveFeed) {
		t.Fatalf("second stop = %v, want ErrNoActiveFeed", err)
	}
	if _, err := st.SwitchFeed(ctx, f.ID, feed.SideLeft, testNow.Add(5*time.Minute)); !errors.Is(err, feed.ErrNoActiveFeed) {
		t.Fatalf("switch after stop = %v, want ErrNoActiveFeed", err)
	}
}

func TestDeleteActiveFeedClosesAndHidesIt(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	f, err := st.CreateFeed(ctx, feed.SideLeft, testNow)
	if err != nil {
		t.Fatal(err)
	}

	if err := st.DeleteFeed(ctx, f.ID, testNow.Add(6*time.Minute)); err != nil {
		t.Fatalf("DeleteFeed: %v", err)
	}
	if _, err := st.GetFeed(ctx, f.ID); !errors.Is(err, feed.ErrNotFound) {
		t.Fatalf("GetFeed after delete = %v, want ErrNotFound", err)
	}
	active, err := st.ActiveFeed(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if active != nil {
		t.Fatalf("active = %+v, want nil", active)
	}
	if err := st.DeleteFeed(ctx, f.ID, testNow.Add(7*time.Minute)); !errors.Is(err, feed.ErrAlreadyDeleted) {
		t.Fatalf("second delete = %v, want ErrAlreadyDeleted", err)
	}

	var ended, deleted string
	if err := st.db.QueryRow(`SELECT ended_at, deleted_at FROM feeds WHERE id = ?`, f.ID).Scan(&ended, &deleted); err != nil {
		t.Fatal(err)
	}
	if ended == "" || deleted == "" {
		t.Fatalf("ended=%q deleted=%q, want both set", ended, deleted)
	}

	last, err := st.LastFeed(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if last != nil {
		t.Fatalf("last feed = %+v, want nil for deleted feed", last)
	}
}

func TestUpdateCompletedFeedRebuildsSegments(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	start := testNow
	f, err := st.CreateFeed(ctx, feed.SideLeft, start)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.StopFeed(ctx, f.ID, start.Add(30*time.Minute)); err != nil {
		t.Fatal(err)
	}

	edit := feed.CompletedEdit{
		StartedAt: start.Add(-10 * time.Minute),
		EndedAt:   start.Add(25 * time.Minute),
		Left:      14 * time.Minute,
		Right:     9 * time.Minute,
	}
	updated, err := st.UpdateCompletedFeed(ctx, f.ID, edit)
	if err != nil {
		t.Fatalf("UpdateCompletedFeed: %v", err)
	}
	left, right := updated.Totals(start)
	if left != 14*time.Minute || right != 9*time.Minute {
		t.Fatalf("totals = %v/%v, want 14m/9m", left, right)
	}
	if len(updated.Segments) != 2 {
		t.Fatalf("segments = %d, want 2", len(updated.Segments))
	}
	if updated.Segments[0].Side != feed.SideLeft || updated.Segments[1].Side != feed.SideRight {
		t.Fatalf("wrong segment order")
	}
	if updated.LastSide != feed.SideRight {
		t.Fatalf("last side = %q, want right", updated.LastSide)
	}

	// Reload from disk to prove the rebuild was persisted, not just returned.
	reloaded, err := st.GetFeed(ctx, f.ID)
	if err != nil {
		t.Fatal(err)
	}
	l2, r2 := reloaded.Totals(start)
	if l2 != 14*time.Minute || r2 != 9*time.Minute {
		t.Fatalf("reloaded totals = %v/%v", l2, r2)
	}
}

func TestUpdateCompletedFeedValidation(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	f, err := st.CreateFeed(ctx, feed.SideLeft, testNow)
	if err != nil {
		t.Fatal(err)
	}

	// Active feeds take the quick-adjust path, not rebuild.
	_, err = st.UpdateCompletedFeed(ctx, f.ID, feed.CompletedEdit{
		StartedAt: testNow,
		EndedAt:   testNow.Add(10 * time.Minute),
		Left:      5 * time.Minute,
	})
	if !errors.Is(err, feed.ErrFeedActive) {
		t.Fatalf("error = %v, want ErrFeedActive", err)
	}

	if _, err := st.StopFeed(ctx, f.ID, testNow.Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	_, err = st.UpdateCompletedFeed(ctx, f.ID, feed.CompletedEdit{
		StartedAt: testNow,
		EndedAt:   testNow.Add(5 * time.Minute),
		Left:      10 * time.Minute,
	})
	var ve *feed.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error = %v, want ValidationError", err)
	}
}

func TestAdjustActiveStart(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	f, err := st.CreateFeed(ctx, feed.SideLeft, testNow)
	if err != nil {
		t.Fatal(err)
	}

	now := testNow.Add(10 * time.Minute)
	updated, err := st.AdjustActiveSegmentStart(ctx, f.ID, -5*time.Minute, now)
	if err != nil {
		t.Fatalf("adjust -5: %v", err)
	}
	if !updated.StartedAt.Equal(testNow.Add(-5 * time.Minute)) {
		t.Fatalf("feed start = %v", updated.StartedAt)
	}
	if got := updated.Segments[0].StartedAt; !got.Equal(testNow.Add(-5 * time.Minute)) {
		t.Fatalf("segment start = %v", got)
	}

	// A later adjustment cannot move the start into the future.
	updated, err = st.AdjustActiveSegmentStart(ctx, f.ID, 20*time.Minute, now)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Segments[0].StartedAt.Equal(now) {
		t.Fatalf("clamped start = %v, want %v", updated.Segments[0].StartedAt, now)
	}

	_, err = st.SetActiveSegmentStart(ctx, f.ID, now.Add(time.Minute), now)
	var ve *feed.ValidationError
	if !errors.As(err, &ve) || ve.Field != "segment_started_at" {
		t.Fatalf("future start error = %v", err)
	}
}

func TestAdjustActiveStartRespectsPreviousSegment(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	f, err := st.CreateFeed(ctx, feed.SideLeft, testNow)
	if err != nil {
		t.Fatal(err)
	}
	switchAt := testNow.Add(5 * time.Minute)
	if _, err := st.SwitchFeed(ctx, f.ID, feed.SideRight, switchAt); err != nil {
		t.Fatal(err)
	}
	now := switchAt.Add(10 * time.Minute)

	updated, err := st.AdjustActiveSegmentStart(ctx, f.ID, -20*time.Minute, now)
	if err != nil {
		t.Fatal(err)
	}
	cur, ok := updated.CurrentSegment()
	if !ok {
		t.Fatal("no current segment")
	}
	if cur.StartedAt.Before(switchAt) {
		t.Fatalf("current side start %v moved before previous end %v", cur.StartedAt, switchAt)
	}
}

func TestRecentFeedsLocalDayBoundsAndActive(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	// 23:30 local on Oct 20.
	day1 := time.Date(2026, 10, 20, 23, 30, 0, 0, loc).UTC()
	f1, err := st.CreateFeed(ctx, feed.SideLeft, day1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.StopFeed(ctx, f1.ID, day1.Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	// 00:30 local on Oct 21, active.
	day2 := time.Date(2026, 10, 21, 0, 30, 0, 0, loc).UTC()
	f2, err := st.CreateFeed(ctx, feed.SideRight, day2)
	if err != nil {
		t.Fatal(err)
	}

	from := time.Date(2026, 10, 21, 0, 0, 0, 0, loc).UTC()
	to := time.Date(2026, 10, 22, 0, 0, 0, 0, loc).UTC()
	feeds, err := st.RecentFeeds(ctx, from, to, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(feeds) != 1 || feeds[0].ID != f2.ID {
		t.Fatalf("feeds = %+v, want only day-2 feed", feeds)
	}
	if len(feeds[0].Segments) != 1 {
		t.Fatal("segments not attached")
	}

	// The active feed is included even when its start is outside the window.
	empty, err := st.RecentFeeds(ctx, from.AddDate(0, 0, 5), to.AddDate(0, 0, 6), 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 1 || empty[0].ID != f2.ID {
		t.Fatalf("active feed not included outside window: %+v", empty)
	}
}

func TestMigrationIsIdempotent(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx, os.DirFS("../../migrations"), testNow.Add(time.Hour)); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	entries, err := os.ReadDir("../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	want := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			want++
		}
	}
	var count int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("schema_migrations rows = %d, want %d", count, want)
	}
}

func TestPartialUniqueIndexesExist(t *testing.T) {
	st := newTestStore(t)

	// Create an ended feed so the feed-level index is not involved.
	res, err := st.db.Exec(`INSERT INTO feeds (started_at, ended_at, created_at) VALUES (?, ?, ?)`,
		encodeTime(testNow), encodeTime(testNow.Add(time.Minute)), encodeTime(testNow))
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := st.db.Exec(`INSERT INTO feed_segments (feed_id, side, started_at) VALUES (?, 'left', ?)`,
		id, encodeTime(testNow)); err != nil {
		t.Fatal(err)
	}
	_, err = st.db.Exec(`INSERT INTO feed_segments (feed_id, side, started_at) VALUES (?, 'right', ?)`,
		id, encodeTime(testNow.Add(time.Second)))
	if !isUniqueViolation(err) {
		t.Fatalf("second open segment error = %v, want unique violation", err)
	}
}

func TestConcurrentStartCreatesOneFeed(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	const workers = 16
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		success int
		errs    []error
	)
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := st.CreateFeed(ctx, feed.SideLeft, testNow)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				success++
				return
			}
			errs = append(errs, err)
		}()
	}
	close(start)
	wg.Wait()

	if success != 1 {
		t.Fatalf("successful starts = %d, want 1 (errors: %v)", success, errs)
	}
	for _, err := range errs {
		if !errors.Is(err, feed.ErrAlreadyActive) {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	var feeds, openSegments int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM feeds WHERE ended_at IS NULL AND deleted_at IS NULL`).Scan(&feeds); err != nil {
		t.Fatal(err)
	}
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM feed_segments WHERE ended_at IS NULL`).Scan(&openSegments); err != nil {
		t.Fatal(err)
	}
	if feeds != 1 || openSegments != 1 {
		t.Fatalf("feeds=%d open=%d, want 1/1", feeds, openSegments)
	}
}
