package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSession(t *testing.T) {
	mockUserID := uuid.New()
	mockUserAgent := "user-agent"
	mockExpiry := 1 * time.Hour

	session, refreshToken, err := NewSession(mockUserID, mockUserAgent, mockExpiry)
	require.NoError(t, err)

	require.NotEqual(t, uuid.Nil, session.ID)
	assert.Equal(t, mockUserID, session.UserID)
	assert.Equal(t, mockUserAgent, session.UserAgent)
	assert.NotEqual(t, refreshToken, string(session.RefreshTokenHash))
	assert.WithinDuration(t, time.Now().Add(mockExpiry), session.ExpiresAt, 1*time.Second)

	t.Run("Invalid-TTL", func(t *testing.T) {
		_, _, err := NewSession(mockUserID, mockUserAgent, -5*time.Minute)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrSessionInvalidTTL)
	})

	t.Run("Invalid-UserID", func(t *testing.T) {
		_, _, err := NewSession(uuid.Nil, mockUserAgent, mockExpiry)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrSessionEmptyUID)
	})

	t.Run("Expired", func(t *testing.T) {
		session, _, err := NewSession(mockUserID, mockUserAgent, mockExpiry)
		require.NoError(t, err)

		session.ExpiresAt = time.Now().Add(-1 * time.Hour)
		assert.True(t, session.IsExpired())
	})
}

func TestSession_Rotate(t *testing.T) {
	mockUserID := uuid.New()
	mockUserAgent := "user-agent"
	mockExpiry := 1 * time.Hour

	session, refreshToken, err := NewSession(mockUserID, mockUserAgent, mockExpiry)
	require.NoError(t, err)
	prevHash := session.RefreshTokenHash

	newRefreshToken, err := session.Rotate()
	assert.NoError(t, err)
	assert.NotEqual(t, prevHash, session.RefreshTokenHash)
	assert.NotEqual(t, refreshToken, newRefreshToken)
}
