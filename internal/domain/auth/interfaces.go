// Package auth provides authentication and authorization functionality.
// This package handles user authentication through sessions, access tokens, and
// password management including secure hashing and password reset flows. It manages
// the complete authentication lifecycle from login through logout, including token
// generation, validation, and session management.
package auth

import (
	"context"

	"github.com/google/uuid"
	"github.com/prawirdani/golang-restapi/internal/domain/user"
)

// Repository defines the persistence operations for authentication data.
type Repository interface {
	// StoreSession creates a new session record.
	StoreSession(ctx context.Context, session *Session) error

	// GetSessionByID retrieves a session by its ID.
	//
	// Returns [domain.ErrNotFound] if no session exists with the given sessionID
	GetSessionByID(ctx context.Context, sessionID uuid.UUID) (*Session, error)

	// GetSessionByRefreshToken retrieves a session by refresh token hash
	//
	// Returns [domain.ErrNotFound] if no session exists with the given tokenHash
	GetSessionByRefreshTokenHash(ctx context.Context, tokenHash []byte) (*Session, error)

	// UpdateSession updates an existing session.
	// Only updates the refresh_token_hash for rotation and revoked_at.
	// Other fields must stays immutable
	UpdateSession(ctx context.Context, session *Session) error

	// StorePasswordRecoveryToken persists new recovery password token.
	StorePasswordRecoveryToken(ctx context.Context, token *PasswordRecoveryToken) error

	// UpdatePasswordRecoveryToken updates an existing password recovery token.
	// Only updates the UsedAt field
	UpdatePasswordRecoveryToken(ctx context.Context, token *PasswordRecoveryToken) error

	// GetPasswordRecoveryToken retrieves a token by its value.
	//
	// Returns [domain.ErrNotFound] if no token exists with the given tokenHash
	GetPasswordRecoveryToken(ctx context.Context, tokenHash []byte) (*PasswordRecoveryToken, error)
}

type UserRepository user.Repository

// MessagePublisher defines the contract for publishing authentication-related
// messages to external systems (e.g., message queues, event buses).
// This enables asynchronous processing of notifications and events.
type MessagePublisher interface {
	// SendPasswordRecoveryEmail publishes a message to trigger a password recovery email.
	// The message contains the user's information and raw password recovery token that will be
	// consumed by an email service worker.
	// Returns an error if the message cannot be published to the queue.
	SendPasswordRecoveryEmail(ctx context.Context, msg PasswordRecoveryEmailMessage) error
}
