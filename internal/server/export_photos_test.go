package server

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"nurtur/internal/album"
)

func TestPartitionPhotoZipParts(t *testing.T) {
	loc := time.UTC
	base := time.Date(2026, 6, 1, 10, 0, 0, 0, loc)
	deletedAt := base.Add(time.Hour)

	photos := []album.Photo{
		{ID: 1, TakenAt: base, ContentType: "image/webp", Bytes: 400},
		{ID: 2, TakenAt: base.Add(time.Minute), ContentType: "image/jpeg", Bytes: 400, DeletedAt: &deletedAt},
		{ID: 3, TakenAt: base.Add(2 * time.Minute), ContentType: "image/png", Bytes: 400},
		{ID: 4, TakenAt: base.Add(3 * time.Minute), ContentType: "image/webp", Bytes: 400},
	}

	// Empty input returns nil.
	if got := partitionPhotoZipParts(nil, loc, maxPhotoZipBytes); len(got) != 0 {
		t.Fatalf("empty input parts = %d, want 0", len(got))
	}

	// All live photos fit in one part when limit is generous.
	single := partitionPhotoZipParts(photos, loc, maxPhotoZipBytes)
	if len(single) != 1 || len(single[0].entries) != 3 {
		t.Fatalf("single part = %+v, want 1 part with 3 live entries", single)
	}

	// With a tight limit (800 bytes), photo 1 + 3 (~600 each with overhead) must split.
	split := partitionPhotoZipParts(photos, loc, 800)
	if len(split) != 3 {
		t.Fatalf("split parts = %d, want 3", len(split))
	}
	for i, part := range split {
		if part.partNumber != i+1 {
			t.Errorf("part[%d].partNumber = %d, want %d", i, part.partNumber, i+1)
		}
		if len(part.entries) != 1 {
			t.Errorf("part[%d] entries = %d, want 1", i, len(part.entries))
		}
		if part.estimatedBytes > 800 {
			t.Errorf("part[%d] estimatedBytes = %d > 800", i, part.estimatedBytes)
		}
	}
}

func TestExportPhotosZipAndSettingsUI(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	// Empty state: settings shows empty note and /export/photos.zip returns valid empty zip.
	rec := env.do(t, http.MethodGet, "/settings", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("settings = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "No album photos to download yet.") {
		t.Fatalf("settings missing empty photo export note:\n%s", rec.Body.String())
	}

	rec = env.do(t, http.MethodGet, "/export/photos.zip", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("empty photos zip = %d", rec.Code)
	}
	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatalf("empty zip reader: %v", err)
	}
	if len(zr.File) != 0 {
		t.Fatalf("empty zip files = %d, want 0", len(zr.File))
	}

	// Seed two live photos and one soft-deleted photo with actual storage bytes.
	payload1 := []byte("webp-image-bytes-one")
	payload2 := []byte("jpeg-image-bytes-two")
	payloadDeleted := []byte("deleted-image-bytes")

	p1 := seedAlbumPhotoWithBytes(t, env, ctx, "00000000000000000000000000000001", testNow, "image/webp", payload1)
	p2 := seedAlbumPhotoWithBytes(t, env, ctx, "00000000000000000000000000000002", testNow.Add(time.Minute), "image/jpeg", payload2)
	pDel := seedAlbumPhotoWithBytes(t, env, ctx, "00000000000000000000000000000003", testNow.Add(2*time.Minute), "image/png", payloadDeleted)
	if err := env.store.SoftDeleteAlbumPhoto(ctx, pDel.ID, testNow.Add(3*time.Minute)); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	// Settings should now show the ZIP download link for 2 photos.
	rec = env.do(t, http.MethodGet, "/settings", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("settings with photos = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `href="/export/photos.zip"`) || !strings.Contains(body, "Download all 2 photos as a ZIP archive.") {
		t.Fatalf("settings missing photo zip link:\n%s", body)
	}

	// Download the zip and verify its contents.
	rec = env.do(t, http.MethodGet, "/export/photos.zip", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("photos zip = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/zip" {
		t.Fatalf("Content-Type = %q, want application/zip", ct)
	}
	if rec.Body.Len() >= maxPhotoZipBytes {
		t.Fatalf("zip size %d >= maxPhotoZipBytes %d", rec.Body.Len(), maxPhotoZipBytes)
	}

	zr, err = zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatalf("zip reader: %v", err)
	}
	if len(zr.File) != 2 {
		t.Fatalf("zip file count = %d, want 2", len(zr.File))
	}

	got := make(map[string][]byte, len(zr.File))
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatalf("read %s: %v", f.Name, err)
		}
		got[f.Name] = data
	}

	wantName1 := fmt.Sprintf("nurtur-%s-%d.webp", p1.TakenAt.In(env.server.loc).Format("2006-01-02-1504"), p1.ID)
	wantName2 := fmt.Sprintf("nurtur-%s-%d.jpg", p2.TakenAt.In(env.server.loc).Format("2006-01-02-1504"), p2.ID)
	if !bytes.Equal(got[wantName1], payload1) {
		t.Errorf("%s bytes = %q, want %q", wantName1, got[wantName1], payload1)
	}
	if !bytes.Equal(got[wantName2], payload2) {
		t.Errorf("%s bytes = %q, want %q", wantName2, got[wantName2], payload2)
	}

	// Out-of-range part returns 404.
	for _, bad := range []string{"/export/photos.zip?part=0", "/export/photos.zip?part=2", "/export/photos.zip?part=abc"} {
		rec = env.do(t, http.MethodGet, bad, nil, false)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s = %d, want 404", bad, rec.Code)
		}
	}
}

func seedAlbumPhotoWithBytes(t *testing.T, env *testEnv, ctx context.Context, uploadID string, takenAt time.Time, contentType string, payload []byte) album.Photo {
	t.Helper()
	fullKey := "test/full-" + uploadID
	thumbKey := "test/thumb-" + uploadID
	if err := env.album.Put(ctx, fullKey, bytes.NewReader(payload), int64(len(payload)), contentType); err != nil {
		t.Fatalf("put full: %v", err)
	}
	if err := env.album.Put(ctx, thumbKey, bytes.NewReader(payload), int64(len(payload)), contentType); err != nil {
		t.Fatalf("put thumb: %v", err)
	}
	photo, created, err := env.store.CreateAlbumPhoto(ctx, album.Photo{
		UploadID:      uploadID,
		TakenAt:       takenAt,
		TakenAtSource: album.SourceManual,
		Width:         100,
		Height:        100,
		Bytes:         int64(len(payload)),
		ThumbBytes:    int64(len(payload)),
		ContentType:   contentType,
		FullKey:       fullKey,
		ThumbKey:      thumbKey,
		CreatedAt:     takenAt,
	})
	if err != nil || !created {
		t.Fatalf("CreateAlbumPhoto: created=%v err=%v", created, err)
	}
	return photo
}
