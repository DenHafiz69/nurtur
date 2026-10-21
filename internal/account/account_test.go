package account

import (
	"errors"
	"testing"
)

func TestNormalizeUsername(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"  Wife  ", "wife", false},
		{"mama.bear", "mama.bear", false},
		{"nur-2", "nur-2", false},
		{"ab", "", true},
		{"has space", "", true},
		{"-leading", "", true},
		{"trailing_", "", true},
		{"emoji🙂", "", true},
	}
	for _, tc := range tests {
		got, err := NormalizeUsername(tc.in)
		if tc.wantErr {
			if !errors.Is(err, ErrInvalidUsername) {
				t.Errorf("NormalizeUsername(%q) error = %v, want ErrInvalidUsername", tc.in, err)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("NormalizeUsername(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
}

func TestHashPasswordRoundTrip(t *testing.T) {
	encoded, err := HashPassword("a-long-passphrase")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if encoded == "a-long-passphrase" {
		t.Fatal("password was stored in plain text")
	}
	if !VerifyPassword(encoded, "a-long-passphrase") {
		t.Fatal("correct password did not verify")
	}
	if VerifyPassword(encoded, "a-long-passphrasf") {
		t.Fatal("wrong password verified")
	}
	if VerifyPassword("garbage", "a-long-passphrase") {
		t.Fatal("malformed hash verified")
	}
}

func TestHashPasswordUsesFreshSalt(t *testing.T) {
	first, err := HashPassword("same-password")
	if err != nil {
		t.Fatal(err)
	}
	second, err := HashPassword("same-password")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("two hashes of the same password should not match")
	}
}
