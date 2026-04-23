package middleware

import (
	"net/http"
	"time"

	m "github.com/go-chi/httprate"
	httpx "github.com/prawirdani/golang-restapi/internal/transport/http"
)

func RateLimit(reqLimit int, windowLength time.Duration) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return m.Limit(reqLimit, windowLength,
			m.WithKeyFuncs(m.KeyByIP, m.KeyByEndpoint),
			m.WithLimitHandler(httpx.Handler(func(ctx *httpx.Context) error { return httpx.ErrRateLimit })),
		)(next)
	}
}
