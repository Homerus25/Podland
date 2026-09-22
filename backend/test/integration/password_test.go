//go:build integration

package integration

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requestJSONWithBearer sends body as JSON with the given method and an
// explicit bearer token through the given client. Password-authenticated
// endpoints (the self-service change) and admin endpoints (the users
// mutations) authenticate with the Authorization header — the refresh
// cookie is not a bearer credential — while the passed client's jar
// still rides along to capture any response Set-Cookie.
func requestJSONWithBearer(t *testing.T, client *http.Client, method, path, bearerToken string, body any) *http.Response {
	t.Helper()

	reqBody, err := json.Marshal(body)
	require.NoError(t, err, "Should marshal request body")

	req, err := http.NewRequest(method, suite.ServerURL+path, bytes.NewReader(reqBody))
	require.NoError(t, err, "Should build request")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+bearerToken)

	resp, err := client.Do(req)
	require.NoError(t, err, "Should send the request")
	return resp
}

// TestAuth_PasswordChange_FullLifecycle is the demoable lifecycle of
// the self-service password change: two devices log in, device A
// changes its password, and the fallout is exactly the spec — device A
// receives a fresh login (body + new refresh cookie) and stays signed
// in, device B's session is revoked, the old password stops working,
// and only the fresh session row survives in the database.
func TestAuth_PasswordChange_FullLifecycle(t *testing.T) {
	suite.SkipIfUnavailable(t)

	username := "it-" + suite.RunID + "-pwuser"
	oldPassword := "it-" + suite.RunID + "-oldpw-" + uuid.New().String()[:8]
	newPassword := "brand-new-pw-" + uuid.New().String()[:8]
	userID, err := suite.SeedUserWithRole(username, oldPassword, "user")
	require.NoError(t, err, "Should seed password-change user")
	db := openSuiteDB(t)

	// Device A and device B log in with separate cookie jars.
	deviceA := suite.CookieJarClient()
	loginA := postJSONWithClient(t, deviceA, "/api/auth/login", map[string]string{
		"username": username,
		"password": oldPassword,
	})
	require.Equal(t, http.StatusOK, loginA.StatusCode, "Device A should log in")
	tokenA := decodeTokenBody(t, loginA)
	require.NoError(t, loginA.Body.Close(), "Should close device A login body")

	deviceB := suite.CookieJarClient()
	loginB := postJSONWithClient(t, deviceB, "/api/auth/login", map[string]string{
		"username": username,
		"password": oldPassword,
	})
	require.Equal(t, http.StatusOK, loginB.StatusCode, "Device B should log in")
	require.NoError(t, loginB.Body.Close(), "Should close device B login body")
	require.Len(t, sessionTokenHashes(t, db, userID), 2, "Two devices should leave two session rows")

	// Device A changes its password (bearer-authenticated): 200 with
	// the fresh login body and a new refresh cookie carrying every
	// security attribute.
	changeResp := requestJSONWithBearer(t, deviceA, http.MethodPost, "/api/auth/password", tokenA, map[string]string{
		"currentPassword": oldPassword,
		"newPassword":     newPassword,
	})
	defer changeResp.Body.Close()
	require.Equal(t, http.StatusOK, changeResp.StatusCode, "Password change should succeed")

	var changeBody map[string]any
	require.NoError(t, json.NewDecoder(changeResp.Body).Decode(&changeBody), "Should decode change body")
	require.Contains(t, changeBody, "token", "Body should carry the fresh access JWT")
	require.NotEmpty(t, changeBody["token"], "Fresh access JWT must not be empty")
	assert.Equal(t, username, changeBody["username"], "Body should carry the caller's username")
	require.Len(t, changeBody, 3, "Body shape must be exactly the fresh login (token + username + role)")

	freshCookie := refreshCookieFrom(t, changeResp)
	assert.NotEmpty(t, freshCookie.Value, "Fresh cookie should carry a new refresh token")
	assert.True(t, freshCookie.HttpOnly, "Fresh cookie must be httpOnly")
	assert.True(t, freshCookie.Secure, "Fresh cookie must be Secure")
	assert.Equal(t, http.SameSiteStrictMode, freshCookie.SameSite, "Fresh cookie must be SameSite=Strict")
	assert.Equal(t, "/api/auth", freshCookie.Path, "Fresh cookie must be scoped to /api/auth")

	// Database effect: every pre-existing row is gone; the only session
	// left is the fresh login's, stored as the hash of the cookie value.
	hashes := sessionTokenHashes(t, db, userID)
	require.Len(t, hashes, 1, "Only the fresh login's session row should survive")
	expectedHash := sha256.Sum256([]byte(freshCookie.Value))
	assert.Equal(t, hex.EncodeToString(expectedHash[:]), hashes[0],
		"The surviving row must be the fresh session (hashed token only)")

	// Device B is signed out: its jar-held cookie no longer addresses a
	// live session.
	refreshB := postJSONWithClient(t, deviceB, "/api/auth/refresh", map[string]string{})
	defer refreshB.Body.Close()
	require.Equal(t, http.StatusUnauthorized, refreshB.StatusCode, "Device B must be signed out")
	assert.Equal(t, "AUTH_REFRESH_INVALID", testutils.DecodeErrorEnvelope(t, refreshB).Error.Code, "The revoked session must report AUTH_REFRESH_INVALID")

	// Device A stays signed in: the fresh cookie its jar adopted
	// refreshes, rotating its own row without adding one.
	refreshA := postJSONWithClient(t, deviceA, "/api/auth/refresh", map[string]string{})
	defer refreshA.Body.Close()
	require.Equal(t, http.StatusOK, refreshA.StatusCode, "The changing device must stay signed in")
	decodeTokenBody(t, refreshA)
	assert.Len(t, sessionTokenHashes(t, db, userID), 1, "Device A's refresh should rotate the fresh row, not add one")

	// The old password is dead; the new one logs in (leaving its own
	// fresh session next to device A's).
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
	assert.Len(t, sessionTokenHashes(t, db, userID), 2, "The new login should add its own session beside device A's")
}

// TestAuth_PasswordChange_Unauthenticated asserts the JWT gate on the
// real server: a caller without an Authorization header is rejected
// with 401 TOKEN_MISSING before the handler ever runs.
func TestAuth_PasswordChange_Unauthenticated(t *testing.T) {
	suite.SkipIfUnavailable(t)

	reqBody, err := json.Marshal(map[string]string{
		"currentPassword": "whatever-old",
		"newPassword":     "brand-new-pw",
	})
	require.NoError(t, err, "Should marshal request body")
	req, err := http.NewRequest(http.MethodPost, suite.ServerURL+"/api/auth/password", bytes.NewReader(reqBody))
	require.NoError(t, err, "Should build request")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "Should POST password endpoint")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Missing token should be rejected with 401")
	assert.Equal(t, "TOKEN_MISSING", testutils.DecodeErrorEnvelope(t, resp).Error.Code, "A missing Authorization header should answer TOKEN_MISSING")
}

// TestAuth_PasswordChange_RejectsWrongCurrentPassword asserts the
// authority gate on the real server: a wrong current password answers
// 401 INVALID_CREDENTIALS and changes nothing — the old password still
// works and the sessions survive.
func TestAuth_PasswordChange_RejectsWrongCurrentPassword(t *testing.T) {
	suite.SkipIfUnavailable(t)

	username := "it-" + suite.RunID + "-pwwrong"
	password := "it-" + suite.RunID + "-wrongpw-" + uuid.New().String()[:8]
	userID, err := suite.SeedUserWithRole(username, password, "user")
	require.NoError(t, err, "Should seed wrong-current user")
	db := openSuiteDB(t)

	device := suite.CookieJarClient()
	login := postJSONWithClient(t, device, "/api/auth/login", map[string]string{
		"username": username,
		"password": password,
	})
	require.Equal(t, http.StatusOK, login.StatusCode, "Device should log in")
	token := decodeTokenBody(t, login)
	require.NoError(t, login.Body.Close(), "Should close login body")

	changeResp := requestJSONWithBearer(t, device, http.MethodPost, "/api/auth/password", token, map[string]string{
		"currentPassword": "not-the-current-password",
		"newPassword":     "replacement-pw",
	})
	defer changeResp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, changeResp.StatusCode, "A wrong current password must be a 401")
	envelope := testutils.DecodeErrorEnvelope(t, changeResp)
	assert.Equal(t, "INVALID_CREDENTIALS", envelope.Error.Code, "Should carry the credentials code")
	assert.NotEmpty(t, envelope.Error.Message, "The error should be clear about the current password")

	assert.Len(t, sessionTokenHashes(t, db, userID), 1, "Sessions must be untouched")

	stillWorks := postJSONWithClient(t, suite.CookieJarClient(), "/api/auth/login", map[string]string{
		"username": username,
		"password": password,
	})
	defer stillWorks.Body.Close()
	assert.Equal(t, http.StatusOK, stillWorks.StatusCode, "The old password must still work after a rejected change")
}

// TestAuth_PasswordChange_RejectsShortPassword asserts the password
// policy on the real server: a new password below 8 characters answers
// 400 PASSWORD_TOO_SHORT and changes nothing.
func TestAuth_PasswordChange_RejectsShortPassword(t *testing.T) {
	suite.SkipIfUnavailable(t)

	username := "it-" + suite.RunID + "-pwshort"
	password := "it-" + suite.RunID + "-shortpw-" + uuid.New().String()[:8]
	userID, err := suite.SeedUserWithRole(username, password, "user")
	require.NoError(t, err, "Should seed short-password user")
	db := openSuiteDB(t)

	device := suite.CookieJarClient()
	login := postJSONWithClient(t, device, "/api/auth/login", map[string]string{
		"username": username,
		"password": password,
	})
	require.Equal(t, http.StatusOK, login.StatusCode, "Device should log in")
	token := decodeTokenBody(t, login)
	require.NoError(t, login.Body.Close(), "Should close login body")

	changeResp := requestJSONWithBearer(t, device, http.MethodPost, "/api/auth/password", token, map[string]string{
		"currentPassword": password,
		"newPassword":     "short7",
	})
	defer changeResp.Body.Close()
	require.Equal(t, http.StatusBadRequest, changeResp.StatusCode, "A too-short password must be a 400")
	assert.Equal(t, "PASSWORD_TOO_SHORT", testutils.DecodeErrorEnvelope(t, changeResp).Error.Code, "Should carry the policy code")

	assert.Len(t, sessionTokenHashes(t, db, userID), 1, "Sessions must be untouched")

	stillWorks := postJSONWithClient(t, suite.CookieJarClient(), "/api/auth/login", map[string]string{
		"username": username,
		"password": password,
	})
	defer stillWorks.Body.Close()
	assert.Equal(t, http.StatusOK, stillWorks.StatusCode, "The old password must still work after a rejected change")
}
