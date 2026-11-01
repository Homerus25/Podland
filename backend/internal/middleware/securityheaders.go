package middleware

import (
	"fmt"
	"net/http"
)

// csp is the exact Content-Security-Policy every response carries
// (pentest 2026-11-01, finding 6). The SPA (Chakra UI v2 / Emotion)
// injects <style> tags at runtime, so style-src needs 'unsafe-inline';
// scripts are Vite-bundled external files, so script-src 'self' holds.
// frame-ancestors 'none' is the modern complement to
// X-Frame-Options: DENY.
const csp = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'"

// SecurityHeaders sets the global security headers on every response:
// X-Content-Type-Options, X-Frame-Options, and the CSP. The headers are
// written before next.ServeHTTP, so they also land on error and
// streaming responses.
//
// HSTS is config-gated, not always-on: the backend never terminates TLS
// itself and deliberately does not trust X-Forwarded-* headers (the
// rate-limit middleware's documented stance), so there is no in-request
// signal to key on. hstsMaxAgeSeconds comes from HSTS_MAX_AGE_SECONDS
// (default 0 = header omitted); RFC 6797 forbids the STS header over
// plain HTTP, which is how this backend serves, so only a
// TLS-terminated deployment sets it.
func SecurityHeaders(hstsMaxAgeSeconds int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Content-Security-Policy", csp)
			if hstsMaxAgeSeconds > 0 {
				h.Set("Strict-Transport-Security", fmt.Sprintf("max-age=%d", hstsMaxAgeSeconds))
			}
			next.ServeHTTP(w, r)
		})
	}
}
