// App wiring tests for the global security-headers middleware (pentest
// 2026-11-01, finding 6): every response — /api/health, the SPA shell,
// and error responses — must carry X-Content-Type-Options,
// X-Frame-Options, and the CSP, because the middleware sits in the
// router's global chain. The middleware's own behaviour (exact CSP
// string, HSTS on/off) is pinned in internal/middleware; these tests pin
// the wiring.
package app_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Homerus25/podland/backend/internal/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// expectedCSP mirrors the middleware's exact CSP string (pinned there);
// the app test keeps its own copy so a CSP change must consciously
// update both the middleware and its wiring documentation.
const expectedCSP = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'"

func assertSecurityHeaders(t *testing.T, resp *http.Response) {
	t.Helper()
	assert.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"),
		"every response must carry X-Content-Type-Options: nosniff")
	assert.Equal(t, "DENY", resp.Header.Get("X-Frame-Options"),
		"every response must carry X-Frame-Options: DENY")
	assert.Equal(t, expectedCSP, resp.Header.Get("Content-Security-Policy"),
		"every response must carry the exact CSP string")
}

// TestApp_SecurityHeaders_OnEveryResponse pins the global wiring: the
// security-headers middleware must sit in the router's global chain, so
// the headers land on the health endpoint, the SPA shell, an API 404,
// and a JWT-gated route's 401 alike — the finding was that NO response
// carried them, /api/health included.
func TestApp_SecurityHeaders_OnEveryResponse(t *testing.T) {
	cfg := newTestConfig(t)

	a, err := app.New(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = a.Close() })

	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)

	health, err := http.Get(srv.URL + "/api/health")
	require.NoError(t, err, "GET /api/health must connect")
	defer health.Body.Close()
	require.Equal(t, http.StatusOK, health.StatusCode)
	assertSecurityHeaders(t, health)
	assert.Empty(t, health.Header.Get("Strict-Transport-Security"),
		"default config must omit HSTS (HSTS_MAX_AGE_SECONDS=0)")

	spa, err := http.Get(srv.URL + "/")
	require.NoError(t, err, "GET / must connect")
	defer spa.Body.Close()
	require.Equal(t, http.StatusOK, spa.StatusCode)
	assertSecurityHeaders(t, spa)

	apiMiss, err := http.Get(srv.URL + "/api/nonexistent")
	require.NoError(t, err, "GET /api/nonexistent must connect")
	defer apiMiss.Body.Close()
	require.Equal(t, http.StatusNotFound, apiMiss.StatusCode)
	assertSecurityHeaders(t, apiMiss)

	unauthorized, err := http.Get(srv.URL + "/api/machines")
	require.NoError(t, err, "GET /api/machines must connect")
	defer unauthorized.Body.Close()
	require.Equal(t, http.StatusUnauthorized, unauthorized.StatusCode)
	assertSecurityHeaders(t, unauthorized)
}

// TestApp_SecurityHeaders_HSTSFlowsThroughWiring pins the config gate at
// the wiring level: a positive HSTS_MAX_AGE_SECONDS must surface as the
// Strict-Transport-Security header on responses (the middleware's own
// on/off behaviour is pinned in internal/middleware; this test pins that
// the configured value reaches the chain).
func TestApp_SecurityHeaders_HSTSFlowsThroughWiring(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.HSTSMaxAgeSeconds = 31536000

	a, err := app.New(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = a.Close() })

	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/api/health")
	require.NoError(t, err, "GET /api/health must connect")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "max-age=31536000", resp.Header.Get("Strict-Transport-Security"),
		"a positive HSTS_MAX_AGE_SECONDS must surface as the STS header")
	assertSecurityHeaders(t, resp)
}
