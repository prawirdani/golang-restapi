package auth

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prawirdani/golang-restapi/internal/audit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func auditCtx(ip net.IP, userAgent string) context.Context {
	return audit.WithContext(context.Background(), audit.Context{
		IP:        ip,
		UserAgent: userAgent,
	})
}

func TestNewSession(t *testing.T) {
	mockUserID := uuid.New()
	mockUserAgent := "user-agent"
	mockIP := net.ParseIP("203.0.113.5")
	mockExpiry := 1 * time.Hour
	ctx := auditCtx(mockIP, mockUserAgent)

	session, refreshToken, err := NewSession(ctx, mockUserID, mockExpiry)
	require.NoError(t, err)

	require.NotEqual(t, uuid.Nil, session.ID)
	assert.Equal(t, mockUserID, session.UserID)
	assert.Equal(t, mockUserAgent, session.UserAgent)
	assert.Equal(t, mockIP, session.IPAddr)
	assert.NotEqual(t, refreshToken, string(session.RefreshTokenHash))
	assert.WithinDuration(t, time.Now().Add(mockExpiry), session.ExpiresAt, 1*time.Second)

	t.Run("Invalid-TTL", func(t *testing.T) {
		_, _, err := NewSession(ctx, mockUserID, -5*time.Minute)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrSessionInvalidTTL)
	})

	t.Run("Invalid-UserID", func(t *testing.T) {
		_, _, err := NewSession(ctx, uuid.Nil, mockExpiry)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrSessionEmptyUID)
	})

	t.Run("Missing-audit-context", func(t *testing.T) {
		_, _, err := NewSession(context.Background(), mockUserID, mockExpiry)
		require.Error(t, err)
		assert.ErrorIs(t, err, audit.ErrCtxNotFound)
	})

	t.Run("Expired", func(t *testing.T) {
		session, _, err := NewSession(ctx, mockUserID, mockExpiry)
		require.NoError(t, err)

		session.ExpiresAt = time.Now().Add(-1 * time.Hour)
		assert.True(t, session.IsExpired())
	})
}

func TestSession_Rotate(t *testing.T) {
	mockUserID := uuid.New()
	mockExpiry := 1 * time.Hour
	ctx := auditCtx(net.ParseIP("203.0.113.5"), "user-agent")

	session, refreshToken, err := NewSession(ctx, mockUserID, mockExpiry)
	require.NoError(t, err)
	prevHash := session.RefreshTokenHash

	newIP := net.ParseIP("198.51.100.7")
	newUA := "new-agent"
	newRefreshToken, err := session.Rotate(auditCtx(newIP, newUA))
	assert.NoError(t, err)
	assert.NotEqual(t, prevHash, session.RefreshTokenHash)
	assert.NotEqual(t, refreshToken, newRefreshToken)
	assert.Equal(t, newUA, session.UserAgent)
	assert.Equal(t, newIP, session.IPAddr)
}
