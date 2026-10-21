// Package account validates local usernames and passwords and hashes
// credentials for storage. Nurtur has no external identity provider: an
// account only exists in this database and is verified on this server.
package account

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	// UsernameMin and UsernameMax bound the normalized username length.
	UsernameMin = 3
	UsernameMax = 32

	// PasswordMin is deliberately modest: this is a private, single-household
	// app, and a long passphrase should not be rejected for lacking symbols.
	PasswordMin = 8
	PasswordMax = 200

	hashPrefix     = "pbkdf2-sha256"
	hashIterations = 210_000
	saltBytes      = 16
	keyBytes       = 32
)

// ErrInvalidUsername and ErrInvalidPassword describe validation failures that
// are safe to show to the person filling in the form.
var (
	ErrInvalidUsername = errors.New("username must be 3–32 characters using letters, numbers, dots, dashes, or underscores")
	ErrInvalidPassword = fmt.Errorf("password must be at least %d characters", PasswordMin)
)

// NormalizeUsername trims and lowercases a username so sign-up and sign-in
// agree regardless of how it was typed. The same value is stored and looked up.
func NormalizeUsername(raw string) (string, error) {
	username := strings.ToLower(strings.TrimSpace(raw))
	if len(username) < UsernameMin || len(username) > UsernameMax {
		return "", ErrInvalidUsername
	}
	for _, r := range username {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '.', r == '-', r == '_':
		default:
			return "", ErrInvalidUsername
		}
	}
	if username[0] == '.' || username[0] == '-' || username[0] == '_' {
		return "", ErrInvalidUsername
	}
	if last := username[len(username)-1]; last == '.' || last == '-' || last == '_' {
		return "", ErrInvalidUsername
	}
	return username, nil
}

// ValidatePassword applies the length rules used at registration time.
func ValidatePassword(raw string) error {
	if len(raw) < PasswordMin || len(raw) > PasswordMax {
		return ErrInvalidPassword
	}
	return nil
}

// HashPassword derives a salted PBKDF2-SHA256 hash. The returned string is
// self-describing so the iteration count can be raised later without breaking
// existing accounts.
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("account: generate salt: %w", err)
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, hashIterations, keyBytes)
	if err != nil {
		return "", fmt.Errorf("account: hash password: %w", err)
	}
	enc := base64.RawStdEncoding
	return fmt.Sprintf("%s$%d$%s$%s", hashPrefix, hashIterations, enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

// VerifyPassword reports whether password matches the stored hash. Malformed
// hashes and mismatches are both plain false: callers treat them the same.
func VerifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != hashPrefix {
		return false
	}
	var iterations int
	if parsed, err := strconv.Atoi(parts[1]); err != nil || parsed < 1 {
		return false
	} else {
		iterations = parsed
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil || len(salt) == 0 {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(want) == 0 {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, iterations, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}
