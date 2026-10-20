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
	Addr         string
	DBPath       string
	LocalOnly    bool
	TursoURL     string
	TursoToken   string
	SyncInterval time.Duration
	Location     *time.Location
	BabyName     string
	LogLevel     slog.Level
}

// Load reads configuration from the environment and applies defaults.
func Load() (Config, error) {
	cfg := Config{
		Addr:         env("NURTUR_ADDR", ":8080"),
		DBPath:       env("NURTUR_DB", "./data/nurtur.db"),
		TursoURL:     strings.TrimSpace(os.Getenv("TURSO_DATABASE_URL")),
		TursoToken:   strings.TrimSpace(os.Getenv("TURSO_AUTH_TOKEN")),
		BabyName:     strings.TrimSpace(os.Getenv("NURTUR_BABY_NAME")),
		SyncInterval: 60 * time.Second,
		Location:     time.Local,
		LogLevel:     slog.LevelInfo,
	}

	localOnly, err := parseBool(os.Getenv("NURTUR_LOCAL_ONLY"))
	if err != nil {
		return Config{}, fmt.Errorf("NURTUR_LOCAL_ONLY: %w", err)
	}
	cfg.LocalOnly = localOnly

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

	if cfg.BabyName == "" {
		cfg.BabyName = "Baby"
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
