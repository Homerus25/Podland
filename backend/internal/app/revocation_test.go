// Package app_test: the access-token revocation wiring pins (issue
// 441). These ride the full app seam — real SQLite with migration 005,
// the real revoked_access_tokens repository, the real JWT middleware —
// so they pin the wiring, not just the service behaviour.
package app_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/app"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// decodeErrorCode reads the parseable code out of an already-read
// error envelope body.
func decodeErrorCode(t *testing.T, raw []byte) string {
	t.Helper()

	var envelope testutils.ErrorEnvelope
	require.NoError(t, json.Unmarshal(raw, &envelope), "Error envelope must decode: %s", string(raw))
	return envelope.Error.Code
}

// startConfiguredApp boots the app, runs first-run setup to create the
// admin, and returns the server URL plus the admin's access token.
func startConfiguredApp(t *testing.T) (string, string) {
	t.Helper()

	a, err := app.New(newTestConfig(t))
	require.NoError(t, err)
	t.Cleanup(func() { _ = a.Close() })

	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)

	resp, raw := postJSON(t, srv.URL, "/api/setup/user", `{"username":"admin","password":"admin-password-1"}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Setup must succeed on a fresh instance: %s", string(raw))

	var body loginBody
	require.NoError(t, json.Unmarshal(raw, &body), "Setup body must decode")
	require.NotEmpty(t, body.Token, "Setup must hand back an access token")
	return srv.URL, body.Token
}

// doWithBearerBody performs the request with the given Authorization
// bearer and JSON body attached and returns the response with the body
// read. doWithBearer is its no-body form.
func doWithBearerBody(t *testing.T, method, baseURL, path, bearer, body string) (*http.Response, []byte) {
	t.Helper()

	req, err := http.NewRequest(method, baseURL+path, strings.NewReader(body))
	require.NoError(t, err, "%s %s must build", method, path)
	req.Header.Set("Authorization", "Bearer "+bearer)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "%s %s must connect", method, path)
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read %s response body", path)
	return resp, raw
}

// doWithBearer performs the request with the given Authorization
// bearer attached and no body.
func doWithBearer(t *testing.T, method, baseURL, path, bearer string) (*http.Response, []byte) {
	t.Helper()
	return doWithBearerBody(t, method, baseURL, path, bearer, "")
}

// TestApp_Handler_LogoutRevokesAccessToken pins the denylist wiring
// (issue 441) over the full seam: after POST /api/auth/logout with the
// Authorization bearer, the old token answers 401 TOKEN_REVOKED on a
// protected endpoint — the exact confirmed-live gap of pentest finding
// 4 (old bearer still 200 on GET /api/users). The token crosses the
// real revoked_access_tokens table and the real JWT middleware, so a
// dropped wiring (an unwired WithRevokedTokenRepo) fails here.
func TestApp_Handler_LogoutRevokesAccessToken(t *testing.T) {
	baseURL, token := startConfiguredApp(t)

	resp, raw := doWithBearer(t, http.MethodPost, baseURL, "/api/auth/logout", token)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Logout must succeed: %s", string(raw))

	protected, protectedRaw := doWithBearer(t, http.MethodGet, baseURL, "/api/users", token)

	require.Equal(t, http.StatusUnauthorized, protected.StatusCode, "The logged-out bearer must be rejected")
	assert.Equal(t, "TOKEN_REVOKED", decodeErrorCode(t, protectedRaw), "The rejection must name the revocation, not a generic invalid token")
}

// TestApp_Handler_PasswordChangeRevokesOldAccessTokens pins the cutoff
// wiring (issue 441) over the full seam: after the self-service password
// change, every access token minted before the change answers 401
// TOKEN_REVOKED while the fresh token the change returns still works.
// The sleep crosses the cutoff's second-precision boundary so the
// pre-change tokens' iat is strictly before the stamp (a token minted
// in the cutoff's own second is not older than it and correctly
// survives).
func TestApp_Handler_PasswordChangeRevokesOldAccessTokens(t *testing.T) {
	baseURL, setupToken := startConfiguredApp(t)

	loginResp, loginRaw := postJSON(t, baseURL, "/api/auth/login", `{"username":"admin","password":"admin-password-1"}`)
	require.Equal(t, http.StatusOK, loginResp.StatusCode, "Login must succeed: %s", string(loginRaw))
	var login loginBody
	require.NoError(t, json.Unmarshal(loginRaw, &login), "Login body must decode")

	time.Sleep(1100 * time.Millisecond)

	changeResp, changeRaw := doWithBearerBody(t, http.MethodPost, baseURL, "/api/auth/password", setupToken,
		`{"currentPassword":"admin-password-1","newPassword":"admin-password-2"}`)
	require.Equal(t, http.StatusOK, changeResp.StatusCode, "Password change must succeed: %s", string(changeRaw))
	var changed loginBody
	require.NoError(t, json.Unmarshal(changeRaw, &changed), "Change body must decode")

	for name, token := range map[string]string{"the setup token": setupToken, "the pre-change login token": login.Token} {
		dead, deadRaw := doWithBearer(t, http.MethodGet, baseURL, "/api/users", token)
		require.Equal(t, http.StatusUnauthorized, dead.StatusCode, "%s must be rejected after the change", name)
		assert.Equal(t, "TOKEN_REVOKED", decodeErrorCode(t, deadRaw),
			"%s's rejection must name the revocation", name)
	}

	live, _ := doWithBearer(t, http.MethodGet, baseURL, "/api/users", changed.Token)
	assert.Equal(t, http.StatusOK, live.StatusCode, "The fresh token minted after the cutoff must still work")
}
