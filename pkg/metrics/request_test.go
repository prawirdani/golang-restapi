package metrics

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resolveStatus mirrors Fiber's default error mapping: a *fiber.Error carries
// its own code, anything else is a 500.
func resolveStatus(err error) int {
	var fe *fiber.Error
	if errors.As(err, &fe) {
		return fe.Code
	}
	return fiber.StatusInternalServerError
}

// newTestApp wires the instrument middleware into a Fiber app with a couple of
// routes so route templates and error statuses can be exercised.
func newTestApp(m *Metrics) *fiber.App {
	app := fiber.New()
	app.Use(m.InstrumentHandler(resolveStatus))
	app.Get("/users/:id", func(c fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})
	app.Get("/boom", func(c fiber.Ctx) error {
		return fiber.NewError(http.StatusTeapot, "boom")
	})
	return app
}

func do(t *testing.T, app *fiber.App, method, target string) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), method, target, nil)
	require.NoError(t, err)
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
}

func TestInstrumentHandler_UsesRouteTemplate(t *testing.T) {
	m := newTestMetrics()
	app := newTestApp(m)

	do(t, app, http.MethodGet, "/users/123")
	do(t, app, http.MethodGet, "/users/456")

	// Both concrete paths collapse onto the "/users/:id" template -> count 2,
	// proving the raw path is not used as a label (which would yield two series).
	got := testutil.ToFloat64(m.ReqCounter.WithLabelValues("/users/:id", http.MethodGet, "200"))
	assert.Equal(t, float64(2), got)

	// The concrete path must NOT exist as its own series.
	raw := testutil.ToFloat64(m.ReqCounter.WithLabelValues("/users/123", http.MethodGet, "200"))
	assert.Equal(t, float64(0), raw)
}

func TestInstrumentHandler_RecordsErrorStatus(t *testing.T) {
	m := newTestMetrics()
	app := newTestApp(m)

	do(t, app, http.MethodGet, "/boom")

	// Handler returned fiber.NewError(418); the status label must reflect it.
	got := testutil.ToFloat64(m.ReqCounter.WithLabelValues("/boom", http.MethodGet, "418"))
	assert.Equal(t, float64(1), got)
}

func TestInstrumentHandler_UnmatchedRoute(t *testing.T) {
	m := newTestMetrics()
	app := newTestApp(m)

	do(t, app, http.MethodGet, "/missing")

	// Fiber reports the "/" template for an unmatched request; the error status
	// (404) must still be captured from the returned fiber.Error.
	got := testutil.ToFloat64(m.ReqCounter.WithLabelValues("/", http.MethodGet, "404"))
	assert.Equal(t, float64(1), got)
}
