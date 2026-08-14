// Package auth provides authentication and authorization functionality.
// This package handles user authentication through sessions, access tokens, and
// password management including secure hashing and password reset flows. It manages
// the complete authentication lifecycle from login through logout, including token
// generation, validation, and session management.
package auth

import (
	"context"

	"github.com/google/uuid"
	"github.com/prawirdani/golang-restapi/internal/user"
)

// Repository defines the persistence operations for authentication data.
type Repository interface {
	// StoreSession creates a new session record.
	StoreSession(ctx context.Context, session *Session) error

	// GetSessionByID retrieves a session by its ID.
	//
	// Returns [apperr.ErrNotFound] if no session exists with the given sessionID
	GetSessionByID(ctx context.Context, sessionID uuid.UUID) (*Session, error)

	// GetSessionByRefreshToken retrieves a session by refresh token hash
	//
	// Returns [apperr.ErrNotFound] if no session exists with the given tokenHash
	GetSessionByRefreshTokenHash(ctx context.Context, tokenHash []byte) (*Session, error)

	// UpdateSession updates an existing session.
	// Only updates the refresh_token_hash for rotation and revoked_at.
	// Other fields must stays immutable
	UpdateSession(ctx context.Context, session *Session) error

	// RevokeUserSessions revokes all active sessions for a user.
	RevokeUserSessions(ctx context.Context, userID uuid.UUID) error

	// PruneExpiredUserSessions deletes expired sessions for a user.
	PruneExpiredUserSessions(ctx context.Context, userID uuid.UUID) error

	// StorePasswordRecoveryToken persists new recovery password token.
	StorePasswordRecoveryToken(ctx context.Context, token *PasswordRecoveryToken) error

	// UpdatePasswordRecoveryToken updates an existing password recovery token.
	// Only updates the UsedAt field
	UpdatePasswordRecoveryToken(ctx context.Context, token *PasswordRecoveryToken) error

	// GetPasswordRecoveryToken retrieves a token by its value.
	//
	// Returns [apperr.ErrNotFound] if no token exists with the given tokenHash
	GetPasswordRecoveryToken(ctx context.Context, tokenHash []byte) (*PasswordRecoveryToken, error)
}

type UserRepository user.Repository

// EventProducer defines the contract for publishing notification events.
//
// Implementations are responsible only for publishing the event.
// Email delivery is handled asynchronously by downstream consumers.
type EventProducer interface {
	ProducePasswordRecoveryEvent(ctx context.Context, msg PasswordRecoveryMessage) error
}
