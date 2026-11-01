package middleware

import (
	"net/http"
	"strings"
)

// CORS configures CORS middleware. The origin decision follows the
// pentest 2026-11-01 finding 5 rework: Access-Control-Allow-Credentials
// is only ever emitted together with a specific exactly-matched
// configured origin — never with the wildcard, never when the list is
// empty. An empty ALLOWED_ORIGINS (the default) fails closed to
// same-origin: no ACAO, no ACAC.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Decide the allowed origin: an exact match reflects the
			// origin and earns credentials; a configured "*" is honored
			// as the spec-valid non-credentialed wildcard; anything
			// else gets no ACAO at all.
			allowOrigin := ""
			allowCredentials := false
			wildcard := false
			for _, allowedOrigin := range allowedOrigins {
				allowedOrigin = strings.TrimSpace(allowedOrigin)
				if allowedOrigin == "*" {
					wildcard = true
					continue
				}
				if origin != "" && origin == allowedOrigin {
					allowOrigin = origin
					allowCredentials = true
					break
				}
			}
			if allowOrigin == "" && wildcard {
				allowOrigin = "*"
			}

			if allowOrigin != "" {
				w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
			}
			if allowCredentials {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Vary", "Origin")
			}

			// Handle preflight requests
			if r.Method == "OPTIONS" {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, Cookie")
				w.Header().Set("Access-Control-Max-Age", "86400")
				w.WriteHeader(http.StatusNoContent)
				return
			}

			// Set default CORS headers for actual requests
			w.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, Cookie")
			w.Header().Set("Access-Control-Expose-Headers", "Request-Id, X-RateLimit-Limit, X-RateLimit-Remaining, X-RateLimit-Reset")

			next.ServeHTTP(w, r)
		})
	}
}
