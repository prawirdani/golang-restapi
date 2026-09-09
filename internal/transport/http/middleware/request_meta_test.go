package middleware

import (
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func mustCIDRs(t *testing.T, ss ...string) []*net.IPNet {
	t.Helper()
	nets := make([]*net.IPNet, 0, len(ss))
	for _, s := range ss {
		_, n, err := net.ParseCIDR(s)
		if err != nil {
			t.Fatalf("bad CIDR %q: %v", s, err)
		}
		nets = append(nets, n)
	}
	return nets
}

func TestClientIP(t *testing.T) {
	trusted := mustCIDRs(t, "10.0.0.0/8")

	tests := []struct {
		name       string
		remoteAddr string
		headers    map[string]string
		want       string
	}{
		{
			name:       "direct client ignores spoofed header",
			remoteAddr: "203.0.113.5:1234",
			headers:    map[string]string{"X-Forwarded-For": "1.2.3.4"},
			want:       "203.0.113.5",
		},
		{
			name:       "trusted proxy uses forwarded client",
			remoteAddr: "10.0.0.5:8080",
			headers:    map[string]string{"X-Forwarded-For": "198.51.100.7"},
			want:       "198.51.100.7",
		},
		{
			name:       "chain skips trusted hops right-to-left",
			remoteAddr: "10.0.0.9:8080",
			headers:    map[string]string{"X-Forwarded-For": "198.51.100.7, 10.0.0.5, 10.0.0.6"},
			want:       "198.51.100.7",
		},
		{
			name:       "all-trusted chain falls back to X-Real-IP",
			remoteAddr: "10.0.0.9:8080",
			headers: map[string]string{
				"X-Forwarded-For": "10.0.0.5",
				"X-Real-IP":       "198.51.100.9",
			},
			want: "198.51.100.9",
		},
		{
			name:       "no headers falls back to proxy IP",
			remoteAddr: "10.0.0.5:8080",
			want:       "10.0.0.5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := http.NewRequest(http.MethodGet, "/", nil)
			assert.NoError(t, err)
			r.RemoteAddr = tt.remoteAddr
			for k, v := range tt.headers {
				r.Header.Set(k, v)
			}
			assert.Equal(t, tt.want, clientIP(r, trusted))
		})
	}
}
