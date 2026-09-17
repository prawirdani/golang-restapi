package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRegistrationToken(t *testing.T) {
	token, raw, err := NewRegistrationToken("John Doe", "john@example.com", time.Hour)
	require.NoError(t, err)

	assert.NotEmpty(t, raw)
	assert.Equal(t, HashStr(raw), token.TokenHash)
	assert.Equal(t, "John Doe", token.Name)
	assert.Equal(t, "john@example.com", token.Email)
	assert.True(t, token.IsValid())
}

func TestRegistrationToken_Use(t *testing.T) {
	token, _, err := NewRegistrationToken("John", "j@example.com", time.Hour)
	require.NoError(t, err)

	require.NoError(t, token.Use())
	assert.True(t, token.UsedAt.NotNull())
	assert.False(t, token.IsValid())

	// A consumed token cannot be used again.
	assert.ErrorIs(t, token.Use(), ErrInvalidRegistrationToken)
}

func TestRegistrationToken_Revoke(t *testing.T) {
	token, _, err := NewRegistrationToken("John", "j@example.com", time.Hour)
	require.NoError(t, err)

	token.Revoke()
	assert.True(t, token.RevokedAt.NotNull())
	assert.False(t, token.IsValid(), "a revoked token must not be valid")

	// Revoked is distinct from used, and blocks Use.
	assert.False(t, token.UsedAt.NotNull())
	assert.ErrorIs(t, token.Use(), ErrInvalidRegistrationToken)
}

func TestRegistrationToken_IsValid_Expired(t *testing.T) {
	token, _, err := NewRegistrationToken("John", "j@example.com", -time.Hour)
	require.NoError(t, err)

	assert.False(t, token.IsValid())
}
