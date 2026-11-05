package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// expectedCSP is the exact CSP the SPA needs (pentest 2026-11-01, finding
// 6): Chakra UI v2 / Emotion injects <style> tags at runtime, so
// style-src needs 'unsafe-inline'; scripts are Vite-bundled external
// files, so script-src 'self' holds.
const expectedCSP = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'"

// assertBaseHeaders pins the three always-on headers on a response.
func assertBaseHeaders(t *testing.T, h http.Header) {
	t.Helper()
	assert.Equal(t, "nosniff", h.Get("X-Content-Type-Options"),
		"every response must carry X-Content-Type-Options: nosniff")
	assert.Equal(t, "DENY", h.Get("X-Frame-Options"),
		"every response must carry X-Frame-Options: DENY")
	assert.Equal(t, expectedCSP, h.Get("Content-Security-Policy"),
		"every response must carry the exact CSP string")
}

func TestSecurityHeaders_SetOnSuccessResponse(t *testing.T) {
	handler := SecurityHeaders(0)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assertBaseHeaders(t, w.Header())
}

func TestSecurityHeaders_SetOnErrorResponse(t *testing.T) {
	handler := SecurityHeaders(0)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/hosts", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assertBaseHeaders(t, w.Header())
}

func TestSecurityHeaders_HSTSOmittedWhenMaxAgeZero(t *testing.T) {
	handler := SecurityHeaders(0)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Header().Get("Strict-Transport-Security"),
		"HSTS must be omitted when maxAge is 0 (the default): the backend serves plain HTTP, and RFC 6797 forbids the STS header over plain HTTP")
	assertBaseHeaders(t, w.Header())
}

func TestSecurityHeaders_HSTSPresentWhenMaxAgePositive(t *testing.T) {
	handler := SecurityHeaders(31536000)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "max-age=31536000", w.Header().Get("Strict-Transport-Security"),
		"HSTS must be emitted with the configured max-age when it is > 0")
	assertBaseHeaders(t, w.Header())
}
