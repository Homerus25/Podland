package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// changePasswordBody is the handler test's request body builder.
type changePasswordBody struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

// performChangePassword drives the ChangePassword handler the way the
// router would: identity in the request context (the JWT middleware's
// contribution), JSON body attached.
func performChangePassword(t *testing.T, h *AuthHandler, userID string, body changePasswordBody) *http.Response {
	t.Helper()

	payload, err := json.Marshal(body)
	require.NoError(t, err, "Should marshal request body")
	return testutils.Perform(h.ChangePassword, testutils.Request{
		Method: http.MethodPost,
		Path:   "/api/auth/password",
		UserID: userID,
		Body:   bytes.NewReader(payload),
	}).Result()
}

// newChangePasswordTestSetup seeds a user with two live sessions and
// wires the handler over their repos, the shape every contract test
// below starts from.
func newChangePasswordTestSetup(t *testing.T, currentPassword string) (*AuthHandler, *testutils.MockUserRepository, *testutils.MockSessionRepository, string) {
	t.Helper()

	repo, sessionRepo, service, userID := seedPasswordChangeUser(t, "testuser", currentPassword)
	return NewAuthHandler(service, repo), repo, sessionRepo, userID
}

// TestAuthHandler_ChangePassword_Success asserts the endpoint contract:
// 200 with the fresh login body (token + username) and a new refresh
// cookie with the spec's four attributes, backed by a live session.
func TestAuthHandler_ChangePassword_Success(t *testing.T) {
	handler, repo, sessionRepo, userID := newChangePasswordTestSetup(t, "old-password-123")

	resp := performChangePassword(t, handler, userID, changePasswordBody{
		CurrentPassword: "old-password-123",
		NewPassword:     "new-pass-8",
	})
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "Successful change should return 200")

	cookie := findRefreshCookie(resp)
	require.NotNil(t, cookie, "Change should set a refresh_token cookie")
	assert.NotEmpty(t, cookie.Value, "Refresh cookie should carry the fresh token")
	assert.True(t, cookie.HttpOnly, "Refresh cookie must be httpOnly")
	assert.True(t, cookie.Secure, "Refresh cookie must be Secure")
	assert.Equal(t, http.SameSiteStrictMode, cookie.SameSite, "Refresh cookie must be SameSite=Strict")
	assert.Equal(t, "/api/auth", cookie.Path, "Refresh cookie must be scoped to /api/auth")

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body), "Should decode response body")
	assert.ElementsMatch(t, []string{"token", "username", "role"}, mapKeys(body), "Body should carry exactly the fresh login fields")
	assert.NotEmpty(t, body["token"], "Body should carry the new access JWT")
	assert.Equal(t, "testuser", body["username"], "Body should carry the caller's username")
	assert.Equal(t, "user", body["role"], "Body should carry the caller's role")

	require.Len(t, sessionRepo.SessionsForUser(userID), 1, "Only the fresh session should remain")
	user, err := repo.GetByID(userID)
	require.NoError(t, err, "Should look up the user")
	require.NotNil(t, user, "User should exist")
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte("new-pass-8")), "Stored hash should match the new password")
}

// TestAuthHandler_ChangePassword_WrongCurrentPassword asserts a wrong
// current password answers 401 INVALID_CREDENTIALS with the clear
// current-password message.
func TestAuthHandler_ChangePassword_WrongCurrentPassword(t *testing.T) {
	handler, _, _, userID := newChangePasswordTestSetup(t, "old-password-123")

	resp := performChangePassword(t, handler, userID, changePasswordBody{
		CurrentPassword: "wrong-current",
		NewPassword:     "new-pass-8",
	})
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Wrong current password should be a 401")
	assert.Equal(t, "INVALID_CREDENTIALS", testutils.DecodeErrorEnvelope(t, resp).Error.Code, "Should carry the credentials code")
}

// TestAuthHandler_ChangePassword_NewPasswordTooShort asserts the policy
// surfaces as 400 PASSWORD_TOO_SHORT.
func TestAuthHandler_ChangePassword_NewPasswordTooShort(t *testing.T) {
	handler, _, _, userID := newChangePasswordTestSetup(t, "old-password-123")

	resp := performChangePassword(t, handler, userID, changePasswordBody{
		CurrentPassword: "old-password-123",
		NewPassword:     "short7",
	})
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "A too-short password should be a 400")
	assert.Equal(t, "PASSWORD_TOO_SHORT", testutils.DecodeErrorEnvelope(t, resp).Error.Code, "Should carry the policy code")
}
