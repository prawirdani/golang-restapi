// Package auth provides authentication and authorization functionality.
// This package handles user authentication through sessions, access tokens, and
// password management including secure hashing and password reset flows. It manages
// the complete authentication lifecycle from login through logout, including token
// generation, validation, and session management.
package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/prawirdani/golang-restapi/internal/apperr"
	"github.com/prawirdani/golang-restapi/internal/rbac"
)

var (
	ErrAccessTokenExpired        = apperr.UnauthorizedErr("access token expired", "AUTH_EXPIRED")
	ErrAccessTokenInvalid        = apperr.UnauthorizedErr("invalid access token", "AUTH_INVALID")
	ErrAccessTokenClaimsNotFound = errors.New("access token claims not found in context")
)

// AccessTokenClaims represents the JWT claims for an access token.
// UserID is a convenience field populated from the standard 'sub' claim
// as a uuid.UUID for type-safe access within the application.
// SessionID ('sid') identifies the server-side session associated with the token,
// enabling optional revocation or session-specific checks.
// RegisteredClaims contains standard JWT fields like exp, iat, and iss.
type AccessTokenClaims struct {
	UserID    uuid.UUID `json:"-"`
	SessionID uuid.UUID `json:"sid"`
	Role      rbac.Role `json:"role"`
	jwt.RegisteredClaims
}

// SignAccessToken generates a new JWT for access token
func SignAccessToken(
	secretKey string,
	ttl time.Duration,
	userID uuid.UUID,
	sessID uuid.UUID,
	role rbac.Role,
) (string, error) {
	now := time.Now()

	claims := AccessTokenClaims{
		SessionID: sessID,
		Role:      role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secretKey))
}

// VerifyAccessToken parses and validates the token, returning the claims if valid.
// Every rejection except expiry is reported as ErrAccessTokenInvalid so callers
// map to a 401 instead of leaking a malformed-token reason; the underlying cause
// stays in the error chain for server-side logging.
func VerifyAccessToken(secretKey, tokenStr string) (*AccessTokenClaims, error) {
	token, err := jwt.ParseWithClaims(
		tokenStr,
		&AccessTokenClaims{},
		func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(secretKey), nil
		},
		// exp is required: a token without it would never expire, which also
		// makes the revocation TTLs meaningless.
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrAccessTokenExpired
		}
		return nil, fmt.Errorf("%w: %w", ErrAccessTokenInvalid, err)
	}

	if token == nil || !token.Valid {
		return nil, ErrAccessTokenInvalid
	}

	claims, ok := token.Claims.(*AccessTokenClaims)
	if !ok {
		return nil, fmt.Errorf("%w: unexpected claims type", ErrAccessTokenInvalid)
	}

	// iat anchors the user-wide revocation watermark comparison, and sid the
	// per-session denylist; a token missing either cannot be revoked.
	if claims.IssuedAt == nil {
		return nil, fmt.Errorf("%w: missing issued-at claim", ErrAccessTokenInvalid)
	}
	if claims.SessionID == uuid.Nil {
		return nil, fmt.Errorf("%w: missing session id claim", ErrAccessTokenInvalid)
	}

	uid, err := uuid.Parse(claims.Subject)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid subject: %w", ErrAccessTokenInvalid, err)
	}
	claims.UserID = uid

	return claims, nil
}
