//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// adminWithID seeds an admin user and logs in through the real login
// endpoint, returning the bearer token, the admin's user ID, and the
// admin's password — the password-reset tests need the ID to count
// session rows (a reset never touches the caller's sessions) and the
// password as the body's adminPassword re-authentication value.
func adminWithID(t *testing.T) (string, string, string) {
	t.Helper()

	username := "it-" + suite.RunID + "-pwadmin-" + uuid.New().String()[:8]
	password := "it-" + suite.RunID + "-adminpw-" + uuid.New().String()[:8]
	adminID, err := suite.SeedUserWithRole(username, password, models.RoleAdmin)
	require.NoError(t, err, "Should seed admin user")

	token, err := realLogin(suite.ServerURL, username, password)
	require.NoError(t, err, "Should log in as the seeded admin")
	return token, adminID, password
}

// assertUpdatedUserBody pins the reset endpoint's wire shape on a decoded
// body: exactly id, username, role, createdAt — never password material.
func assertUpdatedUserBody(t *testing.T, body map[string]any) {
	t.Helper()

	assert.ElementsMatch(t,
		[]string{"id", "username", "role", "createdAt"},
		mapKeysAny(body),
		"Body shape must be exactly the updated user (id + username + role + createdAt)")
}

// TestUsers_ResetPassword_AdminResetRevokesTargetSessions is the
// demoable lifecycle of the admin password reset: the target is signed
// in on two devices, the admin sets a new password through the
// endpoint, and the fallout is exactly the spec — both target sessions
// are revoked (stolen refresh tokens die with the old password), the
// old password stops working, the new one logs in, and the admin's own
// session is untouched.
func TestUsers_ResetPassword_AdminResetRevokesTargetSessions(t *testing.T) {
	suite.SkipIfUnavailable(t)

	adminToken, adminID, adminPassword := adminWithID(t)

	username := "it-" + suite.RunID + "-resetpw-" + uuid.New().String()[:8]
	oldPassword := "it-" + suite.RunID + "-oldpw-" + uuid.New().String()[:8]
	newPassword := "brand-new-pw-" + uuid.New().String()[:8]
	userID, err := suite.SeedUserWithRole(username, oldPassword, "user")
	require.NoError(t, err, "Should seed reset target")
	db := openSuiteDB(t)

	// Two devices log in as the target.
	deviceA := suite.CookieJarClient()
	loginA := postJSONWithClient(t, deviceA, "/api/auth/login", map[string]string{
		"username": username,
		"password": oldPassword,
	})
	require.Equal(t, http.StatusOK, loginA.StatusCode, "Device A should log in")
	require.NoError(t, loginA.Body.Close(), "Should close device A login body")

	deviceB := suite.CookieJarClient()
	loginB := postJSONWithClient(t, deviceB, "/api/auth/login", map[string]string{
		"username": username,
		"password": oldPassword,
	})
	require.Equal(t, http.StatusOK, loginB.StatusCode, "Device B should log in")
	require.NoError(t, loginB.Body.Close(), "Should close device B login body")
	require.Len(t, sessionTokenHashes(t, db, userID), 2, "Two devices should leave two session rows")
	adminSessionsBefore := len(sessionTokenHashes(t, db, adminID))

	// The admin resets the target's password through the endpoint —
	// re-authenticating with their own password: 200 with the updated
	// user — exactly id, username, role, createdAt.
	resetResp := requestJSONWithBearer(t, http.DefaultClient, http.MethodPut, "/api/users/"+userID+"/password", adminToken, map[string]string{
		"adminPassword": adminPassword,
		"newPassword":   newPassword,
	})
	defer resetResp.Body.Close()
	require.Equal(t, http.StatusOK, resetResp.StatusCode, "Admin password reset should return 200")

	var resetBody map[string]any
	require.NoError(t, json.NewDecoder(resetResp.Body).Decode(&resetBody), "Should decode reset body")
	assert.Equal(t, userID, resetBody["id"], "Body should carry the target user's ID")
	assertUpdatedUserBody(t, resetBody)
	assert.NotContains(t, resetBody, "newPassword", "No password material may be echoed")

	// Database effect: every target session row is gone; the admin's
	// own sessions are untouched.
	assert.Empty(t, sessionTokenHashes(t, db, userID), "Every session of the target must be revoked")
	assert.Len(t, sessionTokenHashes(t, db, adminID), adminSessionsBefore, "The admin's own sessions must be untouched")

	// Device B is signed out: its jar-held cookie no longer addresses a
	// live session.
	refreshB := postJSONWithClient(t, deviceB, "/api/auth/refresh", map[string]string{})
	defer refreshB.Body.Close()
	require.Equal(t, http.StatusUnauthorized, refreshB.StatusCode, "Device B must be signed out")
	assert.Equal(t, "AUTH_REFRESH_INVALID", testutils.DecodeErrorEnvelope(t, refreshB).Error.Code, "The revoked session must report AUTH_REFRESH_INVALID")

	// The old password is dead; the new one logs in (leaving its own
	// fresh session row).
	oldLogin := postJSONWithClient(t, suite.CookieJarClient(), "/api/auth/login", map[string]string{
		"username": username,
		"password": oldPassword,
	})
	defer oldLogin.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, oldLogin.StatusCode, "The old password must stop working")

	newLogin := postJSONWithClient(t, suite.CookieJarClient(), "/api/auth/login", map[string]string{
		"username": username,
		"password": newPassword,
	})
	defer newLogin.Body.Close()
	assert.Equal(t, http.StatusOK, newLogin.StatusCode, "The new password must log in")
	assert.Len(t, sessionTokenHashes(t, db, userID), 1, "The new login should be the target's only live session")
}

// TestUsers_ResetPassword_SelfResetRejected pins the self-lockout
// guard on the real server (issue 187): an admin resetting their own
// password through the admin endpoint is refused with 400
// SELF_PASSWORD_RESET regardless of the supplied password's
// correctness — self-service change password is the only self path —
// and nothing is written: the admin's sessions survive and the old
// password keeps working.
func TestUsers_ResetPassword_SelfResetRejected(t *testing.T) {
	suite.SkipIfUnavailable(t)

	username := "it-" + suite.RunID + "-selfpw-" + uuid.New().String()[:8]
	password := "it-" + suite.RunID + "-oldpw-" + uuid.New().String()[:8]
	adminID, err := suite.SeedUserWithRole(username, password, models.RoleAdmin)
	require.NoError(t, err, "Should seed self-resetting admin")
	db := openSuiteDB(t)

	// The admin is signed in on two devices: the bearer token minted by
	// a plain login, and a cookie-jar client.
	adminToken, err := realLogin(suite.ServerURL, username, password)
	require.NoError(t, err, "Should log in as the admin (bearer token)")
	device := suite.CookieJarClient()
	login := postJSONWithClient(t, device, "/api/auth/login", map[string]string{
		"username": username,
		"password": password,
	})
	require.Equal(t, http.StatusOK, login.StatusCode, "Second device should log in")
	require.NoError(t, login.Body.Close(), "Should close second device login body")
	require.Len(t, sessionTokenHashes(t, db, adminID), 2, "Two sign-ins should leave two session rows")

	// Self reset with a wrong password: 400 SELF_PASSWORD_RESET — the
	// guard runs before any verification.
	for name, adminPassword := range map[string]string{
		"wrong password":   "not-the-admins-password",
		"correct password": password,
	} {
		t.Run(name, func(t *testing.T) {
			resetResp := requestJSONWithBearer(t, http.DefaultClient, http.MethodPut, "/api/users/"+adminID+"/password", adminToken, map[string]string{
				"adminPassword": adminPassword,
				"newPassword":   "brand-new-pw",
			})
			defer resetResp.Body.Close()
			require.Equal(t, http.StatusBadRequest, resetResp.StatusCode, "A self reset should be rejected with 400")

			envelope := testutils.DecodeErrorEnvelope(t, resetResp)
			assert.Equal(t, "SELF_PASSWORD_RESET", envelope.Error.Code, "Error body should carry SELF_PASSWORD_RESET")
			assert.Equal(t, "Cannot reset your own password", envelope.Error.Message, "The message should point at the self-service path")
		})
	}

	// Nothing was written: both of the admin's own session rows stand,
	// the second device still refreshes, and the old password works.
	assert.Len(t, sessionTokenHashes(t, db, adminID), 2, "A refused self reset must revoke nothing")

	refresh := postJSONWithClient(t, device, "/api/auth/refresh", map[string]string{})
	defer refresh.Body.Close()
	require.Equal(t, http.StatusOK, refresh.StatusCode, "The admin's second device must stay signed in")

	stillWorks := postJSONWithClient(t, suite.CookieJarClient(), "/api/auth/login", map[string]string{
		"username": username,
		"password": password,
	})
	defer stillWorks.Body.Close()
	assert.Equal(t, http.StatusOK, stillWorks.StatusCode, "The admin's password must be unchanged")
}

// TestUsers_ResetPassword_RejectsWrongAdminPassword pins the re-auth
// gate on the real server: a reset whose adminPassword does not verify
// answers 401 INVALID_CREDENTIALS with the change-password message and
// changes nothing — the target's sessions survive and the old password
// still works.
func TestUsers_ResetPassword_RejectsWrongAdminPassword(t *testing.T) {
	suite.SkipIfUnavailable(t)

	adminToken, _ /* adminID */, _ /* adminPassword */ := adminWithID(t)

	username := "it-" + suite.RunID + "-resetpw-" + uuid.New().String()[:8]
	oldPassword := "it-" + suite.RunID + "-oldpw-" + uuid.New().String()[:8]
	userID, err := suite.SeedUserWithRole(username, oldPassword, "user")
	require.NoError(t, err, "Should seed reset target")
	db := openSuiteDB(t)

	device := suite.CookieJarClient()
	login := postJSONWithClient(t, device, "/api/auth/login", map[string]string{
		"username": username,
		"password": oldPassword,
	})
	require.Equal(t, http.StatusOK, login.StatusCode, "Device should log in")
	require.NoError(t, login.Body.Close(), "Should close device login body")

	resetResp := requestJSONWithBearer(t, http.DefaultClient, http.MethodPut, "/api/users/"+userID+"/password", adminToken, map[string]string{
		"adminPassword": "not-the-admins-password",
		"newPassword":   "brand-new-pw",
	})
	defer resetResp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resetResp.StatusCode, "A wrong admin password must be a 401")

	envelope := testutils.DecodeErrorEnvelope(t, resetResp)
	assert.Equal(t, "INVALID_CREDENTIALS", envelope.Error.Code, "Should carry the credentials code")
	assert.Equal(t, "Current password is incorrect", envelope.Error.Message, "The message must match the change-password path")

	assert.Len(t, sessionTokenHashes(t, db, userID), 1, "Sessions must be untouched")

	stillWorks := postJSONWithClient(t, suite.CookieJarClient(), "/api/auth/login", map[string]string{
		"username": username,
		"password": oldPassword,
	})
	defer stillWorks.Body.Close()
	assert.Equal(t, http.StatusOK, stillWorks.StatusCode, "The target's password must be unchanged")
}

// TestUsers_ResetPassword_RejectsShortPassword asserts the password
// policy on the real server: a new password below 8 characters answers
// 400 PASSWORD_TOO_SHORT and changes nothing — the target's sessions
// survive and the old password still works.
func TestUsers_ResetPassword_RejectsShortPassword(t *testing.T) {
	suite.SkipIfUnavailable(t)

	adminToken, _ /* adminID */, adminPassword := adminWithID(t)

	username := "it-" + suite.RunID + "-shortpw-" + uuid.New().String()[:8]
	password := "it-" + suite.RunID + "-pw-" + uuid.New().String()[:8]
	userID, err := suite.SeedUserWithRole(username, password, "user")
	require.NoError(t, err, "Should seed short-password target")
	db := openSuiteDB(t)

	device := suite.CookieJarClient()
	login := postJSONWithClient(t, device, "/api/auth/login", map[string]string{
		"username": username,
		"password": password,
	})
	require.Equal(t, http.StatusOK, login.StatusCode, "Device should log in")
	require.NoError(t, login.Body.Close(), "Should close login body")

	resetResp := requestJSONWithBearer(t, http.DefaultClient, http.MethodPut, "/api/users/"+userID+"/password", adminToken, map[string]string{
		"adminPassword": adminPassword,
		"newPassword":   "short7",
	})
	defer resetResp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resetResp.StatusCode, "A too-short password must be a 400")
	assert.Equal(t, "PASSWORD_TOO_SHORT", testutils.DecodeErrorEnvelope(t, resetResp).Error.Code, "Should carry the policy code")

	assert.Len(t, sessionTokenHashes(t, db, userID), 1, "Sessions must be untouched")

	stillWorks := postJSONWithClient(t, suite.CookieJarClient(), "/api/auth/login", map[string]string{
		"username": username,
		"password": password,
	})
	defer stillWorks.Body.Close()
	assert.Equal(t, http.StatusOK, stillWorks.StatusCode, "The old password must still work after a rejected reset")
}

// TestUsers_ResetPassword_NonAdminForbidden asserts the admin gate on
// the real server: an authenticated regular user resetting someone's
// password answers 403 ADMIN_REQUIRED and changes nothing.
func TestUsers_ResetPassword_NonAdminForbidden(t *testing.T) {
	suite.SkipIfUnavailable(t)

	username := "it-" + suite.RunID + "-pleb-" + uuid.New().String()[:8]
	password := "it-" + suite.RunID + "-pw-" + uuid.New().String()[:8]
	_, err := suite.SeedUserWithRole(username, password, "user")
	require.NoError(t, err, "Should seed non-admin caller")
	plebToken, err := realLogin(suite.ServerURL, username, password)
	require.NoError(t, err, "Should log in as the non-admin caller")

	targetUsername := "it-" + suite.RunID + "-victim-" + uuid.New().String()[:8]
	targetPassword := "it-" + suite.RunID + "-pw-" + uuid.New().String()[:8]
	targetID, err := suite.SeedUserWithRole(targetUsername, targetPassword, "user")
	require.NoError(t, err, "Should seed reset target")
	db := openSuiteDB(t)

	victimLogin := postJSONWithClient(t, suite.CookieJarClient(), "/api/auth/login", map[string]string{
		"username": targetUsername,
		"password": targetPassword,
	})
	require.Equal(t, http.StatusOK, victimLogin.StatusCode, "Target should log in")
	require.NoError(t, victimLogin.Body.Close(), "Should close target login body")

	resetResp := requestJSONWithBearer(t, http.DefaultClient, http.MethodPut, "/api/users/"+targetID+"/password", plebToken, map[string]string{
		"newPassword": "brand-new-pw",
	})
	defer resetResp.Body.Close()
	require.Equal(t, http.StatusForbidden, resetResp.StatusCode, "Non-admin should be rejected with 403")
	assert.Equal(t, "ADMIN_REQUIRED", testutils.DecodeErrorEnvelope(t, resetResp).Error.Code, "Should carry the ADMIN_REQUIRED code")

	assert.Len(t, sessionTokenHashes(t, db, targetID), 1, "The target's sessions must be untouched")

	stillWorks := postJSONWithClient(t, suite.CookieJarClient(), "/api/auth/login", map[string]string{
		"username": targetUsername,
		"password": targetPassword,
	})
	defer stillWorks.Body.Close()
	assert.Equal(t, http.StatusOK, stillWorks.StatusCode, "The target's password must be unchanged")
}

// TestUsers_ResetPassword_UnknownUser asserts resetting the password of
// a user that does not exist is answered with 404 USER_NOT_FOUND.
func TestUsers_ResetPassword_UnknownUser(t *testing.T) {
	suite.SkipIfUnavailable(t)

	adminToken, _ /* adminID */, adminPassword := adminWithID(t)

	ghostID := uuid.New().String()
	resetResp := requestJSONWithBearer(t, http.DefaultClient, http.MethodPut, "/api/users/"+ghostID+"/password", adminToken, map[string]string{
		"adminPassword": adminPassword,
		"newPassword":   "brand-new-pw",
	})
	defer resetResp.Body.Close()
	require.Equal(t, http.StatusNotFound, resetResp.StatusCode, "A missing target user must be a 404")
	assert.Equal(t, "USER_NOT_FOUND", testutils.DecodeErrorEnvelope(t, resetResp).Error.Code, "Should carry the not-found code")
}

// TestUsers_ResetPassword_Unauthenticated asserts a caller without a
// token is rejected by the JWT middleware with 401 before the admin
// gate or handler ever run.
func TestUsers_ResetPassword_Unauthenticated(t *testing.T) {
	suite.SkipIfUnavailable(t)

	reqBody, err := json.Marshal(map[string]string{"newPassword": "brand-new-pw"})
	require.NoError(t, err, "Should marshal request body")
	req, err := http.NewRequest(http.MethodPut, suite.ServerURL+"/api/users/"+uuid.New().String()+"/password", bytes.NewReader(reqBody))
	require.NoError(t, err, "Should build request")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "Should PUT password endpoint")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Missing token should be rejected with 401")
}
