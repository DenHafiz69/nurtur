package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseDotEnvLine(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		key       string
		value     string
		skip      bool
		wantError bool
	}{
		{name: "plain", in: "NURTUR_TZ=Asia/Kuala_Lumpur", key: "NURTUR_TZ", value: "Asia/Kuala_Lumpur"},
		{name: "spaces", in: "  NURTUR_BABY_NAME = Pip  ", key: "NURTUR_BABY_NAME", value: "Pip"},
		{name: "export", in: "export NURTUR_ADDR=:8080", key: "NURTUR_ADDR", value: ":8080"},
		{name: "comment", in: "# NURTUR_TZ=ignored", skip: true},
		{name: "blank", in: "   ", skip: true},
		{name: "double quotes", in: `KEY="a b"`, key: "KEY", value: "a b"},
		{name: "single quotes", in: `KEY='a # b'`, key: "KEY", value: "a # b"},
		{name: "escaped newline", in: `KEY="line\nnext"`, key: "KEY", value: "line\nnext"},
		{name: "inline comment", in: "KEY=value # note", key: "KEY", value: "value"},
		{name: "hash in value", in: "KEY=abc#def", key: "KEY", value: "abc#def"},
		{name: "empty value", in: "KEY=", key: "KEY", value: ""},
		{name: "missing equals", in: "KEY", wantError: true},
		{name: "bad key", in: "1KEY=value", wantError: true},
		{name: "unterminated quote", in: `KEY="value`, wantError: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			key, value, parsed, err := parseDotEnvLine(tc.in)
			if tc.wantError {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseDotEnvLine: %v", err)
			}
			if tc.skip {
				if parsed {
					t.Fatal("expected the line to be skipped")
				}
				return
			}
			if !parsed {
				t.Fatal("line was skipped")
			}
			if key != tc.key || value != tc.value {
				t.Fatalf("got %q=%q, want %q=%q", key, value, tc.key, tc.value)
			}
		})
	}
}

func TestLoadDotEnvSetsOnlyMissingValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	content := "NURTUR_BABY_NAME=Pip\nNURTUR_TZ=Asia/Kuala_Lumpur\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	clearEnv(t)
	t.Setenv("NURTUR_BABY_NAME", "FromEnvironment")
	if err := os.Unsetenv("NURTUR_TZ"); err != nil {
		t.Fatal(err)
	}
	if err := loadDotEnv(path, true); err != nil {
		t.Fatalf("loadDotEnv: %v", err)
	}
	if got := os.Getenv("NURTUR_BABY_NAME"); got != "FromEnvironment" {
		t.Fatalf("real env was overwritten: %q", got)
	}
	if got := os.Getenv("NURTUR_TZ"); got != "Asia/Kuala_Lumpur" {
		t.Fatalf("NURTUR_TZ = %q", got)
	}
}

func TestLoadUsesEnvFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	content := "NURTUR_LOCAL_ONLY=true\nNURTUR_BABY_NAME=Pip\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	clearEnv(t)
	t.Setenv("NURTUR_ENV_FILE", path)
	if err := os.Unsetenv("NURTUR_LOCAL_ONLY"); err != nil {
		t.Fatal(err)
	}
	if err := os.Unsetenv("NURTUR_BABY_NAME"); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.LocalOnly || cfg.BabyName != "Pip" {
		t.Fatalf("config from .env = %+v", cfg)
	}
}

func TestLoadIgnoresMissingDefaultEnvFile(t *testing.T) {
	clearEnv(t)
	t.Setenv("NURTUR_LOCAL_ONLY", "true")

	// Tests run in the package directory, which has no .env; Load must still
	// succeed when the optional file is absent.
	if _, err := Load(); err != nil {
		t.Fatalf("Load without .env: %v", err)
	}
}

func TestLoadRejectsMissingExplicitEnvFile(t *testing.T) {
	clearEnv(t)
	t.Setenv("NURTUR_LOCAL_ONLY", "true")
	t.Setenv("NURTUR_ENV_FILE", filepath.Join(t.TempDir(), "missing.env"))

	if _, err := Load(); err == nil {
		t.Fatal("Load accepted a missing NURTUR_ENV_FILE")
	}
}
