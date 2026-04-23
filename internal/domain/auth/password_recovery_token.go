package auth

import (
	"time"

	"github.com/google/uuid"
	"github.com/prawirdani/golang-restapi/internal/domain"
	"github.com/prawirdani/golang-restapi/pkg/nullable"
)

var ErrInvalidPasswordRecoveryToken = domain.UnauthorizedErr(
	"invalid or expired password recovery token",
	"AUTH_INVALID_RECOV_TOKEN",
)

type PasswordRecoveryToken struct {
	ID        int                          `json:"id"         db:"id"`
	UserID    uuid.UUID                    `json:"user_id"    db:"user_id"`
	TokenHash []byte                       `json:"-"          db:"token_hash"`
	IssuedAt  time.Time                    `json:"issued_at"  db:"issued_at"`
	ExpiresAt time.Time                    `json:"expires_at" db:"expires_at"`
	UsedAt    nullable.Nullable[time.Time] `json:"used_at"    db:"used_at"`
}

// NewPasswordRecoveryToken creates a new token for the given user with a specified expiration.
func NewPasswordRecoveryToken(userID uuid.UUID, ttl time.Duration) (*PasswordRecoveryToken, string, error) {
	token, err := GenerateOpaqueToken(32, "") // 256-bit token
	if err != nil {
		return nil, "", err
	}

	tokenHash := HashStr(token)

	now := time.Now()
	return &PasswordRecoveryToken{
		UserID:    userID,
		TokenHash: tokenHash,
		IssuedAt:  now,
		ExpiresAt: now.Add(ttl),
	}, token, nil
}

// Expired reports whether the token has passed its expiration time.
func (t PasswordRecoveryToken) Expired() bool {
	return t.ExpiresAt.Before(time.Now())
}

// IsUsed reports whether the token has already been used.
func (t PasswordRecoveryToken) IsUsed() bool {
	return t.UsedAt.NotNull()
}

// Use marks the token as used immediately.
func (t *PasswordRecoveryToken) Use() {
	t.UsedAt.Set(time.Now(), false)
}
