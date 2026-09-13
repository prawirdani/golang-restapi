package http

import (
	"context"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
