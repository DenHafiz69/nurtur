package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"nurtur/internal/feed"
)

type fakeSyncer struct {
	mu          sync.Mutex
	pushes      int
	pulls       int
	checkpoints int
	pushTimes   []time.Time
	pullHook    func()
	pushErr     func(attempt int) error
}

func (f *fakeSyncer) Push(ctx context.Context) error {
	f.mu.Lock()
	f.pushes++
	attempt := f.pushes
	f.pushTimes = append(f.pushTimes, time.Now())
	errFn := f.pushErr
	f.mu.Unlock()
	if errFn != nil {
		return errFn(attempt)
	}
	return nil
}

func (f *fakeSyncer) Pull(ctx context.Context) (bool, error) {
	f.mu.Lock()
	f.pulls++
	hook := f.pullHook
	f.mu.Unlock()
	if hook != nil {
		hook()
	}
	return false, nil
}

func (f *fakeSyncer) Checkpoint(ctx context.Context) error {
	f.mu.Lock()
	f.checkpoints++
	f.mu.Unlock()
	return nil
}

func (f *fakeSyncer) counts() (pushes, pulls, checkpoints int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pushes, f.pulls, f.checkpoints
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func startWorker(t *testing.T, syncer *fakeSyncer, cfg SyncConfig) *SyncWorker {
	t.Helper()
	w := NewSyncWorker(syncer, cfg, discardLogger())
	w.Start(context.Background())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = w.Close(ctx)
	})
	return w
}

func TestSyncPullOnStartup(t *testing.T) {
	fake := &fakeSyncer{}
	startWorker(t, fake, SyncConfig{
		Debounce:           time.Hour,
		PullInterval:       time.Hour,
		CheckpointInterval: time.Hour,
		Timeout:            time.Second,
	})
	waitFor(t, "startup pull", func() bool {
		_, pulls, _ := fake.counts()
		return pulls == 1
	})
}

func TestSyncPushIsDebouncedAndCoalesced(t *testing.T) {
	fake := &fakeSyncer{}
	w := startWorker(t, fake, SyncConfig{
		Debounce:           50 * time.Millisecond,
		PullInterval:       time.Hour,
		CheckpointInterval: time.Hour,
		Timeout:            time.Second,
	})
	waitFor(t, "startup pull", func() bool {
		_, pulls, _ := fake.counts()
		return pulls == 1
	})

	w.Notify()
	w.Notify()
	w.Notify()
	waitFor(t, "one push", func() bool {
		pushes, _, _ := fake.counts()
		return pushes == 1
	})
	time.Sleep(150 * time.Millisecond)
	if pushes, _, _ := fake.counts(); pushes != 1 {
		t.Fatalf("pushes = %d, want 1 after coalescing", pushes)
	}
	if st := w.Status(); st.Dirty {
		t.Fatal("worker should be clean after a successful push")
	}
}

func TestSyncRetriesWithBackoffThenSucceeds(t *testing.T) {
	fake := &fakeSyncer{
		pushErr: func(attempt int) error {
			if attempt <= 2 {
				return errors.New("cloud unavailable")
			}
			return nil
		},
	}
	w := startWorker(t, fake, SyncConfig{
		Debounce:           10 * time.Millisecond,
		PullInterval:       time.Hour,
		CheckpointInterval: time.Hour,
		Timeout:            time.Second,
		BackoffMin:         20 * time.Millisecond,
		BackoffMax:         100 * time.Millisecond,
	})
	waitFor(t, "startup pull", func() bool {
		_, pulls, _ := fake.counts()
		return pulls == 1
	})

	w.Notify()
	waitFor(t, "successful push after retries", func() bool {
		pushes, _, _ := fake.counts()
		st := w.Status()
		return pushes >= 3 && !st.Dirty && st.LastPushError == ""
	})

	fake.mu.Lock()
	times := append([]time.Time(nil), fake.pushTimes...)
	fake.mu.Unlock()
	if len(times) < 3 {
		t.Fatalf("push times = %v", times)
	}
	if gap := times[1].Sub(times[0]); gap < 15*time.Millisecond {
		t.Fatalf("first retry gap = %v, want >= ~20ms", gap)
	}
	if gap := times[2].Sub(times[1]); gap < 30*time.Millisecond {
		t.Fatalf("second retry gap = %v, want >= ~40ms", gap)
	}
	if st := w.Status(); st.LastPush == nil {
		t.Fatal("last push time not recorded")
	}
}

func TestSyncPullOnDemand(t *testing.T) {
	fake := &fakeSyncer{}
	w := startWorker(t, fake, SyncConfig{
		Debounce:           time.Hour,
		PullInterval:       time.Hour,
		CheckpointInterval: time.Hour,
		Timeout:            time.Second,
	})
	waitFor(t, "startup pull", func() bool {
		_, pulls, _ := fake.counts()
		return pulls == 1
	})
	w.PullNow()
	waitFor(t, "on-demand pull", func() bool {
		_, pulls, _ := fake.counts()
		return pulls >= 2
	})
}

func TestSyncPullsOnInterval(t *testing.T) {
	fake := &fakeSyncer{}
	startWorker(t, fake, SyncConfig{
		Debounce:           time.Hour,
		PullInterval:       25 * time.Millisecond,
		CheckpointInterval: time.Hour,
		Timeout:            time.Second,
	})
	waitFor(t, "repeated pulls", func() bool {
		_, pulls, _ := fake.counts()
		return pulls >= 3
	})
}

func TestSyncShutdownFlushesAndCheckpoints(t *testing.T) {
	fake := &fakeSyncer{}
	w := NewSyncWorker(fake, SyncConfig{
		Debounce:           time.Hour, // only shutdown can flush
		PullInterval:       time.Hour,
		CheckpointInterval: time.Hour,
		Timeout:            time.Second,
	}, discardLogger())
	w.Start(context.Background())
	waitFor(t, "startup pull", func() bool {
		_, pulls, _ := fake.counts()
		return pulls == 1
	})

	w.Notify()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := w.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}

	pushes, _, checkpoints := fake.counts()
	if pushes != 1 {
		t.Fatalf("pushes = %d, want shutdown flush", pushes)
	}
	if checkpoints < 1 {
		t.Fatalf("checkpoints = %d, want at least one on shutdown", checkpoints)
	}
	if st := w.Status(); st.Dirty {
		t.Fatal("worker should be clean after shutdown flush")
	}
}

func TestNotifyNeverBlocks(t *testing.T) {
	w := NewSyncWorker(&fakeSyncer{}, SyncConfig{}, discardLogger())
	start := time.Now()
	for i := 0; i < 50_000; i++ {
		w.Notify()
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("Notify blocked for %v", elapsed)
	}
}

func TestSyncFailureDoesNotFailMutation(t *testing.T) {
	st := newTestStore(t)
	fake := &fakeSyncer{
		pushErr: func(int) error { return errors.New("cloud unavailable") },
	}
	w := startWorker(t, fake, SyncConfig{
		Debounce:           5 * time.Millisecond,
		PullInterval:       time.Hour,
		CheckpointInterval: time.Hour,
		Timeout:            time.Second,
		BackoffMin:         10 * time.Millisecond,
		BackoffMax:         20 * time.Millisecond,
	})
	st.SetOnChange(w.Notify)

	started := time.Now()
	if _, err := st.CreateFeed(context.Background(), feed.SideLeft, testNow); err != nil {
		t.Fatalf("mutation failed because of sync: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("mutation waited on sync for %v", elapsed)
	}

	waitFor(t, "failed sync attempt", func() bool {
		pushes, _, _ := fake.counts()
		return pushes >= 1 && w.Status().LastPushError != ""
	})
	if st := w.Status(); !st.Dirty {
		t.Fatal("failed push should leave the worker dirty for retry")
	}
}
