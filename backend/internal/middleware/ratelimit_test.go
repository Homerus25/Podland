package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	"github.com/Homerus25/podland/backend/internal/testutils"
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

	for i := range 3 {
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

	for i := range 2 {
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

	envelope := testutils.DecodeErrorEnvelope(t, result)
	assert.Equal(t, "RATE_LIMIT_EXCEEDED", envelope.Error.Code, "429 body should carry the rate-limit error code")
	assert.Equal(t, "Too many requests", envelope.Error.Message, "429 body should carry the rate-limit message")
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

// TestAllow_EvictsEntriesIdlePastTwoWindows pins the lazy janitor: a
// sweep inside Allow evicts entries whose window started more than
// 2×window ago, and runs at most once per window — a second Allow
// inside the same window must not rescan the store.
func TestAllow_EvictsEntriesIdlePastTwoWindows(t *testing.T) {
	window := 20 * time.Millisecond
	limiter := NewRateLimiter(2, window)

	require.True(t, limiter.Allow("ghost"), "first request should create the entry")

	// Outlive the 2×window retention horizon: after this sleep the
	// entry qualifies for eviction on the next sweep.
	time.Sleep(3 * window)

	require.True(t, limiter.Allow("fresh"), "the sweep-triggering request should be allowed")

	_, exists := limiter.store.entries["ghost"]
	assert.False(t, exists, "an entry idle past 2×window should be evicted by the sweep")
	assert.Contains(t, limiter.store.entries, "fresh",
		"the entry that triggered the sweep should survive it")
	assert.Equal(t, 2, limiter.GetRemaining("ghost"),
		"an evicted key should have its full budget back")

	// The sweep just ran, so a second Allow within the same window
	// must skip it: a backdated entry survives untouched.
	limiter.store.mu.Lock()
	limiter.store.entries["backdated"] = &RateLimitEntry{
		count:       1,
		windowStart: time.Now().Add(-3 * window),
	}
	limiter.store.mu.Unlock()

	require.True(t, limiter.Allow("another"), "in-window request should be allowed")

	assert.Contains(t, limiter.store.entries, "backdated",
		"the sweep must run at most once per window, not on every Allow")
}

// TestHasBudget_UnknownKeyHasFullBudgetWithoutSpending pins the pure
// read behind the failure budgets: an unknown key reports budget, and
// neither the answer nor repeating the check changes anything — no
// store entry is created and the reported budget stays full.
func TestHasBudget_UnknownKeyHasFullBudgetWithoutSpending(t *testing.T) {
	limiter := NewRateLimiter(5, time.Minute)

	assert.True(t, limiter.HasBudget("user-1"), "an unknown key should have its full budget")
	assert.Equal(t, 5, limiter.GetRemaining("user-1"), "an unknown key should report the full budget")

	for range 3 {
		assert.True(t, limiter.HasBudget("user-1"), "the check must never spend budget")
	}

	assert.Equal(t, 5, limiter.GetRemaining("user-1"), "checks must not change the key's budget")
	assert.Empty(t, limiter.store.entries, "a check must not create a store entry")
}

// TestRecord_SpendsOneUnitPerCall pins the spend operation: each
// record burns one unit of its key's budget, and after the configured
// count of records within the window the check reports no budget left.
func TestRecord_SpendsOneUnitPerCall(t *testing.T) {
	limiter := NewRateLimiter(3, time.Minute)

	limiter.Record("user-1")
	assert.True(t, limiter.HasBudget("user-1"), "one of three units spent should leave budget")
	assert.Equal(t, 2, limiter.GetRemaining("user-1"), "one record should have spent one unit")

	limiter.Record("user-1")
	assert.True(t, limiter.HasBudget("user-1"), "two of three units spent should leave budget")

	limiter.Record("user-1")
	assert.False(t, limiter.HasBudget("user-1"), "the configured count of records should exhaust the budget")
	assert.Equal(t, 0, limiter.GetRemaining("user-1"), "an exhausted key should report no budget")
}

// TestRecord_WindowExpiryIsTheOnlyReset pins the reset semantics: an
// exhausted budget survives further records — spending past the
// threshold restores nothing — and only the fixed window's expiry
// brings the budget back, whereupon the next record opens a fresh
// window one unit down.
func TestRecord_WindowExpiryIsTheOnlyReset(t *testing.T) {
	limiter := NewRateLimiter(2, time.Minute)

	limiter.Record("user-1")
	limiter.Record("user-1")
	require.False(t, limiter.HasBudget("user-1"), "the budget should be exhausted")

	limiter.Record("user-1")
	assert.Equal(t, 0, limiter.GetRemaining("user-1"),
		"recording past the threshold must not restore or change the budget")

	// Age the entry past its window the way the clock would.
	limiter.store.mu.Lock()
	limiter.store.entries["user-1"].windowStart = time.Now().Add(-2 * time.Minute)
	limiter.store.mu.Unlock()

	assert.True(t, limiter.HasBudget("user-1"), "only window expiry restores the budget")
	assert.Equal(t, 2, limiter.GetRemaining("user-1"), "an expired window reports the full budget")

	limiter.Record("user-1")
	assert.Equal(t, 1, limiter.GetRemaining("user-1"),
		"a record after expiry opens a fresh window one unit down")
}

// TestRecord_KeysAreIndependent pins the per-key budget: records
// against one key never touch another key's budget.
func TestRecord_KeysAreIndependent(t *testing.T) {
	limiter := NewRateLimiter(2, time.Minute)

	limiter.Record("user-1")
	limiter.Record("user-1")

	assert.False(t, limiter.HasBudget("user-1"), "user-1's budget should be exhausted")
	assert.True(t, limiter.HasBudget("user-2"), "user-2 should still have budget")
	assert.Equal(t, 2, limiter.GetRemaining("user-2"), "user-2's budget should be untouched")
}

// TestRecord_EmptyKeyIsSkipped pins the empty-key convention Allow
// sets: an unkeyed record spends nothing and an unkeyed check always
// has budget — there is no shared "" bucket for keyless callers to
// contend over.
func TestRecord_EmptyKeyIsSkipped(t *testing.T) {
	limiter := NewRateLimiter(2, time.Minute)

	assert.True(t, limiter.HasBudget(""), "an empty key should always have budget")

	limiter.Record("")

	assert.Empty(t, limiter.store.entries, "an unkeyed record must not create a store entry")
}

// TestLimit_ReportsConfiguredBudget pins the accessor the failure-budget
// surfaces advertise as X-RateLimit-Limit on their 429 answers.
func TestLimit_ReportsConfiguredBudget(t *testing.T) {
	assert.Equal(t, 7, NewRateLimiter(7, time.Minute).Limit(), "Limit should report the configured count")
}

// TestRateLimitMiddleware_OverLimitAnswersEnvelope pins the per-user
// 429: the X-RateLimit-* headers, then the same structured
// RATE_LIMIT_EXCEEDED envelope the auth middleware answers, carrying
// the request ID from context. The downstream handler never runs.
func TestRateLimitMiddleware_OverLimitAnswersEnvelope(t *testing.T) {
	limiter := NewRateLimiter(2, time.Minute)
	var calls int
	handler := RateLimitMiddleware(limiter)(countingHandler(&calls))

	request := func() *http.Request {
		req := requestAsUser(http.MethodGet, "/api/hosts", "user-1")
		return req.WithContext(contextutils.SetRequestID(req.Context(), "req-123"))
	}

	for i := range 2 {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, request())
		require.Equal(t, http.StatusOK, w.Result().StatusCode, "Request %d within the budget should pass", i+1)
	}

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, request())

	result := w.Result()
	require.Equal(t, http.StatusTooManyRequests, result.StatusCode, "The request over the budget should be rejected with 429")
	assert.Equal(t, 2, calls, "The handler must not run for a rejected request")
	assert.Equal(t, "2", result.Header.Get("X-RateLimit-Limit"), "429 should advertise the configured limit")
	assert.Equal(t, "0", result.Header.Get("X-RateLimit-Remaining"), "429 should report no remaining budget")
	assert.NotEmpty(t, result.Header.Get("X-RateLimit-Reset"), "429 should report when the window resets")

	envelope := testutils.DecodeErrorEnvelope(t, result)
	assert.Equal(t, "RATE_LIMIT_EXCEEDED", envelope.Error.Code, "429 body should carry the rate-limit error code")
	assert.Equal(t, "Too many requests", envelope.Error.Message, "429 body should carry the rate-limit message")
	assert.Equal(t, "req-123", envelope.RequestID, "429 body should carry the request ID from context")
}
