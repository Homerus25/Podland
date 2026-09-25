package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	apperrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/web"
)

// NewRateLimiter builds a keyed fixed-window limiter: each key spends
// one unit per Allow call within a window that opens on its first
// visit and only its expiry restores the budget. HasBudget/Record are
// the check/spend pair the failure-budget surfaces use.
func NewRateLimiter(requests int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		store: &RateLimitStore{
			mu:        sync.Mutex{},
			entries:   make(map[string]*RateLimitEntry),
			lastSweep: time.Now(),
		},
		requests: requests,
		window:   window,
	}
}

// RateLimiter handles rate limiting. The key identifies whose budget
// is being spent: the per-user middleware passes the authenticated user
// ID, the auth middleware passes the client IP, and the failure-budget
// surfaces pass the claimed identity (username or caller ID).
func (rl *RateLimiter) Allow(key string) bool {
	if key == "" {
		return true // No key, no budget: unsized requests pass unthrottled
	}

	rl.store.mu.Lock()
	defer rl.store.mu.Unlock()

	now := time.Now()

	// Amortized janitor: at most once per window, evict entries whose
	// window started more than 2×window ago, so the map stays bounded
	// by keys active in the last two windows. O(active keys) per
	// window, not per request.
	if now.Sub(rl.store.lastSweep) > rl.window {
		for entryKey, entry := range rl.store.entries {
			if now.Sub(entry.windowStart) > 2*rl.window {
				delete(rl.store.entries, entryKey)
			}
		}
		rl.store.lastSweep = now
	}

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

	return rl.remainingLocked(key)
}

// remainingLocked returns the key's unspent budget in its current
// window. A missing entry or an expired window has the full budget —
// neither is rewritten, the reads leave the store untouched. Callers
// hold the store lock.
func (rl *RateLimiter) remainingLocked(key string) int {
	entry, exists := rl.store.entries[key]
	if !exists {
		return rl.requests
	}

	now := time.Now()
	if now.Sub(entry.windowStart) > rl.window {
		return rl.requests
	}

	return rl.requests - entry.count
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

// Limit returns the configured per-window budget: the number both
// middlewares advertise as X-RateLimit-Limit and the auth handler
// advertises for the failure budget's 429 answers.
func (rl *RateLimiter) Limit() int {
	return rl.requests
}

// HasBudget reports whether the key still has unspent budget without
// spending any: the read the failure-budget surfaces run before a
// credential verification, so a failed check never consumes budget and
// a passed one is spent only by Record. Like Allow, an empty key is
// skipped and always has budget.
func (rl *RateLimiter) HasBudget(key string) bool {
	if key == "" {
		return true
	}

	rl.store.mu.Lock()
	defer rl.store.mu.Unlock()

	return rl.remainingLocked(key) > 0
}

// Record spends one unit of the key's budget: the failure-budget
// surfaces call it when a credential verification fails. The first
// record for a key opens its fixed window; only the window's expiry
// restores spent budget, and recording past the configured count is a
// no-op. The state change is exactly Allow's count-on-call, so Record
// delegates to it — the budget arithmetic keeps one owner. Like Allow,
// an empty key is skipped.
func (rl *RateLimiter) Record(key string) {
	rl.Allow(key)
}

// RateLimitStore stores rate limit information
//
// Entries are evicted lazily from Allow: lastSweep throttles the
// sweep to at most one per window, so no background janitor runs.
type RateLimitStore struct {
	mu        sync.Mutex
	entries   map[string]*RateLimitEntry
	lastSweep time.Time
}

// RateLimitEntry represents rate limit information for one key
type RateLimitEntry struct {
	count       int
	windowStart time.Time
}

// RateLimiter bounds a keyed fixed-window budget behind Allow's
// count-on-call, the pure HasBudget read, and the Record spend.
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
				web.SetRateLimitHeaders(w, rl.Limit(), 0, rl.GetResetIn(userID))
				apperrors.RespondWithError(w, apperrors.ErrRateLimitExceeded, contextutils.GetRequestID(r.Context()))
				return
			}

			// Set rate limit headers
			web.SetRateLimitHeaders(w, rl.Limit(), rl.GetRemaining(userID), rl.GetResetIn(userID))

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
				web.SetRateLimitHeaders(w, rl.Limit(), 0, rl.GetResetIn(ip))
				apperrors.RespondWithError(w, apperrors.ErrRateLimitExceeded, contextutils.GetRequestID(r.Context()))
				return
			}

			// Set rate limit headers
			web.SetRateLimitHeaders(w, rl.Limit(), rl.GetRemaining(ip), rl.GetResetIn(ip))

			next.ServeHTTP(w, r)
		})
	}
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
