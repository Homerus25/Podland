package middleware

import (
	"context"
	"net/http"

	"github.com/yourorg/podland/backend/internal/contextutils"
	"github.com/google/uuid"
)

// RequestIDMiddleware adds a unique request ID to each request context
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := uuid.New().String()
		ctx := context.WithValue(r.Context(), contextutils.RequestIDContextKey, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
