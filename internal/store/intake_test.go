package store

import (
	"context"
	"testing"
	"time"

	"nurtur/internal/intake"
	"nurtur/internal/timeline"
)

func TestIntakeCRUDAndTimeline(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)

	// Bottle: quick-tap duplicate window
	b1, created, err := s.CreateBottle(ctx, intake.MilkBreast, 90, now, now, "", true)
	if err != nil || !created {
		t.Fatalf("CreateBottle b1: created=%v err=%v", created, err)
	}
	b2, created, err := s.CreateBottle(ctx, intake.MilkBreast, 90, now.Add(time.Second), now.Add(time.Second), "", true)
	if err != nil || created || b2.ID != b1.ID {
		t.Fatalf("expected duplicate tap to return b1: id=%d created=%v err=%v", b2.ID, created, err)
	}

	if _, err := s.UpdateBottle(ctx, b1.ID, intake.MilkFormula, 120, now.Add(-5*time.Minute), now, "warmed"); err != nil {
		t.Fatalf("UpdateBottle: %v", err)
	}

	// Pump
	p1, err := s.CreatePump(ctx, intake.SideBoth, 110, 15, now.Add(-20*time.Minute), now, "morning")
	if err != nil {
		t.Fatalf("CreatePump: %v", err)
	}

	// Solid
	sol1, err := s.CreateSolid(ctx, "Avocado puree", intake.PortionSome, intake.ReactionNone, now.Add(-30*time.Minute), now, "liked it")
	if err != nil {
		t.Fatalf("CreateSolid: %v", err)
	}

	// TimelineBetween & DaySummary
	dayStart := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
	dayEnd := dayStart.Add(24 * time.Hour)
	entries, err := s.TimelineBetween(ctx, dayStart, dayEnd, 50)
	if err != nil {
		t.Fatalf("TimelineBetween: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 timeline entries, got %d", len(entries))
	}
	if entries[0].Kind != timeline.Bottle || entries[1].Kind != timeline.Pump || entries[2].Kind != timeline.Solid {
		t.Fatalf("unexpected order: %+v", entries)
	}

	sum, err := s.TimelineDaySummary(ctx, dayStart, dayEnd, now)
	if err != nil {
		t.Fatalf("TimelineDaySummary: %v", err)
	}
	if sum.Bottles != 1 || sum.BottleML != 120 || sum.Pumps != 1 || sum.PumpML != 110 || sum.Solids != 1 {
		t.Fatalf("unexpected summary: %+v", sum)
	}

	// Delete & Restore
	if _, err := s.DeleteBottle(ctx, b1.ID, now); err != nil {
		t.Fatalf("DeleteBottle: %v", err)
	}
	if _, err := s.GetBottle(ctx, b1.ID); err != intake.ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
	if _, err := s.RestoreBottle(ctx, b1.ID); err != nil {
		t.Fatalf("RestoreBottle: %v", err)
	}

	// Account scoping
	ctx2 := WithAccount(ctx, 42)
	entries2, err := s.TimelineBetween(ctx2, dayStart, dayEnd, 50)
	if err != nil {
		t.Fatalf("TimelineBetween account 42: %v", err)
	}
	if len(entries2) != 0 {
		t.Fatalf("expected 0 entries for account 42, got %d", len(entries2))
	}
	if _, err := s.GetPump(ctx2, p1.ID); err != intake.ErrNotFound {
		t.Fatalf("expected ErrNotFound for cross-account pump, got %v", err)
	}
	if _, err := s.GetSolid(ctx2, sol1.ID); err != intake.ErrNotFound {
		t.Fatalf("expected ErrNotFound for cross-account solid, got %v", err)
	}
}
