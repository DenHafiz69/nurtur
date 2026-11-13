// Package intake holds Nurtur's growing-baby feeding & pumping domains:
// bottles (breast milk / formula), pumping sessions, and solid meals.
//
// It has no database or HTTP dependencies.
package intake

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

const (
	// DuplicateWindow turns rapid double-taps on quick-log buttons into one row.
	DuplicateWindow = 2 * time.Second

	MaxNotesLength = 500
	MaxFoodLength  = 120
	MaxVolumeML    = 1000
	MaxDurationMin = 180

	mlPerOZ = 29.5735
)

var (
	ErrNotFound       = errors.New("intake: not found")
	ErrAlreadyDeleted = errors.New("intake: already deleted")
	ErrInvalidUnit    = errors.New("intake: invalid volume unit")
	ErrInvalidMilk    = errors.New("intake: invalid milk type")
	ErrInvalidSide    = errors.New("intake: invalid pump side")
	ErrInvalidPortion = errors.New("intake: invalid solid portion")
	ErrInvalidReact   = errors.New("intake: invalid reaction")
)

// ValidationError is a user-correctable problem with a form field.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

// Unit is the display/input volume unit. Storage is always integer millilitres.
type Unit string

const (
	UnitML Unit = "ml"
	UnitOZ Unit = "oz"
)

func ParseUnit(raw string) (Unit, error) {
	switch Unit(strings.ToLower(strings.TrimSpace(raw))) {
	case "", UnitML:
		return UnitML, nil
	case UnitOZ:
		return UnitOZ, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidUnit, raw)
	}
}

// FormatVolume formats millilitres in the user's preferred unit.
func FormatVolume(ml int, unit Unit) string {
	if unit == UnitOZ {
		oz := float64(ml) / mlPerOZ
		rounded := math.Round(oz*10) / 10
		if math.Abs(rounded-math.Round(rounded)) < 0.05 {
			return fmt.Sprintf("%.0f oz", math.Round(rounded))
		}
		return fmt.Sprintf("%.1f oz", rounded)
	}
	return fmt.Sprintf("%d ml", ml)
}

// ParseVolumeInput parses a user-entered volume in unit into integer millilitres.
func ParseVolumeInput(raw string, unit Unit) (int, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, &ValidationError{Field: "amount", Message: "Enter an amount."}
	}
	if unit == UnitOZ {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 {
			return 0, &ValidationError{Field: "amount", Message: "Amount must be greater than 0."}
		}
		ml := int(math.Round(v * mlPerOZ))
		if ml <= 0 || ml > MaxVolumeML {
			return 0, &ValidationError{Field: "amount", Message: fmt.Sprintf("Amount must be between 0.1 and %.0f oz.", float64(MaxVolumeML)/mlPerOZ)}
		}
		return ml, nil
	}
	v, err := strconv.Atoi(s)
	if err != nil || v <= 0 {
		return 0, &ValidationError{Field: "amount", Message: "Amount must be a whole number greater than 0."}
	}
	if v > MaxVolumeML {
		return 0, &ValidationError{Field: "amount", Message: fmt.Sprintf("Amount must be %d ml or less.", MaxVolumeML)}
	}
	return v, nil
}

// VolumeInputValue formats millilitres for an <input> value in the chosen unit.
func VolumeInputValue(ml int, unit Unit) string {
	if ml <= 0 {
		return ""
	}
	if unit == UnitOZ {
		oz := math.Round((float64(ml)/mlPerOZ)*10) / 10
		return strconv.FormatFloat(oz, 'f', -1, 64)
	}
	return strconv.Itoa(ml)
}

// MilkType is what is in a bottle.
type MilkType string

const (
	MilkBreast  MilkType = "breast_milk"
	MilkFormula MilkType = "formula"
)

func ParseMilkType(raw string) (MilkType, error) {
	switch MilkType(strings.TrimSpace(raw)) {
	case "", MilkBreast:
		return MilkBreast, nil
	case MilkFormula:
		return MilkFormula, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidMilk, raw)
	}
}

func (m MilkType) Title() string {
	switch m {
	case MilkFormula:
		return "Formula"
	default:
		return "Breast milk"
	}
}

// Bottle is one bottle feed.
type Bottle struct {
	ID        int64
	AccountID int64
	LoggedAt  time.Time
	MilkType  MilkType
	AmountML  int
	Notes     string
	CreatedAt time.Time
	DeletedAt *time.Time
}

// ValidateBottle checks bottle fields before insert or update.
func ValidateBottle(milk MilkType, amountML int, loggedAt, now time.Time, notes string) error {
	if milk != MilkBreast && milk != MilkFormula {
		return &ValidationError{Field: "milk_type", Message: "Choose breast milk or formula."}
	}
	if amountML <= 0 || amountML > MaxVolumeML {
		return &ValidationError{Field: "amount", Message: fmt.Sprintf("Amount must be between 1 and %d ml.", MaxVolumeML)}
	}
	if err := validateLoggedAt(loggedAt, now); err != nil {
		return err
	}
	return validateNotes(notes)
}

// Side is which side(s) were pumped.
type Side string

const (
	SideLeft  Side = "left"
	SideRight Side = "right"
	SideBoth  Side = "both"
)

func ParseSide(raw string) (Side, error) {
	switch Side(strings.TrimSpace(raw)) {
	case SideLeft:
		return SideLeft, nil
	case SideRight:
		return SideRight, nil
	case "", SideBoth:
		return SideBoth, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidSide, raw)
	}
}

func (s Side) Title() string {
	switch s {
	case SideLeft:
		return "Left"
	case SideRight:
		return "Right"
	case SideBoth:
		return "Both"
	default:
		return "Pump"
	}
}

// Pump is one pumping session.
type Pump struct {
	ID              int64
	AccountID       int64
	LoggedAt        time.Time
	Side            Side
	AmountML        int
	DurationMinutes int
	Notes           string
	CreatedAt       time.Time
	DeletedAt       *time.Time
}

// ValidatePump checks pump fields before insert or update.
func ValidatePump(side Side, amountML, durationMin int, loggedAt, now time.Time, notes string) error {
	if side != SideLeft && side != SideRight && side != SideBoth {
		return &ValidationError{Field: "side", Message: "Choose left, right, or both."}
	}
	if amountML < 0 || amountML > MaxVolumeML {
		return &ValidationError{Field: "amount", Message: fmt.Sprintf("Amount must be between 0 and %d ml.", MaxVolumeML)}
	}
	if durationMin < 0 || durationMin > MaxDurationMin {
		return &ValidationError{Field: "duration_minutes", Message: fmt.Sprintf("Duration must be between 0 and %d minutes.", MaxDurationMin)}
	}
	if amountML == 0 && durationMin == 0 {
		return &ValidationError{Field: "amount", Message: "Enter an amount or duration."}
	}
	if err := validateLoggedAt(loggedAt, now); err != nil {
		return err
	}
	return validateNotes(notes)
}

// Portion describes how much of a solid meal was eaten.
type Portion string

const (
	PortionNone  Portion = ""
	PortionTaste Portion = "taste"
	PortionSome  Portion = "some"
	PortionMost  Portion = "most"
)

func ParsePortion(raw string) (Portion, error) {
	switch Portion(strings.TrimSpace(raw)) {
	case PortionNone:
		return PortionNone, nil
	case PortionTaste:
		return PortionTaste, nil
	case PortionSome:
		return PortionSome, nil
	case PortionMost:
		return PortionMost, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidPortion, raw)
	}
}

func (p Portion) Title() string {
	switch p {
	case PortionTaste:
		return "Taste"
	case PortionSome:
		return "Some"
	case PortionMost:
		return "Most"
	default:
		return ""
	}
}

// Reaction records any observed food reaction.
type Reaction string

const (
	ReactionEmpty    Reaction = ""
	ReactionNone     Reaction = "none"
	ReactionMild     Reaction = "mild"
	ReactionAllergic Reaction = "allergic"
)

func ParseReaction(raw string) (Reaction, error) {
	switch Reaction(strings.TrimSpace(raw)) {
	case ReactionEmpty:
		return ReactionEmpty, nil
	case ReactionNone:
		return ReactionNone, nil
	case ReactionMild:
		return ReactionMild, nil
	case ReactionAllergic:
		return ReactionAllergic, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidReact, raw)
	}
}

func (r Reaction) Title() string {
	switch r {
	case ReactionNone:
		return "No reaction"
	case ReactionMild:
		return "Mild reaction"
	case ReactionAllergic:
		return "Allergic reaction"
	default:
		return ""
	}
}

// Solid is one solid food meal.
type Solid struct {
	ID        int64
	AccountID int64
	LoggedAt  time.Time
	Food      string
	Portion   Portion
	Reaction  Reaction
	Notes     string
	CreatedAt time.Time
	DeletedAt *time.Time
}

// ValidateSolid checks solid meal fields before insert or update.
func ValidateSolid(food string, portion Portion, reaction Reaction, loggedAt, now time.Time, notes string) error {
	trimmed := strings.TrimSpace(food)
	if trimmed == "" {
		return &ValidationError{Field: "food", Message: "Enter what your baby ate."}
	}
	if len([]rune(trimmed)) > MaxFoodLength {
		return &ValidationError{Field: "food", Message: fmt.Sprintf("Food must be %d characters or fewer.", MaxFoodLength)}
	}
	if portion != PortionNone && portion != PortionTaste && portion != PortionSome && portion != PortionMost {
		return &ValidationError{Field: "portion", Message: "Choose a valid portion."}
	}
	if reaction != ReactionEmpty && reaction != ReactionNone && reaction != ReactionMild && reaction != ReactionAllergic {
		return &ValidationError{Field: "reaction", Message: "Choose a valid reaction."}
	}
	if err := validateLoggedAt(loggedAt, now); err != nil {
		return err
	}
	return validateNotes(notes)
}

func validateLoggedAt(loggedAt, now time.Time) error {
	if loggedAt.IsZero() {
		return &ValidationError{Field: "logged_at", Message: "Time is required."}
	}
	if loggedAt.After(now.Add(time.Minute)) {
		return &ValidationError{Field: "logged_at", Message: "Time can't be in the future."}
	}
	return nil
}

func validateNotes(notes string) error {
	if len([]rune(notes)) > MaxNotesLength {
		return &ValidationError{Field: "notes", Message: fmt.Sprintf("Notes must be %d characters or fewer.", MaxNotesLength)}
	}
	return nil
}
