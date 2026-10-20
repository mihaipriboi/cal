package auth

import (
	"errors"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

// MaxPasswordBytes is bcrypt's input limit; longer passwords are rejected by
// GenerateFromPassword, so they're refused up front with a clear message.
const MaxPasswordBytes = 72

var (
	ErrPasswordTooShort = errors.New("password must be at least 8 characters")
	ErrPasswordTooLong  = errors.New("password must be at most 72 bytes (about 72 plain letters/digits)")
)

// ValidatePassword checks a new password before hashing.
func ValidatePassword(password string) error {
	if utf8.RuneCountInString(password) < 8 {
		return ErrPasswordTooShort
	}
	if len(password) > MaxPasswordBytes {
		return ErrPasswordTooLong
	}
	return nil
}

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	return string(hash), err
}

func VerifyPassword(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
