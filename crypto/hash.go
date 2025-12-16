package crypto

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword hashes a plain text password using bcrypt with a cost of 12.
// Cost 12 is the current sweet spot between security and latency (~200-300ms).
func HashPassword(password string) (string, error) {
	if password == "" {
		return "", errors.New("crypto: password cannot be empty")
	}
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return "", fmt.Errorf("crypto: failed to hash password: %w", err)
	}
	return string(bytes), nil
}

// CheckPassword compares a bcrypt hash with a plaintext password.
// Returns true if they match, false otherwise.
func CheckPassword(hash, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
