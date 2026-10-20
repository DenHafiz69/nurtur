// Package diaper holds Nurtur's diaper domain: the three event types, the
// optional colour/consistency details, and validation for corrections.
//
// It has no database or HTTP dependencies.
package diaper

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Kind is the kind of diaper event.
type Kind string

const (
	Wet   Kind = "wet"
	Dirty Kind = "dirty"
	Both  Kind = "both"
)

// DuplicateWindow is the grace period that turns a rapid double tap into one
// row. Two genuinely separate wet diapers are never seconds apart.
const DuplicateWindow = 2 * time.Second

// MaxNotesLength bounds the free-text note.
const MaxNotesLength = 500

// Domain errors.
var (
	ErrNotFound       = errors.New("diaper: not found")
	ErrAlreadyDeleted = errors.New("diaper: already deleted")
	ErrInvalidKind    = errors.New("diaper: invalid kind")
	ErrInvalidColor   = errors.New("diaper: invalid colour")
	ErrInvalidConsist = errors.New("diaper: invalid consistency")
)

// ValidationError is a user-correctable problem with a form field.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

// ParseKind converts a form value into a Kind.
func ParseKind(raw string) (Kind, error) {
	switch Kind(strings.TrimSpace(raw)) {
	case Wet:
		return Wet, nil
	case Dirty:
		return Dirty, nil
	case Both:
		return Both, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidKind, raw)
	}
}

// Valid reports whether k is a known kind.
func (k Kind) Valid() bool { return k == Wet || k == Dirty || k == Both }

// Title is the capitalised label used in copy.
func (k Kind) Title() string {
	switch k {
	case Wet:
		return "Wet"
	case Dirty:
		return "Dirty"
	case Both:
		return "Both"
	default:
		return "Diaper"
	}
}

// Covers reports whether an event of kind k counts as a wet or dirty event.
// A "both" event satisfies both categories.
func (k Kind) Covers(want Kind) bool {
	if k == want {
		return true
	}
	return k == Both && (want == Wet || want == Dirty)
}

// Noun is the copy used after "logged": "Wet diaper logged."
func (k Kind) Noun() string {
	if k == Both {
		return "Wet and dirty diaper"
	}
	return k.Title() + " diaper"
}

// Color is the optional stool colour.
type Color string

const (
	ColorYellow  Color = "yellow"
	ColorMustard Color = "mustard"
	ColorGreen   Color = "green"
	ColorDark    Color = "dark"
	ColorNone    Color = ""
)

// ParseColor accepts an empty value (not recorded) or a known colour.
func ParseColor(raw string) (Color, error) {
	switch Color(strings.TrimSpace(raw)) {
	case ColorNone:
		return ColorNone, nil
	case ColorYellow:
		return ColorYellow, nil
	case ColorMustard:
		return ColorMustard, nil
	case ColorGreen:
		return ColorGreen, nil
	case ColorDark:
		return ColorDark, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidColor, raw)
	}
}

// Title is the display label for a colour.
func (c Color) Title() string {
	switch c {
	case ColorYellow:
		return "Yellow"
	case ColorMustard:
		return "Mustard"
	case ColorGreen:
		return "Green"
	case ColorDark:
		return "Dark"
	default:
		return ""
	}
}

// Consistency is the optional stool consistency.
type Consistency string

const (
	Seedy       Consistency = "seedy"
	Liquid      Consistency = "liquid"
	Hard        Consistency = "hard"
	ConsistNone Consistency = ""
)

// ParseConsistency accepts an empty value or a known consistency.
func ParseConsistency(raw string) (Consistency, error) {
	switch Consistency(strings.TrimSpace(raw)) {
	case ConsistNone:
		return ConsistNone, nil
	case Seedy:
		return Seedy, nil
	case Liquid:
		return Liquid, nil
	case Hard:
		return Hard, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidConsist, raw)
	}
}

// Title is the display label for a consistency.
func (c Consistency) Title() string {
	switch c {
	case Seedy:
		return "Seedy"
	case Liquid:
		return "Liquid"
	case Hard:
		return "Hard"
	default:
		return ""
	}
}

// Diaper is one logged event.
type Diaper struct {
	ID          int64
	LoggedAt    time.Time
	Kind        Kind
	Color       Color
	Consistency Consistency
	Notes       string
	CreatedAt   time.Time
	DeletedAt   *time.Time
}

// Wet reports whether the event counts as wet.
func (d Diaper) Wet() bool { return d.Kind.Covers(Wet) }

// DirtyEvent reports whether the event counts as dirty.
func (d Diaper) DirtyEvent() bool { return d.Kind.Covers(Dirty) }

// Summary is the detail line shown under the row title, e.g.
// "Mustard · Seedy". It is empty when no details were recorded.
func (d Diaper) Summary() string {
	parts := make([]string, 0, 2)
	if d.Color.Title() != "" {
		parts = append(parts, d.Color.Title())
	}
	if d.Consistency.Title() != "" {
		parts = append(parts, d.Consistency.Title())
	}
	return strings.Join(parts, " · ")
}

// Edit is a correction to a logged diaper. The kind is fixed; details and the
// logged time may change.
type Edit struct {
	LoggedAt    time.Time
	Color       Color
	Consistency Consistency
	Notes       string
}

// Validate checks a correction against the clock.
func (e Edit) Validate(now time.Time) error {
	if e.LoggedAt.IsZero() {
		return &ValidationError{Field: "logged_at", Message: "Pick a time."}
	}
	if e.LoggedAt.After(now.Add(time.Minute)) {
		return &ValidationError{Field: "logged_at", Message: "That time is in the future. Try again?"}
	}
	if !e.Color.valid() {
		return &ValidationError{Field: "color", Message: "Pick a colour from the list."}
	}
	if !e.Consistency.valid() {
		return &ValidationError{Field: "consistency", Message: "Pick a consistency from the list."}
	}
	if len([]rune(strings.TrimSpace(e.Notes))) > MaxNotesLength {
		return &ValidationError{Field: "notes", Message: "Keep the note a little shorter."}
	}
	return nil
}

func (c Color) valid() bool {
	switch c {
	case ColorNone, ColorYellow, ColorMustard, ColorGreen, ColorDark:
		return true
	default:
		return false
	}
}

func (c Consistency) valid() bool {
	switch c {
	case ConsistNone, Seedy, Liquid, Hard:
		return true
	default:
		return false
	}
}
