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

const albumColumns = `id, account_id, upload_id, taken_at, taken_at_source, COALESCE(caption, ''), favorite, COALESCE(tag, ''), width, height, bytes, thumb_bytes, content_type, full_key, thumb_key, created_at, deleted_at`

// AlbumFilter narrows album queries to favorites or a single milestone tag.
type AlbumFilter struct {
	FavoritesOnly bool
	Tag           string
}

func albumFilterArgs(filter AlbumFilter) (int, string) {
	fav := 0
	if filter.FavoritesOnly {
		fav = 1
	}
	return fav, strings.TrimSpace(filter.Tag)
}

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
	photo.Tag = strings.TrimSpace(photo.Tag)
	photo.TakenAt = normalizeTime(photo.TakenAt)
	photo.CreatedAt = normalizeTime(photo.CreatedAt)
	if err := photo.Validate(); err != nil {
		return album.Photo{}, false, err
	}
	if err := album.ValidateTag(photo.Tag); err != nil {
		return album.Photo{}, false, err
	}

	favInt := 0
	if photo.Favorite {
		favInt = 1
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO album_photos (
			account_id, upload_id, taken_at, taken_at_source, caption, favorite, tag, width, height,
			bytes, thumb_bytes, content_type, full_key, thumb_key, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		photo.AccountID,
		photo.UploadID,
		encodeTime(photo.TakenAt),
		string(photo.TakenAtSource),
		nullableString(photo.Caption),
		favInt,
		photo.Tag,
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

// AllAlbumPhotos returns every photo for the current account, including
// soft-deleted rows, oldest first. It is used by the logical export.
func (s *Store) AllAlbumPhotos(ctx context.Context) ([]album.Photo, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+albumColumns+` FROM album_photos
		WHERE account_id = ? ORDER BY taken_at ASC, id ASC`,
		AccountID(ctx))
	if err != nil {
		return nil, fmt.Errorf("store: export album photos: %w", err)
	}
	defer rows.Close()

	var out []album.Photo
	for rows.Next() {
		photo, err := scanAlbumPhoto(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, photo)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: export album photos: %w", err)
	}
	return out, nil
}

// ExpiredAlbumPhotos returns soft-deleted photos across every account whose
// undo window has passed. The retention sweep is system-wide, not user-scoped.
func (s *Store) ExpiredAlbumPhotos(ctx context.Context, before time.Time) ([]album.Photo, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+albumColumns+` FROM album_photos
		WHERE deleted_at IS NOT NULL AND deleted_at < ?
		ORDER BY deleted_at ASC, id ASC`,
		encodeTime(before))
	if err != nil {
		return nil, fmt.Errorf("store: expired album photos: %w", err)
	}
	defer rows.Close()

	var out []album.Photo
	for rows.Next() {
		photo, err := scanAlbumPhoto(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, photo)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: expired album photos: %w", err)
	}
	return out, nil
}

// HardDeleteAlbumPhoto removes one already-deleted row after its objects have
// been removed. Restored rows are left alone.
func (s *Store) HardDeleteAlbumPhoto(ctx context.Context, id int64) (bool, error) {
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM album_photos WHERE id = ? AND deleted_at IS NOT NULL`, id)
	if err != nil {
		return false, fmt.Errorf("store: hard delete album photo: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: hard delete album photo rows: %w", err)
	}
	if n == 1 {
		s.notifyChange()
		return true, nil
	}
	return false, nil
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
	return s.ListAlbumPhotosFiltered(ctx, before, AlbumFilter{}, limit)
}

// ListAlbumPhotosFiltered returns live photos matching filter, newest-first.
func (s *Store) ListAlbumPhotosFiltered(ctx context.Context, before time.Time, filter AlbumFilter, limit int) ([]album.Photo, error) {
	if limit <= 0 || limit > maxAlbumListLimit {
		limit = defaultAlbumListLimit
	}

	fav, tag := albumFilterArgs(filter)
	query := `SELECT ` + albumColumns + ` FROM album_photos
		WHERE account_id = ? AND deleted_at IS NULL
		AND (? = 0 OR favorite = 1) AND (? = '' OR tag = ?)`
	args := []any{AccountID(ctx), fav, tag, tag}
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

// ListAlbumPhotosBetween returns live photos in the half-open local-day range
// [from, to), newest first. It is how the album loads one whole day at a time.
func (s *Store) ListAlbumPhotosBetween(ctx context.Context, from, to time.Time, limit int) ([]album.Photo, error) {
	return s.ListAlbumPhotosBetweenFiltered(ctx, from, to, AlbumFilter{}, limit)
}

// ListAlbumPhotosBetweenFiltered returns live photos matching filter in [from, to).
func (s *Store) ListAlbumPhotosBetweenFiltered(ctx context.Context, from, to time.Time, filter AlbumFilter, limit int) ([]album.Photo, error) {
	if !to.After(from) {
		return nil, nil
	}
	if limit <= 0 || limit > maxAlbumListLimit {
		limit = maxAlbumListLimit
	}
	fav, tag := albumFilterArgs(filter)
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+albumColumns+` FROM album_photos
		WHERE account_id = ? AND deleted_at IS NULL AND taken_at >= ? AND taken_at < ?
		AND (? = 0 OR favorite = 1) AND (? = '' OR tag = ?)
		ORDER BY taken_at DESC, id DESC
		LIMIT ?`,
		AccountID(ctx), encodeTime(from), encodeTime(to), fav, tag, tag, limit)
	if err != nil {
		return nil, fmt.Errorf("store: album photos between: %w", err)
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
		return nil, fmt.Errorf("store: album photos between: %w", err)
	}
	return photos, nil
}

// HasAlbumPhotosBefore reports whether any live photo is older than t. It
// drives "Load earlier days" without loading an extra page just to check.
func (s *Store) HasAlbumPhotosBefore(ctx context.Context, before time.Time) (bool, error) {
	return s.HasAlbumPhotosBeforeFiltered(ctx, before, AlbumFilter{})
}

// HasAlbumPhotosBeforeFiltered reports whether any live photo matching filter is older than t.
func (s *Store) HasAlbumPhotosBeforeFiltered(ctx context.Context, before time.Time, filter AlbumFilter) (bool, error) {
	fav, tag := albumFilterArgs(filter)
	var exists int
	err := s.db.QueryRowContext(ctx, `
		SELECT CASE WHEN EXISTS (
			SELECT 1 FROM album_photos
			WHERE account_id = ? AND deleted_at IS NULL AND taken_at < ?
			AND (? = 0 OR favorite = 1) AND (? = '' OR tag = ?)
		) THEN 1 ELSE 0 END`,
		AccountID(ctx), encodeTime(before), fav, tag, tag,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("store: album before: %w", err)
	}
	return exists == 1, nil
}

// CountAlbumPhotos returns the total number of live photos in the current account's album.
func (s *Store) CountAlbumPhotos(ctx context.Context) (int, error) {
	return s.CountAlbumPhotosFiltered(ctx, AlbumFilter{})
}

// CountAlbumPhotosFiltered returns the number of live photos matching filter.
func (s *Store) CountAlbumPhotosFiltered(ctx context.Context, filter AlbumFilter) (int, error) {
	fav, tag := albumFilterArgs(filter)
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(1) FROM album_photos
		WHERE account_id = ? AND deleted_at IS NULL
		AND (? = 0 OR favorite = 1) AND (? = '' OR tag = ?)`,
		AccountID(ctx), fav, tag, tag,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("store: count album photos: %w", err)
	}
	return count, nil
}

// ListAlbumTags returns distinct non-empty milestone tags on live photos in the account.
func (s *Store) ListAlbumTags(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT tag FROM album_photos
		WHERE account_id = ? AND deleted_at IS NULL AND tag != ''
		ORDER BY tag ASC`,
		AccountID(ctx),
	)
	if err != nil {
		return nil, fmt.Errorf("store: list album tags: %w", err)
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, fmt.Errorf("store: scan album tag: %w", err)
		}
		tags = append(tags, tag)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list album tags: %w", err)
	}
	return tags, nil
}

// UpdateAlbumPhoto corrects the capture date and caption. A manual correction
// always records taken_at_source='manual' so the UI can explain where the date
// came from.
func (s *Store) UpdateAlbumPhoto(ctx context.Context, id int64, takenAt time.Time, caption string) (album.Photo, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE album_photos
		SET taken_at = ?, taken_at_source = ?, caption = ?
		WHERE id = ? AND account_id = ? AND deleted_at IS NULL`,
		encodeTime(normalizeTime(takenAt)), string(album.SourceManual), nullableString(strings.TrimSpace(caption)),
		id, AccountID(ctx))
	if err != nil {
		return album.Photo{}, fmt.Errorf("store: update album photo: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return album.Photo{}, fmt.Errorf("store: update album photo rows: %w", err)
	}
	if n == 0 {
		return album.Photo{}, album.ErrNotFound
	}
	photo, err := s.GetAlbumPhoto(ctx, id)
	if err != nil {
		return album.Photo{}, err
	}
	s.notifyChange()
	return photo, nil
}

// UpdateAlbumPhotoDetails updates capture date, caption, milestone tag, and favorite state.
func (s *Store) UpdateAlbumPhotoDetails(ctx context.Context, id int64, takenAt time.Time, caption, tag string, favorite bool) (album.Photo, error) {
	tag = strings.TrimSpace(tag)
	if err := album.ValidateTag(tag); err != nil {
		return album.Photo{}, err
	}
	favInt := 0
	if favorite {
		favInt = 1
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE album_photos
		SET taken_at = ?, taken_at_source = ?, caption = ?, tag = ?, favorite = ?
		WHERE id = ? AND account_id = ? AND deleted_at IS NULL`,
		encodeTime(normalizeTime(takenAt)), string(album.SourceManual),
		nullableString(strings.TrimSpace(caption)), tag, favInt,
		id, AccountID(ctx))
	if err != nil {
		return album.Photo{}, fmt.Errorf("store: update album photo details: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return album.Photo{}, fmt.Errorf("store: update album photo details rows: %w", err)
	}
	if n == 0 {
		return album.Photo{}, album.ErrNotFound
	}
	photo, err := s.GetAlbumPhoto(ctx, id)
	if err != nil {
		return album.Photo{}, err
	}
	s.notifyChange()
	return photo, nil
}

// SetAlbumPhotoFavorite sets the favorite flag on a live photo.
func (s *Store) SetAlbumPhotoFavorite(ctx context.Context, id int64, favorite bool) (album.Photo, error) {
	favInt := 0
	if favorite {
		favInt = 1
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE album_photos
		SET favorite = ?
		WHERE id = ? AND account_id = ? AND deleted_at IS NULL`,
		favInt, id, AccountID(ctx))
	if err != nil {
		return album.Photo{}, fmt.Errorf("store: set album favorite: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return album.Photo{}, fmt.Errorf("store: set album favorite rows: %w", err)
	}
	if n == 0 {
		return album.Photo{}, album.ErrNotFound
	}
	photo, err := s.GetAlbumPhoto(ctx, id)
	if err != nil {
		return album.Photo{}, err
	}
	s.notifyChange()
	return photo, nil
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

// AccountAlbumPhotos returns every album photo (including soft-deleted ones)
// belonging to accountID, so a hard-delete sweep can remove all stored objects.
func (s *Store) AccountAlbumPhotos(ctx context.Context, accountID int64) ([]album.Photo, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+albumColumns+` FROM album_photos WHERE account_id = ? ORDER BY id ASC`,
		accountID)
	if err != nil {
		return nil, fmt.Errorf("store: list account album photos: %w", err)
	}
	defer rows.Close()

	photos := make([]album.Photo, 0, 16)
	for rows.Next() {
		photo, err := scanAlbumPhoto(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan account album photo: %w", err)
		}
		photos = append(photos, photo)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list account album photos: %w", err)
	}
	return photos, nil
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
		fav     int
		tag     string
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
		&fav,
		&tag,
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
	photo.Favorite = fav != 0
	photo.Tag = tag
	return photo, nil
}
