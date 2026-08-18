package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAuthHandler_Login_SetsRefreshCookieAttributes asserts login sets
// the refresh token as a cookie carrying all four security attributes
// from the spec: httpOnly (invisible to JavaScript), Secure,
// SameSite=Strict, and Path=/api/auth (never sent anywhere else).
func TestAuthHandler_Login_SetsRefreshCookieAttributes(t *testing.T) {
	mockRepo := seedLoginUser(t, "testuser", "test-password-123")
	service, err := NewTestAuthServiceWithUserRepo(mockRepo)
	require.NoError(t, err, "Should create auth service without error")

	handler := NewAuthHandler(service, mockRepo)

	reqBody, err := json.Marshal(LoginRequest{Username: "testuser", Password: "test-password-123"})
	require.NoError(t, err, "Should marshal request without error")

	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()

	handler.Login(w, req)

	resp := w.Result()
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "Login should succeed with correct credentials")

	var refreshCookie *http.Cookie
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "refresh_token" {
			refreshCookie = cookie
		}
	}
	require.NotNil(t, refreshCookie, "Login should set a refresh_token cookie")
	assert.NotEmpty(t, refreshCookie.Value, "Refresh cookie should carry the refresh token")
	assert.True(t, refreshCookie.HttpOnly, "Refresh cookie must be httpOnly so JavaScript cannot read it")
	assert.True(t, refreshCookie.Secure, "Refresh cookie must be Secure")
	assert.Equal(t, http.SameSiteStrictMode, refreshCookie.SameSite, "Refresh cookie must be SameSite=Strict")
	assert.Equal(t, "/api/auth", refreshCookie.Path, "Refresh cookie must be scoped to /api/auth")
}

// TestAuthHandler_Login_BodyUnchanged asserts the login response body is
// still exactly the access JWT — the refresh token lives only in the
// cookie, not in the JSON payload.
func TestAuthHandler_Login_BodyUnchanged(t *testing.T) {
	mockRepo := seedLoginUser(t, "testuser", "test-password-123")
	service, err := NewTestAuthServiceWithUserRepo(mockRepo)
	require.NoError(t, err, "Should create auth service without error")

	handler := NewAuthHandler(service, mockRepo)

	reqBody, err := json.Marshal(LoginRequest{Username: "testuser", Password: "test-password-123"})
	require.NoError(t, err, "Should marshal request without error")

	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()

	handler.Login(w, req)

	resp := w.Result()
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "Login should succeed")

	var body map[string]any
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err, "Should decode response body")
	assert.Equal(t, []string{"token"}, mapKeys(body), "Response body should contain only the access token field")
}

// mapKeys returns the keys of a decoded JSON object.
func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
