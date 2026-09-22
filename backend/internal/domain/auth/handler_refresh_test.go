package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// performRefreshWithCookie drives the Refresh handler with the given
// refresh cookie attached (nil = no cookie header at all) and returns the
// recorded response.
func performRefreshWithCookie(h *AuthHandler, cookie *http.Cookie) *http.Response {
	req := httptest.NewRequest("POST", "/api/auth/refresh", nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	h.Refresh(w, req)
	return w.Result()
}

// findRefreshCookie returns the refresh_token cookie from a response, or
// nil when the response does not set one.
func findRefreshCookie(resp *http.Response) *http.Cookie {
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "refresh_token" {
			return cookie
		}
	}
	return nil
}

// TestAuthHandler_Refresh_SetsRotatedCookieAndAccessToken asserts the
// happy path: 200, a body carrying the new access JWT and the username
// it belongs to, and a new refresh cookie with all four security
// attributes and a rotated value.
func TestAuthHandler_Refresh_SetsRotatedCookieAndAccessToken(t *testing.T) {
	mockRepo := seedLoginUser(t, "testuser", "test-password-123")
	sessionRepo := testutils.NewMockSessionRepository()
	service, err := NewTestAuthServiceWithSessionRepo(mockRepo, sessionRepo, 30*24*time.Hour)
	require.NoError(t, err, "Should create auth service without error")

	_, loginToken, _, err := service.Login("testuser", "test-password-123")
	require.NoError(t, err, "Should log in to mint a refresh token")

	handler := NewAuthHandler(service, mockRepo)
	resp := performRefreshWithCookie(handler, &http.Cookie{Name: "refresh_token", Value: loginToken})
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "Valid cookie should refresh")

	rotatedCookie := findRefreshCookie(resp)
	require.NotNil(t, rotatedCookie, "Refresh should set a refresh_token cookie")
	assert.NotEqual(t, loginToken, rotatedCookie.Value, "Refresh cookie must carry the rotated token, not the presented one")
	assert.True(t, rotatedCookie.HttpOnly, "Refresh cookie must be httpOnly so JavaScript cannot read it")
	assert.True(t, rotatedCookie.Secure, "Refresh cookie must be Secure")
	assert.Equal(t, http.SameSiteStrictMode, rotatedCookie.SameSite, "Refresh cookie must be SameSite=Strict")
	assert.Equal(t, "/api/auth", rotatedCookie.Path, "Refresh cookie must be scoped to /api/auth")

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body), "Should decode refresh body")
	assert.ElementsMatch(t, []string{"token", "username", "role"}, mapKeys(body), "Refresh body should match the login body's shape")
	assert.NotEmpty(t, body["token"], "Refresh body should carry the new access JWT")
	assert.Equal(t, "testuser", body["username"], "Refresh body should carry the session user's username")
	assert.Equal(t, models.RoleUser, body["role"], "Refresh body should carry the session user's role")
}

// TestAuthHandler_Refresh_MissingCookieIsInvalid asserts a request with
// no refresh cookie at all gets the invalid-token 401 — same envelope a
// replayed token produces, so callers cannot distinguish cookie-less
// probing from token guessing.
func TestAuthHandler_Refresh_MissingCookieIsInvalid(t *testing.T) {
	mockRepo := seedLoginUser(t, "testuser", "test-password-123")
	service, err := NewTestAuthServiceWithUserRepo(mockRepo)
	require.NoError(t, err, "Should create auth service without error")

	handler := NewAuthHandler(service, mockRepo)
	resp := performRefreshWithCookie(handler, nil)
	defer resp.Body.Close()

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Missing cookie must be a 401")
	assert.Equal(t, "AUTH_REFRESH_INVALID", testutils.DecodeErrorEnvelope(t, resp).Error.Code, "Missing cookie must map to AUTH_REFRESH_INVALID")
}

// TestAuthHandler_Refresh_UnknownCookieValueIsInvalid asserts a cookie
// whose token no session matches (replay of an already-rotated token, or
// a fabricated value) is a 401 with the invalid code.
func TestAuthHandler_Refresh_UnknownCookieValueIsInvalid(t *testing.T) {
	mockRepo := seedLoginUser(t, "testuser", "test-password-123")
	service, err := NewTestAuthServiceWithUserRepo(mockRepo)
	require.NoError(t, err, "Should create auth service without error")

	handler := NewAuthHandler(service, mockRepo)
	resp := performRefreshWithCookie(handler, &http.Cookie{Name: "refresh_token", Value: "forged-token-value"})
	defer resp.Body.Close()

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Unknown token must be a 401")
	assert.Equal(t, "AUTH_REFRESH_INVALID", testutils.DecodeErrorEnvelope(t, resp).Error.Code, "Unknown token must map to AUTH_REFRESH_INVALID")
	assert.Nil(t, findRefreshCookie(resp), "A rejected refresh must not set a new cookie")
}

// TestAuthHandler_Refresh_ExpiredSessionUsesExpiredCode asserts an
// expired session is a 401 with the session-expired code, so the client
// can prompt for re-login without implying anything was stolen.
func TestAuthHandler_Refresh_ExpiredSessionUsesExpiredCode(t *testing.T) {
	mockRepo := seedLoginUser(t, "testuser", "test-password-123")
	sessionRepo := testutils.NewMockSessionRepository()
	sessionRepo.AddSession(&models.Session{
		ID:        "expired-session",
		UserID:    "user-testuser",
		TokenHash: sha256Hex("expired-cookie-token"),
		CreatedAt: time.Now().Add(-48 * time.Hour),
		ExpiresAt: time.Now().Add(-24 * time.Hour),
	})
	service, err := NewTestAuthServiceWithSessionRepo(mockRepo, sessionRepo, 30*24*time.Hour)
	require.NoError(t, err, "Should create auth service without error")

	handler := NewAuthHandler(service, mockRepo)
	resp := performRefreshWithCookie(handler, &http.Cookie{Name: "refresh_token", Value: "expired-cookie-token"})
	defer resp.Body.Close()

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Expired session must be a 401")
	assert.Equal(t, "AUTH_SESSION_EXPIRED", testutils.DecodeErrorEnvelope(t, resp).Error.Code, "Expired session must map to AUTH_SESSION_EXPIRED")
	assert.Nil(t, findRefreshCookie(resp), "An expired refresh must not set a new cookie")
}
