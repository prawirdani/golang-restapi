package config

import (
	"fmt"
	"net"
	"os"
	"strings"
)

// Proxy holds trusted-reverse-proxy configuration for client IP resolution.
type Proxy struct {
	// TrustedProxies is a comma-separated list of CIDRs or single IPs whose
	// X-Forwarded-For / X-Real-IP headers are trusted. Requests arriving
	// directly (peer not in this list) ignore forwarded headers entirely, so
	// the header cannot be spoofed by clients.
	TrustedProxies []*net.IPNet
}

func (p *Proxy) Parse() error {
	if val := os.Getenv("TRUSTED_PROXIES"); val != "" {
		proxiesStr := strings.Split(val, ",")
		nets := make([]*net.IPNet, 0, len(p.TrustedProxies))

		for _, s := range proxiesStr {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			if _, ipnet, err := net.ParseCIDR(s); err == nil {
				nets = append(nets, ipnet)
				continue
			}
			ip := net.ParseIP(s)
			if ip == nil {
				return fmt.Errorf("invalid TRUSTED_PROXIES entry %q: not an IP or CIDR", s)
			}

			bits := 128
			if ip.To4() != nil {
				bits = 32
			}
			nets = append(nets, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
		}

		p.TrustedProxies = nets
	}
	return nil
}
