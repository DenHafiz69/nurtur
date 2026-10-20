package server

import "testing"

func TestSafeNext(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", "/"},
		{"/", "/"},
		{"/feeds", "/feeds"},
		{"/feeds?side=left", "/feeds?side=left"},
		{"/feeds#night", "/feeds"},
		{"https://evil.example", "/"},
		{"//evil.example", "/"},
		{"/\\evil.example", "/"},
		{"/feeds\\..\\evil", "/"},
		{"feeds", "/"},
	}
	for _, tc := range tests {
		if got := safeNext(tc.in); got != tc.want {
			t.Errorf("safeNext(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
