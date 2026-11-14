package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"nurtur/internal/album"
)

func testAlbumPhoto(n int) album.Photo {
	upload := fmt.Sprintf("%032x", n)
	return album.Photo{
		UploadID:      upload,
		TakenAt:       testNow.Add(-time.Duration(n) * time.Hour),
		TakenAtSource: album.SourceEXIF,
		Caption:       fmt.Sprintf("photo %d", n),
		Width:         2560,
		Height:        1920,
		Bytes:         600_000,
		ThumbBytes:    32_000,
		ContentType:   "image/webp",
		FullKey:       "accounts/1/album/" + upload + "/full.webp",
		ThumbKey:      "accounts/1/album/" + upload + "/thumb.webp",
		CreatedAt:     testNow,
	}
}

func TestAlbumPhotoInsertListAndPaging(t *testing.T) {
	st := newTestStore(t)
	ctx := WithAccount(context.Background(), 1)

	ids := make([]int64, 0, 5)
	for i := 1; i <= 5; i++ {
		photo, created, err := st.CreateAlbumPhoto(ctx, testAlbumPhoto(i))
		if err != nil {
			t.Fatalf("CreateAlbumPhoto %d: %v", i, err)
		}
		if !created || photo.ID == 0 {
			t.Fatalf("photo %d = %+v, created=%v", i, photo, created)
		}
		ids = append(ids, photo.ID)
	}

	page, err := st.ListAlbumPhotos(ctx, time.Time{}, 2)
	if err != nil {
		t.Fatalf("ListAlbumPhotos: %v", err)
	}
	if len(page) != 2 || page[0].ID != ids[0] || page[1].ID != ids[1] {
		t.Fatalf("first page = %+v", page)
	}

	more, err := st.ListAlbumPhotos(ctx, page[len(page)-1].TakenAt, 2)
	if err != nil {
		t.Fatalf("ListAlbumPhotos page 2: %v", err)
	}
	if len(more) != 2 || more[0].ID != ids[2] || more[1].ID != ids[3] {
		t.Fatalf("second page = %+v", more)
	}

	has, err := st.HasAlbumPhotosBefore(ctx, more[len(more)-1].TakenAt)
	if err != nil {
		t.Fatalf("HasAlbumPhotosBefore: %v", err)
	}
	if !has {
		t.Fatal("HasAlbumPhotosBefore = false with one photo left")
	}
	last, err := st.ListAlbumPhotos(ctx, more[len(more)-1].TakenAt, 2)
	if err != nil {
		t.Fatalf("ListAlbumPhotos last page: %v", err)
	}
	if len(last) != 1 || last[0].ID != ids[4] {
		t.Fatalf("last page = %+v", last)
	}
	has, err = st.HasAlbumPhotosBefore(ctx, last[0].TakenAt)
	if err != nil {
		t.Fatalf("HasAlbumPhotosBefore last: %v", err)
	}
	if has {
		t.Fatal("HasAlbumPhotosBefore = true after the oldest photo")
	}
}

func TestAlbumPhotoInsertIsIdempotent(t *testing.T) {
	st := newTestStore(t)
	ctx := WithAccount(context.Background(), 1)
	first := testAlbumPhoto(1)

	createdPhoto, created, err := st.CreateAlbumPhoto(ctx, first)
	if err != nil || !created {
		t.Fatalf("first insert = %+v, created=%v, err=%v", createdPhoto, created, err)
	}

	retried := testAlbumPhoto(1)
	retried.Width = 100 // a retry may arrive with corrected metadata
	samePhoto, created, err := st.CreateAlbumPhoto(ctx, retried)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if created {
		t.Fatal("retry created a second row")
	}
	if samePhoto.ID != createdPhoto.ID {
		t.Fatalf("retry id = %d, want %d", samePhoto.ID, createdPhoto.ID)
	}
}

func TestAlbumPhotoUpdateDateAndCaption(t *testing.T) {
	st := newTestStore(t)
	ctx := WithAccount(context.Background(), 1)

	photo, _, err := st.CreateAlbumPhoto(ctx, testAlbumPhoto(1))
	if err != nil {
		t.Fatalf("CreateAlbumPhoto: %v", err)
	}
	changed := testNow.AddDate(0, 0, -3)
	updated, err := st.UpdateAlbumPhoto(ctx, photo.ID, changed, "  A calmer caption  ")
	if err != nil {
		t.Fatalf("UpdateAlbumPhoto: %v", err)
	}
	if !updated.TakenAt.Equal(changed) {
		t.Fatalf("taken at = %v, want %v", updated.TakenAt, changed)
	}
	if updated.TakenAtSource != album.SourceManual {
		t.Fatalf("source = %q, want manual", updated.TakenAtSource)
	}
	if updated.Caption != "A calmer caption" {
		t.Fatalf("caption = %q", updated.Caption)
	}

	other := WithAccount(context.Background(), 2)
	if _, err := st.UpdateAlbumPhoto(other, photo.ID, changed, ""); !errors.Is(err, album.ErrNotFound) {
		t.Fatalf("foreign update error = %v, want ErrNotFound", err)
	}
}

func TestAlbumPhotoExportExpiryAndHardDelete(t *testing.T) {
	st := newTestStore(t)
	ctx := WithAccount(context.Background(), 1)
	live, _, err := st.CreateAlbumPhoto(ctx, testAlbumPhoto(1))
	if err != nil {
		t.Fatal(err)
	}
	old, _, err := st.CreateAlbumPhoto(ctx, testAlbumPhoto(2))
	if err != nil {
		t.Fatal(err)
	}
	recent, _, err := st.CreateAlbumPhoto(ctx, testAlbumPhoto(3))
	if err != nil {
		t.Fatal(err)
	}

	all, err := st.AllAlbumPhotos(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 || all[len(all)-1].ID != live.ID {
		t.Fatalf("AllAlbumPhotos = %+v", all)
	}

	if err := st.SoftDeleteAlbumPhoto(ctx, old.ID, testNow.AddDate(0, 0, -40)); err != nil {
		t.Fatal(err)
	}
	if err := st.SoftDeleteAlbumPhoto(ctx, recent.ID, testNow.AddDate(0, 0, -5)); err != nil {
		t.Fatal(err)
	}
	expired, err := st.ExpiredAlbumPhotos(ctx, testNow.AddDate(0, 0, -30))
	if err != nil {
		t.Fatal(err)
	}
	if len(expired) != 1 || expired[0].ID != old.ID {
		t.Fatalf("ExpiredAlbumPhotos = %+v", expired)
	}

	if deleted, err := st.HardDeleteAlbumPhoto(ctx, old.ID); err != nil || !deleted {
		t.Fatalf("hard delete = %v, %v", deleted, err)
	}
	if _, err := st.GetAlbumPhoto(ctx, old.ID); !errors.Is(err, album.ErrNotFound) {
		t.Fatalf("hard-deleted Get = %v", err)
	}
	if deleted, err := st.HardDeleteAlbumPhoto(ctx, live.ID); err != nil || deleted {
		t.Fatalf("live hard delete = %v, %v", deleted, err)
	}
}

func TestAlbumPhotoSoftDeleteAndRestore(t *testing.T) {
	st := newTestStore(t)
	ctx := WithAccount(context.Background(), 1)

	photo, _, err := st.CreateAlbumPhoto(ctx, testAlbumPhoto(1))
	if err != nil {
		t.Fatalf("CreateAlbumPhoto: %v", err)
	}
	if err := st.SoftDeleteAlbumPhoto(ctx, photo.ID, testNow); err != nil {
		t.Fatalf("SoftDeleteAlbumPhoto: %v", err)
	}
	if _, err := st.GetAlbumPhoto(ctx, photo.ID); !errors.Is(err, album.ErrNotFound) {
		t.Fatalf("Get after delete error = %v, want ErrNotFound", err)
	}
	photos, err := st.ListAlbumPhotos(ctx, time.Time{}, 10)
	if err != nil {
		t.Fatalf("ListAlbumPhotos: %v", err)
	}
	if len(photos) != 0 {
		t.Fatalf("deleted photo still listed: %+v", photos)
	}
	if err := st.SoftDeleteAlbumPhoto(ctx, photo.ID, testNow); !errors.Is(err, album.ErrAlreadyDeleted) {
		t.Fatalf("double delete error = %v, want ErrAlreadyDeleted", err)
	}

	restored, changed, err := st.RestoreAlbumPhoto(ctx, photo.ID)
	if err != nil {
		t.Fatalf("RestoreAlbumPhoto: %v", err)
	}
	if !changed || restored.ID != photo.ID {
		t.Fatalf("restore = %+v, changed=%v", restored, changed)
	}
	if _, changed, err := st.RestoreAlbumPhoto(ctx, photo.ID); err != nil || changed {
		t.Fatalf("second restore changed=%v, err=%v", changed, err)
	}
	if _, err := st.GetAlbumPhoto(ctx, photo.ID); err != nil {
		t.Fatalf("Get after restore: %v", err)
	}
}

func TestAlbumPhotoAccountIsolation(t *testing.T) {
	st := newTestStore(t)
	wife := WithAccount(context.Background(), 1)
	husband := WithAccount(context.Background(), 2)

	photo, _, err := st.CreateAlbumPhoto(wife, testAlbumPhoto(1))
	if err != nil {
		t.Fatalf("wife photo: %v", err)
	}
	if _, err := st.GetAlbumPhoto(husband, photo.ID); !errors.Is(err, album.ErrNotFound) {
		t.Fatalf("husband Get error = %v, want ErrNotFound", err)
	}
	if err := st.SoftDeleteAlbumPhoto(husband, photo.ID, testNow); !errors.Is(err, album.ErrNotFound) {
		t.Fatalf("husband delete error = %v, want ErrNotFound", err)
	}
	photos, err := st.ListAlbumPhotos(husband, time.Time{}, 10)
	if err != nil {
		t.Fatalf("husband list: %v", err)
	}
	if len(photos) != 0 {
		t.Fatalf("husband saw wife photos: %+v", photos)
	}

	// The upload key is unique per account, not globally, so each account can
	// reuse the same client-generated id without leaking a row.
	husbandPhoto, created, err := st.CreateAlbumPhoto(husband, testAlbumPhoto(1))
	if err != nil || !created {
		t.Fatalf("husband photo = %+v, created=%v, err=%v", husbandPhoto, created, err)
	}
	if husbandPhoto.ID == photo.ID {
		t.Fatal("cross-account upload id returned the other account's row")
	}
}

func TestAlbumFavoritesAndTags(t *testing.T) {
	st := newTestStore(t)
	ctx := WithAccount(context.Background(), 1)

	p1, _, err := st.CreateAlbumPhoto(ctx, testAlbumPhoto(1))
	if err != nil {
		t.Fatalf("CreateAlbumPhoto 1: %v", err)
	}
	p2, _, err := st.CreateAlbumPhoto(ctx, testAlbumPhoto(2))
	if err != nil {
		t.Fatalf("CreateAlbumPhoto 2: %v", err)
	}

	if _, err := st.SetAlbumPhotoFavorite(ctx, p1.ID, true); err != nil {
		t.Fatalf("SetAlbumPhotoFavorite: %v", err)
	}
	if _, err := st.UpdateAlbumPhotoDetails(ctx, p2.ID, p2.TakenAt, "First smile!", "First smile", false); err != nil {
		t.Fatalf("UpdateAlbumPhotoDetails: %v", err)
	}

	favCount, err := st.CountAlbumPhotosFiltered(ctx, AlbumFilter{FavoritesOnly: true})
	if err != nil || favCount != 1 {
		t.Fatalf("favCount = %d, err = %v, want 1", favCount, err)
	}
	favs, err := st.ListAlbumPhotosFiltered(ctx, time.Time{}, AlbumFilter{FavoritesOnly: true}, 10)
	if err != nil || len(favs) != 1 || favs[0].ID != p1.ID || !favs[0].Favorite {
		t.Fatalf("favs = %+v, err = %v", favs, err)
	}

	tagCount, err := st.CountAlbumPhotosFiltered(ctx, AlbumFilter{Tag: "First smile"})
	if err != nil || tagCount != 1 {
		t.Fatalf("tagCount = %d, err = %v, want 1", tagCount, err)
	}
	tags, err := st.ListAlbumTags(ctx)
	if err != nil || len(tags) != 1 || tags[0] != "First smile" {
		t.Fatalf("tags = %v, err = %v", tags, err)
	}
}
