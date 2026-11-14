package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"nurtur/internal/album"
	"nurtur/internal/store"
)

func TestAlbumPageShell(t *testing.T) {
	env := newTestEnv(t)
	rec := env.do(t, http.MethodGet, "/album", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`<h2 class="page-title">Album</h2>`,
		`data-view="album"`,
		"Photos of the small moments.",
		"No photos yet.",
		"Snap the first one.",
		`data-album-add`,
		`id="album-camera-input"`,
		`id="album-library-input"`,
		`class="tabbar-link is-active"`,
		`aria-current="page"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("album page missing %q:\n%s", want, body)
		}
	}

	start := strings.Index(body, `class="tabbar"`)
	if start < 0 {
		t.Fatal("tabbar not found")
	}
	end := strings.Index(body[start:], "</nav>")
	if end < 0 {
		t.Fatal("tabbar has no closing nav")
	}
	tabbar := body[start : start+end]
	home := strings.Index(tabbar, `href="/"`)
	albumTab := strings.Index(tabbar, `href="/album"`)
	history := strings.Index(tabbar, `href="/timeline"`)
	settings := strings.Index(tabbar, `href="/settings"`)
	if home < 0 || albumTab < 0 || history < 0 || settings < 0 {
		t.Fatalf("tabbar is missing a tab:\n%s", tabbar)
	}
	if !(home < albumTab && albumTab < history && history < settings) {
		t.Fatalf("album tab is not directly right of home:\n%s", tabbar)
	}
}

func TestAlbumPartialReturnsDaysFragment(t *testing.T) {
	env := newTestEnv(t)
	rec := env.do(t, http.MethodGet, "/album/partial", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "<!doctype") || strings.Contains(body, `class="tabbar"`) {
		t.Fatalf("partial rendered the whole shell:\n%s", body)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
}

func TestAlbumUploadAndRead(t *testing.T) {
	env := newTestEnv(t)
	webp := testAlbumWebP(t)
	rec := env.uploadPhoto(t, albumUploadForm{
		full:     webp,
		thumb:    webp,
		fullType: "image/webp",
		uploadID: "0123456789abcdef0123456789abcdef",
		takenAt:  testNow.Add(-time.Hour).Format(time.RFC3339),
		source:   "exif",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload albumUploadResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode upload response: %v\n%s", err, rec.Body.String())
	}
	if !payload.OK || payload.ID == 0 || payload.TileHTML == "" {
		t.Fatalf("upload response = %+v", payload)
	}
	if payload.DayKey != "2026-10-20" || payload.OutOfRange {
		t.Fatalf("day = %q, outOfRange=%v", payload.DayKey, payload.OutOfRange)
	}
	if !strings.Contains(payload.TileHTML, `data-photo-id="`) {
		t.Fatalf("tile HTML missing id:\n%s", payload.TileHTML)
	}

	rec = env.do(t, http.MethodGet, "/album", nil, false)
	body := rec.Body.String()
	for _, want := range []string{`data-photo-id="1"`, `src="/album/photos/1/thumb"`, "11:00 AM"} {
		if !strings.Contains(body, want) {
			t.Fatalf("album after upload missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "No photos yet.") {
		t.Fatal("empty state survived the first upload")
	}
	if strings.Contains(body, "ZgotmplZ") {
		t.Fatal("album tile contains an unsanitized template placeholder")
	}

	rec = env.do(t, http.MethodGet, "/album/photos/1/thumb", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("thumb status = %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "image/webp" {
		t.Fatalf("thumb Content-Type = %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, max-age=31536000, immutable" {
		t.Fatalf("thumb Cache-Control = %q", got)
	}
	if !bytes.Equal(rec.Body.Bytes(), webp) {
		t.Fatal("thumb bytes did not round trip")
	}
	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("thumb ETag is empty")
	}

	req := httptest.NewRequest(http.MethodGet, "/album/photos/1/thumb", nil)
	req.Header.Set("If-None-Match", etag)
	w := httptest.NewRecorder()
	env.handler.ServeHTTP(w, req)
	if w.Code != http.StatusNotModified {
		t.Fatalf("conditional GET status = %d, want 304", w.Code)
	}

	rec = env.do(t, http.MethodGet, "/album/photos/1/full?download=1", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("full status = %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.Contains(got, `attachment; filename="nurtur-2026-10-20-1100.webp"`) {
		t.Fatalf("Content-Disposition = %q", got)
	}
}

func TestAlbumUploadIsIdempotent(t *testing.T) {
	env := newTestEnv(t)
	form := albumUploadForm{
		full:     testAlbumWebP(t),
		thumb:    testAlbumWebP(t),
		fullType: "image/webp",
		uploadID: "0123456789abcdef0123456789abcdef",
		takenAt:  testNow.Format(time.RFC3339),
		source:   "upload",
	}
	first := env.uploadPhoto(t, form)
	if first.Code != http.StatusCreated {
		t.Fatalf("first upload = %d %s", first.Code, first.Body.String())
	}
	second := env.uploadPhoto(t, form)
	if second.Code != http.StatusOK {
		t.Fatalf("retry status = %d, want 200\n%s", second.Code, second.Body.String())
	}
	var firstPayload, secondPayload albumUploadResponse
	if err := json.Unmarshal(first.Body.Bytes(), &firstPayload); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(second.Body.Bytes(), &secondPayload); err != nil {
		t.Fatal(err)
	}
	if firstPayload.ID != secondPayload.ID {
		t.Fatalf("retry ids = %d and %d", firstPayload.ID, secondPayload.ID)
	}
	photos, err := env.store.ListAlbumPhotos(context.Background(), time.Time{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(photos) != 1 {
		t.Fatalf("photos = %d, want 1", len(photos))
	}
}

func TestAlbumUploadRejectsBadInput(t *testing.T) {
	env := newTestEnv(t)
	tests := []struct {
		name   string
		form   albumUploadForm
		status int
	}{
		{
			name: "wrong type",
			form: albumUploadForm{
				full: []byte("<html>not a photo</html>"), thumb: []byte("<html>not a photo</html>"),
				fullType: "text/html", uploadID: "0123456789abcdef0123456789abcdef",
				takenAt: testNow.Format(time.RFC3339), source: "upload",
			},
			status: http.StatusUnprocessableEntity,
		},
		{
			name: "big full",
			form: albumUploadForm{
				full: bytes.Repeat([]byte{0}, album.MaxFullBytes+1), thumb: testAlbumWebP(t),
				fullType: "image/webp", uploadID: "0123456789abcdef0123456789abcdef",
				takenAt: testNow.Format(time.RFC3339), source: "upload",
			},
			status: http.StatusRequestEntityTooLarge,
		},
		{
			name: "too wide",
			form: albumUploadForm{
				full: testAlbumWidePNG(t), thumb: testAlbumWebP(t),
				fullType: "image/png", uploadID: "0123456789abcdef0123456789abcdef",
				takenAt: testNow.Format(time.RFC3339), source: "upload",
			},
			status: http.StatusUnprocessableEntity,
		},
		{
			name: "invalid upload id",
			form: albumUploadForm{
				full: testAlbumWebP(t), thumb: testAlbumWebP(t),
				fullType: "image/webp", uploadID: "nope",
				takenAt: testNow.Format(time.RFC3339), source: "upload",
			},
			status: http.StatusBadRequest,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := env.uploadPhoto(t, tt.form)
			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d\n%s", rec.Code, tt.status, rec.Body.String())
			}
			var payload albumUploadResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
				t.Fatalf("error response is not JSON: %v\n%s", err, rec.Body.String())
			}
			if payload.Error == "" {
				t.Fatal("error response has no message")
			}
		})
	}
}

func TestAlbumUploadBadDateFallsBackToUploadTime(t *testing.T) {
	env := newTestEnv(t)
	rec := env.uploadPhoto(t, albumUploadForm{
		full:     testAlbumWebP(t),
		thumb:    testAlbumWebP(t),
		fullType: "image/webp",
		uploadID: "0123456789abcdef0123456789abcdef",
		takenAt:  testNow.Add(72 * time.Hour).Format(time.RFC3339),
		source:   "exif",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d\n%s", rec.Code, rec.Body.String())
	}
	photos, err := env.store.ListAlbumPhotos(context.Background(), time.Time{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(photos) != 1 || photos[0].TakenAtSource != album.SourceUpload {
		t.Fatalf("photo = %+v", photos)
	}
	if !photos[0].TakenAt.Equal(testNow) {
		t.Fatalf("taken at = %v, want %v", photos[0].TakenAt, testNow)
	}
}

func TestAlbumUploadJPEGFallback(t *testing.T) {
	env := newTestEnv(t)
	jpegBytes := testAlbumJPEG(t)
	rec := env.uploadPhoto(t, albumUploadForm{
		full:     jpegBytes,
		thumb:    jpegBytes,
		fullType: "image/jpeg",
		uploadID: "0123456789abcdef0123456789abcdef",
		takenAt:  testNow.Format(time.RFC3339),
		source:   "upload",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d\n%s", rec.Code, rec.Body.String())
	}
	rec = env.do(t, http.MethodGet, "/album/photos/1/full", nil, false)
	if got := rec.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Fatalf("Content-Type = %q", got)
	}
	if !bytes.Equal(rec.Body.Bytes(), jpegBytes) {
		t.Fatal("JPEG bytes did not round trip")
	}
}

func TestAlbumUploadNoJSRedirects(t *testing.T) {
	env := newTestEnv(t)
	body, contentType := albumMultipart(t, map[string]string{}, map[string][]byte{"photo": testAlbumWebP(t)})
	req := httptest.NewRequest(http.MethodPost, "/album/photos", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	env.handler.ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303\n%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Location"); got != "/album?added=1" {
		t.Fatalf("Location = %q", got)
	}

	rec := env.do(t, http.MethodGet, "/album?added=1", nil, false)
	if !strings.Contains(rec.Body.String(), "Photo added.") {
		t.Fatal("no-JS upload toast missing")
	}
	if !strings.Contains(rec.Body.String(), `data-photo-id="1"`) {
		t.Fatal("no-JS upload did not appear in the album")
	}
}

func TestAlbumNoJSRejectsHEIC(t *testing.T) {
	env := newTestEnv(t)
	heic := []byte{0, 0, 0, 0x18, 'f', 't', 'y', 'p', 'h', 'e', 'i', 'c', 0, 0, 0, 0}
	body, contentType := albumMultipart(t, map[string]string{}, map[string][]byte{"photo": heic})
	req := httptest.NewRequest(http.MethodPost, "/album/photos", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	env.handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422\n%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Turn on JavaScript") {
		t.Fatalf("HEIC message missing:\n%s", w.Body.String())
	}
}

func TestAlbumPageJumpsOverEmptyDays(t *testing.T) {
	env := newTestEnv(t)
	old := seedAlbumPhoto(t, env.store, context.Background(), "0123456789abcdef0123456789abcdef", testNow.AddDate(0, 0, -8))

	rec := env.do(t, http.MethodGet, "/album", nil, false)
	body := rec.Body.String()
	if !strings.Contains(body, fmt.Sprintf(`data-photo-id="%d"`, old.ID)) {
		t.Fatalf("eight-day-old photo was hidden behind empty days:\n%s", body)
	}
	if strings.Contains(body, "Load earlier days") {
		t.Fatalf("single old photo should not need a second page:\n%s", body)
	}
}

func TestAlbumGridShowsDateTimesAndStableFrames(t *testing.T) {
	env := newTestEnv(t)
	rec := env.uploadPhoto(t, albumUploadForm{
		full:     testAlbumWebP(t),
		thumb:    testAlbumWebP(t),
		fullType: "image/webp",
		uploadID: "0123456789abcdef0123456789abcdef",
		takenAt:  testNow.Add(-time.Hour).Format(time.RFC3339),
		source:   "exif",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d\n%s", rec.Code, rec.Body.String())
	}

	rec = env.do(t, http.MethodGet, "/album", nil, false)
	body := rec.Body.String()
	for _, want := range []string{
		`class="album-grid"`,
		`class="album-day-label">Today</span>`,
		`class="album-day-date"> · ` + testNow.Format("Monday, January 2") + `</span>`,
		`<time class="album-tile-time" datetime="2026-10-20T11:00:00Z">11:00 AM</time>`,
		`class="album-tile-frame is-square"`,
		`width="1"`,
		`height="1"`,
		`loading="lazy"`,
		`decoding="async"`,
		`1 photo`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("album grid missing %q:\n%s", want, body)
		}
	}
}

func TestAlbumPagingNeverSplitsADay(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	// A full page's budget is spent on today, so yesterday must wait for the
	// next request even though the day boundary is right after it.
	for i := 0; i < defaultAlbumLimit; i++ {
		seedAlbumPhoto(t, env.store, ctx, fmt.Sprintf("%032x", i+1), testNow.Add(-time.Duration(i+1)*time.Minute))
	}
	old := seedAlbumPhoto(t, env.store, ctx, fmt.Sprintf("%032x", 1000), testNow.AddDate(0, 0, -1))

	rec := env.do(t, http.MethodGet, "/album", nil, false)
	body := rec.Body.String()
	if got := strings.Count(body, "data-photo-id="); got != defaultAlbumLimit {
		t.Fatalf("first page tiles = %d, want %d", got, defaultAlbumLimit)
	}
	if strings.Contains(body, fmt.Sprintf(`data-photo-id="%d"`, old.ID)) {
		t.Fatal("yesterday's photo leaked into a day that was already full")
	}
	if !strings.Contains(body, "Load earlier days") {
		t.Fatalf("first page has no paging control:\n%s", body)
	}
	wantCount := fmt.Sprintf(">%d photos</p>", defaultAlbumLimit+1)
	if !strings.Contains(body, wantCount) {
		t.Fatalf("first page count did not include older unloaded photo (want %s):\n%s", wantCount, body)
	}

	dayStart := time.Date(testNow.Year(), testNow.Month(), testNow.Day(), 0, 0, 0, 0, time.UTC)
	cursor := url.QueryEscape(dayStart.Format(time.RFC3339))
	rec = env.do(t, http.MethodGet, "/album/partial?before="+cursor, nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("partial status = %d\n%s", rec.Code, rec.Body.String())
	}
	partial := rec.Body.String()
	if !strings.Contains(partial, fmt.Sprintf(`data-photo-id="%d"`, old.ID)) {
		t.Fatalf("yesterday's photo did not load on the next page:\n%s", partial)
	}
	if got := strings.Count(partial, "album-day-title"); got != 1 {
		t.Fatalf("partial rendered %d day headings, want only the day with photos:\n%s", got, partial)
	}
	if strings.Contains(partial, "Load earlier days") {
		t.Fatalf("partial offered another page with no older photos:\n%s", partial)
	}
}

func TestAlbumTotalCountAcrossUploadDeleteRestore(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	for i := 0; i < defaultAlbumLimit; i++ {
		seedAlbumPhoto(t, env.store, ctx, fmt.Sprintf("%032x", i+1), testNow.Add(-time.Duration(i+1)*time.Minute))
	}
	older := seedAlbumPhoto(t, env.store, ctx, fmt.Sprintf("%032x", 1000), testNow.AddDate(0, 0, -2))

	// Uploading an out-of-range older photo returns the full album total.
	rec := env.uploadPhoto(t, albumUploadForm{
		full:     testAlbumWebP(t),
		thumb:    testAlbumWebP(t),
		fullType: "image/webp",
		uploadID: "fedcba9876543210fedcba9876543210",
		takenAt:  testNow.AddDate(0, 0, -5).Format(time.RFC3339),
		extra:    map[string]string{"loaded_before": testNow.AddDate(0, 0, -1).Format(time.RFC3339)},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d\n%s", rec.Code, rec.Body.String())
	}
	var up albumUploadResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &up); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	if up.TotalCount != defaultAlbumLimit+2 {
		t.Fatalf("upload total_count = %d, want %d", up.TotalCount, defaultAlbumLimit+2)
	}

	// Deleting a photo returns the updated total including older photos.
	delRec := env.albumJSON(t, http.MethodDelete, fmt.Sprintf("/album/photos/%d", older.ID), nil)
	if delRec.Code != http.StatusOK {
		t.Fatalf("delete status = %d\n%s", delRec.Code, delRec.Body.String())
	}
	var del albumDeleteResponse
	if err := json.Unmarshal(delRec.Body.Bytes(), &del); err != nil {
		t.Fatalf("decode delete response: %v", err)
	}
	if del.TotalCount != defaultAlbumLimit+1 {
		t.Fatalf("delete total_count = %d, want %d", del.TotalCount, defaultAlbumLimit+1)
	}

	// Restoring via HTMX returns an OOB #album-count with the restored total.
	restRec := env.do(t, http.MethodPost, del.UndoURL, nil, true)
	if restRec.Code != http.StatusOK {
		t.Fatalf("restore status = %d\n%s", restRec.Code, restRec.Body.String())
	}
	wantOOB := fmt.Sprintf(`id="album-count" class="album-count" hx-swap-oob="outerHTML">%d photos</p>`, defaultAlbumLimit+2)
	if !strings.Contains(restRec.Body.String(), wantOOB) {
		t.Fatalf("restore response missing updated OOB count %q:\n%s", wantOOB, restRec.Body.String())
	}
}

func TestAlbumDeletedPhotoReturns404(t *testing.T) {
	env := newTestEnv(t)
	rec := env.uploadPhoto(t, albumUploadForm{
		full:     testAlbumWebP(t),
		thumb:    testAlbumWebP(t),
		fullType: "image/webp",
		uploadID: "0123456789abcdef0123456789abcdef",
		takenAt:  testNow.Format(time.RFC3339),
		source:   "upload",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d\n%s", rec.Code, rec.Body.String())
	}
	var payload albumUploadResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if err := env.store.SoftDeleteAlbumPhoto(context.Background(), payload.ID, testNow); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	for _, path := range []string{
		fmt.Sprintf("/album/photos/%d/thumb", payload.ID),
		fmt.Sprintf("/album/photos/%d/full", payload.ID),
	} {
		rec := env.do(t, http.MethodGet, path, nil, false)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want 404", path, rec.Code)
		}
	}
	rec = env.do(t, http.MethodGet, "/album", nil, false)
	if strings.Contains(rec.Body.String(), fmt.Sprintf(`data-photo-id="%d"`, payload.ID)) {
		t.Fatal("deleted photo still rendered in the album")
	}
	if !strings.Contains(rec.Body.String(), "No photos yet.") {
		t.Fatal("empty state did not return after deleting the only photo")
	}
}

func TestAlbumViewerMarkup(t *testing.T) {
	env := newTestEnv(t)
	rec := env.uploadPhoto(t, albumUploadForm{
		full:     testAlbumWebP(t),
		thumb:    testAlbumWebP(t),
		fullType: "image/webp",
		uploadID: "0123456789abcdef0123456789abcdef",
		takenAt:  testNow.Add(-time.Hour).Format(time.RFC3339),
		source:   "exif",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d\n%s", rec.Code, rec.Body.String())
	}

	body := env.do(t, http.MethodGet, "/album", nil, false).Body.String()
	for _, want := range []string{
		`id="album-viewer"`,
		`data-album-viewer-image`,
		`data-album-viewer-prev`,
		`data-album-viewer-next`,
		`data-album-viewer-edit`,
		`data-album-viewer-delete-form`,
		`data-full-label="` + testNow.Add(-time.Hour).Format("Monday, January 2 at 3:04 PM") + `"`,
		`data-edit-url="/album/photos/1/edit"`,
		`data-download-url="/album/photos/1/full?download=1"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("viewer markup missing %q:\n%s", want, body)
		}
	}
}

func TestAlbumEditPageAndUpdate(t *testing.T) {
	env := newTestEnv(t)
	rec := env.uploadPhoto(t, albumUploadForm{
		full:     testAlbumWebP(t),
		thumb:    testAlbumWebP(t),
		fullType: "image/webp",
		uploadID: "0123456789abcdef0123456789abcdef",
		takenAt:  testNow.Add(-time.Hour).Format(time.RFC3339),
		source:   "exif",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d\n%s", rec.Code, rec.Body.String())
	}

	rec = env.do(t, http.MethodGet, "/album/photos/1/edit", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("edit sheet status = %d\n%s", rec.Code, rec.Body.String())
	}
	sheet := rec.Body.String()
	for _, want := range []string{
		`data-album-edit-form`,
		`action="/album/photos/1"`,
		`name="taken_at" value="2026-10-20T11:00:00"`,
		`maxlength="280"`,
	} {
		if !strings.Contains(sheet, want) {
			t.Fatalf("edit sheet missing %q:\n%s", want, sheet)
		}
	}

	rec = env.do(t, http.MethodGet, "/album/photos/1/edit", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("full edit page status = %d\n%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `data-album-edit-form`) {
		t.Fatalf("full edit page has no sheet:\n%s", rec.Body.String())
	}

	rec = env.albumJSON(t, http.MethodPatch, "/album/photos/1", url.Values{
		"taken_at":      {"2026-10-19T09:30:00"},
		"caption":       {"  First smile  "},
		"loaded_before": {"2026-10-13T00:00:00Z"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d\n%s", rec.Code, rec.Body.String())
	}
	var payload albumEditResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode update: %v\n%s", err, rec.Body.String())
	}
	if !payload.OK || payload.ID != 1 || payload.DayKey != "2026-10-19" {
		t.Fatalf("update payload = %+v", payload)
	}
	if payload.Caption != "First smile" || payload.OutOfRange {
		t.Fatalf("caption/outOfRange = %q/%v", payload.Caption, payload.OutOfRange)
	}
	if !strings.Contains(payload.TileHTML, `data-photo-id="1"`) {
		t.Fatalf("updated tile missing id:\n%s", payload.TileHTML)
	}

	photo, err := env.store.GetAlbumPhoto(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if photo.TakenAtSource != album.SourceManual || photo.Caption != "First smile" {
		t.Fatalf("stored photo = %+v", photo)
	}
}

func TestAlbumEditValidation(t *testing.T) {
	env := newTestEnv(t)
	env.uploadPhoto(t, albumUploadForm{
		full:     testAlbumWebP(t),
		thumb:    testAlbumWebP(t),
		fullType: "image/webp",
		uploadID: "0123456789abcdef0123456789abcdef",
		takenAt:  testNow.Format(time.RFC3339),
		source:   "upload",
	})

	tests := []struct {
		name  string
		form  url.Values
		field string
	}{
		{
			name:  "future date",
			form:  url.Values{"taken_at": {testNow.Add(25 * time.Hour).Format("2006-01-02T15:04:05")}},
			field: "taken_at",
		},
		{
			name: "long caption",
			form: url.Values{
				"taken_at": {testNow.Format("2006-01-02T15:04:05")},
				"caption":  {strings.Repeat("a", album.MaxCaption+1)},
			},
			field: "caption",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := env.albumJSON(t, http.MethodPatch, "/album/photos/1", tt.form)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d\n%s", rec.Code, rec.Body.String())
			}
			var payload albumEditResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Errors[tt.field] == "" {
				t.Fatalf("field %q has no error: %+v", tt.field, payload)
			}
		})
	}
}

func TestAlbumDeleteAndUndo(t *testing.T) {
	env := newTestEnv(t)
	rec := env.uploadPhoto(t, albumUploadForm{
		full:     testAlbumWebP(t),
		thumb:    testAlbumWebP(t),
		fullType: "image/webp",
		uploadID: "0123456789abcdef0123456789abcdef",
		takenAt:  testNow.Format(time.RFC3339),
		source:   "upload",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d\n%s", rec.Code, rec.Body.String())
	}
	rec = env.uploadPhoto(t, albumUploadForm{
		full:     testAlbumWebP(t),
		thumb:    testAlbumWebP(t),
		fullType: "image/webp",
		uploadID: "fedcba9876543210fedcba9876543210",
		takenAt:  testNow.Add(-time.Hour).Format(time.RFC3339),
		source:   "upload",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("second upload status = %d\n%s", rec.Code, rec.Body.String())
	}

	rec = env.albumJSON(t, http.MethodDelete, "/album/photos/1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status = %d\n%s", rec.Code, rec.Body.String())
	}
	var deleted albumDeleteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &deleted); err != nil {
		t.Fatal(err)
	}
	if !deleted.OK || deleted.DayKey != "2026-10-20" || deleted.UndoURL != "/album/photos/1/restore" {
		t.Fatalf("delete payload = %+v", deleted)
	}

	body := env.do(t, http.MethodGet, "/album", nil, false).Body.String()
	if strings.Contains(body, `data-photo-id="1"`) {
		t.Fatal("deleted photo still rendered")
	}
	if rec := env.do(t, http.MethodGet, "/album/photos/1/thumb", nil, false); rec.Code != http.StatusNotFound {
		t.Fatalf("deleted thumb status = %d", rec.Code)
	}

	rec = env.do(t, http.MethodPost, "/album/photos/1/restore", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("restore status = %d\n%s", rec.Code, rec.Body.String())
	}
	restored := rec.Body.String()
	for _, want := range []string{
		`id="album-day-2026-10-20"`,
		`data-photo-id="1"`,
		`data-photo-id="2"`,
		`id="toast-region"`,
		"Photo restored.",
	} {
		if !strings.Contains(restored, want) {
			t.Fatalf("restore response missing %q:\n%s", want, restored)
		}
	}
	body = env.do(t, http.MethodGet, "/album", nil, false).Body.String()
	if !strings.Contains(body, `data-photo-id="1"`) {
		t.Fatal("restored photo did not return")
	}

	rec = env.albumJSON(t, http.MethodDelete, "/album/photos/1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("second delete status = %d", rec.Code)
	}
	rec = env.do(t, http.MethodPost, "/album/photos/1/restore", nil, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("no-JS restore status = %d\n%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != "/album?restored=1#album-day-2026-10-20" {
		t.Fatalf("no-JS restore Location = %q", got)
	}
}

func TestAlbumEditAndDeleteNoJSRedirect(t *testing.T) {
	env := newTestEnv(t)
	env.uploadPhoto(t, albumUploadForm{
		full:     testAlbumWebP(t),
		thumb:    testAlbumWebP(t),
		fullType: "image/webp",
		uploadID: "0123456789abcdef0123456789abcdef",
		takenAt:  testNow.Format(time.RFC3339),
		source:   "upload",
	})

	rec := env.do(t, http.MethodPost, "/album/photos/1", url.Values{
		"taken_at": {testNow.Format("2006-01-02T15:04:05")},
		"caption":  {"No script"},
	}, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("edit redirect = %d\n%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != "/album?edited=1#album-photo-1" {
		t.Fatalf("edit Location = %q", got)
	}

	rec = env.do(t, http.MethodPost, "/album/photos/1/delete", nil, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("delete redirect = %d\n%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != "/album?deleted=1" {
		t.Fatalf("delete Location = %q", got)
	}
}

func TestAlbumUploadLimiter(t *testing.T) {
	limiter := newAlbumUploadLimiter()
	for i := 0; i < albumUploadLimit; i++ {
		if !limiter.allow(7, testNow) {
			t.Fatalf("upload %d was blocked", i+1)
		}
	}
	if limiter.allow(7, testNow) {
		t.Fatal("limiter allowed an upload over the window budget")
	}
	if !limiter.allow(8, testNow) {
		t.Fatal("another account shared the first account's window")
	}
	if !limiter.allow(7, testNow.Add(albumUploadWindow)) {
		t.Fatal("limiter did not reset after the window")
	}
}

func TestAlbumUploadReturns429WhenThrottled(t *testing.T) {
	env := newTestEnv(t)
	for i := 0; i < albumUploadLimit; i++ {
		env.server.albumRate.allow(0, testNow)
	}
	rec := env.uploadPhoto(t, albumUploadForm{
		full:     testAlbumWebP(t),
		thumb:    testAlbumWebP(t),
		fullType: "image/webp",
		uploadID: "0123456789abcdef0123456789abcdef",
		takenAt:  testNow.Format(time.RFC3339),
		source:   "upload",
	})
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429\n%s", rec.Code, rec.Body.String())
	}
	var payload albumUploadResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Error == "" {
		t.Fatal("throttle response has no message")
	}
}

func TestAlbumRetentionSweep(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	old := seedAlbumPhoto(t, env.store, ctx, "0123456789abcdef0123456789abcdef", testNow.AddDate(0, 0, -40))
	recent := seedAlbumPhoto(t, env.store, ctx, "fedcba9876543210fedcba9876543210", testNow.AddDate(0, 0, -5))
	for _, photo := range []album.Photo{old, recent} {
		for _, key := range []string{photo.FullKey, photo.ThumbKey} {
			if err := env.album.Put(ctx, key, strings.NewReader("bytes"), 5, "image/webp"); err != nil {
				t.Fatalf("put %s: %v", key, err)
			}
		}
	}
	if err := env.store.SoftDeleteAlbumPhoto(ctx, old.ID, testNow.AddDate(0, 0, -40)); err != nil {
		t.Fatal(err)
	}
	if err := env.store.SoftDeleteAlbumPhoto(ctx, recent.ID, testNow.AddDate(0, 0, -5)); err != nil {
		t.Fatal(err)
	}

	removed, kept, err := env.server.sweepExpiredAlbums(ctx, testNow)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if removed != 1 || kept != 0 {
		t.Fatalf("sweep removed=%d kept=%d", removed, kept)
	}
	for _, key := range []string{old.FullKey, old.ThumbKey} {
		if _, _, err := env.album.Stat(ctx, key); !errors.Is(err, album.ErrNotFound) {
			t.Fatalf("old object %s still present: %v", key, err)
		}
	}
	expired, err := env.store.ExpiredAlbumPhotos(ctx, testNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(expired) != 1 || expired[0].ID != recent.ID {
		t.Fatalf("expired rows = %+v", expired)
	}
}

func TestExportIncludesAlbumMetadata(t *testing.T) {
	env := newTestEnv(t)
	photo := seedAlbumPhoto(t, env.store, context.Background(), "0123456789abcdef0123456789abcdef", testNow)
	photo.Caption = "First smile"
	if _, err := env.store.UpdateAlbumPhoto(context.Background(), photo.ID, photo.TakenAt, photo.Caption); err != nil {
		t.Fatal(err)
	}

	rec := env.do(t, http.MethodGet, "/export.json", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("json status = %d", rec.Code)
	}
	for _, want := range []string{
		`"album_photos"`,
		`"full_key"`,
		`"thumb_key"`,
		`"taken_at_source": "manual"`,
		`"caption": "First smile"`,
		`"photo_storage": "external`,
	} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("json export missing %q\n%s", want, rec.Body.String())
		}
	}

	rec = env.do(t, http.MethodGet, "/export.sql", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("sql status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "INSERT INTO album_photos") {
		t.Fatalf("sql export missing album rows:\n%s", rec.Body.String())
	}
}

func TestAlbumReadIsAccountScoped(t *testing.T) {
	env := newTestEnv(t)
	photo, _, err := env.store.CreateAlbumPhoto(store.WithAccount(context.Background(), 55), album.Photo{
		UploadID:      "0123456789abcdef0123456789abcdef",
		TakenAt:       testNow,
		TakenAtSource: album.SourceUpload,
		Width:         1,
		Height:        1,
		Bytes:         42,
		ThumbBytes:    42,
		ContentType:   "image/webp",
		FullKey:       "accounts/55/album/0123456789abcdef0123456789abcdef/full.webp",
		ThumbKey:      "accounts/55/album/0123456789abcdef0123456789abcdef/thumb.webp",
		CreatedAt:     testNow,
	})
	if err != nil {
		t.Fatalf("seed foreign photo: %v", err)
	}
	id := strconv.FormatInt(photo.ID, 10)
	rec := env.do(t, http.MethodGet, "/album/photos/"+id+"/thumb", nil, false)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("foreign photo status = %d, want 404", rec.Code)
	}
	rec = env.albumJSON(t, http.MethodPatch, "/album/photos/"+id, url.Values{
		"taken_at": {testNow.Format("2006-01-02T15:04:05")},
	})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("foreign update status = %d, want 404", rec.Code)
	}
	rec = env.albumJSON(t, http.MethodDelete, "/album/photos/"+id, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("foreign delete status = %d, want 404", rec.Code)
	}
	rec = env.do(t, http.MethodPost, "/album/photos/"+id+"/restore", nil, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("foreign restore status = %d, want 404", rec.Code)
	}
}

/* ---------- helpers ---------- */

type albumUploadForm struct {
	full     []byte
	thumb    []byte
	fullType string
	uploadID string
	takenAt  string
	source   string
	extra    map[string]string
}

func (e *testEnv) albumJSON(t *testing.T, method, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req := httptest.NewRequest(method, path, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	return rec
}

func seedAlbumPhoto(t *testing.T, st *store.Store, ctx context.Context, uploadID string, takenAt time.Time) album.Photo {
	t.Helper()
	photo, _, err := st.CreateAlbumPhoto(ctx, album.Photo{
		UploadID:      uploadID,
		TakenAt:       takenAt,
		TakenAtSource: album.SourceEXIF,
		Width:         1,
		Height:        1,
		Bytes:         42,
		ThumbBytes:    42,
		ContentType:   "image/webp",
		FullKey:       "accounts/0/album/" + uploadID + "/full.webp",
		ThumbKey:      "accounts/0/album/" + uploadID + "/thumb.webp",
		CreatedAt:     testNow,
	})
	if err != nil {
		t.Fatalf("seed album photo %s: %v", uploadID, err)
	}
	return photo
}

func (e *testEnv) uploadPhoto(t *testing.T, form albumUploadForm) *httptest.ResponseRecorder {
	t.Helper()
	files := map[string][]byte{"full": form.full, "thumb": form.thumb}
	fields := map[string]string{
		"upload_id":       form.uploadID,
		"taken_at":        form.takenAt,
		"taken_at_source": form.source,
	}
	for key, value := range form.extra {
		fields[key] = value
	}
	body, contentType := albumMultipart(t, fields, files)
	req := httptest.NewRequest(http.MethodPost, "/album/photos", body)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	return rec
}

func albumMultipart(t *testing.T, fields map[string]string, files map[string][]byte) (io.Reader, string) {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("write field %s: %v", key, err)
		}
	}
	for name, data := range files {
		part, err := writer.CreateFormFile(name, name+".bin")
		if err != nil {
			t.Fatalf("create part %s: %v", name, err)
		}
		if _, err := part.Write(data); err != nil {
			t.Fatalf("write part %s: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, writer.FormDataContentType()
}

func testAlbumWebP(t *testing.T) []byte {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString("UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func testAlbumWidePNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, album.MaxDimension+1, 1))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func testAlbumJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 220, G: 160, B: 120, A: 255})
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
