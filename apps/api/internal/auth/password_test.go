package auth

import (
	"strings"
	"testing"
)

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if !VerifyPassword("password123", hash) {
		t.Fatal("expected password to verify")
	}
	if VerifyPassword("wrong-password", hash) {
		t.Fatal("expected wrong password to fail")
	}
}

func TestValidatePassword(t *testing.T) {
	cases := []struct {
		name string
		pw   string
		want error
	}{
		{"7 chars", "abcdefg", ErrPasswordTooShort},
		{"8 chars", "abcdefgh", nil},
		{"72 bytes", strings.Repeat("a", 72), nil},
		{"73 bytes", strings.Repeat("a", 73), ErrPasswordTooLong},
		{"37 two-byte runes = 74 bytes", strings.Repeat("é", 37), ErrPasswordTooLong},
		{"8 two-byte runes counts as 8 chars", strings.Repeat("é", 8), nil},
	}
	for _, tc := range cases {
		if got := ValidatePassword(tc.pw); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
	// Everything ValidatePassword accepts must hash.
	if _, err := HashPassword(strings.Repeat("a", MaxPasswordBytes)); err != nil {
		t.Fatalf("hash at limit: %v", err)
	}
}
