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
			// Get token from Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				errors.RespondWithError(w, errors.ErrTokenInvalid, GetRequestID(r.Context()))
				return
			}

			// Extract token from Bearer scheme
			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				errors.RespondWithError(w, errors.ErrTokenInvalid, GetRequestID(r.Context()))
				return
			}

			tokenString := parts[1]

			// Validate token
			claims, err := authService.ValidateToken(tokenString)
			if err != nil {
				// Check if token is expired
				if strings.Contains(err.Error(), "expired") {
					errors.RespondWithError(w, errors.ErrTokenExpired, GetRequestID(r.Context()))
				} else {
					errors.RespondWithError(w, errors.ErrTokenInvalid, GetRequestID(r.Context()))
				}
				return
			}

			// Add user ID to context
			ctx := contextutils.SetUserID(r.Context(), claims.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}