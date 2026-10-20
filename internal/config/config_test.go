package config

import (
	"log/slog"
	"testing"
	"time"
)

func clearEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"NURTUR_ADDR",
		"NURTUR_DB",
		"NURTUR_LOCAL_ONLY",
		"TURSO_DATABASE_URL",
		"TURSO_AUTH_TOKEN",
		"NURTUR_SYNC_INTERVAL",
		"NURTUR_TZ",
		"NURTUR_BABY_NAME",
		"NURTUR_NIGHT_START",
		"NURTUR_NIGHT_END",
		"NURTUR_LOG_LEVEL",
	} {
		t.Setenv(key, "")
	}
}

func TestLoadDefaults(t *testing.T) {
	clearEnv(t)
	t.Setenv("NURTUR_LOCAL_ONLY", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Addr != ":8080" || cfg.DBPath != "./data/nurtur.db" {
		t.Fatalf("defaults = %+v", cfg)
	}
	if !cfg.LocalOnly || cfg.BabyName != "Baby" {
		t.Fatalf("defaults = %+v", cfg)
	}
	if cfg.SyncInterval != 60*time.Second || cfg.Location != time.Local || cfg.LogLevel != slog.LevelInfo {
		t.Fatalf("defaults = %+v", cfg)
	}
	if cfg.NightStart != 19*time.Hour || cfg.NightEnd != 7*time.Hour {
		t.Fatalf("night window = %v–%v", cfg.NightStart, cfg.NightEnd)
	}
}

func TestLoadOverrides(t *testing.T) {
	clearEnv(t)
	t.Setenv("NURTUR_ADDR", "127.0.0.1:9001")
	t.Setenv("NURTUR_DB", "/tmp/nurtur.db")
	t.Setenv("NURTUR_LOCAL_ONLY", "true")
	t.Setenv("NURTUR_SYNC_INTERVAL", "15s")
	t.Setenv("NURTUR_TZ", "America/New_York")
	t.Setenv("NURTUR_BABY_NAME", "Pip")
	t.Setenv("NURTUR_LOG_LEVEL", "debug")
	t.Setenv("NURTUR_NIGHT_START", "20:30")
	t.Setenv("NURTUR_NIGHT_END", "06:15")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Addr != "127.0.0.1:9001" || cfg.DBPath != "/tmp/nurtur.db" || cfg.BabyName != "Pip" {
		t.Fatalf("cfg = %+v", cfg)
	}
	if cfg.SyncInterval != 15*time.Second {
		t.Fatalf("sync interval = %v", cfg.SyncInterval)
	}
	if cfg.Location.String() != "America/New_York" {
		t.Fatalf("location = %v", cfg.Location)
	}
	if cfg.LogLevel != slog.LevelDebug {
		t.Fatalf("log level = %v", cfg.LogLevel)
	}
	if cfg.NightStart != 20*time.Hour+30*time.Minute || cfg.NightEnd != 6*time.Hour+15*time.Minute {
		t.Fatalf("night window = %v–%v", cfg.NightStart, cfg.NightEnd)
	}
}

func TestLoadRequiresSyncCredentialsUnlessLocal(t *testing.T) {
	clearEnv(t)
	if _, err := Load(); err == nil {
		t.Fatal("Load succeeded without sync credentials or local-only mode")
	}

	clearEnv(t)
	t.Setenv("NURTUR_LOCAL_ONLY", "true")
	if _, err := Load(); err != nil {
		t.Fatalf("local-only Load failed: %v", err)
	}

	clearEnv(t)
	t.Setenv("TURSO_DATABASE_URL", "libsql://example.turso.io")
	t.Setenv("TURSO_AUTH_TOKEN", "token")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("sync Load: %v", err)
	}
	if cfg.LocalOnly {
		t.Fatal("sync config marked local-only")
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name string
		key  string
		val  string
	}{
		{"timezone", "NURTUR_TZ", "Mars/Olympus"},
		{"sync interval", "NURTUR_SYNC_INTERVAL", "soon"},
		{"negative sync interval", "NURTUR_SYNC_INTERVAL", "-5s"},
		{"bool", "NURTUR_LOCAL_ONLY", "maybe"},
		{"log level", "NURTUR_LOG_LEVEL", "loud"},
		{"night start", "NURTUR_NIGHT_START", "25:00"},
		{"night end", "NURTUR_NIGHT_END", "7pm"},
		{"night window equal", "NURTUR_NIGHT_START", "07:00"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			t.Setenv("NURTUR_LOCAL_ONLY", "true")
			t.Setenv(tt.key, tt.val)
			if _, err := Load(); err == nil {
				t.Fatalf("Load accepted %s=%q", tt.key, tt.val)
			}
		})
	}
}
