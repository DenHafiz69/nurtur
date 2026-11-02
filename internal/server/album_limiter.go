package server

import (
	"sync"
	"time"
)

const (
	albumUploadWindow = time.Minute
	albumUploadLimit  = 60
)

type albumUploadState struct {
	count int
	reset time.Time
}

// albumUploadLimiter is a small per-account fixed-window limiter for the
// one-photo-per-request upload path. It is intentionally in memory: this app
// runs as a single instance, and a restart should not preserve a throttle.
type albumUploadLimiter struct {
	mu      sync.Mutex
	entries map[int64]*albumUploadState
}

func newAlbumUploadLimiter() *albumUploadLimiter {
	return &albumUploadLimiter{entries: make(map[int64]*albumUploadState)}
}

func (l *albumUploadLimiter) allow(account int64, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry, ok := l.entries[account]
	if !ok || !now.Before(entry.reset) {
		l.entries[account] = &albumUploadState{count: 1, reset: now.Add(albumUploadWindow)}
		l.prune(now)
		return true
	}
	if entry.count >= albumUploadLimit {
		return false
	}
	entry.count++
	return true
}

// prune keeps the map bounded on a long-running server with many accounts.
func (l *albumUploadLimiter) prune(now time.Time) {
	if len(l.entries) <= 1024 {
		return
	}
	for account, entry := range l.entries {
		if !now.Before(entry.reset) {
			delete(l.entries, account)
		}
	}
}
