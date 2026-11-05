package hosts

import (
	"context"
	"net"
	"net/netip"

	"github.com/Homerus25/podland/backend/internal/errors"
)

// HostPolicy is the hosts domain's opt-in egress block on the
// registration/update probe target (pentest 2026-11-01 finding 1): a
// host registration dials any user-supplied hostname:port from the
// Podland server's network vantage point, so deployments that must not
// reach internal ranges (the 169.254.169.254 cloud-metadata endpoint
// included) can reject blocked hosts before any dial.
//
// The zero value is disabled — a no-op that resolves nothing. Podland's
// primary use case is LAN/localhost podman hosts, so the block is
// opt-in (HOST_BLOCK_PRIVATE_RANGES, default false) and a
// service constructed without WithHostPolicy behaves exactly as before.
type HostPolicy struct {
	enabled  bool
	resolver hostResolver
}

// hostResolver is the policy's single I/O seam: resolve a hostname to the
// IPs the dial would target. Production wires the default resolver over
// net.DefaultResolver; tests swap the field — the range table and the
// fail-closed contract are pinned without touching the network.
type hostResolver func(ctx context.Context, hostname string) ([]netip.Addr, error)

// defaultResolve resolves a hostname through the standard library's
// default resolver. IP literals never reach it (they are checked
// directly), so the A/AAAA answer is the dial's real target set.
func defaultResolve(ctx context.Context, hostname string) ([]netip.Addr, error) {
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, hostname)
	if err != nil {
		return nil, err
	}
	addrs := make([]netip.Addr, 0, len(ips))
	for _, ip := range ips {
		if addr, ok := netip.AddrFromSlice(ip.IP); ok {
			addrs = append(addrs, addr.Unmap())
		}
	}
	return addrs, nil
}

// NewHostPolicy returns the host policy for the given enabled flag,
// wired over the default resolver. NewHostPolicy(false) is the zero
// value: a disabled no-op.
func NewHostPolicy(enabled bool) HostPolicy {
	if !enabled {
		return HostPolicy{}
	}
	return HostPolicy{enabled: true, resolver: defaultResolve}
}

// blockedRanges is the block list: loopback, RFC1918, link-local (v4 and
// v6 — the v4 half covers the 169.254.169.254 cloud-metadata endpoint),
// CGNAT, IPv6 ULA, and the unspecified v4 range. Every resolved IP (or
// a literal) is checked against each prefix.
var blockedRanges = []netip.Prefix{
	netip.MustParsePrefix("127.0.0.0/8"),    // loopback v4
	netip.MustParsePrefix("::1/128"),        // loopback v6
	netip.MustParsePrefix("10.0.0.0/8"),     // RFC1918
	netip.MustParsePrefix("172.16.0.0/12"),  // RFC1918
	netip.MustParsePrefix("192.168.0.0/16"), // RFC1918
	netip.MustParsePrefix("169.254.0.0/16"), // link-local v4 (cloud metadata)
	netip.MustParsePrefix("fe80::/10"),      // link-local v6
	netip.MustParsePrefix("100.64.0.0/10"),  // CGNAT
	netip.MustParsePrefix("fc00::/7"),       // IPv6 ULA
	netip.MustParsePrefix("0.0.0.0/8"),      // unspecified v4
}

// Check applies the policy to a candidate hostname. Disabled, it is a
// no-op that resolves nothing. Enabled, an IP literal is checked
// directly (no DNS); a hostname is resolved and every answered IP is
// checked — one blocked address rejects the host. An unresolvable
// hostname fails closed (HOST_NOT_ALLOWED): a name that does not resolve
// now could point at a blocked range later, so the block must not be
// bypassable by registering a name ahead of its DNS record.
func (p HostPolicy) Check(ctx context.Context, hostname string) error {
	if !p.enabled {
		return nil
	}

	if addr, err := netip.ParseAddr(hostname); err == nil {
		return p.checkAddr(addr.Unmap())
	}

	addrs, err := p.resolver(ctx, hostname)
	if err != nil || len(addrs) == 0 {
		return errors.ErrHostNotAllowed
	}
	for _, addr := range addrs {
		if err := p.checkAddr(addr.Unmap()); err != nil {
			return err
		}
	}
	return nil
}

// checkAddr answers ErrHostNotAllowed when the address falls in any
// blocked range.
func (p HostPolicy) checkAddr(addr netip.Addr) error {
	for _, prefix := range blockedRanges {
		if prefix.Contains(addr) {
			return errors.ErrHostNotAllowed
		}
	}
	return nil
}

// WithHostPolicy injects the host policy and returns the service — the
// WithMaxUploadBytes builder pattern (internal/app wires it from
// HOST_BLOCK_PRIVATE_RANGES at startup). The policy runs in
// Create right before the probe and in Update whenever a hostname is
// set, so a blocked host can never be persisted.
func (s *HostService) WithHostPolicy(policy HostPolicy) *HostService {
	s.hostPolicy = policy
	return s
}
