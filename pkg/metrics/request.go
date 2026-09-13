package metrics

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"
)

// InstrumentHandler is a Fiber middleware that records Prometheus request
// metrics (duration + count) labelled by route template, method, and status.
//
// resolveStatus maps a handler-returned error to the HTTP status the app's
// ErrorHandler will send. It is required because Fiber runs the ErrorHandler
// only after the middleware chain unwinds, so c.Response().StatusCode() is not
// yet the final status when a handler returned an error.
func (m *Metrics) InstrumentHandler(resolveStatus func(error) int) fiber.Handler {
	return func(c fiber.Ctx) error {
		start := time.Now()

		chainErr := c.Next()

		status := c.Response().StatusCode()
		if chainErr != nil {
			status = resolveStatus(chainErr)
		}

		// Use the matched route template (e.g. "/users/:id") rather than the raw
		// path to keep label cardinality bounded. Fiber reports "/" for unmatched
		// (404) requests, which is itself a fixed low-cardinality label.
		path := "unknown"
		if r := c.Route(); r != nil && r.Path != "" {
			path = r.Path
		}

		duration := time.Since(start).Seconds()
		statusStr := strconv.Itoa(status)
		m.ReqDuration.WithLabelValues(path, c.Method(), statusStr).Observe(duration)
		m.ReqCounter.WithLabelValues(path, c.Method(), statusStr).Inc()

		return chainErr
	}
}
