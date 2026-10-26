package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The refresh-cookie contract of remembered sessions (issue 379): a
// login that opts in via the request's rememberDevice field gets a
// persistent refresh cookie — Max-Age equal to the remembered TTL —
// and every refresh of that session re-sets the Max-Age to the full
// remembered TTL, so the cookie slides in lockstep with the session
// row. A login without the flag keeps today's cookie byte-identically:
// no Max-Age, a browser-session lifetime. All other cookie attributes
// are untouched either way.

// rememberedTTLs builds a service over distinct session and remembered
// TTLs, so cookie Max-Age assertions can tell the two apart.
func rememberedTTLs(t *testing.T) (*AuthService, *testutils.MockUserRepository) {
	t.Helper()

	repo := seedLoginUser(t, "testuser", "test-password-123")
	service, err := NewTestAuthServiceWithRememberTTL(repo, testutils.NewMockSessionRepository(), 30*24*time.Hour, 90*24*time.Hour)
	require.NoError(t, err, "Should create auth service without error")
	return service, repo
}

// performLogin drives the Login handler with the given request payload
// and returns the recorded response.
func performLogin(h *AuthHandler, payload map[string]any) *http.Response {
	reqBody, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()
	h.Login(w, req)
	return w.Result()
}

// TestAuthHandler_Login_RememberedDeviceSetsPersistentCookie asserts a
// rememberDevice login answers a refresh cookie whose Max-Age is the
// full remembered TTL, keeping the four security attributes.
func TestAuthHandler_Login_RememberedDeviceSetsPersistentCookie(t *testing.T) {
	service, repo := rememberedTTLs(t)
	handler := NewAuthHandler(service, repo)

	resp := performLogin(handler, map[string]any{
		"username":       "testuser",
		"password":       "test-password-123",
		"rememberDevice": true,
	})
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "Remembered login should succeed")

	cookie := findRefreshCookie(resp)
	require.NotNil(t, cookie, "Remembered login should set a refresh_token cookie")
	assert.Equal(t, int((90 * 24 * time.Hour).Seconds()), cookie.MaxAge,
		"Remembered login cookie must carry the full remembered TTL as Max-Age")
	assert.True(t, cookie.HttpOnly, "Remembered cookie must stay httpOnly")
	assert.True(t, cookie.Secure, "Remembered cookie must stay Secure")
	assert.Equal(t, http.SameSiteStrictMode, cookie.SameSite, "Remembered cookie must stay SameSite=Strict")
	assert.Equal(t, "/api/auth", cookie.Path, "Remembered cookie must stay scoped to /api/auth")
}

// TestAuthHandler_Login_WithoutRememberDeviceKeepsSessionCookie asserts
// a login without the flag sets no Max-Age at all — the cookie stays a
// browser-session cookie, byte-identical to today's.
func TestAuthHandler_Login_WithoutRememberDeviceKeepsSessionCookie(t *testing.T) {
	service, repo := rememberedTTLs(t)
	handler := NewAuthHandler(service, repo)

	for name, payload := range map[string]map[string]any{
		"flag absent": {"username": "testuser", "password": "test-password-123"},
		"flag false":  {"username": "testuser", "password": "test-password-123", "rememberDevice": false},
	} {
		resp := performLogin(handler, payload)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "%s: login should succeed", name)

		cookie := findRefreshCookie(resp)
		require.NotNil(t, cookie, "%s: login should set a refresh_token cookie", name)
		assert.Zero(t, cookie.MaxAge, "%s: unremembered cookie must carry no Max-Age", name)
	}
}

// TestAuthHandler_Refresh_RememberedSessionResetsCookieMaxAge asserts
// refreshing a remembered session re-sets the rotated cookie's Max-Age
// to the full remembered TTL, so the cookie slides in lockstep with
// the renewed session row.
func TestAuthHandler_Refresh_RememberedSessionResetsCookieMaxAge(t *testing.T) {
	service, repo := rememberedTTLs(t)

	_, loginToken, _, _, err := service.Login("testuser", "test-password-123", true)
	require.NoError(t, err, "Remembered login should succeed")

	handler := NewAuthHandler(service, repo)
	resp := performRefreshWithCookie(handler, &http.Cookie{Name: "refresh_token", Value: loginToken})
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "Refreshing a remembered session should succeed")

	cookie := findRefreshCookie(resp)
	require.NotNil(t, cookie, "Remembered refresh should set a refresh_token cookie")
	assert.NotEqual(t, loginToken, cookie.Value, "Refresh cookie must carry the rotated token")
	assert.Equal(t, int((90 * 24 * time.Hour).Seconds()), cookie.MaxAge,
		"Remembered refresh must re-set the full remembered TTL as the cookie's Max-Age")
}

// TestAuthHandler_Refresh_UnrememberedSessionKeepsSessionCookie asserts
// refreshing a plain session keeps today's cookie: rotated value, no
// Max-Age.
func TestAuthHandler_Refresh_UnrememberedSessionKeepsSessionCookie(t *testing.T) {
	service, repo := rememberedTTLs(t)

	_, loginToken, _, _, err := service.Login("testuser", "test-password-123", false)
	require.NoError(t, err, "Plain login should succeed")

	handler := NewAuthHandler(service, repo)
	resp := performRefreshWithCookie(handler, &http.Cookie{Name: "refresh_token", Value: loginToken})
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "Refreshing a plain session should succeed")

	cookie := findRefreshCookie(resp)
	require.NotNil(t, cookie, "Plain refresh should set a refresh_token cookie")
	assert.NotEqual(t, loginToken, cookie.Value, "Refresh cookie must carry the rotated token")
	assert.Zero(t, cookie.MaxAge, "Plain refresh must carry no Max-Age")
}
