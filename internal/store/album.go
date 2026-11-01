package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"nurtur/internal/album"
)

const albumColumns = `id, account_id, upload_id, taken_at, taken_at_source, COALESCE(caption, ''), width, height, bytes, thumb_bytes, content_type, full_key, thumb_key, created_at, deleted_at`

const (
	defaultAlbumListLimit = 60
	maxAlbumListLimit     = 500
)

// CreateAlbumPhoto inserts one photo after validating it. The account comes
// from ctx, never from the caller. If upload_id already exists in this
// account, the existing row is returned and created is false: that is the
// idempotent retry path after a dropped response.
func (s *Store) CreateAlbumPhoto(ctx context.Context, photo album.Photo) (album.Photo, bool, error) {
	photo.AccountID = AccountID(ctx)
	photo.UploadID = strings.TrimSpace(photo.UploadID)
	photo.Caption = strings.TrimSpace(photo.Caption)
	photo.TakenAt = normalizeTime(photo.TakenAt)
	photo.CreatedAt = normalizeTime(photo.CreatedAt)
	if err := photo.Validate(); err != nil {
		return album.Photo{}, false, err
	}

	res, err := s.db.ExecContext(ctx, `
		INSERT INTO album_photos (
			account_id, upload_id, taken_at, taken_at_source, caption, width, height,
			bytes, thumb_bytes, content_type, full_key, thumb_key, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		photo.AccountID,
		photo.UploadID,
		encodeTime(photo.TakenAt),
		string(photo.TakenAtSource),
		nullableString(photo.Caption),
		photo.Width,
		photo.Height,
		photo.Bytes,
		photo.ThumbBytes,
		photo.ContentType,
		photo.FullKey,
		photo.ThumbKey,
		encodeTime(photo.CreatedAt),
	)
	if err != nil {
		if isUniqueViolation(err) {
			existing, getErr := s.albumPhotoByUploadID(ctx, photo.UploadID)
			if getErr != nil {
				return album.Photo{}, false, getErr
			}
			return existing, false, nil
		}
		return album.Photo{}, false, fmt.Errorf("store: insert album photo: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return album.Photo{}, false, fmt.Errorf("store: album photo id: %w", err)
	}
	photo.ID = id
	s.notifyChange()
	return photo, true, nil
}

// GetAlbumPhoto returns one live photo owned by the current account.
func (s *Store) GetAlbumPhoto(ctx context.Context, id int64) (album.Photo, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+albumColumns+` FROM album_photos
		 WHERE id = ? AND account_id = ? AND deleted_at IS NULL`,
		id, AccountID(ctx))
	photo, err := scanAlbumPhoto(row)
	if errors.Is(err, sql.ErrNoRows) {
		return album.Photo{}, album.ErrNotFound
	}
	if err != nil {
		return album.Photo{}, err
	}
	return photo, nil
}

// ListAlbumPhotos returns live photos newest-first, optionally stopping at a
// day boundary. A zero before value starts from the newest photo. The limit is
// clamped to the per-day ceiling used by the album page.
func (s *Store) ListAlbumPhotos(ctx context.Context, before time.Time, limit int) ([]album.Photo, error) {
	if limit <= 0 || limit > maxAlbumListLimit {
		limit = defaultAlbumListLimit
	}

	query := `SELECT ` + albumColumns + ` FROM album_photos
		WHERE account_id = ? AND deleted_at IS NULL`
	args := []any{AccountID(ctx)}
	if !before.IsZero() {
		query += ` AND taken_at < ?`
		args = append(args, encodeTime(before))
	}
	query += ` ORDER BY taken_at DESC, id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list album photos: %w", err)
	}
	defer rows.Close()

	var photos []album.Photo
	for rows.Next() {
		photo, err := scanAlbumPhoto(rows)
		if err != nil {
			return nil, err
		}
		photos = append(photos, photo)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list album photos: %w", err)
	}
	return photos, nil
}

// HasAlbumPhotosBefore reports whether any live photo is older than t. It
// drives "Load earlier days" without loading an extra page just to check.
func (s *Store) HasAlbumPhotosBefore(ctx context.Context, before time.Time) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx, `
		SELECT CASE WHEN EXISTS (
			SELECT 1 FROM album_photos
			WHERE account_id = ? AND deleted_at IS NULL AND taken_at < ?
		) THEN 1 ELSE 0 END`,
		AccountID(ctx), encodeTime(before),
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("store: album before: %w", err)
	}
	return exists == 1, nil
}

// SoftDeleteAlbumPhoto hides a photo immediately. Objects stay in storage for
// the 30-day sweep so the undo window is free and lossless.
func (s *Store) SoftDeleteAlbumPhoto(ctx context.Context, id int64, now time.Time) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE album_photos SET deleted_at = ?
		WHERE id = ? AND account_id = ? AND deleted_at IS NULL`,
		encodeTime(normalizeTime(now)), id, AccountID(ctx))
	if err != nil {
		return fmt.Errorf("store: soft delete album photo: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: soft delete album photo rows: %w", err)
	}
	if n == 1 {
		s.notifyChange()
		return nil
	}

	var exists int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM album_photos WHERE id = ? AND account_id = ?`,
		id, AccountID(ctx),
	).Scan(&exists); err != nil {
		return fmt.Errorf("store: check album photo: %w", err)
	}
	if exists == 0 {
		return album.ErrNotFound
	}
	return album.ErrAlreadyDeleted
}

// RestoreAlbumPhoto clears deleted_at. restored=false means the photo was
// already visible; the row is still returned so the caller can redraw a tile.
func (s *Store) RestoreAlbumPhoto(ctx context.Context, id int64) (album.Photo, bool, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE album_photos SET deleted_at = NULL
		WHERE id = ? AND account_id = ? AND deleted_at IS NOT NULL`,
		id, AccountID(ctx))
	if err != nil {
		return album.Photo{}, false, fmt.Errorf("store: restore album photo: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return album.Photo{}, false, fmt.Errorf("store: restore album photo rows: %w", err)
	}

	photo, err := s.GetAlbumPhoto(ctx, id)
	if err != nil {
		return album.Photo{}, false, err
	}
	if n == 1 {
		s.notifyChange()
		return photo, true, nil
	}
	return photo, false, nil
}

func (s *Store) albumPhotoByUploadID(ctx context.Context, uploadID string) (album.Photo, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+albumColumns+` FROM album_photos
		 WHERE account_id = ? AND upload_id = ?`,
		AccountID(ctx), uploadID)
	photo, err := scanAlbumPhoto(row)
	if errors.Is(err, sql.ErrNoRows) {
		return album.Photo{}, album.ErrNotFound
	}
	if err != nil {
		return album.Photo{}, err
	}
	return photo, nil
}

func scanAlbumPhoto(s scanner) (album.Photo, error) {
	var (
		photo   album.Photo
		taken   string
		source  string
		caption string
		created string
		deleted sql.NullString
	)
	if err := s.Scan(
		&photo.ID,
		&photo.AccountID,
		&photo.UploadID,
		&taken,
		&source,
		&caption,
		&photo.Width,
		&photo.Height,
		&photo.Bytes,
		&photo.ThumbBytes,
		&photo.ContentType,
		&photo.FullKey,
		&photo.ThumbKey,
		&created,
		&deleted,
	); err != nil {
		return album.Photo{}, err
	}
	var err error
	if photo.TakenAt, err = decodeTime(taken); err != nil {
		return album.Photo{}, err
	}
	if photo.CreatedAt, err = decodeTime(created); err != nil {
		return album.Photo{}, err
	}
	if photo.DeletedAt, err = decodeNullTime(deleted); err != nil {
		return album.Photo{}, err
	}
	photo.TakenAtSource = album.TakenAtSource(source)
	photo.Caption = caption
	return photo, nil
}
