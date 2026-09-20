package identity

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

// ParseCIDRs parses a comma-separated list of CIDR ranges, e.g.
// "127.0.0.1/32,::1/128,10.0.0.0/8". Bare IPs are accepted and treated as
// /32 or /128.
func ParseCIDRs(s string) ([]*net.IPNet, error) {
	var out []*net.IPNet
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !strings.Contains(part, "/") {
			ip := net.ParseIP(part)
			if ip == nil {
				return nil, fmt.Errorf("identity: invalid IP %q", part)
			}
			bits := 32
			if ip.To4() == nil {
				bits = 128
			}
			out = append(out, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
			continue
		}
		_, n, err := net.ParseCIDR(part)
		if err != nil {
			return nil, fmt.Errorf("identity: invalid CIDR %q: %w", part, err)
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		return nil, errors.New("identity: no CIDRs supplied")
	}
	return out, nil
}
