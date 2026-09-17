package auth

import (
	"fmt"
	"time"

	"github.com/prawirdani/golang-restapi/internal/apperr"
	"github.com/prawirdani/golang-restapi/pkg/nullable"
)

// ErrInvalidRegistrationToken covers unknown, expired, revoked, and
// already-consumed tokens.
var ErrInvalidRegistrationToken = apperr.UnauthorizedErr(
	"invalid or expired registration token",
	"AUTH_INVALID_REGISTRATION_TOKEN",
)

// RegistrationToken is a single-use, time-boxed invitation to create an
// account. Only the token hash is persisted; the raw value is emailed once.
// UsedAt marks a consumed token; RevokedAt marks one superseded by a newer
// invitation for the same email.
type RegistrationToken struct {
	ID        int                          `json:"id"         db:"id"`
	Name      string                       `json:"name"       db:"name"`  // User Name
	Email     string                       `json:"email"      db:"email"` // User Email
	TokenHash []byte                       `json:"-"          db:"token_hash"`
	CreatedAt time.Time                    `json:"created_at" db:"created_at"`
	ExpiresAt time.Time                    `json:"expires_at" db:"expires_at"`
	UsedAt    nullable.Nullable[time.Time] `json:"used_at"    db:"used_at"`
	RevokedAt nullable.Nullable[time.Time] `json:"revoked_at" db:"revoked_at"`
}

// NewRegistrationToken builds a token for name/email expiring after ttl.
// It returns the token (with hashed value) and the raw value to email.
func NewRegistrationToken(name, email string, ttl time.Duration) (*RegistrationToken, string, error) {
	rawToken, err := GenerateOpaqueToken(32, "regt") // 256-bit token
	if err != nil {
		return nil, "", fmt.Errorf("new registration token: %w", err)
	}
	tokenHash := HashStr(rawToken)
	now := time.Now()

	return &RegistrationToken{
		Name:      name,
		Email:     email,
		TokenHash: tokenHash,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}, rawToken, nil
}

// IsValid reports whether the token is unused, unrevoked, and not yet expired.
func (t RegistrationToken) IsValid() bool {
	return !t.UsedAt.NotNull() &&
		!t.RevokedAt.NotNull() &&
		t.ExpiresAt.After(time.Now())
}

// Revoke marks the token as superseded so it can no longer be used. Distinct
// from Use, which marks a token consumed by a completed registration.
func (t *RegistrationToken) Revoke() {
	t.RevokedAt.Set(time.Now(), false)
}

// Use use the registration token by marking the UsedAt to current time.
// Returns [ErrInvalidRegistrationToken] if token is invalid.
func (t *RegistrationToken) Use() error {
	if !t.IsValid() {
		return ErrInvalidRegistrationToken
	}

	t.UsedAt.Set(time.Now(), false)
	return nil
}
