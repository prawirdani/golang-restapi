// Package revocation defines the consumer-side ports for access-token
// revocation. Implementations are backed by a shared store (e.g. Redis) so a
// revoked token stays rejected across every API instance, not just the one
// that processed the revocation.
package revocation

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Checker reports whether an access token has been revoked.
type Checker interface {
	// IsRevoked reports whether the access token is revoked. issuedAt is the
	// token's iat claim.
	IsRevoked(ctx context.Context, userID, sessionID uuid.UUID, issuedAt time.Time) (bool, error)
}

// Revoker records revocations that Checker must observe.
type Revoker interface {
	// RevokeAllForUser revokes every access token issued to userID at or
	// before the moment of the call, including tokens that were already
	// issued but are still within their TTL.
	RevokeAllForUser(ctx context.Context, userID uuid.UUID) error

	// RevokeSession revokes every access token bound to sessionID, regardless
	// of the user that owns it.
	RevokeSession(ctx context.Context, sessionID uuid.UUID) error
}

// Store is the full revocation contract, combining the read (Checker) and
// write (Revoker) sides. Services that only read should depend on Checker.
type Store interface {
	Checker
	Revoker
}
