// Package auth provides authentication and authorization functionality.
// This package handles user authentication through sessions, access tokens, and
// password management including secure hashing and password reset flows. It manages
// the complete authentication lifecycle from login through logout, including token
// generation, validation, and session management.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	"github.com/prawirdani/golang-restapi/internal/apperr"
	"golang.org/x/crypto/bcrypt"
)

var ErrWrongCredentials = apperr.UnauthorizedErr("check your credentials", "AUTH_CREDENTIALS")

// bcryptCost is the work factor used for password hashing.
const bcryptCost = 12

// dummyHash is a precomputed bcrypt hash used to equalize login timing when
// the target user does not exist, preventing user enumeration via timing.
var dummyHash = func() []byte {
	hashed, err := bcrypt.GenerateFromPassword([]byte("timing-equalizer"), bcryptCost)
	if err != nil {
		panic(err)
	}
	return hashed
}()

// HashPassword generates a bcrypt hash from a plaintext password
func HashPassword(plain string) ([]byte, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(plain), bcryptCost)
	if err != nil {
		return nil, err
	}
	return hashedPassword, nil
}

// VerifyPassword compares a plaintext password with a bcrypt hash
func VerifyPassword(plain, hashed string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hashed), []byte(plain))
	if err != nil {
		return ErrWrongCredentials
	}
	return nil
}

// DummyVerify runs a bcrypt comparison against a precomputed dummy hash to
// equalize response time on the user-not-found login path.
func DummyVerify() {
	_ = bcrypt.CompareHashAndPassword(dummyHash, []byte("x"))
}

// HashStr computes a SHA-256 hash of the input string
func HashStr(str string) []byte {
	sum := sha256.Sum256([]byte(str))
	return sum[:]
}

// GenerateOpaqueToken creates a high-entropy, URL-safe token
// - size: number of random bytes (e.g., 32 = 256-bit)
// - prefix: optional string prepended to the token
func GenerateOpaqueToken(size int, prefix string) (string, error) {
	if size <= 0 {
		return "", fmt.Errorf("invalid token size: %d", size)
	}

	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate opaque token: %w", err)
	}

	token := base64.RawURLEncoding.EncodeToString(b)

	if prefix != "" {
		return fmt.Sprintf("%s_%s", prefix, token), nil
	}
	return token, nil
}
