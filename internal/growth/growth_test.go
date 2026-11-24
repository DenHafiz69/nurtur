package growth

import (
	"testing"
	"time"
)

func TestMeasurementValidateAndFormat(t *testing.T) {
	now := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)

	m := Measurement{
		LoggedAt: now.Add(-time.Hour),
		WeightG:  4250,
		HeightMM: 545,
		HeadMM:   372,
		Notes:    "6-week checkup",
	}
	if err := m.Validate(now); err != nil {
		t.Fatalf("Validate valid: %v", err)
	}
	if got := m.Summary(UnitMetric); got != "4.25 kg · 54.5 cm · head 37.2 cm · 6-week checkup" {
		t.Errorf("Summary(metric) = %q", got)
	}
	if got := m.Summary(UnitImperial); got == "" {
		t.Errorf("Summary(imperial) is empty")
	}

	// Missing all measurements fails.
	empty := Measurement{LoggedAt: now}
	if err := empty.Validate(now); err == nil {
		t.Fatal("expected error for empty measurement")
	}

	// Future date fails.
	future := Measurement{LoggedAt: now.Add(48 * time.Hour), WeightG: 4000}
	if err := future.Validate(now); err == nil {
		t.Fatal("expected error for future date")
	}

	// ParseWeight and ParseLength round-trip.
	g, err := ParseWeight("4.25", UnitMetric)
	if err != nil || g != 4250 {
		t.Fatalf("ParseWeight metric = %d, err=%v", g, err)
	}
	mm, err := ParseLength("54.5", "height", UnitMetric)
	if err != nil || mm != 545 {
		t.Fatalf("ParseLength metric = %d, err=%v", mm, err)
	}
	if _, err := ParseWeight("-1", UnitMetric); err == nil {
		t.Fatal("expected error for negative weight")
	}
}
