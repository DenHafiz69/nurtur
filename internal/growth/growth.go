// Package growth models baby weight, height/length, and head circumference measurements.
// It has no database or HTTP dependencies.
package growth

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxNotes    = 500
	MaxWeightG  = 100000 // 100 kg
	MaxHeightMM = 2500   // 250 cm
	MaxHeadMM   = 1000   // 100 cm
)

var (
	ErrNotFound       = errors.New("growth: measurement not found")
	ErrAlreadyDeleted = errors.New("growth: measurement already deleted")
)

// ValidationError is a user-facing field failure.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

// Unit selects metric (kg/cm) or imperial (lb/in) display and input parsing.
type Unit string

const (
	UnitMetric   Unit = "metric"
	UnitImperial Unit = "imperial"
)

// ParseUnit normalizes a stored or submitted unit string.
func ParseUnit(raw string) Unit {
	if strings.EqualFold(strings.TrimSpace(raw), string(UnitImperial)) {
		return UnitImperial
	}
	return UnitMetric
}

// Measurement is one growth check-in. Weight is stored in grams; height and head
// circumference are stored in millimeters.
type Measurement struct {
	ID        int64
	AccountID int64
	LoggedAt  time.Time
	WeightG   int
	HeightMM  int
	HeadMM    int
	Notes     string
	CreatedAt time.Time
	DeletedAt *time.Time
}

// Validate checks timestamps, measurement bounds, and notes length.
func (m Measurement) Validate(now time.Time) error {
	if m.LoggedAt.IsZero() {
		return &ValidationError{Field: "logged_at", Message: "Choose a date and time."}
	}
	if m.LoggedAt.After(now.Add(24 * time.Hour)) {
		return &ValidationError{Field: "logged_at", Message: "That time is in the future."}
	}
	if m.WeightG < 0 || m.WeightG > MaxWeightG {
		return &ValidationError{Field: "weight", Message: "Enter a realistic weight."}
	}
	if m.HeightMM < 0 || m.HeightMM > MaxHeightMM {
		return &ValidationError{Field: "height", Message: "Enter a realistic height."}
	}
	if m.HeadMM < 0 || m.HeadMM > MaxHeadMM {
		return &ValidationError{Field: "head", Message: "Enter a realistic head circumference."}
	}
	if m.WeightG == 0 && m.HeightMM == 0 && m.HeadMM == 0 {
		return &ValidationError{Field: "weight", Message: "Enter at least weight, height, or head circumference."}
	}
	if utf8.RuneCountInString(strings.TrimSpace(m.Notes)) > MaxNotes {
		return &ValidationError{Field: "notes", Message: fmt.Sprintf("Keep notes under %d characters.", MaxNotes)}
	}
	return nil
}

// Summary formats non-zero measurements for timeline and status display.
func (m Measurement) Summary(unit Unit) string {
	var parts []string
	if m.WeightG > 0 {
		parts = append(parts, FormatWeight(m.WeightG, unit))
	}
	if m.HeightMM > 0 {
		parts = append(parts, FormatLength(m.HeightMM, unit))
	}
	if m.HeadMM > 0 {
		parts = append(parts, "head "+FormatLength(m.HeadMM, unit))
	}
	if notes := strings.TrimSpace(m.Notes); notes != "" {
		parts = append(parts, notes)
	}
	return strings.Join(parts, " · ")
}

const (
	gramsPerPound = 453.59237
	mmPerInch     = 25.4
)

// FormatWeight renders grams as kg (metric) or lb (imperial).
func FormatWeight(grams int, unit Unit) string {
	if grams <= 0 {
		return ""
	}
	if unit == UnitImperial {
		return fmt.Sprintf("%.2f lb", float64(grams)/gramsPerPound)
	}
	return fmt.Sprintf("%.2f kg", float64(grams)/1000.0)
}

// FormatWeightInput renders grams as a numeric string for form inputs.
func FormatWeightInput(grams int, unit Unit) string {
	if grams <= 0 {
		return ""
	}
	if unit == UnitImperial {
		return fmt.Sprintf("%.2f", float64(grams)/gramsPerPound)
	}
	return fmt.Sprintf("%.2f", float64(grams)/1000.0)
}

// FormatLength renders millimeters as cm (metric) or in (imperial).
func FormatLength(mm int, unit Unit) string {
	if mm <= 0 {
		return ""
	}
	if unit == UnitImperial {
		return fmt.Sprintf("%.1f in", float64(mm)/mmPerInch)
	}
	return fmt.Sprintf("%.1f cm", float64(mm)/10.0)
}

// FormatLengthInput renders millimeters as a numeric string for form inputs.
func FormatLengthInput(mm int, unit Unit) string {
	if mm <= 0 {
		return ""
	}
	if unit == UnitImperial {
		return fmt.Sprintf("%.1f", float64(mm)/mmPerInch)
	}
	return fmt.Sprintf("%.1f", float64(mm)/10.0)
}

// ParseWeight parses a decimal weight in kg (metric) or lb (imperial) into grams.
func ParseWeight(raw string, unit Unit) (int, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, nil
	}
	val, err := strconv.ParseFloat(trimmed, 64)
	if err != nil || math.IsNaN(val) || math.IsInf(val, 0) || val <= 0 {
		return 0, &ValidationError{Field: "weight", Message: "Enter a valid weight."}
	}
	if unit == UnitImperial {
		return int(math.Round(val * gramsPerPound)), nil
	}
	return int(math.Round(val * 1000.0)), nil
}

// ParseLength parses a decimal length in cm (metric) or in (imperial) into millimeters.
func ParseLength(raw, field string, unit Unit) (int, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, nil
	}
	val, err := strconv.ParseFloat(trimmed, 64)
	if err != nil || math.IsNaN(val) || math.IsInf(val, 0) || val <= 0 {
		return 0, &ValidationError{Field: field, Message: "Enter a valid measurement."}
	}
	if unit == UnitImperial {
		return int(math.Round(val * mmPerInch)), nil
	}
	return int(math.Round(val * 10.0)), nil
}
