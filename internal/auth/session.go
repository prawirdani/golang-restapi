// Package auth provides authentication and authorization functionality.
// This package handles user authentication through sessions, access tokens, and
// password management including secure hashing and password reset flows. It manages
// the complete authentication lifecycle from login through logout, including token
// generation, validation, and session management.
package auth

import (
	"errors"
	"net"
	"time"

	"github.com/google/uuid"
	"github.com/prawirdani/golang-restapi/internal/apperr"
	"github.com/prawirdani/golang-restapi/pkg/nullable"
)

var (
	ErrSessionInvalid    = apperr.UnauthorizedErr("session has been expired or revoked", "AUTH_INVALID_SESSION")
	ErrSessionEmptyUID   = errors.New("user_id must not be empty")
	ErrSessionInvalidTTL = errors.New("session ttl must be greater than 0")
)

// Session represents a single login session for a user
type Session struct {
	ID               uuid.UUID                    `db:"id"`                 // session ID (UUIDv7)
	UserID           uuid.UUID                    `db:"user_id"`            // owner user ID
	RefreshTokenHash []byte                       `db:"refresh_token_hash"` // SHA-256 hash of refresh token
	IPAddr           net.IP                       `db:"ip_addr"`            // client ip
	UserAgent        string                       `db:"user_agent"`         // client info
	AccessedAt       time.Time                    `db:"accessed_at"`        // creation time
	CreatedAt        time.Time                    `db:"created_at"`         // last activity
	ExpiresAt        time.Time                    `db:"expires_at"`         // session expiry
	RevokedAt        nullable.Nullable[time.Time] `db:"revoked_at"`         // revocation timestamp
}

// NewSession creates a new session with a generated refresh token
// Returns session struct (with hashed token) and raw token for client
func NewSession(userID uuid.UUID, userAgent string, ip net.IP, ttl time.Duration) (*Session, string, error) {
	if ttl <= 0 {
		return nil, "", ErrSessionInvalidTTL
	}
	if userID == uuid.Nil {
		return nil, "", ErrSessionEmptyUID
	}

	sessID, err := uuid.NewV7()
	if err != nil {
		return nil, "", err
	}

	// Generate opaque refresh token
	refreshToken, err := generateRefreshToken()
	if err != nil {
		return nil, "", err
	}

	// Hash the token for secure DB storage
	refreshTokenHash := HashStr(refreshToken)

	now := time.Now()
	sess := Session{
		ID:               sessID,
		UserID:           userID,
		UserAgent:        userAgent,
		RefreshTokenHash: refreshTokenHash,
		ExpiresAt:        now.Add(ttl),
		AccessedAt:       now,
		IPAddr:           ip,
	}

	return &sess, refreshToken, nil
}

// IsExpired returns true if the session is past its expiry
func (s Session) IsExpired() bool {
	return s.ExpiresAt.Before(time.Now())
}

// Revoke marks the session as immediately invalid
func (s *Session) Revoke() {
	s.RevokedAt.Set(time.Now(), false)
}

// Rotate generates a new refresh token, updates the session hash,
// and returns the new raw token for the client
func (s *Session) Rotate(meta SessionMeta) (string, error) {
	newToken, err := generateRefreshToken()
	if err != nil {
		return "", err
	}

	// Update hash, access time and meta
	s.RefreshTokenHash = HashStr(newToken)
	s.AccessedAt = time.Now()
	s.IPAddr = meta.IPAddr
	s.UserAgent = meta.UserAgent

	return newToken, nil
}

const refreshTokenBytes = 32 // 256-bit refresh token

// generateRefreshToken creates a high-entropy opaque token
func generateRefreshToken() (string, error) {
	return GenerateOpaqueToken(refreshTokenBytes, "rt")
}
