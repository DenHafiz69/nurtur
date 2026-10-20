package store

import (
	"context"
	"log/slog"
	"sync"
	"time"

	turso "turso.tech/database/tursogo"
)

// Syncer is the narrow interface the sync worker needs. Domain code never
// touches Turso Cloud directly; tests inject a fake.
type Syncer interface {
	Push(ctx context.Context) error
	Pull(ctx context.Context) (bool, error)
	Checkpoint(ctx context.Context) error
}

// StatsSyncer is implemented by the real Turso adapter.
type StatsSyncer interface {
	Stats(ctx context.Context) (SyncStats, error)
}

// SyncStats is local sync engine telemetry, used by /healthz?verbose=1.
type SyncStats struct {
	PendingOps       int64  `json:"pending_ops"`
	MainWALBytes     int64  `json:"main_wal_bytes"`
	LastPushUnixTime int64  `json:"last_push_unix_time"`
	LastPullUnixTime int64  `json:"last_pull_unix_time"`
	Revision         string `json:"revision,omitempty"`
}

// SyncConfig controls debounce, retry, and periodic maintenance.
type SyncConfig struct {
	Debounce           time.Duration
	PullInterval       time.Duration
	CheckpointInterval time.Duration
	Timeout            time.Duration
	BackoffMin         time.Duration
	BackoffMax         time.Duration
}

// DefaultSyncConfig returns production defaults.
func DefaultSyncConfig(pullInterval time.Duration) SyncConfig {
	if pullInterval <= 0 {
		pullInterval = 60 * time.Second
	}
	return SyncConfig{
		Debounce:           3 * time.Second,
		PullInterval:       pullInterval,
		CheckpointInterval: 5 * time.Minute,
		Timeout:            20 * time.Second,
		BackoffMin:         2 * time.Second,
		BackoffMax:         60 * time.Second,
	}
}

func (c SyncConfig) normalized() SyncConfig {
	d := DefaultSyncConfig(c.PullInterval)
	if c.Debounce > 0 {
		d.Debounce = c.Debounce
	}
	if c.CheckpointInterval > 0 {
		d.CheckpointInterval = c.CheckpointInterval
	}
	if c.Timeout > 0 {
		d.Timeout = c.Timeout
	}
	if c.BackoffMin > 0 {
		d.BackoffMin = c.BackoffMin
	}
	if c.BackoffMax >= d.BackoffMin {
		d.BackoffMax = c.BackoffMax
	}
	return d
}

// SyncStatus is a snapshot for /healthz and the eventual sync chip.
type SyncStatus struct {
	Started           bool       `json:"started"`
	Pushing           bool       `json:"pushing"`
	Dirty             bool       `json:"dirty"`
	PendingSince      *time.Time `json:"pending_since,omitempty"`
	PendingAgeSeconds int64      `json:"pending_age_seconds,omitempty"`
	LastPush          *time.Time `json:"last_push,omitempty"`
	LastPull          *time.Time `json:"last_pull,omitempty"`
	LastPushError     string     `json:"last_push_error,omitempty"`
	LastPullError     string     `json:"last_pull_error,omitempty"`
	LastErrorAt       *time.Time `json:"last_error_at,omitempty"`
	Retries           int        `json:"retries"`
}

// SyncWorker serialises all Turso Cloud traffic. Mutations only call Notify,
// which never blocks, so a cloud outage cannot delay a feed or diaper log.
type SyncWorker struct {
	syncer Syncer
	cfg    SyncConfig
	log    *slog.Logger

	notifyCh chan struct{}
	pullCh   chan struct{}
	stopCh   chan struct{}
	doneCh   chan struct{}

	startOnce sync.Once
	closeOnce sync.Once

	mu            sync.Mutex
	started       bool
	dirty         bool
	pushing       bool
	pendingSince  time.Time
	lastPush      time.Time
	lastPull      time.Time
	lastPushErr   string
	lastPushErrAt time.Time
	lastPullErr   string
	lastPullErrAt time.Time
	retries       int
}

// NewSyncWorker builds a worker around a Syncer.
func NewSyncWorker(syncer Syncer, cfg SyncConfig, logger *slog.Logger) *SyncWorker {
	if logger == nil {
		logger = slog.Default()
	}
	return &SyncWorker{
		syncer:   syncer,
		cfg:      cfg.normalized(),
		log:      logger,
		notifyCh: make(chan struct{}, 1),
		pullCh:   make(chan struct{}, 1),
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
}

// Start launches the worker. It pulls once immediately, then pushes local
// changes after a debounce and pulls on the configured interval.
func (w *SyncWorker) Start(ctx context.Context) {
	w.startOnce.Do(func() {
		w.mu.Lock()
		w.started = true
		w.mu.Unlock()
		go w.run(ctx)
	})
}

// Notify records a local mutation and schedules a debounced push. It is safe to
// call from request handlers and never blocks on the network.
func (w *SyncWorker) Notify() {
	now := time.Now().UTC()
	w.mu.Lock()
	if !w.dirty {
		w.dirty = true
		w.pendingSince = now
	}
	w.mu.Unlock()

	select {
	case w.notifyCh <- struct{}{}:
	default:
	}
}

// PullNow asks the worker to pull remote changes as soon as it is free. It is
// non-blocking and safe to call from HTTP handlers.
func (w *SyncWorker) PullNow() {
	select {
	case w.pullCh <- struct{}{}:
	default:
	}
}

// Close stops the worker, flushes pending changes, and checkpoints the WAL.
func (w *SyncWorker) Close(ctx context.Context) error {
	w.mu.Lock()
	started := w.started
	w.mu.Unlock()
	if !started {
		return nil
	}
	w.closeOnce.Do(func() { close(w.stopCh) })
	select {
	case <-w.doneCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Status returns a consistent snapshot for health reporting.
func (w *SyncWorker) Status() SyncStatus {
	w.mu.Lock()
	defer w.mu.Unlock()
	st := SyncStatus{
		Started:       w.started,
		Pushing:       w.pushing,
		Dirty:         w.dirty,
		LastPushError: w.lastPushErr,
		LastPullError: w.lastPullErr,
		Retries:       w.retries,
	}
	if !w.pendingSince.IsZero() {
		t := w.pendingSince
		st.PendingSince = &t
		st.PendingAgeSeconds = int64(time.Since(w.pendingSince).Seconds())
	}
	if !w.lastPush.IsZero() {
		t := w.lastPush
		st.LastPush = &t
	}
	if !w.lastPull.IsZero() {
		t := w.lastPull
		st.LastPull = &t
	}
	switch {
	case !w.lastPushErrAt.IsZero() && w.lastPushErrAt.After(w.lastPullErrAt):
		t := w.lastPushErrAt
		st.LastErrorAt = &t
	case !w.lastPullErrAt.IsZero():
		t := w.lastPullErrAt
		st.LastErrorAt = &t
	}
	return st
}

// Stats asks the underlying engine for WAL and CDC details. Local-only stores
// have no worker, so this is only reachable in sync mode.
func (w *SyncWorker) Stats(ctx context.Context) (SyncStats, error) {
	stats, ok := w.syncer.(StatsSyncer)
	if !ok {
		return SyncStats{}, nil
	}
	callCtx, cancel := context.WithTimeout(ctx, w.cfg.Timeout)
	defer cancel()
	return stats.Stats(callCtx)
}

func (w *SyncWorker) run(ctx context.Context) {
	defer close(w.doneCh)

	// Startup pull happens before the event loop so remote changes from another
	// device (or a bootstrap) are applied before the first interval.
	w.pull(ctx)

	pullTicker := time.NewTicker(w.cfg.PullInterval)
	defer pullTicker.Stop()
	checkpointTicker := time.NewTicker(w.cfg.CheckpointInterval)
	defer checkpointTicker.Stop()

	var (
		debounceTimer *time.Timer
		debounceC     <-chan time.Time
		retryTimer    *time.Timer
		retryC        <-chan time.Time
		backoff       = w.cfg.BackoffMin
	)

	stopTimer := func(t **time.Timer, c *<-chan time.Time) {
		if *t != nil {
			(*t).Stop()
			*t = nil
		}
		*c = nil
	}

	for {
		select {
		case <-ctx.Done():
			w.flush()
			return
		case <-w.stopCh:
			w.flush()
			return

		case <-w.notifyCh:
			// A new local write supersedes a pending retry wait; try again after
			// the normal debounce.
			stopTimer(&retryTimer, &retryC)
			if debounceTimer == nil {
				debounceTimer = time.NewTimer(w.cfg.Debounce)
				debounceC = debounceTimer.C
			} else {
				if !debounceTimer.Stop() {
					select {
					case <-debounceTimer.C:
					default:
					}
				}
				debounceTimer.Reset(w.cfg.Debounce)
			}

		case <-debounceC:
			debounceTimer = nil
			debounceC = nil
			if !w.isDirty() {
				continue
			}
			if err := w.push(ctx); err != nil {
				backoff = w.nextBackoff(backoff)
				retryTimer = time.NewTimer(backoff)
				retryC = retryTimer.C
			} else {
				backoff = w.cfg.BackoffMin
			}

		case <-retryC:
			retryTimer = nil
			retryC = nil
			if !w.isDirty() {
				continue
			}
			if err := w.push(ctx); err != nil {
				backoff = w.nextBackoff(backoff)
				retryTimer = time.NewTimer(backoff)
				retryC = retryTimer.C
			} else {
				backoff = w.cfg.BackoffMin
			}

		case <-w.pullCh:
			w.pull(ctx)

		case <-pullTicker.C:
			w.pull(ctx)

		case <-checkpointTicker.C:
			w.checkpoint(ctx)
		}
	}
}

func (w *SyncWorker) isDirty() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.dirty
}

func (w *SyncWorker) push(ctx context.Context) error {
	w.mu.Lock()
	w.pushing = true
	w.mu.Unlock()
	defer func() {
		w.mu.Lock()
		w.pushing = false
		w.mu.Unlock()
	}()

	callCtx, cancel := context.WithTimeout(ctx, w.cfg.Timeout)
	defer cancel()
	err := w.syncer.Push(callCtx)

	now := time.Now().UTC()
	w.mu.Lock()
	if err != nil {
		w.lastPushErr = err.Error()
		w.lastPushErrAt = now
		w.retries++
	} else {
		w.lastPush = now
		w.lastPushErr = ""
		w.lastPushErrAt = time.Time{}
		w.dirty = false
		w.pendingSince = time.Time{}
		w.retries = 0
	}
	w.mu.Unlock()

	if err != nil {
		w.log.Warn("sync push failed", "error", err)
		return err
	}
	w.log.Debug("sync push complete")
	return nil
}

func (w *SyncWorker) pull(ctx context.Context) {
	callCtx, cancel := context.WithTimeout(ctx, w.cfg.Timeout)
	defer cancel()
	changed, err := w.syncer.Pull(callCtx)

	now := time.Now().UTC()
	w.mu.Lock()
	if err != nil {
		w.lastPullErr = err.Error()
		w.lastPullErrAt = now
	} else {
		w.lastPull = now
		w.lastPullErr = ""
		w.lastPullErrAt = time.Time{}
	}
	w.mu.Unlock()

	if err != nil {
		w.log.Warn("sync pull failed", "error", err)
		return
	}
	if changed {
		w.log.Info("sync pulled remote changes")
	}
}

func (w *SyncWorker) checkpoint(ctx context.Context) {
	callCtx, cancel := context.WithTimeout(ctx, w.cfg.Timeout)
	defer cancel()
	if err := w.syncer.Checkpoint(callCtx); err != nil {
		w.log.Warn("sync checkpoint failed", "error", err)
		return
	}
	w.log.Debug("sync checkpoint complete")
}

// flush runs during shutdown. It deliberately uses a fresh context: the app
// context is usually already cancelled when Close is called.
func (w *SyncWorker) flush() {
	ctx, cancel := context.WithTimeout(context.Background(), w.cfg.Timeout)
	defer cancel()

	if w.isDirty() {
		if err := w.push(ctx); err != nil {
			w.log.Error("sync flush failed; changes remain local", "error", err)
		}
	}
	if err := w.syncer.Checkpoint(ctx); err != nil {
		w.log.Warn("sync shutdown checkpoint failed", "error", err)
	}
}

func (w *SyncWorker) nextBackoff(current time.Duration) time.Duration {
	next := current * 2
	if next < w.cfg.BackoffMin {
		next = w.cfg.BackoffMin
	}
	if next > w.cfg.BackoffMax {
		next = w.cfg.BackoffMax
	}
	return next
}

// TursoSyncer adapts the Turso sync database to Syncer.
type TursoSyncer struct {
	db *turso.TursoSyncDb
}

// NewTursoSyncer wraps a Turso sync database.
func NewTursoSyncer(db *turso.TursoSyncDb) *TursoSyncer {
	return &TursoSyncer{db: db}
}

// Push sends local changes to Turso Cloud.
func (t *TursoSyncer) Push(ctx context.Context) error { return t.db.Push(ctx) }

// Pull applies remote changes locally.
func (t *TursoSyncer) Pull(ctx context.Context) (bool, error) { return t.db.Pull(ctx) }

// Checkpoint bounds the local WAL.
func (t *TursoSyncer) Checkpoint(ctx context.Context) error { return t.db.Checkpoint(ctx) }

// Stats converts the engine stats into our stable health shape.
func (t *TursoSyncer) Stats(ctx context.Context) (SyncStats, error) {
	raw, err := t.db.Stats(ctx)
	if err != nil {
		return SyncStats{}, err
	}
	return SyncStats{
		PendingOps:       raw.CdcOperations,
		MainWALBytes:     raw.MainWalSize,
		LastPushUnixTime: raw.LastPushUnixTime,
		LastPullUnixTime: raw.LastPullUnixTime,
		Revision:         raw.Revision,
	}, nil
}
