package middleware

import (
	"net"
	"net/http"
	"slices"
	"strings"

	"github.com/prawirdani/golang-restapi/internal/audit"
)

// RequestMeta captures ambient request metadata (client IP and user-agent) and
// stores it in the context for the audit recorder to consume automatically.
//
// The client IP is resolved safely: forwarded headers (X-Forwarded-For,
// X-Real-IP) are honored ONLY when the direct peer is a configured trusted
// proxy. Otherwise RemoteAddr is used, so a directly-connected client cannot
// spoof the header.
func RequestMeta(trustedProxies []*net.IPNet) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			meta := audit.RequestMeta{
				IP:        clientIP(r, trustedProxies),
				UserAgent: r.UserAgent(),
			}
			ctx := audit.WithRequestMeta(r.Context(), meta)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// clientIP resolves the originating client IP. See [RequestMeta].
func clientIP(r *http.Request, trusted []*net.IPNet) string {
	remoteIP := parseIP(r.RemoteAddr)

	// Direct connection (peer not trusted): ignore forwarded headers.
	if !isTrusted(remoteIP, trusted) {
		return remoteIP.String()
	}

	// Trusted proxy: walk X-Forwarded-For right-to-left, skipping trusted hops;
	// the first untrusted entry is the client.
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		for _, part := range slices.Backward(parts) {
			ip := parseIP(part)
			if ip == nil {
				continue
			}
			if !isTrusted(ip, trusted) {
				return ip.String()
			}
		}
	}

	// Fall back to X-Real-IP for single-proxy setups that set it directly.
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		if ip := parseIP(xri); ip != nil {
			return ip.String()
		}
	}

	return remoteIP.String()
}

// parseIP extracts an IP from a "host:port" remote address or a bare IP string.
func parseIP(s string) net.IP {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if host, _, err := net.SplitHostPort(s); err == nil {
		s = host
	}
	return net.ParseIP(s)
}

func isTrusted(ip net.IP, trusted []*net.IPNet) bool {
	if ip == nil {
		return false
	}
	for _, n := range trusted {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
