package intake

import (
	"testing"
	"time"
)

func TestVolumeFormattingAndParsing(t *testing.T) {
	if got := FormatVolume(120, UnitML); got != "120 ml" {
		t.Fatalf("FormatVolume ml = %q", got)
	}
	if got := FormatVolume(118, UnitOZ); got != "4 oz" {
		t.Fatalf("FormatVolume oz = %q", got)
	}
	if got := FormatVolume(74, UnitOZ); got != "2.5 oz" {
		t.Fatalf("FormatVolume 2.5 oz = %q", got)
	}

	ml, err := ParseVolumeInput("90", UnitML)
	if err != nil || ml != 90 {
		t.Fatalf("ParseVolumeInput ml = %d, %v", ml, err)
	}
	ml, err = ParseVolumeInput("4", UnitOZ)
	if err != nil || ml != 118 {
		t.Fatalf("ParseVolumeInput oz = %d, %v", ml, err)
	}
	if _, err := ParseVolumeInput("0", UnitML); err == nil {
		t.Fatal("expected error for 0 ml")
	}
	if _, err := ParseVolumeInput("2000", UnitML); err == nil {
		t.Fatal("expected error for > MaxVolumeML")
	}
}

func TestValidateBottlePumpSolid(t *testing.T) {
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)

	if err := ValidateBottle(MilkBreast, 90, now, now, ""); err != nil {
		t.Fatalf("valid bottle failed: %v", err)
	}
	if err := ValidateBottle(MilkFormula, 0, now, now, ""); err == nil {
		t.Fatal("expected error for 0 ml bottle")
	}
	if err := ValidateBottle(MilkBreast, 90, now.Add(5*time.Minute), now, ""); err == nil {
		t.Fatal("expected error for future bottle")
	}

	if err := ValidatePump(SideBoth, 110, 15, now, now, ""); err != nil {
		t.Fatalf("valid pump failed: %v", err)
	}
	if err := ValidatePump(SideLeft, 0, 0, now, now, ""); err == nil {
		t.Fatal("expected error when both amount and duration are 0")
	}

	if err := ValidateSolid("Avocado puree", PortionSome, ReactionNone, now, now, ""); err != nil {
		t.Fatalf("valid solid failed: %v", err)
	}
	if err := ValidateSolid("   ", PortionTaste, ReactionEmpty, now, now, ""); err == nil {
		t.Fatal("expected error for empty food")
	}
}
