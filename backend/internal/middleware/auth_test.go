package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourorg/podland/backend/internal/contextutils"
	"github.com/yourorg/podland/backend/internal/domain/auth"
)

// observedUserIDHandler records the user ID the middleware put in the
// request context, so tests can assert what the downstream handler sees.
func observedUserIDHandler(observed *string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*observed = contextutils.GetUserIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
}

// TestOptionalJWTMiddleware_NoHeader_PassesThroughUnauthenticated
// asserts that a request without an Authorization header still reaches
// the handler, with no user ID in the context. GET /api/auth/status
// relies on this to answer {authenticated: false} instead of 401.
func TestOptionalJWTMiddleware_NoHeader_PassesThroughUnauthenticated(t *testing.T) {
	service, err := auth.NewTestAuthService()
	require.NoError(t, err, "Should create auth service without error")

	var observed string
	handler := OptionalJWTMiddleware(service)(observedUserIDHandler(&observed))

	req := httptest.NewRequest("GET", "/api/auth/status", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode, "Anonymous request should reach the handler")
	assert.Empty(t, observed, "No user ID should be set for an anonymous request")
}

// TestOptionalJWTMiddleware_ValidToken_SetsUserID asserts that a valid
// Bearer token authenticates the request: the handler runs with the
// token's user ID in the context.
func TestOptionalJWTMiddleware_ValidToken_SetsUserID(t *testing.T) {
	service, err := auth.NewTestAuthService()
	require.NoError(t, err, "Should create auth service without error")

	token, err := auth.CreateValidTestToken(service, "user-optional-1")
	require.NoError(t, err, "Should mint a valid test token")

	var observed string
	handler := OptionalJWTMiddleware(service)(observedUserIDHandler(&observed))

	req := httptest.NewRequest("GET", "/api/auth/status", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode, "Authenticated request should reach the handler")
	assert.Equal(t, "user-optional-1", observed, "User ID from the token should be in the context")
}

// TestOptionalJWTMiddleware_InvalidToken_Rejects asserts that a presented
// but invalid token is rejected with 401 rather than silently downgraded
// to anonymous — a caller presenting credentials must learn they are bad.
func TestOptionalJWTMiddleware_InvalidToken_Rejects(t *testing.T) {
	service, err := auth.NewTestAuthService()
	require.NoError(t, err, "Should create auth service without error")

	var observed string
	handler := OptionalJWTMiddleware(service)(observedUserIDHandler(&observed))

	req := httptest.NewRequest("GET", "/api/auth/status", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Result().StatusCode, "Invalid token should be rejected")
	assert.Empty(t, observed, "Handler must not run for an invalid token")
}
