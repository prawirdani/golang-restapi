// Package auth provides authentication and authorization functionality.
// This package handles user authentication through sessions, access tokens, and
// password management including secure hashing and password reset flows. It manages
// the complete authentication lifecycle from login through logout, including token
// generation, validation, and session management.
package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/prawirdani/golang-restapi/internal/domain"
)

var (
	ErrAccessTokenExpired        = domain.UnauthorizedErr("access token expired", "AUTH_EXPIRED")
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
	jwt.RegisteredClaims
}

// SignAccessToken generates a new JWT for access token
func SignAccessToken(
	secretKey string,
	userID uuid.UUID,
	sessID uuid.UUID,
	ttl time.Duration,
) (string, error) {
	now := time.Now()

	claims := AccessTokenClaims{
		SessionID: sessID,
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
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrAccessTokenExpired
		}
		return nil, fmt.Errorf("parsing token: %w", err)
	}

	if token == nil || !token.Valid {
		return nil, errors.New("invalid access token")
	}

	claims, ok := token.Claims.(*AccessTokenClaims)
	if !ok {
		return nil, errors.New("invalid token claims type")
	}

	uid, err := uuid.Parse(claims.Subject)
	if err != nil {
		return nil, fmt.Errorf("get access token ctx: invalid type of user id: %w", err)
	}
	claims.UserID = uid

	return claims, nil
}

type accessTokenCtxKey struct{}

var atCtx accessTokenCtxKey

// SetAccessTokenCtx sets the access token jwt claims to the context.
func SetAccessTokenCtx(ctx context.Context, claims *AccessTokenClaims) context.Context {
	return context.WithValue(ctx, atCtx, claims)
}

// GetAccessTokenCtx retrieves the access token jwt claims from the context.
func GetAccessTokenCtx(ctx context.Context) (*AccessTokenClaims, error) {
	claims, ok := ctx.Value(atCtx).(*AccessTokenClaims)
	if !ok {
		return nil, ErrAccessTokenClaimsNotFound
	}
	return claims, nil
}
