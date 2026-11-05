package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	"github.com/Homerus25/podland/backend/internal/domain/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// observedUserIDHandler records the user ID the middleware put in the
// request context, so tests can assert what the downstream handler sees.
func observedUserIDHandler(observed *string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*observed = contextutils.GetUserIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
}

// TestJWTMiddleware_RejectionCodes pins the required chain's 401
// vocabulary: a missing Authorization header answers TOKEN_MISSING
// (no credentials were presented at all), while a malformed header, an
// invalid token, or an expired one answers with the matching sentinel
// from ValidateToken's two-error vocabulary — credentials were
// presented and they are bad. These are the codes the domain doku
// files' 401 lines document.
func TestJWTMiddleware_RejectionCodes(t *testing.T) {
	service, err := auth.NewTestAuthService()
	require.NoError(t, err, "Should create auth service without error")

	expiredToken, err := auth.CreateExpiredTestToken(service, "user-expired-1")
	require.NoError(t, err, "Should mint an expired test token")

	var observed string
	handler := JWTMiddleware(service)(observedUserIDHandler(&observed))

	cases := map[string]struct {
		authHeader string
		wantCode   string
	}{
		"missing header":   {authHeader: "", wantCode: "TOKEN_MISSING"},
		"malformed header": {authHeader: "Token abc", wantCode: "TOKEN_INVALID"},
		"invalid token":    {authHeader: "Bearer not-a-real-token", wantCode: "TOKEN_INVALID"},
		"expired token":    {authHeader: "Bearer " + expiredToken, wantCode: "TOKEN_EXPIRED"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			observed = ""
			req := httptest.NewRequest("GET", "/api/hosts", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			resp := w.Result()
			defer resp.Body.Close()
			require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Unauthenticated request should be rejected with 401")

			var envelope struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&envelope), "Should decode the error envelope")
			assert.Equal(t, tc.wantCode, envelope.Error.Code, "The rejection code should match the documented vocabulary")
			assert.Empty(t, observed, "Handler must not run for a rejected request")
		})
	}
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
