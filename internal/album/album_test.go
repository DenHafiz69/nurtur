package album

import (
	"errors"
	"strings"
	"testing"
	"time"
)

var testNow = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

func validPhoto() Photo {
	return Photo{
		UploadID:      "0123456789abcdef0123456789abcdef",
		TakenAt:       testNow.Add(-time.Hour),
		TakenAtSource: SourceEXIF,
		Caption:       "First smile",
		Width:         2560,
		Height:        1920,
		Bytes:         600_000,
		ThumbBytes:    32_000,
		ContentType:   "image/webp",
		FullKey:       "accounts/1/album/0123456789abcdef0123456789abcdef/full.webp",
		ThumbKey:      "accounts/1/album/0123456789abcdef0123456789abcdef/thumb.webp",
		CreatedAt:     testNow,
	}
}

func TestPhotoValidate(t *testing.T) {
	if err := validPhoto().Validate(); err != nil {
		t.Fatalf("valid photo rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*Photo)
		field  string
	}{
		{"upload id", func(p *Photo) { p.UploadID = "not-a-key" }, "upload_id"},
		{"source", func(p *Photo) { p.TakenAtSource = "guess" }, "taken_at_source"},
		{"zero date", func(p *Photo) { p.TakenAt = time.Time{} }, "taken_at"},
		{"zero dimensions", func(p *Photo) { p.Width = 0 }, "dimensions"},
		{"too wide", func(p *Photo) { p.Width = MaxDimension + 1 }, "dimensions"},
		{"full size", func(p *Photo) { p.Bytes = MaxFullBytes + 1 }, "full"},
		{"thumb size", func(p *Photo) { p.ThumbBytes = MaxThumbBytes + 1 }, "thumb"},
		{"content type", func(p *Photo) { p.ContentType = "image/heic" }, "content_type"},
		{"key", func(p *Photo) { p.FullKey = "" }, "key"},
		{"caption", func(p *Photo) { p.Caption = strings.Repeat("a", MaxCaption+1) }, "caption"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			photo := validPhoto()
			tt.mutate(&photo)
			var ve *ValidationError
			err := photo.Validate()
			if !errors.As(err, &ve) {
				t.Fatalf("error = %v, want ValidationError", err)
			}
			if ve.Field != tt.field {
				t.Fatalf("field = %q, want %q", ve.Field, tt.field)
			}
		})
	}
}

func TestValidateEdit(t *testing.T) {
	if err := ValidateEdit(testNow.Add(-time.Hour), " First smile ", testNow); err != nil {
		t.Fatalf("valid edit rejected: %v", err)
	}
	if err := ValidateEdit(testNow.AddDate(-20, 0, 0), "", testNow); err != nil {
		t.Fatalf("manual correction may point far into the past: %v", err)
	}

	tests := []struct {
		name    string
		takenAt time.Time
		caption string
		field   string
	}{
		{"zero date", time.Time{}, "", "taken_at"},
		{"future date", testNow.Add(25 * time.Hour), "", "taken_at"},
		{"long caption", testNow, strings.Repeat("a", MaxCaption+1), "caption"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ve *ValidationError
			if !errors.As(ValidateEdit(tt.takenAt, tt.caption, testNow), &ve) {
				t.Fatal("error is not a ValidationError")
			}
			if ve.Field != tt.field {
				t.Fatalf("field = %q, want %q", ve.Field, tt.field)
			}
		})
	}
}

func TestValidUploadID(t *testing.T) {
	valid := []string{
		"0123456789abcdef0123456789abcdef",
		"01234567-89ab-cdef-0123-456789abcdef",
	}
	for _, id := range valid {
		if !ValidUploadID(id) {
			t.Errorf("ValidUploadID(%q) = false", id)
		}
	}
	invalid := []string{
		"",
		"0123456789ABCDEF0123456789ABCDEF",
		"0123456789abcdef0123456789abcde",
		"01234567-89AB-cdef-0123-456789abcdef",
		"../0123456789abcdef0123456789abcdef",
	}
	for _, id := range invalid {
		if ValidUploadID(id) {
			t.Errorf("ValidUploadID(%q) = true", id)
		}
	}
}

func TestTakenAtFromSources(t *testing.T) {
	now := testNow
	good := now.Add(-2 * time.Hour)
	old := now.AddDate(-11, 0, 0)
	future := now.Add(48 * time.Hour)

	tests := []struct {
		name     string
		exif     *time.Time
		modified *time.Time
		want     time.Time
		source   TakenAtSource
	}{
		{"exif wins", &good, &now, good, SourceEXIF},
		{"future exif falls back to file", &future, &good, good, SourceFile},
		{"old exif falls back to file", &old, &good, good, SourceFile},
		{"no exif uses file", nil, &good, good, SourceFile},
		{"bad file uses upload", &future, &old, now, SourceUpload},
		{"nothing uses upload", nil, nil, now, SourceUpload},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, source := TakenAtFromSources(tt.exif, tt.modified, now, now)
			if !got.Equal(tt.want) || source != tt.source {
				t.Fatalf("got %v/%s, want %v/%s", got, source, tt.want, tt.source)
			}
		})
	}
}
