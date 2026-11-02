// Package album describes one household photo and the rules for accepting it.
//
// Metadata lives in SQLite; bytes live behind the Storage seam. This package
// deliberately has no HTTP or database code so the same rules apply to the
// upload handler, the store, and the tests.
package album

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// MaxCaption is the longest caption the edit sheet accepts.
	MaxCaption = 280
	// MaxDimension is the largest edge the browser may upload. It is a guard
	// against decompression bombs, not a target size.
	MaxDimension = 12000
	// MaxUploadBytes is the total multipart request budget.
	MaxUploadBytes = 12 << 20
	// MaxFullBytes is the display copy budget.
	MaxFullBytes = 8 << 20
	// MaxThumbBytes is the thumbnail budget.
	MaxThumbBytes = 1 << 20
)

// TakenAtSource records how a photo's date was chosen. The UI uses it to say
// whether a date came from the camera or was corrected by hand.
type TakenAtSource string

const (
	SourceEXIF   TakenAtSource = "exif"
	SourceFile   TakenAtSource = "file"
	SourceUpload TakenAtSource = "upload"
	SourceManual TakenAtSource = "manual"
)

// Valid reports whether s is one of the sources the database allows.
func (s TakenAtSource) Valid() bool {
	switch s {
	case SourceEXIF, SourceFile, SourceUpload, SourceManual:
		return true
	default:
		return false
	}
}

// Photo is one row in album_photos. TakenAt is stored in UTC and grouped into
// local days only when rendered.
type Photo struct {
	ID            int64
	AccountID     int64
	UploadID      string
	TakenAt       time.Time
	TakenAtSource TakenAtSource
	Caption       string
	Width         int
	Height        int
	Bytes         int64
	ThumbBytes    int64
	ContentType   string
	FullKey       string
	ThumbKey      string
	CreatedAt     time.Time
	DeletedAt     *time.Time
}

// ErrNotFound is returned when a photo does not exist in the current account.
var ErrNotFound = errors.New("album: photo not found")

// ErrAlreadyDeleted is returned when a soft delete or restore is a no-op.
var ErrAlreadyDeleted = errors.New("album: photo already deleted")

// ValidationError is a user-facing field failure. Handlers can render it next
// to the offending control instead of a generic server error.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

var (
	hexUploadID  = regexp.MustCompile(`^[0-9a-f]{32}$`)
	uuidUploadID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

// ValidUploadID accepts the two shapes the browser generates: 128 random bits
// as lowercase hex, or a canonical lowercase UUID. It is also the retry key.
func ValidUploadID(raw string) bool {
	return hexUploadID.MatchString(raw) || uuidUploadID.MatchString(raw)
}

// ValidContentType reports whether the stored MIME type is one Nurtur serves.
func ValidContentType(raw string) bool {
	switch raw {
	case "image/webp", "image/jpeg", "image/png":
		return true
	default:
		return false
	}
}

// Photo.Validate checks everything the database and the browser rely on. The
// account is bound by the request context, not by the caller.
func (p Photo) Validate() error {
	if !ValidUploadID(p.UploadID) {
		return &ValidationError{Field: "upload_id", Message: "That upload key isn't valid."}
	}
	if !p.TakenAtSource.Valid() {
		return &ValidationError{Field: "taken_at_source", Message: "That date source isn't valid."}
	}
	if p.TakenAt.IsZero() {
		return &ValidationError{Field: "taken_at", Message: "Choose when the photo was taken."}
	}
	if p.CreatedAt.IsZero() {
		return &ValidationError{Field: "created_at", Message: "The photo has no upload time."}
	}
	if p.Width <= 0 || p.Height <= 0 || p.Width > MaxDimension || p.Height > MaxDimension {
		return &ValidationError{Field: "dimensions", Message: "That photo's size isn't supported."}
	}
	if p.Bytes <= 0 || p.Bytes > MaxFullBytes {
		return &ValidationError{Field: "full", Message: "That photo is too large."}
	}
	if p.ThumbBytes <= 0 || (p.ThumbBytes > MaxThumbBytes && p.ThumbKey != p.FullKey) {
		return &ValidationError{Field: "thumb", Message: "That thumbnail is too large."}
	}
	if !ValidContentType(p.ContentType) {
		return &ValidationError{Field: "content_type", Message: "That photo format isn't supported."}
	}
	if strings.TrimSpace(p.FullKey) == "" || strings.TrimSpace(p.ThumbKey) == "" {
		return &ValidationError{Field: "key", Message: "The photo storage key is missing."}
	}
	if utf8.RuneCountInString(strings.TrimSpace(p.Caption)) > MaxCaption {
		return &ValidationError{Field: "caption", Message: fmt.Sprintf("Keep the caption under %d characters.", MaxCaption)}
	}
	return nil
}

// TakenAtFromSources picks the first plausible timestamp in this order: EXIF,
// file modification time, upload time. A camera clock more than a day ahead or
// more than ten years behind is treated as wrong so one obvious lie cannot
// move a photo to 1970 or tomorrow.
func TakenAtFromSources(exif, fileModified *time.Time, uploadedAt, now time.Time) (time.Time, TakenAtSource) {
	if exif != nil && PlausibleTakenAt(*exif, now) {
		return exif.UTC(), SourceEXIF
	}
	if fileModified != nil && PlausibleTakenAt(*fileModified, now) {
		return fileModified.UTC(), SourceFile
	}
	return uploadedAt.UTC(), SourceUpload
}

// PlausibleTakenAt applies the shared camera-clock bounds.
func PlausibleTakenAt(t, now time.Time) bool {
	if t.IsZero() {
		return false
	}
	if t.After(now.Add(24 * time.Hour)) {
		return false
	}
	return !t.Before(now.AddDate(-10, 0, 0))
}

// ValidateEdit checks the date and caption a household member can correct by
// hand. Unlike upload dates, a manual correction may point anywhere in the
// past; only a date more than a day ahead is dismissed as a mistake.
func ValidateEdit(takenAt time.Time, caption string, now time.Time) error {
	if takenAt.IsZero() {
		return &ValidationError{Field: "taken_at", Message: "Choose when the photo was taken."}
	}
	if takenAt.After(now.Add(24 * time.Hour)) {
		return &ValidationError{Field: "taken_at", Message: "That date is in the future."}
	}
	if utf8.RuneCountInString(strings.TrimSpace(caption)) > MaxCaption {
		return &ValidationError{Field: "caption", Message: fmt.Sprintf("Keep the caption under %d characters.", MaxCaption)}
	}
	return nil
}
