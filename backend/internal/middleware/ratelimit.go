package middleware

import (
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/yourorg/podland/backend/internal/contextutils"
	apperrors "github.com/yourorg/podland/backend/internal/errors"
)

// RateLimiter implements a token bucket rate limiter
func NewRateLimiter(requests int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		store: &RateLimitStore{
			mu:      sync.Mutex{},
			entries: make(map[string]*RateLimitEntry),
		},
		requests: requests,
		window:   window,
	}
}

// RateLimiter handles rate limiting. The key identifies whose budget
// is being spent: the per-user middleware passes the authenticated user
// ID, the auth middleware passes the client IP.
func (rl *RateLimiter) Allow(key string) bool {
	if key == "" {
		return true // Skip rate limiting for unauthenticated requests
	}

	rl.store.mu.Lock()
	defer rl.store.mu.Unlock()

	now := time.Now()

	entry, exists := rl.store.entries[key]
	if !exists {
		entry = &RateLimitEntry{
			count:       1,
			windowStart: now,
		}
		rl.store.entries[key] = entry
		return true
	}

	// Check if window has expired
	if now.Sub(entry.windowStart) > rl.window {
		entry.count = 1
		entry.windowStart = now
		return true
	}

	// Check if limit exceeded
	if entry.count >= rl.requests {
		return false
	}

	entry.count++
	return true
}

// GetRemaining returns the number of remaining requests for a key
func (rl *RateLimiter) GetRemaining(key string) int {
	if key == "" {
		return rl.requests
	}

	rl.store.mu.Lock()
	defer rl.store.mu.Unlock()

	entry, exists := rl.store.entries[key]
	if !exists {
		return rl.requests
	}

	now := time.Now()
	if now.Sub(entry.windowStart) > rl.window {
		return rl.requests
	}

	remaining := rl.requests - entry.count
	return remaining
}

// GetResetIn returns the duration until the rate limit resets for a key
func (rl *RateLimiter) GetResetIn(key string) time.Duration {
	if key == "" {
		return 0
	}

	rl.store.mu.Lock()
	defer rl.store.mu.Unlock()

	entry, exists := rl.store.entries[key]
	if !exists {
		return 0
	}

	now := time.Now()
	elapsed := now.Sub(entry.windowStart)
	if elapsed > rl.window {
		return 0
	}

	return rl.window - elapsed
}

// RateLimitStore stores rate limit information
type RateLimitStore struct {
	mu      sync.Mutex
	entries map[string]*RateLimitEntry
}

// RateLimitEntry represents rate limit information for one key
type RateLimitEntry struct {
	count       int
	windowStart time.Time
}

// RateLimitMiddleware creates a middleware for rate limiting
type RateLimiter struct {
	store    *RateLimitStore
	requests int
	window   time.Duration
}

// RateLimitMiddleware returns a middleware that limits requests
func RateLimitMiddleware(rl *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := contextutils.GetUserIDFromContext(r.Context())

			if !rl.Allow(userID) {
				w.Header().Set("Content-Type", "application/json")
				rl.setRateLimitHeaders(w, 0, rl.GetResetIn(userID))
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}

			// Set rate limit headers
			rl.setRateLimitHeaders(w, rl.GetRemaining(userID), rl.GetResetIn(userID))

			next.ServeHTTP(w, r)
		})
	}
}

// AuthRateLimitMiddleware returns a middleware that limits requests per
// client IP for the unauthenticated auth surface (login, refresh). Those
// routes have no authenticated user for RateLimitMiddleware to key on,
// so this limiter keys on the client's IP instead. Rejected requests get
// 429 with the structured RATE_LIMIT_EXCEEDED envelope — like every
// other error on the auth surface — plus the same X-RateLimit-* headers
// the per-user middleware sets.
func AuthRateLimitMiddleware(rl *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)

			if !rl.Allow(ip) {
				rl.setRateLimitHeaders(w, 0, rl.GetResetIn(ip))
				apperrors.RespondWithError(w, apperrors.ErrRateLimitExceeded, contextutils.GetRequestID(r.Context()))
				return
			}

			// Set rate limit headers
			rl.setRateLimitHeaders(w, rl.GetRemaining(ip), rl.GetResetIn(ip))

			next.ServeHTTP(w, r)
		})
	}
}

// setRateLimitHeaders writes the X-RateLimit-* trio both middlewares
// advertise: the configured limit, the key's remaining budget, and the
// seconds until its window resets.
func (rl *RateLimiter) setRateLimitHeaders(w http.ResponseWriter, remaining int, resetIn time.Duration) {
	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.requests))
	w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
	w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(int64(resetIn.Seconds()), 10))
}

// clientIP returns the host portion of the request's remote address. It
// deliberately ignores X-Forwarded-For and friends: without a
// trusted-proxy mechanism those headers are client-controlled, and
// honouring them would let a brute-forcer mint a fresh budget per
// request. Behind a reverse proxy every client shares the proxy's IP —
// one shared bucket — which is the safer failure mode.
func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// Cleanup removes old entries from the store
func (rl *RateLimiter) Cleanup() {
	for {
		time.Sleep(rl.window)
		rl.store.mu.Lock()
		now := time.Now()
		for key, entry := range rl.store.entries {
			if now.Sub(entry.windowStart) > rl.window*2 {
				delete(rl.store.entries, key)
			}
		}
		rl.store.mu.Unlock()
	}
}
