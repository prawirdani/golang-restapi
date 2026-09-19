package http

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/prawirdani/golang-restapi/internal/auth"
	"github.com/prawirdani/golang-restapi/internal/rbac"
	sharedMocks "github.com/prawirdani/golang-restapi/internal/testing/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const authTestSecret = "test-secret-for-middleware-tests-0123456789"

// serve runs a request through SecurityHeaders. When secure is true, the
// request advertises X-Forwarded-Proto: https over a trust-proxy app, which is
// how helmet decides to emit HSTS.
func serve(t *testing.T, production, secure bool) http.Header {
	t.Helper()

	// TrustProxy + a matching proxy entry so X-Forwarded-Proto is honored
	// (app.Test presents a 0.0.0.0 remote address).
	app := fiber.New(fiber.Config{
		TrustProxy:       true,
		TrustProxyConfig: fiber.TrustProxyConfig{Proxies: []string{"0.0.0.0"}},
	})
	app.Use(SecurityHeaders(production))
	app.Get("/", func(c fiber.Ctx) error { return c.SendStatus(http.StatusOK) })

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	require.NoError(t, err)
	if secure {
		req.Header.Set("X-Forwarded-Proto", "https")
	}

	resp, err := app.Test(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	return resp.Header
}

func TestSecurityHeaders(t *testing.T) {
	t.Run("baseline headers always set", func(t *testing.T) {
		h := serve(t, false, false)
		assert.Equal(t, "nosniff", h.Get("X-Content-Type-Options"))
		assert.Equal(t, "DENY", h.Get("X-Frame-Options"))
		assert.Equal(t, "strict-origin-when-cross-origin", h.Get("Referrer-Policy"))
		assert.Equal(t, "default-src 'none'; frame-ancestors 'none'", h.Get("Content-Security-Policy"))
		assert.NotEmpty(t, h.Get("Permissions-Policy"))
	})

	t.Run("HSTS only in production over a secure request", func(t *testing.T) {
		assert.NotEmpty(t, serve(t, true, true).Get("Strict-Transport-Security"))
		assert.Empty(t, serve(t, false, true).Get("Strict-Transport-Security"))
		assert.Empty(t, serve(t, true, false).Get("Strict-Transport-Security"))
	})
}

// newAuthApp mounts the authenticator in front of a 200 handler. The
// authenticated handler is never expected to run on failure paths.
func newAuthApp(checker *sharedMocks.Checker, failClosed bool) *fiber.App {
	// Mirror NewRouter's ErrorHandler so apperr/Error sentinels map to their
	// real HTTP status instead of Fiber's default 500.
	app := fiber.New(fiber.Config{
		ErrorHandler: func(c fiber.Ctx, err error) error {
			e := ParseError(err)
			return c.Status(e.Status()).JSON(map[string]any{"error": e})
		},
	})
	am := NewAuthenticatorMiddleware(authTestSecret, checker, nil, failClosed)
	app.Use(am.Authenticate)
	app.Get("/", func(c fiber.Ctx) error { return c.SendStatus(http.StatusOK) })
	return app
}

// doAuthRequest drives an authenticated GET; an empty token sends no
// Authorization header.
func doAuthRequest(t *testing.T, app *fiber.App, token string) int {
	t.Helper()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	require.NoError(t, err)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := app.Test(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	return resp.StatusCode
}

func TestAuthenticate(t *testing.T) {
	signToken := func(t *testing.T, uid, sid uuid.UUID) string {
		t.Helper()
		token, err := auth.SignAccessToken(authTestSecret, time.Minute, uid, sid, rbac.RoleAdmin)
		require.NoError(t, err)
		return token
	}

	t.Run("valid token with live checker passes", func(t *testing.T) {
		checker := sharedMocks.NewChecker(t)
		t.Cleanup(func() { checker.AssertExpectations(t) })
		uid, sid := uuid.New(), uuid.New()

		checker.EXPECT().IsRevoked(mock.Anything, uid, sid, mock.Anything).Return(false, nil).Once()

		app := newAuthApp(checker, false)
		assert.Equal(t, http.StatusOK, doAuthRequest(t, app, signToken(t, uid, sid)))
	})

	t.Run("revoked token is rejected", func(t *testing.T) {
		checker := sharedMocks.NewChecker(t)
		t.Cleanup(func() { checker.AssertExpectations(t) })
		uid, sid := uuid.New(), uuid.New()

		checker.EXPECT().IsRevoked(mock.Anything, uid, sid, mock.Anything).Return(true, nil).Once()

		app := newAuthApp(checker, false)
		assert.Equal(t, http.StatusUnauthorized, doAuthRequest(t, app, signToken(t, uid, sid)))
	})

	t.Run("checker error fails open by default", func(t *testing.T) {
		checker := sharedMocks.NewChecker(t)
		t.Cleanup(func() { checker.AssertExpectations(t) })
		uid, sid := uuid.New(), uuid.New()

		checker.EXPECT().IsRevoked(mock.Anything, uid, sid, mock.Anything).Return(false, assert.AnError).Once()

		app := newAuthApp(checker, false)
		assert.Equal(t, http.StatusOK, doAuthRequest(t, app, signToken(t, uid, sid)))
	})

	t.Run("checker error fails closed when configured", func(t *testing.T) {
		checker := sharedMocks.NewChecker(t)
		t.Cleanup(func() { checker.AssertExpectations(t) })
		uid, sid := uuid.New(), uuid.New()

		checker.EXPECT().IsRevoked(mock.Anything, uid, sid, mock.Anything).Return(false, assert.AnError).Once()

		app := newAuthApp(checker, true)
		assert.Equal(t, http.StatusUnauthorized, doAuthRequest(t, app, signToken(t, uid, sid)))
	})

	t.Run("revoked with checker error fails closed", func(t *testing.T) {
		checker := sharedMocks.NewChecker(t)
		t.Cleanup(func() { checker.AssertExpectations(t) })
		uid, sid := uuid.New(), uuid.New()

		checker.EXPECT().IsRevoked(mock.Anything, uid, sid, mock.Anything).Return(true, assert.AnError).Once()

		app := newAuthApp(checker, true)
		assert.Equal(t, http.StatusUnauthorized, doAuthRequest(t, app, signToken(t, uid, sid)))
	})

	t.Run("garbage token never reaches the checker", func(t *testing.T) {
		checker := sharedMocks.NewChecker(t)
		t.Cleanup(func() { checker.AssertExpectations(t) })

		app := newAuthApp(checker, false)
		assert.Equal(t, http.StatusUnauthorized, doAuthRequest(t, app, "not-a-real-token"))
		checker.AssertNotCalled(t, "IsRevoked")
	})

	t.Run("missing token never reaches the checker", func(t *testing.T) {
		checker := sharedMocks.NewChecker(t)
		t.Cleanup(func() { checker.AssertExpectations(t) })

		app := newAuthApp(checker, false)
		assert.Equal(t, http.StatusUnauthorized, doAuthRequest(t, app, ""))
		checker.AssertNotCalled(t, "IsRevoked")
	})
}
