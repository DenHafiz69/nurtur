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
		"NURTUR_ENV_FILE",
		"NURTUR_LOCAL_ONLY",
		"TURSO_DATABASE_URL",
		"TURSO_AUTH_TOKEN",
		"NURTUR_SYNC_INTERVAL",
		"NURTUR_TZ",
		"NURTUR_BABY_NAME",
		"NURTUR_NIGHT_START",
		"NURTUR_NIGHT_END",
		"NURTUR_LOG_LEVEL",
		"NURTUR_PASSCODE",
		"NURTUR_SIGNUP_KEY",
		"NURTUR_SESSION_SECRET",
		"NURTUR_COOKIE_SECURE",
		"NURTUR_ADMIN_PASSWORD",
		"NURTUR_PUBLIC_URL",
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
	if !cfg.LocalOnly || cfg.BabyName != "" {
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

func TestLoadPasscodeConfiguration(t *testing.T) {
	clearEnv(t)
	t.Setenv("NURTUR_LOCAL_ONLY", "true")
	t.Setenv("NURTUR_PASSCODE", "let-me-in")
	t.Setenv("NURTUR_SESSION_SECRET", "a-long-random-secret")
	t.Setenv("NURTUR_COOKIE_SECURE", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Passcode != "let-me-in" || cfg.SessionSecret != "a-long-random-secret" || !cfg.SecureCookie {
		t.Fatalf("passcode config = %+v", cfg)
	}

	clearEnv(t)
	t.Setenv("NURTUR_LOCAL_ONLY", "true")
	t.Setenv("NURTUR_SESSION_SECRET", "orphan-secret")
	if _, err := Load(); err == nil {
		t.Fatal("Load accepted a session secret without a passcode or signup key")
	}
}

func TestLoadSignupKeyConfiguration(t *testing.T) {
	clearEnv(t)
	t.Setenv("NURTUR_LOCAL_ONLY", "true")
	t.Setenv("NURTUR_SIGNUP_KEY", "only-for-my-wife")
	t.Setenv("NURTUR_SESSION_SECRET", "a-long-random-secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SignupKey != "only-for-my-wife" || cfg.SessionSecret != "a-long-random-secret" {
		t.Fatalf("signup config = %+v", cfg)
	}
}

func TestLoadAdminConfiguration(t *testing.T) {
	clearEnv(t)
	t.Setenv("NURTUR_LOCAL_ONLY", "true")
	t.Setenv("NURTUR_ADMIN_PASSWORD", "admin-password")
	t.Setenv("NURTUR_SESSION_SECRET", "a-long-random-secret")
	t.Setenv("NURTUR_PUBLIC_URL", "https://nurtur.example.com/")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AdminPassword != "admin-password" {
		t.Fatalf("admin password = %q", cfg.AdminPassword)
	}
	if cfg.PublicURL != "https://nurtur.example.com" {
		t.Fatalf("public URL = %q", cfg.PublicURL)
	}
}

func TestLoadRejectsInvalidAdminValues(t *testing.T) {
	tests := []struct {
		name string
		key  string
		val  string
	}{
		{"short password", "NURTUR_ADMIN_PASSWORD", "short"},
		{"relative public URL", "NURTUR_PUBLIC_URL", "/nurtur"},
		{"public URL without host", "NURTUR_PUBLIC_URL", "https://"},
		{"public URL with query", "NURTUR_PUBLIC_URL", "https://nurtur.example.com?x=1"},
		{"public URL with path", "NURTUR_PUBLIC_URL", "https://nurtur.example.com/app"},
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
