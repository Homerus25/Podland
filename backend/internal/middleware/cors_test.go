package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// corsProbe runs one request through the CORS middleware around a stub
// handler and returns the recorder. The wire header surface is the only
// seam a browser sees, so that is what the tests assert.
func corsProbe(t *testing.T, allowedOrigins []string, method, origin string) *httptest.ResponseRecorder {
	t.Helper()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(method, "/api/hosts", nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	CORS(allowedOrigins)(next).ServeHTTP(w, req)
	return w
}

// TestCORS_OriginDecisionMatrix pins the pentest 2026-11-01 finding 5
// contract: Access-Control-Allow-Credentials is only ever emitted
// together with a specific exactly-matched configured origin — never
// with the wildcard, never when the list is empty (the default fails
// closed to same-origin). The wildcard subtests are the pin that no
// configuration can produce ACAO=* together with ACAC=true.
func TestCORS_OriginDecisionMatrix(t *testing.T) {
	configured := []string{"https://app.example.com", "https://other.example.com"}

	cases := map[string]struct {
		allowedOrigins []string
		method         string
		origin         string
		wantACAO       string // "" means the header must be absent
		wantACAC       bool
		wantVaryOrigin bool
	}{
		"empty list, actual request from any origin": {allowedOrigins: nil, method: "GET", origin: "https://evil.example.com"},
		"empty list, preflight from any origin":      {allowedOrigins: nil, method: "OPTIONS", origin: "https://evil.example.com"},
		"empty list, actual request without origin":  {allowedOrigins: nil, method: "GET"},
		"wildcard, actual request":                   {allowedOrigins: []string{"*"}, method: "GET", origin: "https://evil.example.com", wantACAO: "*"},
		"wildcard, preflight":                        {allowedOrigins: []string{"*"}, method: "OPTIONS", origin: "https://evil.example.com", wantACAO: "*"},
		"configured, exact match, actual request":    {allowedOrigins: configured, method: "GET", origin: "https://app.example.com", wantACAO: "https://app.example.com", wantACAC: true, wantVaryOrigin: true},
		"configured, exact match, preflight":         {allowedOrigins: configured, method: "OPTIONS", origin: "https://app.example.com", wantACAO: "https://app.example.com", wantACAC: true, wantVaryOrigin: true},
		"configured, non-matching origin, actual":    {allowedOrigins: configured, method: "GET", origin: "https://evil.example.com"},
		"configured, non-matching origin, preflight": {allowedOrigins: configured, method: "OPTIONS", origin: "https://evil.example.com"},
		"configured, request without origin header":  {allowedOrigins: configured, method: "GET"},
		"wildcard plus specific, matching specific":  {allowedOrigins: []string{"*", "https://app.example.com"}, method: "GET", origin: "https://app.example.com", wantACAO: "https://app.example.com", wantACAC: true, wantVaryOrigin: true},
		"wildcard plus specific, matching neither":   {allowedOrigins: []string{"*", "https://app.example.com"}, method: "GET", origin: "https://evil.example.com", wantACAO: "*"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			w := corsProbe(t, tc.allowedOrigins, tc.method, tc.origin)

			if tc.wantACAO == "" {
				assert.NotContains(t, w.Header(), "Access-Control-Allow-Origin", "No origin may be allowed here")
			} else {
				assert.Equal(t, tc.wantACAO, w.Header().Get("Access-Control-Allow-Origin"), "Allowed origin must match the decision")
			}

			if tc.wantACAC {
				assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"), "Credentials are only for exactly-matched origins")
			} else {
				assert.NotContains(t, w.Header(), "Access-Control-Allow-Credentials", "Credentials must never ride a wildcard or a non-match")
			}

			if tc.wantVaryOrigin {
				assert.Equal(t, "Origin", w.Header().Get("Vary"), "Reflected origins must be marked cache-varying")
			}
		})
	}
}

// TestCORS_PreflightShortCircuit pins the preflight ritual around the
// origin decision: every OPTIONS gets the 204 short-circuit and the
// unconditional Allow-Methods / Allow-Headers / Max-Age headers — even
// a disallowed origin, which the browser then fails for lack of ACAO —
// while the next handler never runs.
func TestCORS_PreflightShortCircuit(t *testing.T) {
	for _, allowedOrigins := range [][]string{nil, {"*"}, {"https://app.example.com"}} {
		w := corsProbe(t, allowedOrigins, "OPTIONS", "https://evil.example.com")

		assert.Equal(t, http.StatusNoContent, w.Code, "Preflight must short-circuit with 204")
		assert.Equal(t, "GET, POST, PUT, DELETE, PATCH, OPTIONS", w.Header().Get("Access-Control-Allow-Methods"))
		assert.Equal(t, "Origin, Content-Type, Accept, Authorization, Cookie", w.Header().Get("Access-Control-Allow-Headers"))
		assert.Equal(t, "86400", w.Header().Get("Access-Control-Max-Age"))
		assert.Empty(t, w.Body.String(), "Preflight must not reach the next handler")
	}
}

// TestCORS_ActualRequestPassthrough pins that actual requests reach the
// next handler and keep the unconditional Allow-Headers / Expose-Headers
// surface regardless of the origin decision.
func TestCORS_ActualRequestPassthrough(t *testing.T) {
	for _, allowedOrigins := range [][]string{nil, {"*"}, {"https://app.example.com"}} {
		w := corsProbe(t, allowedOrigins, "GET", "https://evil.example.com")

		require.Equal(t, http.StatusOK, w.Code, "Actual requests must reach the next handler")
		assert.Equal(t, "Origin, Content-Type, Accept, Authorization, Cookie", w.Header().Get("Access-Control-Allow-Headers"))
		assert.Equal(t, "Request-Id, X-RateLimit-Limit, X-RateLimit-Remaining, X-RateLimit-Reset", w.Header().Get("Access-Control-Expose-Headers"))
	}
}
