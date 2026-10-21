package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"nurtur/internal/account"
	"nurtur/internal/diaper"
	"nurtur/internal/feed"
)

func openUnmigratedStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(context.Background(), Options{
		Path:      filepath.Join(t.TempDir(), "nurtur.db"),
		LocalOnly: true,
		Logger:    discardLogger(),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func legacyMigrations(t *testing.T) fstest.MapFS {
	t.Helper()
	fsys := fstest.MapFS{}
	for _, name := range []string{"0001_init.sql", "0002_phase2_indexes.sql", "0003_accounts.sql"} {
		body, err := os.ReadFile(filepath.Join("../../migrations", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		fsys[name] = &fstest.MapFile{Data: body}
	}
	return fsys
}

func TestScopingMigrationPreservesExistingData(t *testing.T) {
	ctx := context.Background()
	st := openUnmigratedStore(t)
	if err := st.Migrate(ctx, legacyMigrations(t), testNow); err != nil {
		t.Fatalf("legacy migrate: %v", err)
	}

	hash, err := account.HashPassword("correct horse")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	// Simulate an install that already has one account and a realistic week of
	// household data in the pre-scoping schema.
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := st.db.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("seed %q: %v", query, err)
		}
	}
	exec(`INSERT INTO accounts (id, username, password_hash, created_at) VALUES (1, 'wife', ?, ?)`,
		hash, encodeTime(testNow))

	feedOneStart := testNow.Add(-3 * time.Hour)
	feedOneMid := testNow.Add(-150 * time.Minute)
	feedOneEnd := testNow.Add(-2 * time.Hour)
	exec(`INSERT INTO feeds (id, started_at, ended_at, last_side, notes, created_at) VALUES (1, ?, ?, 'right', 'evening cluster', ?)`,
		encodeTime(feedOneStart), encodeTime(feedOneEnd), encodeTime(feedOneStart))
	exec(`INSERT INTO feed_segments (id, feed_id, side, started_at, ended_at) VALUES (1, 1, 'left', ?, ?)`,
		encodeTime(feedOneStart), encodeTime(feedOneMid))
	exec(`INSERT INTO feed_segments (id, feed_id, side, started_at, ended_at) VALUES (2, 1, 'right', ?, ?)`,
		encodeTime(feedOneMid), encodeTime(feedOneEnd))
	exec(`INSERT INTO feeds (id, started_at, created_at) VALUES (2, ?, ?)`,
		encodeTime(testNow.Add(-30*time.Minute)), encodeTime(testNow.Add(-30*time.Minute)))
	exec(`INSERT INTO feed_segments (id, feed_id, side, started_at) VALUES (3, 2, 'left', ?)`,
		encodeTime(testNow.Add(-30*time.Minute)))

	exec(`INSERT INTO diapers (id, logged_at, type, color, consistency, notes, created_at) VALUES (1, ?, 'wet', 'yellow', 'seedy', 'morning change', ?)`,
		encodeTime(testNow.Add(-90*time.Minute)), encodeTime(testNow.Add(-90*time.Minute)))
	exec(`INSERT INTO diapers (id, logged_at, type, color, consistency, notes, created_at) VALUES (2, ?, 'dirty', 'green', 'seedy', 'after lunch', ?)`,
		encodeTime(testNow.Add(-45*time.Minute)), encodeTime(testNow.Add(-45*time.Minute)))

	exec(`INSERT INTO sleeps (id, started_at, ended_at, notes, created_at) VALUES (1, ?, ?, 'long nap', ?)`,
		encodeTime(testNow.Add(-4*time.Hour)), encodeTime(testNow.Add(-3*time.Hour)), encodeTime(testNow.Add(-4*time.Hour)))
	exec(`INSERT INTO sleeps (id, started_at, created_at) VALUES (2, ?, ?)`,
		encodeTime(testNow.Add(-20*time.Minute)), encodeTime(testNow.Add(-20*time.Minute)))

	exec(`INSERT INTO settings (key, value) VALUES ('baby_name', 'Aisyah')`)
	exec(`INSERT INTO settings (key, value) VALUES ('night_start', '20:00')`)
	exec(`INSERT INTO settings (key, value) VALUES ('night_end', '06:00')`)

	if err := st.Migrate(ctx, os.DirFS("../../migrations"), testNow); err != nil {
		t.Fatalf("scoping migrate: %v", err)
	}

	// Every legacy row belongs to the one existing account.
	for _, table := range []string{"feeds", "feed_segments", "diapers", "sleeps", "settings"} {
		var total, scoped int
		if err := st.db.QueryRowContext(ctx,
			`SELECT COUNT(1), COUNT(CASE WHEN account_id = 1 THEN 1 END) FROM `+table).
			Scan(&total, &scoped); err != nil {
			t.Fatalf("%s counts: %v", table, err)
		}
		if total == 0 || total != scoped {
			t.Errorf("%s rows: total=%d scoped=%d, want all scoped", table, total, scoped)
		}
	}

	acct, err := st.AccountByUsername(ctx, "wife")
	if err != nil {
		t.Fatalf("wife account: %v", err)
	}
	if acct.ID != 1 || !account.VerifyPassword(acct.PasswordHash, "correct horse") {
		t.Fatalf("wife login was not preserved: %+v", acct)
	}
	scoped := WithAccount(ctx, acct.ID)

	// Active feed and its open segment survived.
	active, err := st.ActiveFeed(scoped)
	if err != nil || active == nil || active.ID != 2 || len(active.Segments) != 1 {
		t.Fatalf("active feed = %+v, err=%v", active, err)
	}
	if active.Segments[0].Side != feed.SideLeft || !active.Segments[0].Open() {
		t.Fatalf("active segment = %+v", active.Segments[0])
	}

	// The completed feed kept its notes, last side, and both segments.
	completed, err := st.GetFeed(scoped, 1)
	if err != nil || completed == nil {
		t.Fatalf("completed feed = %+v, err=%v", completed, err)
	}
	if completed.Notes != "evening cluster" || completed.LastSide != feed.SideRight || len(completed.Segments) != 2 {
		t.Fatalf("completed feed = %+v", completed)
	}
	if completed.StartedAt != feedOneStart || completed.EndedAt == nil || *completed.EndedAt != feedOneEnd {
		t.Fatalf("completed feed times = %v .. %v", completed.StartedAt, completed.EndedAt)
	}

	// Diapers kept their details and notes.
	lastWet, err := st.LastDiaper(scoped, diaper.Wet)
	if err != nil || lastWet == nil || lastWet.ID != 1 || lastWet.Notes != "morning change" || lastWet.Color != diaper.Color("yellow") {
		t.Fatalf("last wet = %+v, err=%v", lastWet, err)
	}
	recent, err := st.RecentDiapers(scoped, testNow.Add(-24*time.Hour), testNow.Add(time.Hour), 20)
	if err != nil || len(recent) != 2 || recent[0].ID != 2 || recent[0].Notes != "after lunch" {
		t.Fatalf("recent diapers = %+v, err=%v", recent, err)
	}

	// Both sleeps survived, including the open one.
	activeSleep, err := st.ActiveSleep(scoped)
	if err != nil || activeSleep == nil || activeSleep.ID != 2 {
		t.Fatalf("active sleep = %+v, err=%v", activeSleep, err)
	}
	lastSleep, err := st.LastEndedSleep(scoped)
	if err != nil || lastSleep == nil || lastSleep.ID != 1 || lastSleep.Notes != "long nap" {
		t.Fatalf("last ended sleep = %+v, err=%v", lastSleep, err)
	}

	// Settings kept all three values.
	values, err := st.Settings(scoped)
	if err != nil {
		t.Fatalf("settings: %v", err)
	}
	want := map[string]string{"baby_name": "Aisyah", "night_start": "20:00", "night_end": "06:00"}
	for key, value := range want {
		if values[key] != value {
			t.Errorf("setting %s = %q, want %q", key, values[key], value)
		}
	}

	// A second account starts with an empty namespace.
	second, err := st.CreateAccount(ctx, "husband", "hash", testNow)
	if err != nil {
		t.Fatalf("second account: %v", err)
	}
	secondCtx := WithAccount(ctx, second.ID)
	if got, err := st.ActiveFeed(secondCtx); err != nil || got != nil {
		t.Fatalf("second account feed = %+v, err=%v", got, err)
	}
	if got, err := st.RecentDiapers(secondCtx, testNow.Add(-24*time.Hour), testNow.Add(time.Hour), 20); err != nil || len(got) != 0 {
		t.Fatalf("second account diapers = %+v, err=%v", got, err)
	}
	if got, err := st.Settings(secondCtx); err != nil || len(got) != 0 {
		t.Fatalf("second account settings = %+v, err=%v", got, err)
	}
}

func TestSettingsSchemaRepairAfterMissedRename(t *testing.T) {
	ctx := context.Background()
	st := openUnmigratedStore(t)
	if err := st.Migrate(ctx, os.DirFS("../../migrations"), testNow); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := st.db.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("exec %q: %v", query, err)
		}
	}

	exec(`INSERT INTO settings (account_id, key, value) VALUES (0, 'baby_name', 'Aisyah')`)

	// Simulate the Turso sync engine applying migration 0004 except for the
	// final table rewrite: the migration is recorded, settings_scoped holds the
	// rows, and settings does not exist.
	exec(`CREATE TABLE settings_scoped (
		account_id INTEGER NOT NULL DEFAULT 0,
		key        TEXT NOT NULL,
		value      TEXT NOT NULL,
		PRIMARY KEY (account_id, key)
	)`)
	exec(`INSERT INTO settings_scoped (account_id, key, value)
		SELECT account_id, key, value FROM settings`)
	exec(`DROP TABLE settings`)

	// Another startup reapplies migrations (0004 is already recorded) and must
	// repair the missing table before any handler touches it.
	if err := st.Migrate(ctx, os.DirFS("../../migrations"), testNow); err != nil {
		t.Fatalf("repair migrate: %v", err)
	}

	name, ok, err := st.Setting(ctx, "baby_name")
	if err != nil || !ok || name != "Aisyah" {
		t.Fatalf("repaired baby_name = %q, ok=%v, err=%v", name, ok, err)
	}
	if exists, _, err := st.tableShape(ctx, `PRAGMA table_info(settings_scoped)`); err != nil || exists {
		t.Fatalf("settings_scoped exists=%v err=%v, want removed", exists, err)
	}

	// This is the path signup takes; it failed with "no such table: settings"
	// before the repair.
	acct, err := st.CreateAccountWithSettings(ctx, "wife", "hash", map[string]string{
		"baby_name": "Bilal",
	}, testNow)
	if err != nil {
		t.Fatalf("create account after repair: %v", err)
	}
	got, ok, err := st.Setting(WithAccount(ctx, acct.ID), "baby_name")
	if err != nil || !ok || got != "Bilal" {
		t.Fatalf("new account baby_name = %q, ok=%v, err=%v", got, ok, err)
	}
}

func TestFirstAccountClaimsLegacyNamespace(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	// Data created before any account exists lives in account 0.
	if _, err := st.CreateFeed(ctx, "left", testNow); err != nil {
		t.Fatalf("legacy feed: %v", err)
	}
	if err := st.SetSettings(ctx, map[string]string{"baby_name": "Legacy"}); err != nil {
		t.Fatalf("legacy settings: %v", err)
	}

	acct, err := st.CreateAccount(ctx, "wife", "hash", testNow)
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	scoped := WithAccount(ctx, acct.ID)
	if f, err := st.ActiveFeed(scoped); err != nil || f == nil {
		t.Fatalf("claimed feed = %+v, err=%v", f, err)
	}
	if name, ok, err := st.Setting(scoped, "baby_name"); err != nil || !ok || name != "Legacy" {
		t.Fatalf("claimed baby_name = %q, ok=%v, err=%v", name, ok, err)
	}

	second, err := st.CreateAccount(ctx, "husband", "hash", testNow)
	if err != nil {
		t.Fatalf("create second account: %v", err)
	}
	if f, err := st.ActiveFeed(WithAccount(ctx, second.ID)); err != nil || f != nil {
		t.Fatalf("second account inherited legacy data: %+v, err=%v", f, err)
	}
}
