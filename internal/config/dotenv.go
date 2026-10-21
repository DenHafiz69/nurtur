package config

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// loadDotEnv reads a dotenv file and sets variables that are not already in the
// process environment. Real environment variables always win, so a deployment
// can override a checked-out .env without editing the file.
//
// The format is intentionally small: KEY=value, optional `export`, blank lines,
// # comments, and single- or double-quoted values. Variable expansion and
// command substitution are not supported because secrets should stay literal.
func loadDotEnv(path string, mustExist bool) error {
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) && !mustExist {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		key, value, ok, err := parseDotEnvLine(scanner.Text())
		if err != nil {
			return fmt.Errorf("%s:%d: %w", path, lineNo, err)
		}
		if !ok {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("%s:%d: set %s: %w", path, lineNo, key, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func parseDotEnvLine(raw string) (key, value string, ok bool, err error) {
	line := strings.TrimSpace(strings.TrimPrefix(raw, "\ufeff"))
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false, nil
	}
	if strings.HasPrefix(line, "export ") || strings.HasPrefix(line, "export\t") {
		line = strings.TrimSpace(line[len("export"):])
	}

	key, value, found := strings.Cut(line, "=")
	if !found {
		return "", "", false, errors.New("expected KEY=value")
	}
	key = strings.TrimSpace(key)
	if !validEnvKey(key) {
		return "", "", false, fmt.Errorf("invalid variable name %q", key)
	}
	value = strings.TrimSpace(value)

	if len(value) > 0 && (value[0] == '"' || value[0] == '\'') {
		quote := value[0]
		if len(value) < 2 || value[len(value)-1] != quote {
			return "", "", false, errors.New("unterminated quoted value")
		}
		value = value[1 : len(value)-1]
		if quote == '"' {
			value = unescapeDotEnv(value)
		}
		return key, value, true, nil
	}

	// Strip an unquoted trailing comment: `KEY=value # note`.
	if index := strings.Index(value, " #"); index >= 0 {
		value = strings.TrimSpace(value[:index])
	}
	return key, value, true, nil
}

func validEnvKey(key string) bool {
	if key == "" {
		return false
	}
	for i, r := range key {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case r == '_':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}

func unescapeDotEnv(value string) string {
	var out strings.Builder
	out.Grow(len(value))
	for i := 0; i < len(value); i++ {
		if value[i] != '\\' || i+1 >= len(value) {
			out.WriteByte(value[i])
			continue
		}
		i++
		switch value[i] {
		case 'n':
			out.WriteByte('\n')
		case 'r':
			out.WriteByte('\r')
		case 't':
			out.WriteByte('\t')
		case '"':
			out.WriteByte('"')
		case '\\':
			out.WriteByte('\\')
		default:
			out.WriteByte('\\')
			out.WriteByte(value[i])
		}
	}
	return out.String()
}
