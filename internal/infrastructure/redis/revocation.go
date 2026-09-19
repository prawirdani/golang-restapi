package redis

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/prawirdani/golang-restapi/internal/ports/revocation"
	"github.com/redis/go-redis/v9"
)

const (
	revokedUserKeyPrefix    = "revoked:user:"
	revokedSessionKeyPrefix = "revoked:sess:"

	// revocationSkew widens both the marker TTL and the watermark comparison
	// so a token minted moments after RevokeAllForUser, but with an iat that
	// rounds before the watermark, is still rejected. Over-revoking the few
	// seconds around the watermark is the safe direction.
	revocationSkew = 5 * time.Second
)

// RevocationStore implements [revocation.Store] on Redis. Markers only ever
// hold a timestamp or a presence flag — never a raw JWT or any secret.
type RevocationStore struct {
	client *redis.Client
	jwtTTL time.Duration
}

var _ revocation.Store = (*RevocationStore)(nil)

// NewRevocationStore constructs a RevocationStore over the given Redis client.
// jwtTTL is the access-token lifetime; it bounds how long revocation markers
// must be kept.
func NewRevocationStore(client *redis.Client, jwtTTL time.Duration) *RevocationStore {
	return &RevocationStore{
		client: client,
		jwtTTL: jwtTTL,
	}
}

// revocationTTL returns how long a revocation marker must live: the access
// token's remaining lifetime plus the skew margin. A non-positive jwtTTL means
// no access token can still be valid, so no marker is needed and 0 is
// returned.
func revocationTTL(jwtTTL time.Duration) time.Duration {
	if jwtTTL <= 0 {
		return 0
	}
	return jwtTTL + revocationSkew
}

func revokedUserKey(userID uuid.UUID) string {
	return revokedUserKeyPrefix + userID.String()
}

func revokedSessionKey(sessionID uuid.UUID) string {
	return revokedSessionKeyPrefix + sessionID.String()
}

// revokedByWatermark reports whether a token issued at issuedAt predates the
// revocation watermark. Extending the revoked window revocationSkew PAST the
// watermark compensates for cross-instance clock skew and over-revokes, the
// safe direction. Cost: a brand-new login within skew of a revocation is
// briefly rejected too — deliberate, and preferable to a token issued just
// before the revocation surviving.
func revokedByWatermark(issuedAt time.Time, watermarkUnixNano int64) bool {
	watermark := time.Unix(0, watermarkUnixNano)
	return !issuedAt.After(watermark.Add(revocationSkew))
}

// markerValue extracts the i-th MGet result, reporting whether it was present.
func markerValue(values []any, i int) (string, bool) {
	if i >= len(values) || values[i] == nil {
		return "", false
	}
	switch v := values[i].(type) {
	case string:
		return v, true
	case []byte:
		return string(v), true
	default:
		return fmt.Sprint(v), true
	}
}

// decide maps a token's revocation markers to a verdict. A present session
// marker wins outright. Otherwise the user watermark is parsed and compared;
// a corrupt or non-positive watermark is an error rather than a silent
// "not revoked", so a bad value fails loudly instead of failing open.
func decide(userMarker, sessionMarker string, issuedAt time.Time) (bool, error) {
	if sessionMarker != "" {
		return true, nil
	}
	if userMarker == "" {
		return false, nil
	}

	watermark, err := strconv.ParseInt(userMarker, 10, 64)
	if err != nil {
		return false, fmt.Errorf("parse revocation watermark: %w", err)
	}
	if watermark <= 0 {
		return false, fmt.Errorf("invalid revocation watermark: %d", watermark)
	}

	return revokedByWatermark(issuedAt, watermark), nil
}

// IsRevoked implements [revocation.Checker]. It reads both markers in a single
// MGet and delegates the decision to [decide].
func (s *RevocationStore) IsRevoked(
	ctx context.Context,
	userID, sessionID uuid.UUID,
	issuedAt time.Time,
) (bool, error) {
	values, err := s.client.MGet(
		ctx,
		revokedUserKey(userID),
		revokedSessionKey(sessionID),
	).Result()
	if err != nil {
		return false, fmt.Errorf("get revocation markers: %w", err)
	}

	userMarker, _ := markerValue(values, 0)
	sessionMarker, _ := markerValue(values, 1)

	return decide(userMarker, sessionMarker, issuedAt)
}

// RevokeAllForUser implements [revocation.Revoker].
func (s *RevocationStore) RevokeAllForUser(
	ctx context.Context,
	userID uuid.UUID,
) error {
	ttl := revocationTTL(s.jwtTTL)
	if ttl == 0 {
		return nil
	}

	if err := s.client.Set(
		ctx,
		revokedUserKey(userID),
		strconv.FormatInt(time.Now().UnixNano(), 10),
		ttl,
	).Err(); err != nil {
		return fmt.Errorf("revoke user tokens: %w", err)
	}
	return nil
}

// RevokeSession implements [revocation.Revoker].
func (s *RevocationStore) RevokeSession(
	ctx context.Context,
	sessionID uuid.UUID,
) error {
	ttl := revocationTTL(s.jwtTTL)
	if ttl == 0 {
		return nil
	}

	if err := s.client.Set(
		ctx,
		revokedSessionKey(sessionID),
		"1",
		ttl,
	).Err(); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}
