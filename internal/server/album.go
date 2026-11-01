package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	_ "golang.org/x/image/webp"

	"nurtur/internal/album"
	"nurtur/internal/clock"
	"nurtur/internal/store"
	"nurtur/internal/view"
)

const (
	defaultAlbumLimit = 60
	maxAlbumDays      = 7
	albumDayLoadCap   = 500
	albumFieldLimit   = 4 << 10
)

var (
	errAlbumPartTooLarge = errors.New("album: upload part is too large")
	errAlbumHEIC         = errors.New("album: HEIC needs browser conversion")
)

// handleAlbumPage renders the fourth tab with whole local days, newest first.
func (s *Server) handleAlbumPage(w http.ResponseWriter, r *http.Request) {
	data, err := s.buildPage(r.Context(), nonceFrom(r.Context()), nil)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	data.View = "album"
	cursor, err := parseTimelineCursor(r.URL.Query().Get("before"))
	if err != nil {
		s.badRequest(w, "That album link isn't valid.")
		return
	}
	page, err := s.buildAlbum(r.Context(), cursor)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	data.Album = page
	if r.URL.Query().Get("added") == "1" {
		data.Toast = &view.Toast{Message: "Photo added.", Kind: "success"}
	}
	s.renderPage(w, r, http.StatusOK, data)
}

// handleAlbumPartial is the paging seam used by "Load earlier days".
func (s *Server) handleAlbumPartial(w http.ResponseWriter, r *http.Request) {
	cursor, err := parseTimelineCursor(r.URL.Query().Get("before"))
	if err != nil {
		s.badRequest(w, "That album link isn't valid.")
		return
	}
	page, err := s.buildAlbum(r.Context(), cursor)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	data, err := s.buildPage(r.Context(), nonceFrom(r.Context()), nil)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	data.View = "album"
	data.Album = page
	s.writeFragments(w, r, http.StatusOK, "album-days", data, false, nil, "", false, "", "")
}

// buildAlbum loads whole local days, newest first, until the photo budget is
// met. Paginating by day means a date heading never splits across pages.
func (s *Server) buildAlbum(ctx context.Context, before time.Time) (*view.AlbumPageView, error) {
	now := s.clock.Now().UTC()

	var dayEnd time.Time
	if before.IsZero() {
		_, dayEnd = clock.DayBounds(now, s.loc)
	} else {
		dayEnd = before.UTC()
	}
	page := &view.AlbumPageView{}
	if dayEnd.After(now.Add(48 * time.Hour)) {
		dayEnd = now
	}

	dayStart := previousLocalDayStart(dayEnd, s.loc)
	nextBoundary := dayEnd
	for days := 0; days < maxAlbumDays; days++ {
		photos, err := s.store.ListAlbumPhotosBetween(ctx, dayStart, dayEnd, albumDayLoadCap)
		if err != nil {
			return nil, err
		}
		if len(photos) > 0 {
			group := view.AlbumDay{
				Key:   view.DateKey(dayStart, s.loc),
				Label: view.DayLabelRelative(dayStart, now, s.loc),
				Start: dayStart,
			}
			for _, photo := range photos {
				group.Rows = append(group.Rows, s.albumRow(photo))
			}
			page.Days = append(page.Days, group)
			page.Count += len(photos)
		}

		nextBoundary = dayStart
		if page.Count >= defaultAlbumLimit {
			break
		}
		dayEnd = dayStart
		dayStart = previousLocalDayStart(dayEnd, s.loc)
	}

	hasMore, err := s.store.HasAlbumPhotosBefore(ctx, nextBoundary)
	if err != nil {
		return nil, err
	}
	page.HasMore = hasMore
	page.HasDays = len(page.Days) > 0
	page.NextCursor = nextBoundary.UTC().Format(time.RFC3339)
	page.Oldest = page.NextCursor
	return page, nil
}

func (s *Server) albumRow(photo album.Photo) view.AlbumRow {
	timeLabel := view.FormatClock(photo.TakenAt, s.loc)
	alt := strings.TrimSpace(photo.Caption)
	if alt == "" {
		alt = "Photo from " + timeLabel
	}
	return view.AlbumRow{
		ID:         photo.ID,
		TakenAt:    photo.TakenAt,
		TimeLabel:  timeLabel,
		Alt:        alt,
		ThumbURL:   fmt.Sprintf("/album/photos/%d/thumb", photo.ID),
		FullURL:    fmt.Sprintf("/album/photos/%d/full", photo.ID),
		Width:      photo.Width,
		Height:     photo.Height,
		RatioClass: albumRatioClass(photo.Width, photo.Height),
		Caption:    photo.Caption,
	}
}

func albumRatioClass(width, height int) string {
	if width <= 0 || height <= 0 {
		return "is-square"
	}
	ratio := float64(width) / float64(height)
	switch {
	case ratio < 0.9:
		return "is-portrait"
	case ratio > 1.1:
		return "is-landscape"
	default:
		return "is-square"
	}
}

func (s *Server) albumDayLabel(takenAt time.Time) string {
	start, _ := clock.DayBounds(takenAt, s.loc)
	return view.DayLabelRelative(start, s.clock.Now().UTC(), s.loc)
}

func (s *Server) albumDayKey(takenAt time.Time) string {
	start, _ := clock.DayBounds(takenAt, s.loc)
	return view.DateKey(start, s.loc)
}

/* ---------- Upload ---------- */

type albumUploadInput struct {
	full       []byte
	thumb      []byte
	uploadID   string
	takenAt    string
	source     string
	caption    string
	loadedFrom string
}

type albumUploadResponse struct {
	OK         bool   `json:"ok"`
	ID         int64  `json:"id,omitempty"`
	TakenAt    string `json:"taken_at,omitempty"`
	TileHTML   string `json:"tile_html,omitempty"`
	DayKey     string `json:"day_key,omitempty"`
	DayLabel   string `json:"day_label,omitempty"`
	DayStart   string `json:"day_start,omitempty"`
	OutOfRange bool   `json:"out_of_range,omitempty"`
	DayURL     string `json:"day_url,omitempty"`
	Error      string `json:"error,omitempty"`
}

func (s *Server) handleAlbumUpload(w http.ResponseWriter, r *http.Request) {
	wantsJSON := strings.Contains(r.Header.Get("Accept"), "application/json")
	r.Body = http.MaxBytesReader(w, r.Body, album.MaxUploadBytes)

	input, err := parseAlbumUpload(r)
	if err != nil {
		status := http.StatusBadRequest
		message := "That photo couldn't be read. Try another one."
		var maxErr *http.MaxBytesError
		if errors.Is(err, errAlbumPartTooLarge) || errors.As(err, &maxErr) {
			status = http.StatusRequestEntityTooLarge
			message = "That photo is too large. Try a smaller one."
		}
		s.albumUploadError(w, r, wantsJSON, status, message)
		return
	}

	now := s.clock.Now().UTC()
	takenAt, source := resolveAlbumTakenAt(input.takenAt, input.source, now)
	if strings.TrimSpace(input.caption) != "" && len([]rune(strings.TrimSpace(input.caption))) > album.MaxCaption {
		s.albumUploadError(w, r, wantsJSON, http.StatusUnprocessableEntity,
			fmt.Sprintf("Keep the caption under %d characters.", album.MaxCaption))
		return
	}
	if !album.ValidUploadID(input.uploadID) {
		s.albumUploadError(w, r, wantsJSON, http.StatusBadRequest, "That upload key isn't valid.")
		return
	}

	fullType, width, height, err := inspectAlbumImage(input.full)
	if err != nil {
		message := "That photo format isn't supported. Try a JPEG, PNG, or WebP."
		if errors.Is(err, errAlbumHEIC) {
			message = "This looks like an HEIC photo. Turn on JavaScript to convert it."
		}
		s.albumUploadError(w, r, wantsJSON, http.StatusUnprocessableEntity, message)
		return
	}
	thumbType, _, _, err := inspectAlbumImage(input.thumb)
	if err != nil || thumbType != fullType {
		s.albumUploadError(w, r, wantsJSON, http.StatusUnprocessableEntity,
			"That thumbnail couldn't be read. Try the photo again.")
		return
	}

	account := store.AccountID(r.Context())
	ext := albumFileExtension(fullType)
	fullKey := fmt.Sprintf("accounts/%d/album/%s/full.%s", account, input.uploadID, ext)
	thumbKey := fmt.Sprintf("accounts/%d/album/%s/thumb.%s", account, input.uploadID, ext)
	single := bytes.Equal(input.full, input.thumb)

	storage := s.album
	if storage == nil {
		s.albumUploadError(w, r, wantsJSON, http.StatusInternalServerError,
			"Photo storage isn't ready. Try again in a moment.")
		return
	}

	if err := storage.Put(r.Context(), fullKey, bytes.NewReader(input.full), int64(len(input.full)), fullType); err != nil {
		s.albumUploadError(w, r, wantsJSON, http.StatusBadGateway,
			"Couldn't save that photo. Check the connection and try again.")
		return
	}
	if !single {
		if err := storage.Put(r.Context(), thumbKey, bytes.NewReader(input.thumb), int64(len(input.thumb)), fullType); err != nil {
			_ = storage.Remove(r.Context(), fullKey)
			s.albumUploadError(w, r, wantsJSON, http.StatusBadGateway,
				"Couldn't save that photo. Check the connection and try again.")
			return
		}
	} else {
		thumbKey = fullKey
	}

	photo := album.Photo{
		UploadID:      input.uploadID,
		TakenAt:       takenAt,
		TakenAtSource: source,
		Caption:       strings.TrimSpace(input.caption),
		Width:         width,
		Height:        height,
		Bytes:         int64(len(input.full)),
		ThumbBytes:    int64(len(input.thumb)),
		ContentType:   fullType,
		FullKey:       fullKey,
		ThumbKey:      thumbKey,
		CreatedAt:     now,
	}
	stored, created, err := s.store.CreateAlbumPhoto(r.Context(), photo)
	if err != nil {
		_ = storage.Remove(r.Context(), fullKey)
		if thumbKey != fullKey {
			_ = storage.Remove(r.Context(), thumbKey)
		}
		s.albumUploadError(w, r, wantsJSON, http.StatusInternalServerError,
			"Couldn't save that photo. Try again.")
		return
	}
	if !created {
		// A retry can arrive with the same keys (normal) or a different
		// fallback format (rare). Keep only the objects the stored row uses.
		if fullKey != stored.FullKey {
			_ = storage.Remove(r.Context(), fullKey)
		}
		if thumbKey != stored.ThumbKey {
			_ = storage.Remove(r.Context(), thumbKey)
		}
	}

	s.albumUploadSuccess(w, r, wantsJSON, stored, created, input.loadedFrom)
}

func (s *Server) albumUploadSuccess(w http.ResponseWriter, r *http.Request, wantsJSON bool, photo album.Photo, created bool, loadedFrom string) {
	if !wantsJSON {
		http.Redirect(w, r, "/album?added=1", http.StatusSeeOther)
		return
	}

	row := s.albumRow(photo)
	var tile bytes.Buffer
	if err := s.renderer.Execute(&tile, "album-tile", row); err != nil {
		s.serverError(w, r, err)
		return
	}

	dayKey := s.albumDayKey(photo.TakenAt)
	dayLabel := s.albumDayLabel(photo.TakenAt)
	start, end := clock.DayBounds(photo.TakenAt, s.loc)
	outOfRange := false
	if oldest, err := time.Parse(time.RFC3339, strings.TrimSpace(loadedFrom)); err == nil && !oldest.IsZero() {
		outOfRange = start.Before(oldest.UTC())
	}
	dayURL := "/album?before=" + url.QueryEscape(end.UTC().Format(time.RFC3339)) + "#album-day-" + dayKey

	resp := albumUploadResponse{
		OK:         true,
		ID:         photo.ID,
		TakenAt:    view.ISO(photo.TakenAt),
		TileHTML:   tile.String(),
		DayKey:     dayKey,
		DayLabel:   dayLabel,
		DayStart:   start.UTC().Format(time.RFC3339),
		OutOfRange: outOfRange,
		DayURL:     dayURL,
	}
	status := http.StatusCreated
	if !created {
		status = http.StatusOK
	}
	writeAlbumJSON(w, status, resp)
}

func (s *Server) albumUploadError(w http.ResponseWriter, r *http.Request, wantsJSON bool, status int, message string) {
	if wantsJSON {
		writeAlbumJSON(w, status, albumUploadResponse{Error: message})
		return
	}
	data, err := s.buildPage(r.Context(), nonceFrom(r.Context()), &view.Toast{Message: message, Kind: "error"})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	data.View = "album"
	page, err := s.buildAlbum(r.Context(), time.Time{})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	data.Album = page
	s.renderPage(w, r, status, data)
}

func writeAlbumJSON(w http.ResponseWriter, status int, payload albumUploadResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func parseAlbumUpload(r *http.Request) (*albumUploadInput, error) {
	reader, err := r.MultipartReader()
	if err != nil {
		return nil, fmt.Errorf("album: read multipart: %w", err)
	}
	input := &albumUploadInput{}
	var single []byte

	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		name := part.FormName()
		switch name {
		case "full", "thumb", "photo":
			limit := int64(album.MaxFullBytes)
			if name == "thumb" {
				limit = album.MaxThumbBytes
			}
			body, err := readAlbumPart(part, limit)
			_ = part.Close()
			if err != nil {
				return nil, err
			}
			switch name {
			case "full":
				input.full = body
			case "thumb":
				input.thumb = body
			case "photo":
				single = body
			}
		case "upload_id", "taken_at", "taken_at_source", "caption", "loaded_before":
			body, err := readAlbumPart(part, albumFieldLimit)
			_ = part.Close()
			if err != nil {
				return nil, err
			}
			value := strings.TrimSpace(string(body))
			switch name {
			case "upload_id":
				input.uploadID = value
			case "taken_at":
				input.takenAt = value
			case "taken_at_source":
				input.source = value
			case "caption":
				input.caption = value
			case "loaded_before":
				input.loadedFrom = value
			}
		default:
			_ = part.Close()
		}
	}

	if len(single) > 0 {
		if len(input.full) > 0 || len(input.thumb) > 0 {
			return nil, fmt.Errorf("album: mixed single and variant uploads")
		}
		input.full = single
		input.thumb = single
	}
	if len(input.full) == 0 {
		return nil, fmt.Errorf("album: upload has no photo")
	}
	if len(input.thumb) == 0 {
		input.thumb = input.full
	}
	if input.uploadID == "" {
		input.uploadID = newAlbumUploadID()
	}
	if !album.ValidUploadID(input.uploadID) {
		return nil, fmt.Errorf("album: invalid upload id")
	}
	if utf8RuneCount(input.takenAt) > 128 || utf8RuneCount(input.loadedFrom) > 128 {
		return nil, fmt.Errorf("album: upload fields are too long")
	}
	return input, nil
}

func readAlbumPart(r io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, errAlbumPartTooLarge
	}
	return body, nil
}

func inspectAlbumImage(data []byte) (string, int, int, error) {
	if len(data) == 0 {
		return "", 0, 0, fmt.Errorf("album: empty image")
	}
	contentType := http.DetectContentType(data)
	if !album.ValidContentType(contentType) {
		if isHEICImage(data) {
			return "", 0, 0, errAlbumHEIC
		}
		return "", 0, 0, fmt.Errorf("album: unsupported content type %q", contentType)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", 0, 0, fmt.Errorf("album: decode image header: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > album.MaxDimension || cfg.Height > album.MaxDimension {
		return "", 0, 0, fmt.Errorf("album: image dimensions are out of range")
	}
	return contentType, cfg.Width, cfg.Height, nil
}

func isHEICImage(data []byte) bool {
	if len(data) < 12 || string(data[4:8]) != "ftyp" {
		return false
	}
	switch string(data[8:12]) {
	case "heic", "heix", "hevc", "hevx", "mif1", "msf1":
		return true
	default:
		return false
	}
}

func resolveAlbumTakenAt(raw, source string, now time.Time) (time.Time, album.TakenAtSource) {
	if raw != "" {
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil && album.PlausibleTakenAt(parsed, now) {
			chosen := album.TakenAtSource(source)
			if !chosen.Valid() || chosen == album.SourceManual {
				chosen = album.SourceUpload
			}
			return parsed.UTC(), chosen
		}
	}
	return now.UTC(), album.SourceUpload
}

func albumFileExtension(contentType string) string {
	switch contentType {
	case "image/jpeg":
		return "jpg"
	case "image/png":
		return "png"
	default:
		return "webp"
	}
}

func newAlbumUploadID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("%032x", time.Now().UnixNano())
	}
	return hex.EncodeToString(raw[:])
}

func utf8RuneCount(value string) int {
	return len([]rune(value))
}

/* ---------- Photo bytes ---------- */

func (s *Server) handleAlbumThumb(w http.ResponseWriter, r *http.Request) {
	s.serveAlbumPhoto(w, r, "thumb")
}

func (s *Server) handleAlbumFull(w http.ResponseWriter, r *http.Request) {
	s.serveAlbumPhoto(w, r, "full")
}

func (s *Server) serveAlbumPhoto(w http.ResponseWriter, r *http.Request, variant string) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		s.notFound(w)
		return
	}
	photo, err := s.store.GetAlbumPhoto(r.Context(), id)
	if errors.Is(err, album.ErrNotFound) {
		s.notFound(w)
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if s.album == nil {
		s.serverError(w, r, fmt.Errorf("server: album storage is not configured"))
		return
	}

	key := photo.FullKey
	if variant == "thumb" {
		key = photo.ThumbKey
	}
	body, size, err := s.album.Get(r.Context(), key)
	if errors.Is(err, album.ErrNotFound) {
		s.notFound(w)
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	defer body.Close()

	w.Header().Set("Content-Type", photo.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	etag := fmt.Sprintf("\"album-%d-%d\"", photo.ID, size)
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	if r.URL.Query().Get("download") == "1" {
		ext := albumFileExtension(photo.ContentType)
		name := "nurtur-" + photo.TakenAt.In(s.loc).Format("2006-01-02-1504") + "." + ext
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, body)
}
