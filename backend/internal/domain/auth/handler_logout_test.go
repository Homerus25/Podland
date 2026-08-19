package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourorg/podland/backend/internal/testutils"
)

// performLogoutWithCookie drives the Logout handler with the given
// refresh cookie attached (nil = no cookie header at all) and returns
// the recorded response.
func performLogoutWithCookie(h *AuthHandler, cookie *http.Cookie) *http.Response {
	req := httptest.NewRequest("POST", "/api/auth/logout", nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	h.Logout(w, req)
	return w.Result()
}

// assertClearedRefreshCookie asserts the response's refresh_token cookie
// is a deletion directive: same name, path, and attributes as the cookie
// login sets, but empty and already expired, so the browser drops it.
func assertClearedRefreshCookie(t *testing.T, resp *http.Response) {
	t.Helper()

	cookie := findRefreshCookie(resp)
	require.NotNil(t, cookie, "Logout must set a refresh_token cookie to clear it")
	assert.Empty(t, cookie.Value, "The clearing cookie must carry no token")
	assert.Equal(t, "/api/auth", cookie.Path, "The clearing cookie must keep the /api/auth path")
	assert.True(t, cookie.HttpOnly, "The clearing cookie must stay httpOnly")
	assert.True(t, cookie.Secure, "The clearing cookie must stay Secure")
	assert.Equal(t, http.SameSiteStrictMode, cookie.SameSite, "The clearing cookie must stay SameSite=Strict")
	expired := cookie.MaxAge < 0 || !cookie.Expires.After(time.Now())
	assert.True(t, expired, "The clearing cookie must be already expired (MaxAge=%d, Expires=%v)", cookie.MaxAge, cookie.Expires)
}

// assertLogoutSuccess asserts logout's response is the historical no-op
// shape: a 200 whose body is exactly the success message.
func assertLogoutSuccess(t *testing.T, resp *http.Response) {
	t.Helper()

	require.Equal(t, http.StatusOK, resp.StatusCode, "Logout must succeed")
	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body), "Should decode logout body")
	assert.Equal(t, []string{"message"}, mapKeys(body), "Logout body shape must not change")
	assert.Equal(t, "Logged out successfully", body["message"], "Logout body message must not change")
}

// TestAuthHandler_Logout_WithSessionCookieRevokesAndClears asserts the
// full valid-cookie logout: the session row behind the cookie is
// revoked, the refresh cookie is cleared with the same attributes, and
// the response keeps its historical shape.
func TestAuthHandler_Logout_WithSessionCookieRevokesAndClears(t *testing.T) {
	mockRepo := seedLoginUser(t, "testuser", "test-password-123")
	sessionRepo := testutils.NewMockSessionRepository()
	service, err := NewTestAuthServiceWithSessionRepo(mockRepo, sessionRepo, 30*24*time.Hour)
	require.NoError(t, err, "Should create auth service without error")

	_, loginToken, _, err := service.Login("testuser", "test-password-123")
	require.NoError(t, err, "Should log in to mint a refresh token")

	handler := NewAuthHandler(service, mockRepo)
	resp := performLogoutWithCookie(handler, &http.Cookie{Name: "refresh_token", Value: loginToken})
	defer resp.Body.Close()

	assertLogoutSuccess(t, resp)
	assertClearedRefreshCookie(t, resp)

	revokedRow, err := sessionRepo.GetByTokenHash(sha256Hex(loginToken))
	require.NoError(t, err, "Lookup of the logged-out hash should not error")
	assert.Nil(t, revokedRow, "Logout must delete the session row the cookie addressed")
}

// TestAuthHandler_Logout_WithoutCookieStillSucceeds asserts logout is
// idempotent at the HTTP seam: a caller with no refresh cookie at all
// still gets the historical success response, plus the clearing cookie.
func TestAuthHandler_Logout_WithoutCookieStillSucceeds(t *testing.T) {
	mockRepo := seedLoginUser(t, "testuser", "test-password-123")
	service, err := NewTestAuthServiceWithUserRepo(mockRepo)
	require.NoError(t, err, "Should create auth service without error")

	handler := NewAuthHandler(service, mockRepo)
	resp := performLogoutWithCookie(handler, nil)
	defer resp.Body.Close()

	assertLogoutSuccess(t, resp)
	assertClearedRefreshCookie(t, resp)
}

// TestAuthHandler_Logout_WithUnknownCookieStillSucceeds asserts a
// cookie whose token matches no session row (forged value, or a second
// logout after revocation) succeeds and still clears the cookie — the
// browser must drop the stale token either way.
func TestAuthHandler_Logout_WithUnknownCookieStillSucceeds(t *testing.T) {
	mockRepo := seedLoginUser(t, "testuser", "test-password-123")
	sessionRepo := testutils.NewMockSessionRepository()
	service, err := NewTestAuthServiceWithSessionRepo(mockRepo, sessionRepo, 30*24*time.Hour)
	require.NoError(t, err, "Should create auth service without error")

	_, loginToken, _, err := service.Login("testuser", "test-password-123")
	require.NoError(t, err, "Should log in to mint a refresh token")
	require.NoError(t, service.Logout(loginToken), "First logout should revoke the session")

	handler := NewAuthHandler(service, mockRepo)
	resp := performLogoutWithCookie(handler, &http.Cookie{Name: "refresh_token", Value: loginToken})
	defer resp.Body.Close()

	assertLogoutSuccess(t, resp)
	assertClearedRefreshCookie(t, resp)
}

// TestAuthHandler_Logout_StorageFailureStillClearsCookie asserts the
// "in every case" of logout: even when the session store fails (500
// envelope), the refresh cookie is still cleared — the browser must
// never keep a token its user tried to give up.
func TestAuthHandler_Logout_StorageFailureStillClearsCookie(t *testing.T) {
	mockRepo := seedLoginUser(t, "testuser", "test-password-123")
	storageFailure := errors.New("session store unavailable")
	sessionRepo := testutils.NewMockSessionRepositoryWithErrors(nil, nil, storageFailure, nil)
	service, err := NewTestAuthServiceWithSessionRepo(mockRepo, sessionRepo, 30*24*time.Hour)
	require.NoError(t, err, "Should create auth service without error")

	handler := NewAuthHandler(service, mockRepo)
	resp := performLogoutWithCookie(handler, &http.Cookie{Name: "refresh_token", Value: "any-token"})
	defer resp.Body.Close()

	require.Equal(t, http.StatusInternalServerError, resp.StatusCode, "A storage failure must surface as a 500")
	assert.Equal(t, "INTERNAL_ERROR", decodeErrorEnvelope(t, resp), "Storage failures map to the generic internal-error code")
	assertClearedRefreshCookie(t, resp)
}
