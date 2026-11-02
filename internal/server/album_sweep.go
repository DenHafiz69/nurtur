package server

import (
	"context"
	"errors"
	"strings"
	"time"

	"nurtur/internal/album"
)

const albumRetention = 30 * 24 * time.Hour

// StartAlbumSweep runs the retention sweep at startup and once a day after
// that. Deleted rows keep their objects for the undo window; after that the
// bytes go first, then the metadata row.
func (s *Server) StartAlbumSweep(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	go func() {
		s.runAlbumSweep(ctx)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runAlbumSweep(ctx)
			}
		}
	}()
}

func (s *Server) runAlbumSweep(ctx context.Context) {
	removed, kept, err := s.sweepExpiredAlbums(ctx, s.clock.Now().UTC())
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			s.log.Error("album retention sweep failed", "error", err)
		}
		return
	}
	s.log.Info("album retention sweep",
		"removed", removed,
		"kept", kept,
		"retention_days", int(albumRetention.Hours()/24),
	)
}

func (s *Server) sweepExpiredAlbums(ctx context.Context, now time.Time) (int, int, error) {
	if s.album == nil {
		return 0, 0, nil
	}
	photos, err := s.store.ExpiredAlbumPhotos(ctx, now.Add(-albumRetention))
	if err != nil {
		return 0, 0, err
	}

	removed := 0
	kept := 0
	for _, photo := range photos {
		if err := ctx.Err(); err != nil {
			return removed, kept, err
		}
		failed := false
		keys := albumObjectKeys(photo)
		for _, key := range keys {
			if err := s.album.Remove(ctx, key); err != nil {
				// Never log the object key itself; the photo id is enough to
				// find the row, and the key may name private storage.
				s.log.Warn("album sweep could not remove an object",
					"photo_id", photo.ID,
					"error", redactAlbumError(err, keys...),
				)
				failed = true
			}
		}
		if failed {
			kept++
			continue
		}
		deleted, err := s.store.HardDeleteAlbumPhoto(ctx, photo.ID)
		if err != nil {
			s.log.Warn("album sweep could not remove a row",
				"photo_id", photo.ID,
				"error", err,
			)
			kept++
			continue
		}
		if deleted {
			removed++
		} else {
			// The photo was restored while the sweep was running.
			kept++
		}
	}
	return removed, kept, nil
}

func albumObjectKeys(photo album.Photo) []string {
	if photo.ThumbKey == photo.FullKey {
		return []string{photo.FullKey}
	}
	return []string{photo.FullKey, photo.ThumbKey}
}

// cleanupAlbumObjects is the best-effort path after a failed upload or a
// duplicate retry. It never blocks the response and never logs keys.
func (s *Server) cleanupAlbumObjects(ctx context.Context, storage album.Storage, keys ...string) {
	if storage == nil {
		return
	}
	seen := make(map[string]bool, len(keys))
	for _, key := range keys {
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		if err := storage.Remove(ctx, key); err != nil {
			s.log.Warn("album upload cleanup failed", "error", redactAlbumError(err, keys...))
		}
	}
}

func redactAlbumError(err error, keys ...string) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	for _, key := range keys {
		if key != "" {
			message = strings.ReplaceAll(message, key, "[redacted]")
		}
	}
	return message
}
