package hosts

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeResolver builds a resolver seam that answers the given IPs (or the
// given error) for every hostname — the host policy's only I/O, swapped
// in tests so the policy's range logic is pinned without touching the
// network.
func fakeResolver(ips []netip.Addr, err error) hostResolver {
	return func(context.Context, string) ([]netip.Addr, error) { return ips, err }
}

// newTestPolicy builds an enabled policy over the fake resolver.
func newTestPolicy(ips []netip.Addr, err error) HostPolicy {
	return HostPolicy{enabled: true, resolver: fakeResolver(ips, err)}
}

// TestHostPolicy_LiteralIPs pins the blocked-range table for IP literals
// (pentest 2026-11-01 finding 1): loopback, RFC1918, link-local (including
// the 169.254.169.254 cloud-metadata endpoint), CGNAT, IPv6 ULA, IPv6
// link-local, and the unspecified range are rejected; public addresses
// pass. A literal is checked directly — the resolver seam must not run.
func TestHostPolicy_LiteralIPs(t *testing.T) {
	blocked := []string{
		"127.0.0.1",              // loopback v4
		"127.255.255.254",        // loopback v4, non-localhost member
		"10.0.0.1",               // RFC1918 10/8
		"172.16.0.1",             // RFC1918 172.16/12
		"172.31.255.254",         // RFC1918 172.16/12 upper edge
		"192.168.1.1",            // RFC1918 192.168/16
		"169.254.169.254",        // link-local — the cloud-metadata endpoint
		"fe80::1",                // link-local v6
		"100.64.0.1",             // CGNAT 100.64/10
		"100.127.255.254",        // CGNAT upper edge
		"fc00::1",                // IPv6 ULA fc00::/7
		"fd12:3456:789a::1",      // IPv6 ULA, fd half of fc00::/7
		"0.0.0.0",                // unspecified 0.0.0.0/8
		"::ffff:127.0.0.1",       // v4-mapped loopback
		"::ffff:169.254.169.254", // v4-mapped link-local
	}
	allowed := []string{
		"8.8.8.8",                  // public v4
		"1.2.3.4",                  // public v4
		"100.128.0.1",              // just past the CGNAT range
		"172.32.0.1",               // just past RFC1918 172.16/12
		"2001:db8::1",              // public v6 (documentation range, not ULA)
		"2a00:1450:4001:81b::200e", // public v6
	}

	// The resolver is poisoned: a literal must be checked directly, so
	// any resolver invocation fails the test loudly.
	policy := newTestPolicy(nil, errors.New("resolver must not run for IP literals"))

	for _, ip := range blocked {
		err := policy.Check(context.Background(), ip)
		var appErr *appErrors.AppError
		require.ErrorAs(t, err, &appErr, "blocked literal %s must answer HOST_NOT_ALLOWED", ip)
		assert.Equal(t, "HOST_NOT_ALLOWED", appErr.Code, "blocked literal %s", ip)
		assert.Equal(t, http.StatusBadRequest, appErr.Status, "blocked literal %s", ip)
	}
	for _, ip := range allowed {
		assert.NoError(t, policy.Check(context.Background(), ip), "public literal %s must pass", ip)
	}
}

// TestHostPolicy_HostnameResolution pins the DNS-resolution half of the
// policy: every resolved IP is checked (one blocked address in the answer
// set rejects the host), a name resolving only to public IPs passes, and
// an unresolvable name fails closed — the block must not open a "register
// a name that fails to resolve now, point it at 169.254.169.254 later"
// bypass.
func TestHostPolicy_HostnameResolution(t *testing.T) {
	t.Run("hostname resolving to any blocked IP is rejected", func(t *testing.T) {
		policy := newTestPolicy([]netip.Addr{
			mustAddr(t, "93.184.216.34"),
			mustAddr(t, "169.254.169.254"),
		}, nil)

		err := policy.Check(context.Background(), "metadata.example.com")
		var appErr *appErrors.AppError
		require.ErrorAs(t, err, &appErr)
		assert.Equal(t, "HOST_NOT_ALLOWED", appErr.Code)
		assert.Equal(t, http.StatusBadRequest, appErr.Status)
	})

	t.Run("hostname resolving only to public IPs passes", func(t *testing.T) {
		policy := newTestPolicy([]netip.Addr{
			mustAddr(t, "93.184.216.34"),
			mustAddr(t, "2606:2800:220:1:248:1893:25c8:1946"),
		}, nil)

		assert.NoError(t, policy.Check(context.Background(), "example.com"))
	})

	t.Run("unresolvable hostname fails closed", func(t *testing.T) {
		policy := newTestPolicy(nil, errors.New("no such host"))

		err := policy.Check(context.Background(), "no-such-host.example.com")
		var appErr *appErrors.AppError
		require.ErrorAs(t, err, &appErr,
			"an unresolvable hostname must fail closed with HOST_NOT_ALLOWED")
		assert.Equal(t, "HOST_NOT_ALLOWED", appErr.Code)
		assert.Equal(t, http.StatusBadRequest, appErr.Status)
	})

	t.Run("empty resolution answer fails closed", func(t *testing.T) {
		policy := newTestPolicy(nil, nil)

		err := policy.Check(context.Background(), "empty.example.com")
		var appErr *appErrors.AppError
		require.ErrorAs(t, err, &appErr,
			"a hostname resolving to zero IPs must fail closed")
		assert.Equal(t, "HOST_NOT_ALLOWED", appErr.Code)
	})
}

// TestHostPolicy_Disabled pins the opt-in contract: the zero value (and
// NewHostPolicy(false)) is a no-op that never resolves the hostname —
// with the block off, registration adds no DNS lookup and no new failure
// modes.
func TestHostPolicy_Disabled(t *testing.T) {
	resolverRan := false
	policy := HostPolicy{enabled: false, resolver: func(context.Context, string) ([]netip.Addr, error) {
		resolverRan = true
		return nil, nil
	}}

	assert.NoError(t, policy.Check(context.Background(), "127.0.0.1"),
		"disabled policy must not block a loopback literal")
	assert.NoError(t, policy.Check(context.Background(), "metadata.example.com"),
		"disabled policy must not block any hostname")
	assert.False(t, resolverRan,
		"disabled policy must not resolve anything — no DNS resolution is added when the block is off")

	assert.Equal(t, HostPolicy{}, NewHostPolicy(false),
		"NewHostPolicy(false) is the zero value: disabled")
}

// mustAddr parses a literal for the test tables.
func mustAddr(t *testing.T, s string) netip.Addr {
	t.Helper()
	addr, err := netip.ParseAddr(s)
	require.NoError(t, err)
	return addr
}
