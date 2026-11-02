package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
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

func TestAlbumPagingReachesOlderDays(t *testing.T) {
	env := newTestEnv(t)
	photo := album.Photo{
		UploadID:      "0123456789abcdef0123456789abcdef",
		TakenAt:       testNow.AddDate(0, 0, -8),
		TakenAtSource: album.SourceEXIF,
		Width:         1,
		Height:        1,
		Bytes:         42,
		ThumbBytes:    42,
		ContentType:   "image/webp",
		FullKey:       "accounts/0/album/0123456789abcdef0123456789abcdef/full.webp",
		ThumbKey:      "accounts/0/album/0123456789abcdef0123456789abcdef/thumb.webp",
		CreatedAt:     testNow,
	}
	if _, _, err := env.store.CreateAlbumPhoto(context.Background(), photo); err != nil {
		t.Fatalf("seed old photo: %v", err)
	}

	rec := env.do(t, http.MethodGet, "/album", nil, false)
	body := rec.Body.String()
	if strings.Contains(body, `data-photo-id="1"`) {
		t.Fatal("an eight-day-old photo appeared in the first seven days")
	}
	if !strings.Contains(body, "Load earlier days") {
		t.Fatalf("first page has no link to older days:\n%s", body)
	}

	cursor := url.QueryEscape(testNow.AddDate(0, 0, -7).Format("2006-01-02") + "T00:00:00Z")
	rec = env.do(t, http.MethodGet, "/album/partial?before="+cursor, nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("partial status = %d\n%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `data-photo-id="1"`) {
		t.Fatalf("older photo did not load:\n%s", rec.Body.String())
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
	rec := env.do(t, http.MethodGet, "/album/photos/"+strconv.FormatInt(photo.ID, 10)+"/thumb", nil, false)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("foreign photo status = %d, want 404", rec.Code)
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
