package http

import (
	"context"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/prawirdani/golang-restapi/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMetricsMiddleware_ResolvesDomainErrorStatus guards the ordering gotcha:
// Fiber runs the app ErrorHandler only after the middleware chain unwinds, so
// the metrics middleware must resolve the status via ParseError rather than
// reading c.Response().StatusCode() (which is still 200 at that point).
func TestMetricsMiddleware_ResolvesDomainErrorStatus(t *testing.T) {
	// Build unregistered vectors to avoid clashing with the global registry.
	m := &metrics.Metrics{
		ReqDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{Name: "test_duration"}, []string{"path", "method", "status_code"}),
		ReqCounter: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "test_total"}, []string{"path", "method", "status_code"}),
	}

	app := fiber.New(fiber.Config{
		ErrorHandler: func(c fiber.Ctx, err error) error {
			e := ParseError(err)
			return c.Status(e.Status()).JSON(map[string]any{"error": e})
		},
	})
	app.Use(m.InstrumentHandler(func(err error) int { return ParseError(err).Status() }))
	app.Get("/secure", func(c fiber.Ctx) error { return ErrReqUnauthorized })

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/secure", nil)
	require.NoError(t, err)
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	got := testutil.ToFloat64(m.ReqCounter.WithLabelValues("/secure", http.MethodGet, "401"))
	assert.Equal(t, float64(1), got)
}
