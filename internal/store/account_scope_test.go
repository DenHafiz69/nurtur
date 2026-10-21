package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"nurtur/internal/diaper"
	"nurtur/internal/feed"
)

func TestAccountDataIsolation(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	wife, err := st.CreateAccount(ctx, "wife", "hash-one", testNow)
	if err != nil {
		t.Fatalf("create wife: %v", err)
	}
	husband, err := st.CreateAccount(ctx, "husband", "hash-two", testNow)
	if err != nil {
		t.Fatalf("create husband: %v", err)
	}
	wifeCtx := WithAccount(ctx, wife.ID)
	husbandCtx := WithAccount(ctx, husband.ID)

	wifeFeed, err := st.CreateFeed(wifeCtx, feed.SideLeft, testNow)
	if err != nil {
		t.Fatalf("wife feed: %v", err)
	}
	wifeDiaper, _, err := st.CreateDiaper(wifeCtx, diaper.Wet, testNow)
	if err != nil {
		t.Fatalf("wife diaper: %v", err)
	}
	if _, err := st.StartSleep(wifeCtx, testNow); err != nil {
		t.Fatalf("wife sleep: %v", err)
	}
	if err := st.SetSettings(wifeCtx, map[string]string{
		"baby_name":   "Aisyah",
		"night_start": "20:00",
	}); err != nil {
		t.Fatalf("wife settings: %v", err)
	}

	// The other account starts empty, even while the first account has a live
	// feed, diaper, sleep, and settings.
	if got, err := st.ActiveFeed(husbandCtx); err != nil || got != nil {
		t.Fatalf("husband active feed = %+v, err=%v", got, err)
	}
	if got, err := st.LastDiaper(husbandCtx, diaper.Wet); err != nil || got != nil {
		t.Fatalf("husband last diaper = %+v, err=%v", got, err)
	}
	if got, err := st.ActiveSleep(husbandCtx); err != nil || got != nil {
		t.Fatalf("husband active sleep = %+v, err=%v", got, err)
	}
	if name, ok, err := st.Setting(husbandCtx, "baby_name"); err != nil || ok {
		t.Fatalf("husband baby_name = %q, ok=%v, err=%v", name, ok, err)
	}
	entries, err := st.TimelineBetween(husbandCtx, testNow.Add(-time.Hour), testNow.Add(time.Hour), 50)
	if err != nil || len(entries) != 0 {
		t.Fatalf("husband timeline = %d entries, err=%v", len(entries), err)
	}

	// IDs are not a way around the scope: another account cannot read, edit,
	// or delete the first account's rows.
	if _, err := st.GetFeed(husbandCtx, wifeFeed.ID); !errors.Is(err, feed.ErrNotFound) {
		t.Fatalf("cross-account GetFeed error = %v", err)
	}
	if err := st.DeleteFeed(husbandCtx, wifeFeed.ID, testNow); !errors.Is(err, feed.ErrNotFound) {
		t.Fatalf("cross-account DeleteFeed error = %v", err)
	}
	if _, _, err := st.RestoreDiaper(husbandCtx, wifeDiaper.ID); !errors.Is(err, diaper.ErrNotFound) {
		t.Fatalf("cross-account RestoreDiaper error = %v", err)
	}

	// Each account can run its own feed and sleep at the same time.
	if _, err := st.CreateFeed(husbandCtx, feed.SideRight, testNow); err != nil {
		t.Fatalf("husband feed while wife feeds: %v", err)
	}
	if _, err := st.StartSleep(husbandCtx, testNow); err != nil {
		t.Fatalf("husband sleep while wife sleeps: %v", err)
	}

	wifeName, _, err := st.Setting(wifeCtx, "baby_name")
	if err != nil || wifeName != "Aisyah" {
		t.Fatalf("wife baby_name = %q, err=%v", wifeName, err)
	}
	wifeStart, _, err := st.Setting(wifeCtx, "night_start")
	if err != nil || wifeStart != "20:00" {
		t.Fatalf("wife night_start = %q, err=%v", wifeStart, err)
	}
}
