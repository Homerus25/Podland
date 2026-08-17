package middleware

import (
	"net/http"
	"strings"

	"github.com/yourorg/podland/backend/internal/contextutils"
	"github.com/yourorg/podland/backend/internal/domain/auth"
	"github.com/yourorg/podland/backend/internal/errors"
)

// JWTMiddleware creates middleware for JWT authentication
func JWTMiddleware(authService *auth.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := bearerClaims(w, r, authService)
			if claims == nil {
				return
			}

			// Add user ID to context
			ctx := contextutils.SetUserID(r.Context(), claims.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// OptionalJWTMiddleware validates the Authorization: Bearer header when
// one is presented and continues without it otherwise. It backs
// endpoints that must serve both authenticated and anonymous callers —
// GET /api/auth/status answers {authenticated: false} for anonymous
// requests instead of 401. A presented-but-invalid token is still
// rejected with 401: a caller presenting credentials must learn they
// are bad, not be silently downgraded to anonymous.
func OptionalJWTMiddleware(authService *auth.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") == "" {
				next.ServeHTTP(w, r)
				return
			}

			claims := bearerClaims(w, r, authService)
			if claims == nil {
				return
			}

			// Add user ID to context
			ctx := contextutils.SetUserID(r.Context(), claims.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// bearerClaims validates the Authorization: Bearer scheme and the token
// against the auth service. On any failure it writes the error response
// and returns nil; on success it returns the token claims.
func bearerClaims(w http.ResponseWriter, r *http.Request, authService *auth.AuthService) *auth.TokenClaims {
	// Get token from Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		errors.RespondWithError(w, errors.ErrTokenInvalid, contextutils.GetRequestID(r.Context()))
		return nil
	}

	// Extract token from Bearer scheme
	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		errors.RespondWithError(w, errors.ErrTokenInvalid, contextutils.GetRequestID(r.Context()))
		return nil
	}

	// Validate token
	claims, err := authService.ValidateToken(parts[1])
	if err != nil {
		// Check if token is expired
		if strings.Contains(err.Error(), "expired") {
			errors.RespondWithError(w, errors.ErrTokenExpired, contextutils.GetRequestID(r.Context()))
		} else {
			errors.RespondWithError(w, errors.ErrTokenInvalid, contextutils.GetRequestID(r.Context()))
		}
		return nil
	}

	return claims
}
