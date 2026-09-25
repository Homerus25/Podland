package users

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/domain/auth"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// putUserPasswordAs PUTs the given JSON body to
// /api/users/{targetID}/password with a bearer token minted for
// callerID (empty string sends no Authorization header).
func putUserPasswordAs(t *testing.T, router http.Handler, authService *auth.AuthService, callerID, targetID, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPut, "/api/users/"+targetID+"/password", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if callerID != "" {
		token, err := auth.CreateValidTestToken(authService, callerID)
		require.NoError(t, err, "Should mint a test token")
		req.Header.Set("Authorization", "Bearer "+token)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// seedTargetSessions adds two live sessions for user-1 and one for
// admin-1, the shape the revocation assertions below start from.
func seedTargetSessions(t *testing.T, sessionRepo *testutils.MockSessionRepository) {
	t.Helper()

	created := time.Now()
	sessionRepo.AddSession(&models.Session{ID: "s-1", UserID: "user-1", TokenHash: "stolen-hash-1", CreatedAt: created, ExpiresAt: created.Add(time.Hour)})
	sessionRepo.AddSession(&models.Session{ID: "s-2", UserID: "user-1", TokenHash: "stolen-hash-2", CreatedAt: created, ExpiresAt: created.Add(time.Hour)})
	sessionRepo.AddSession(&models.Session{ID: "s-3", UserID: "admin-1", TokenHash: "admin-hash-1", CreatedAt: created, ExpiresAt: created.Add(time.Hour)})
}

// TestUserHandler_ResetPassword_AdminResetsUser asserts the happy path:
// an admin resets a user's password with their own password in the
// body, gets 200 with the updated user — exactly id, username, role,
// createdAt — every one of the target's sessions is revoked, and the
// admin's own session survives.
func TestUserHandler_ResetPassword_AdminResetsUser(t *testing.T) {
	router, authService, sessionRepo := newUsersTestRouter(t)
	seedTargetSessions(t, sessionRepo)

	w := putUserPasswordAs(t, router, authService, "admin-1", "user-1",
		`{"adminPassword":"known-admin-password","newPassword":"brand-new-pw"}`)

	resp := w.Result()
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "Admin password reset should return 200")

	var body map[string]any
	err := json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err, "Should decode the updated-user response")
	assert.Equal(t, "user-1", body["id"], "Response should carry the target user's ID")
	assert.Equal(t, "pleb", body["username"], "Response should carry the target's username")

	assert.ElementsMatch(t,
		[]string{"id", "username", "role", "createdAt"},
		mapKeys(body),
		"Response must expose exactly the documented fields — never password material")

	assert.Empty(t, sessionRepo.SessionsForUser("user-1"), "Every session of the target must be revoked")
	assert.Len(t, sessionRepo.SessionsForUser("admin-1"), 1, "The admin's own session must be untouched")
}

// TestUserHandler_ResetPassword_WrongAdminPasswordRejected pins the
// re-auth gate at the wire: a wrong admin password answers 401
// INVALID_CREDENTIALS carrying the change-password message, and writes
// nothing — the target's sessions survive.
func TestUserHandler_ResetPassword_WrongAdminPasswordRejected(t *testing.T) {
	router, authService, sessionRepo := newUsersTestRouter(t)
	seedTargetSessions(t, sessionRepo)

	w := putUserPasswordAs(t, router, authService, "admin-1", "user-1",
		`{"adminPassword":"not-the-admins-password","newPassword":"brand-new-pw"}`)

	resp := w.Result()
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "A wrong admin password should be rejected with 401")

	envelope := testutils.DecodeErrorEnvelope(t, resp)
	assert.Equal(t, "INVALID_CREDENTIALS", envelope.Error.Code, "Error body should carry INVALID_CREDENTIALS")
	assert.Equal(t, "Current password is incorrect", envelope.Error.Message, "The message must match the change-password path")

	assert.Len(t, sessionRepo.SessionsForUser("user-1"), 2, "Sessions must be untouched")
}

// TestUserHandler_ResetPassword_SelfResetRejected pins the self-lockout
// guard at the wire: an admin PUTting their own ID gets 400
// SELF_PASSWORD_RESET even with a wrong password — the guard runs
// before verification — and nothing is written.
func TestUserHandler_ResetPassword_SelfResetRejected(t *testing.T) {
	router, authService, sessionRepo := newUsersTestRouter(t)
	seedTargetSessions(t, sessionRepo)

	w := putUserPasswordAs(t, router, authService, "admin-1", "admin-1",
		`{"adminPassword":"not-the-admins-password","newPassword":"brand-new-pw"}`)

	resp := w.Result()
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, "A self reset should be rejected with 400")

	envelope := testutils.DecodeErrorEnvelope(t, resp)
	assert.Equal(t, "SELF_PASSWORD_RESET", envelope.Error.Code, "Error body should carry SELF_PASSWORD_RESET")
	assert.Equal(t, "Cannot reset your own password", envelope.Error.Message, "The message should point at the self-service path")

	assert.Len(t, sessionRepo.SessionsForUser("admin-1"), 1, "The admin's own session must be untouched")
	assert.Len(t, sessionRepo.SessionsForUser("user-1"), 2, "Other users' sessions must be untouched")
}

// TestUserHandler_ResetPassword_ShortPasswordRejected asserts the
// policy surfaces as 400 PASSWORD_TOO_SHORT and changes nothing — the
// target's sessions survive.
func TestUserHandler_ResetPassword_ShortPasswordRejected(t *testing.T) {
	router, authService, sessionRepo := newUsersTestRouter(t)
	seedTargetSessions(t, sessionRepo)

	w := putUserPasswordAs(t, router, authService, "admin-1", "user-1",
		`{"adminPassword":"known-admin-password","newPassword":"short7"}`)

	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "A too-short password should be rejected with 400")
	assert.Equal(t, "PASSWORD_TOO_SHORT", testutils.DecodeErrorEnvelope(t, resp).Error.Code, "Error body should carry PASSWORD_TOO_SHORT")

	assert.Len(t, sessionRepo.SessionsForUser("user-1"), 2, "Sessions must be untouched")
}

// TestUserHandler_ResetPassword_MissingPasswordRejected asserts the
// decode ritual: a body missing either required field — adminPassword
// or newPassword — fails the required tag and answers 400
// VALIDATION_FAILED before the service runs.
func TestUserHandler_ResetPassword_MissingPasswordRejected(t *testing.T) {
	router, authService, sessionRepo := newUsersTestRouter(t)
	seedTargetSessions(t, sessionRepo)

	for name, body := range map[string]string{
		"missing adminPassword": `{"newPassword":"brand-new-pw"}`,
		"empty adminPassword":   `{"adminPassword":"","newPassword":"brand-new-pw"}`,
		"missing newPassword":   `{"adminPassword":"known-admin-password"}`,
		"empty newPassword":     `{"adminPassword":"known-admin-password","newPassword":""}`,
	} {
		t.Run(name, func(t *testing.T) {
			w := putUserPasswordAs(t, router, authService, "admin-1", "user-1", body)

			resp := w.Result()
			defer resp.Body.Close()
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "A missing password should be rejected with 400")
			assert.Equal(t, "VALIDATION_FAILED", testutils.DecodeErrorEnvelope(t, resp).Error.Code, "Error body should carry VALIDATION_FAILED")
		})
	}

	assert.Len(t, sessionRepo.SessionsForUser("user-1"), 2, "Sessions must be untouched")
}

// TestUserHandler_ResetPassword_TargetNotFound asserts resetting the
// password of a user that does not exist is answered with 404
// USER_NOT_FOUND.
func TestUserHandler_ResetPassword_TargetNotFound(t *testing.T) {
	router, authService, _ := newUsersTestRouter(t)

	w := putUserPasswordAs(t, router, authService, "admin-1", "ghost-id",
		`{"adminPassword":"known-admin-password","newPassword":"brand-new-pw"}`)

	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "A missing target user should be rejected with 404")
	assert.Equal(t, "USER_NOT_FOUND", testutils.DecodeErrorEnvelope(t, resp).Error.Code, "Error body should carry USER_NOT_FOUND")
}

// TestUserHandler_ResetPassword_NonAdminForbidden asserts an
// authenticated caller with the regular user role gets 403
// ADMIN_REQUIRED and nothing changes.
func TestUserHandler_ResetPassword_NonAdminForbidden(t *testing.T) {
	router, authService, sessionRepo := newUsersTestRouter(t)
	seedTargetSessions(t, sessionRepo)

	w := putUserPasswordAs(t, router, authService, "user-1", "admin-1", `{"newPassword":"brand-new-pw"}`)

	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "Non-admin should be rejected with 403")
	assert.Equal(t, "ADMIN_REQUIRED", testutils.DecodeErrorEnvelope(t, resp).Error.Code, "Error body should carry the ADMIN_REQUIRED code")

	assert.Len(t, sessionRepo.SessionsForUser("admin-1"), 1, "The admin's sessions must be untouched")
}

// TestUserHandler_ResetPassword_Unauthenticated asserts a caller
// without a token is rejected by the JWT middleware with 401 before
// the admin gate or handler ever run.
func TestUserHandler_ResetPassword_Unauthenticated(t *testing.T) {
	router, authService, _ := newUsersTestRouter(t)

	w := putUserPasswordAs(t, router, authService, "", "user-1", `{"newPassword":"brand-new-pw"}`)

	assert.Equal(t, http.StatusUnauthorized, w.Result().StatusCode, "Missing token should be rejected with 401")
}
