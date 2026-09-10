package auth

import (
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSession(t *testing.T) {
	mockUserID := uuid.New()
	mockUserAgent := "user-agent"
	mockIP := net.ParseIP("203.0.113.5")
	mockExpiry := 1 * time.Hour

	session, refreshToken, err := NewSession(mockUserID, mockUserAgent, mockIP, mockExpiry)
	require.NoError(t, err)

	require.NotEqual(t, uuid.Nil, session.ID)
	assert.Equal(t, mockUserID, session.UserID)
	assert.Equal(t, mockUserAgent, session.UserAgent)
	assert.Equal(t, mockIP, session.IPAddr)
	assert.NotEqual(t, refreshToken, string(session.RefreshTokenHash))
	assert.WithinDuration(t, time.Now().Add(mockExpiry), session.ExpiresAt, 1*time.Second)

	t.Run("Invalid-TTL", func(t *testing.T) {
		_, _, err := NewSession(mockUserID, mockUserAgent, mockIP, -5*time.Minute)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrSessionInvalidTTL)
	})

	t.Run("Invalid-UserID", func(t *testing.T) {
		_, _, err := NewSession(uuid.Nil, mockUserAgent, mockIP, mockExpiry)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrSessionEmptyUID)
	})

	t.Run("Expired", func(t *testing.T) {
		session, _, err := NewSession(mockUserID, mockUserAgent, mockIP, mockExpiry)
		require.NoError(t, err)

		session.ExpiresAt = time.Now().Add(-1 * time.Hour)
		assert.True(t, session.IsExpired())
	})
}

func TestSession_Rotate(t *testing.T) {
	mockUserID := uuid.New()
	mockUserAgent := "user-agent"
	mockIP := net.ParseIP("203.0.113.5")
	mockExpiry := 1 * time.Hour

	session, refreshToken, err := NewSession(mockUserID, mockUserAgent, mockIP, mockExpiry)
	require.NoError(t, err)
	prevHash := session.RefreshTokenHash

	newMeta := SessionMeta{UserAgent: "new-agent", IPAddr: net.ParseIP("198.51.100.7")}
	newRefreshToken, err := session.Rotate(newMeta)
	assert.NoError(t, err)
	assert.NotEqual(t, prevHash, session.RefreshTokenHash)
	assert.NotEqual(t, refreshToken, newRefreshToken)
	assert.Equal(t, newMeta.UserAgent, session.UserAgent)
	assert.Equal(t, newMeta.IPAddr, session.IPAddr)
}
