// Package config loads Nurtur's environment configuration.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the fully validated runtime configuration.
type Config struct {
	Addr          string
	DBPath        string
	LocalOnly     bool
	TursoURL      string
	TursoToken    string
	SyncInterval  time.Duration
	Location      *time.Location
	BabyName      string
	LogLevel      slog.Level
	NightStart    time.Duration
	NightEnd      time.Duration
	Passcode      string
	SignupKey     string
	SessionSecret string
	SecureCookie  bool
}

// Load reads configuration from the environment and applies defaults. An
// optional dotenv file is loaded first: .env by default, or the path in
// NURTUR_ENV_FILE. Variables already set in the process environment win.
func Load() (Config, error) {
	envPath := strings.TrimSpace(os.Getenv("NURTUR_ENV_FILE"))
	explicitEnvFile := envPath != ""
	if envPath == "" {
		envPath = ".env"
	}
	if err := loadDotEnv(envPath, explicitEnvFile); err != nil {
		return Config{}, fmt.Errorf("env file %s: %w", envPath, err)
	}

	cfg := Config{
		Addr:         env("NURTUR_ADDR", ":8080"),
		DBPath:       env("NURTUR_DB", "./data/nurtur.db"),
		TursoURL:     strings.TrimSpace(os.Getenv("TURSO_DATABASE_URL")),
		TursoToken:   strings.TrimSpace(os.Getenv("TURSO_AUTH_TOKEN")),
		BabyName:     strings.TrimSpace(os.Getenv("NURTUR_BABY_NAME")),
		SyncInterval: 60 * time.Second,
		Location:     time.Local,
		LogLevel:     slog.LevelInfo,
		NightStart:   19 * time.Hour,
		NightEnd:     7 * time.Hour,
	}

	localOnly, err := parseBool(os.Getenv("NURTUR_LOCAL_ONLY"))
	if err != nil {
		return Config{}, fmt.Errorf("NURTUR_LOCAL_ONLY: %w", err)
	}
	cfg.LocalOnly = localOnly

	secureCookie, err := parseBool(os.Getenv("NURTUR_COOKIE_SECURE"))
	if err != nil {
		return Config{}, fmt.Errorf("NURTUR_COOKIE_SECURE: %w", err)
	}
	cfg.SecureCookie = secureCookie
	cfg.Passcode = strings.TrimSpace(os.Getenv("NURTUR_PASSCODE"))
	cfg.SignupKey = strings.TrimSpace(os.Getenv("NURTUR_SIGNUP_KEY"))
	cfg.SessionSecret = strings.TrimSpace(os.Getenv("NURTUR_SESSION_SECRET"))
	if cfg.Passcode == "" && cfg.SignupKey == "" && cfg.SessionSecret != "" {
		return Config{}, fmt.Errorf("NURTUR_SESSION_SECRET is set but neither NURTUR_PASSCODE nor NURTUR_SIGNUP_KEY is set")
	}

	if raw := strings.TrimSpace(os.Getenv("NURTUR_SYNC_INTERVAL")); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("NURTUR_SYNC_INTERVAL: %w", err)
		}
		if d <= 0 {
			return Config{}, fmt.Errorf("NURTUR_SYNC_INTERVAL must be positive")
		}
		cfg.SyncInterval = d
	}

	if raw := strings.TrimSpace(os.Getenv("NURTUR_TZ")); raw != "" {
		loc, err := time.LoadLocation(raw)
		if err != nil {
			return Config{}, fmt.Errorf("NURTUR_TZ %q: %w", raw, err)
		}
		cfg.Location = loc
	}

	if raw := strings.TrimSpace(os.Getenv("NURTUR_LOG_LEVEL")); raw != "" {
		level, err := parseLevel(raw)
		if err != nil {
			return Config{}, fmt.Errorf("NURTUR_LOG_LEVEL: %w", err)
		}
		cfg.LogLevel = level
	}

	if raw := strings.TrimSpace(os.Getenv("NURTUR_NIGHT_START")); raw != "" {
		d, err := parseClock(raw)
		if err != nil {
			return Config{}, fmt.Errorf("NURTUR_NIGHT_START: %w", err)
		}
		cfg.NightStart = d
	}
	if raw := strings.TrimSpace(os.Getenv("NURTUR_NIGHT_END")); raw != "" {
		d, err := parseClock(raw)
		if err != nil {
			return Config{}, fmt.Errorf("NURTUR_NIGHT_END: %w", err)
		}
		cfg.NightEnd = d
	}
	if cfg.NightStart == cfg.NightEnd {
		return Config{}, fmt.Errorf("NURTUR_NIGHT_START and NURTUR_NIGHT_END must differ")
	}

	if !cfg.LocalOnly {
		if cfg.TursoURL == "" || cfg.TursoToken == "" {
			return Config{}, fmt.Errorf(
				"sync is not configured: set TURSO_DATABASE_URL and TURSO_AUTH_TOKEN, " +
					"or set NURTUR_LOCAL_ONLY=true for local development")
		}
	}

	return cfg, nil
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func parseBool(raw string) (bool, error) {
	if strings.TrimSpace(raw) == "" {
		return false, nil
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("must be true or false")
	}
	return v, nil
}

func parseLevel(raw string) (slog.Level, error) {
	switch strings.ToLower(raw) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("must be one of debug, info, warn, error")
	}
}

// parseClock reads an HH:MM wall-clock value as a duration since midnight.
func parseClock(raw string) (time.Duration, error) {
	parsed, err := time.Parse("15:04", strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("must use HH:MM (for example 19:00)")
	}
	return time.Duration(parsed.Hour())*time.Hour + time.Duration(parsed.Minute())*time.Minute, nil
}
