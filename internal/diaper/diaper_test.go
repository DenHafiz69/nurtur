package diaper

import (
	"errors"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

func TestKindCoversBoth(t *testing.T) {
	tests := []struct {
		kind   Kind
		want   Kind
		covers bool
	}{
		{Wet, Wet, true},
		{Wet, Dirty, false},
		{Dirty, Dirty, true},
		{Dirty, Wet, false},
		{Both, Wet, true},
		{Both, Dirty, true},
		{Both, Both, true},
	}
	for _, tt := range tests {
		if got := tt.kind.Covers(tt.want); got != tt.covers {
			t.Errorf("%s covers %s = %v, want %v", tt.kind, tt.want, got, tt.covers)
		}
	}
}

func TestParseKind(t *testing.T) {
	for _, raw := range []string{"wet", "dirty", "both"} {
		kind, err := ParseKind(raw)
		if err != nil || string(kind) != raw {
			t.Fatalf("ParseKind(%q) = %q, %v", raw, kind, err)
		}
	}
	if _, err := ParseKind("explosive"); !errors.Is(err, ErrInvalidKind) {
		t.Fatalf("invalid kind error = %v", err)
	}
}

func TestSummary(t *testing.T) {
	d := Diaper{Kind: Both, Color: ColorMustard, Consistency: Seedy}
	if got := d.Summary(); got != "Mustard · Seedy" {
		t.Fatalf("Summary = %q", got)
	}
	d = Diaper{Kind: Wet}
	if got := d.Summary(); got != "" {
		t.Fatalf("empty Summary = %q", got)
	}
}

func TestEditValidate(t *testing.T) {
	valid := Edit{LoggedAt: now.Add(-time.Minute), Color: ColorYellow, Consistency: Seedy}
	if err := valid.Validate(now); err != nil {
		t.Fatalf("valid edit rejected: %v", err)
	}

	future := valid
	future.LoggedAt = now.Add(2 * time.Minute)
	if err := future.Validate(now); err == nil {
		t.Fatal("future edit accepted")
	}

	badColor := valid
	badColor.Color = "purple"
	if err := badColor.Validate(now); err == nil {
		t.Fatal("unknown colour accepted")
	}

	long := valid
	long.Notes = string(make([]rune, MaxNotesLength+1))
	if err := long.Validate(now); err == nil {
		t.Fatal("overlong note accepted")
	}
}
