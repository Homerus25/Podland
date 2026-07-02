package middleware

import (
	"net/http"
	"strings"
)

// CORS configures CORS middleware
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Check if origin is allowed
			if len(allowedOrigins) == 0 {
				// Allow all if no origins specified
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else {
				// Check if origin is in the allowed list
				for _, allowedOrigin := range allowedOrigins {
					allowedOrigin = strings.TrimSpace(allowedOrigin)
					if origin == allowedOrigin {
						w.Header().Set("Access-Control-Allow-Origin", origin)
						w.Header().Set("Vary", "Origin")
						break
					}
					// Allow all for development convenience
					if allowedOrigin == "*" {
						w.Header().Set("Access-Control-Allow-Origin", "*")
						break
					}
				}
			}

			// Handle preflight requests
			if r.Method == "OPTIONS" {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, Cookie")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Max-Age", "86400")
				w.WriteHeader(http.StatusNoContent)
				return
			}

			// Set default CORS headers for actual requests
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, Cookie")
			w.Header().Set("Access-Control-Expose-Headers", "Request-Id, X-RateLimit-Limit, X-RateLimit-Remaining, X-RateLimit-Reset")

			next.ServeHTTP(w, r)
		})
	}
}
