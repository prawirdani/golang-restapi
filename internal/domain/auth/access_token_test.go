package auth

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const secret = "secret"

func TestSignAccessToken(t *testing.T) {
	uid := uuid.New()
	sid := uuid.New()

	token, err := SignAccessToken(secret, uid, sid, time.Minute*5)
	assert.NoError(t, err)
	assert.NotEmpty(t, token)
}

func TestVerifyAccessToken(t *testing.T) {
	uid := uuid.New()
	sid := uuid.New()
	token, err := SignAccessToken(secret, uid, sid, time.Minute*5)
	require.NoError(t, err)

	claims, err := VerifyAccessToken(secret, token)
	require.NoError(t, err)
	assert.Equal(t, uid, claims.UserID)

	t.Run("Expired", func(t *testing.T) {
		uid := uuid.New()
		sid := uuid.New()

		token, err := SignAccessToken(secret, uid, sid, -time.Minute*1)
		require.NoError(t, err)

		_, err = VerifyAccessToken(secret, token)
		assert.Error(t, err)
		assert.Equal(t, ErrAccessTokenExpired, err)
	})

	t.Run("Invalid", func(t *testing.T) {
		claims, err := VerifyAccessToken(secret, "invalid-token-string")
		assert.Error(t, err)
		assert.Nil(t, claims)
	})
}

func TestAccessTokenContext(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockClaims := &AccessTokenClaims{}

		ctx := SetAccessTokenCtx(context.Background(), mockClaims)
		claims, err := GetAccessTokenCtx(ctx)
		assert.NoError(t, err)
		assert.Equal(t, claims, claims)
	})

	t.Run("ctx-not-exist", func(t *testing.T) {
		claims, err := GetAccessTokenCtx(context.Background())
		assert.Error(t, err)
		assert.Nil(t, claims)
		assert.Equal(t, ErrAccessTokenClaimsNotFound, err)
	})
}
