package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/yourorg/podland/backend/internal/contextutils"
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

// RateLimiter handles rate limiting
func (rl *RateLimiter) Allow(userID string) bool {
	if userID == "" {
		return true // Skip rate limiting for unauthenticated requests
	}

	rl.store.mu.Lock()
	defer rl.store.mu.Unlock()

	now := time.Now()

	entry, exists := rl.store.entries[userID]
	if !exists {
		entry = &RateLimitEntry{
			count:     1,
			windowStart: now,
		}
		rl.store.entries[userID] = entry
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

// GetRemaining returns the number of remaining requests for a user
func (rl *RateLimiter) GetRemaining(userID string) int {
	if userID == "" {
		return rl.requests
	}

	rl.store.mu.Lock()
	defer rl.store.mu.Unlock()

	entry, exists := rl.store.entries[userID]
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

// GetResetIn returns the duration until the rate limit resets for a user
func (rl *RateLimiter) GetResetIn(userID string) time.Duration {
	if userID == "" {
		return 0
	}

	rl.store.mu.Lock()
	defer rl.store.mu.Unlock()

	entry, exists := rl.store.entries[userID]
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

// RateLimitEntry represents rate limit information for a user
type RateLimitEntry struct {
	count       int
	windowStart time.Time
}

// RateLimitMiddleware creates a middleware for rate limiting
type RateLimiter struct {
	store      *RateLimitStore
	requests   int
	window     time.Duration
}

// RateLimitMiddleware returns a middleware that limits requests
func RateLimitMiddleware(rl *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := contextutils.GetUserIDFromContext(r.Context())

			if !rl.Allow(userID) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.requests))
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(int64(rl.GetResetIn(userID).Seconds()), 10))
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}

			// Set rate limit headers
			remaining := rl.GetRemaining(userID)
			resetIn := rl.GetResetIn(userID)
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.requests))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(int64(resetIn.Seconds()), 10))

			next.ServeHTTP(w, r)
		})
	}
}

// Cleanup removes old entries from the store
func (rl *RateLimiter) Cleanup() {
	for {
		time.Sleep(rl.window)
		rl.store.mu.Lock()
		now := time.Now()
		for userID, entry := range rl.store.entries {
			if now.Sub(entry.windowStart) > rl.window*2 {
				delete(rl.store.entries, userID)
			}
		}
		rl.store.mu.Unlock()
	}
}
