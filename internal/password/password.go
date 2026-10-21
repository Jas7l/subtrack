// Package password hashes and checks user passwords with bcrypt. It is shared
// by the auth service (login, password change) and the user service (an admin
// setting a password for a new user), so the length rules live in one place.
package password

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"

	"subtrack/internal/apperr"
)

const (
	// MinLength is the shortest password accepted, in bytes.
	MinLength = 8
	// MaxLength is bcrypt's input limit: it ignores (and x/crypto rejects)
	// anything past 72 bytes, so a longer password is refused up front rather
	// than silently truncated.
	MaxLength = 72
)

// Validate checks the length rules. The returned error wraps
// apperr.ErrInvalidInput, so the transport layer answers 400.
func Validate(pw string) error {
	switch {
	case len(pw) < MinLength:
		return fmt.Errorf("%w: password must be at least %d characters", apperr.ErrInvalidInput, MinLength)
	case len(pw) > MaxLength:
		return fmt.Errorf("%w: password must be at most %d bytes", apperr.ErrInvalidInput, MaxLength)
	}
	return nil
}

// Hash validates pw and returns its bcrypt hash at the default cost.
func Hash(pw string) (string, error) {
	if err := Validate(pw); err != nil {
		return "", err
	}
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("password: hash: %w", err)
	}
	return string(h), nil
}

// Check reports whether pw matches hash. A malformed hash is reported as a
// mismatch: either way the caller must refuse the login.
func Check(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}
