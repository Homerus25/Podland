package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingHandler records downstream handler invocations and answers 200,
// standing in for the login/refresh handlers the auth limiter guards.
func countingHandler(calls *int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls++
		w.WriteHeader(http.StatusOK)
	})
}

// requestFromIP builds a request that appears to come from the given
// client address, the way the net/http server populates RemoteAddr.
func requestFromIP(method, target, remoteAddr string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	req.RemoteAddr = remoteAddr
	return req
}

// TestAuthRateLimitMiddleware_UnderLimitPassesThrough asserts requests
// below the per-IP budget reach the handler and carry the same
// X-RateLimit-* headers the per-user middleware sets.
func TestAuthRateLimitMiddleware_UnderLimitPassesThrough(t *testing.T) {
	limiter := NewRateLimiter(3, time.Minute)
	var calls int
	handler := AuthRateLimitMiddleware(limiter)(countingHandler(&calls))

	for i := 0; i < 3; i++ {
		req := requestFromIP(http.MethodPost, "/api/auth/login", "203.0.113.10:5555")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Result().StatusCode,
			"Request %d within the budget should reach the handler", i+1)
		assert.Equal(t, "3", w.Result().Header.Get("X-RateLimit-Limit"),
			"Allowed responses should advertise the configured limit")
	}

	assert.Equal(t, 3, calls, "All three in-budget requests should reach the handler")
}

// TestAuthRateLimitMiddleware_OverLimitReturns429 asserts the request
// that exhausts the per-IP budget gets 429 with the structured error
// envelope and rate-limit headers, and the downstream handler never runs.
func TestAuthRateLimitMiddleware_OverLimitReturns429(t *testing.T) {
	limiter := NewRateLimiter(2, time.Minute)
	var calls int
	handler := AuthRateLimitMiddleware(limiter)(countingHandler(&calls))

	for i := 0; i < 2; i++ {
		req := requestFromIP(http.MethodPost, "/api/auth/login", "203.0.113.10:5555")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Result().StatusCode, "Request %d within the budget should pass", i+1)
	}

	req := requestFromIP(http.MethodPost, "/api/auth/login", "203.0.113.10:5555")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	result := w.Result()
	require.Equal(t, http.StatusTooManyRequests, result.StatusCode, "The request over the budget should be rejected with 429")
	assert.Equal(t, 2, calls, "The handler must not run for a rejected request")
	assert.Equal(t, "2", result.Header.Get("X-RateLimit-Limit"), "429 should advertise the configured limit")
	assert.Equal(t, "0", result.Header.Get("X-RateLimit-Remaining"), "429 should report no remaining budget")
	assert.NotEmpty(t, result.Header.Get("X-RateLimit-Reset"), "429 should report when the window resets")

	code, message := decodeErrorEnvelope(t, result)
	assert.Equal(t, "RATE_LIMIT_EXCEEDED", code, "429 body should carry the rate-limit error code")
	assert.Equal(t, "Too many requests", message, "429 body should carry the rate-limit message")
}

// TestAuthRateLimitMiddleware_KeysByIPNotUser asserts the budget is
// per client IP: a second IP gets a fresh budget even for the same
// caller, and an authenticated user context does not widen the bucket —
// login and refresh are unauthenticated routes, so the IP is the only
// key available.
func TestAuthRateLimitMiddleware_KeysByIPNotUser(t *testing.T) {
	limiter := NewRateLimiter(1, time.Minute)
	var calls int
	handler := AuthRateLimitMiddleware(limiter)(countingHandler(&calls))

	// Exhaust IP A's single-request budget. The request deliberately
	// carries a user context: if the limiter keyed on it, this is the
	// case that would miscount.
	reqA := requestAsUser(http.MethodPost, "/api/auth/login", "user-1")
	reqA.RemoteAddr = "203.0.113.10:5555"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, reqA)
	require.Equal(t, http.StatusOK, w.Result().StatusCode, "IP A's first request should pass")

	reqA2 := requestAsUser(http.MethodPost, "/api/auth/login", "user-1")
	reqA2.RemoteAddr = "203.0.113.10:5555"
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, reqA2)
	assert.Equal(t, http.StatusTooManyRequests, w.Result().StatusCode, "IP A's second request should exhaust its budget")

	// A different IP sails through even with the same user context.
	reqB := requestAsUser(http.MethodPost, "/api/auth/login", "user-1")
	reqB.RemoteAddr = "198.51.100.7:4444"
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, reqB)
	assert.Equal(t, http.StatusOK, w.Result().StatusCode, "IP B should have its own budget")
	assert.Equal(t, 2, calls, "Only the two in-budget requests should reach the handler")
}

// decodeErrorEnvelope reads a structured error envelope body.
func decodeErrorEnvelope(t *testing.T, resp *http.Response) (code, message string) {
	t.Helper()

	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&envelope), "Should decode the error envelope")
	return envelope.Error.Code, envelope.Error.Message
}
