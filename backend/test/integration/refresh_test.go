//go:build integration

package integration

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// postJSONWithClient POSTs body as JSON through the given client and
// returns the raw response, so tests can inspect both body and cookies.
func postJSONWithClient(t *testing.T, client *http.Client, path string, body any) *http.Response {
	t.Helper()

	reqBody, err := json.Marshal(body)
	require.NoError(t, err, "Should marshal request body")

	resp, err := client.Post(suite.ServerURL+path, "application/json", bytes.NewReader(reqBody))
	require.NoError(t, err, "Should POST to "+path)
	return resp
}

// decodeTokenBody reads a {"token": ...} JSON body (login/refresh shape).
func decodeTokenBody(t *testing.T, resp *http.Response) string {
	t.Helper()

	var body struct {
		Token string `json:"token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body), "Should decode token body")
	require.NotEmpty(t, body.Token, "Body should carry a token")
	return body.Token
}

// getWithBearer performs an authenticated GET with an explicit access
// token — unlike suite.AuthenticatedClient, which is pinned to the
// suite-level login token.
func getWithBearer(t *testing.T, path, accessToken string) *http.Response {
	t.Helper()

	req, err := http.NewRequest("GET", suite.ServerURL+path, nil)
	require.NoError(t, err, "Should build request")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "Should GET "+path)
	return resp
}

// postWithCookie performs a POST carrying exactly one explicit cookie and
// no jar, so a replayed value cannot be shadowed by a newer stored one.
func postWithCookie(t *testing.T, path string, cookie *http.Cookie) *http.Response {
	t.Helper()

	req, err := http.NewRequest("POST", suite.ServerURL+path, nil)
	require.NoError(t, err, "Should build request")
	req.AddCookie(cookie)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "Should POST "+path)
	return resp
}

// TestAuth_Refresh_RotatesCookieAndKeepsSecondDeviceWorking is the
// demoable cycle of the refresh feature: a device logs in through a
// cookie-jar client, refreshes (new access JWT + rotated cookie, one
// session row — not two), cannot replay the old cookie, and a second
// device's independent session still refreshes fine afterwards.
func TestAuth_Refresh_RotatesCookieAndKeepsSecondDeviceWorking(t *testing.T) {
	suite.SkipIfUnavailable(t)

	username := "it-" + suite.RunID + "-refreshuser"
	password := "it-" + suite.RunID + "-refreshpw-" + uuid.New().String()[:8]
	userID, err := suite.SeedUserWithRole(username, password, "user")
	require.NoError(t, err, "Should seed refresh-test user")
	db := openSuiteDB(t)

	// Device 1 logs in through a jar client; the refresh cookie now rides
	// along on every /api/auth request, browser-style.
	device1 := suite.CookieJarClient()
	loginResp := postJSONWithClient(t, device1, "/api/auth/login", map[string]string{
		"username": username,
		"password": password,
	})
	// Decode (and thereby assert) the login body still carries an access
	// JWT; its value is not compared against the refresh token — see the
	// deterministic-signage note below.
	decodeTokenBody(t, loginResp)
	oldCookie := refreshCookieFrom(t, loginResp)
	require.NoError(t, loginResp.Body.Close(), "Should close login body")
	require.Len(t, sessionTokenHashes(t, db, userID), 1, "Login should leave one session row")

	// Refresh with the jar-held cookie: 200, a fresh access JWT, and a
	// rotated refresh cookie with the spec's four attributes.
	refreshResp := postJSONWithClient(t, device1, "/api/auth/refresh", map[string]string{})
	require.Equal(t, http.StatusOK, refreshResp.StatusCode, "Refresh with the login cookie should succeed")
	newAccessToken := decodeTokenBody(t, refreshResp)
	newCookie := refreshCookieFrom(t, refreshResp)
	require.NoError(t, refreshResp.Body.Close(), "Should close refresh body")

	// The new access JWT authenticates the same user (JWTs minted within
	// the same second for the same user may be byte-identical — RS256
	// signing is deterministic — so the cycle's rotation guarantee lives
	// in the refresh cookie and the session row, asserted below).
	assert.NotEqual(t, oldCookie.Value, newCookie.Value, "Refresh must rotate the cookie value")
	assert.True(t, newCookie.HttpOnly, "Refresh cookie must be httpOnly")
	assert.True(t, newCookie.Secure, "Refresh cookie must be Secure")
	assert.Equal(t, http.SameSiteStrictMode, newCookie.SameSite, "Refresh cookie must be SameSite=Strict")
	assert.Equal(t, "/api/auth", newCookie.Path, "Refresh cookie must be scoped to /api/auth")

	// Rotation replaces the row: still exactly one, keyed by the new
	// cookie's hash — the old token's row is gone, not duplicated.
	hashes := sessionTokenHashes(t, db, userID)
	require.Len(t, hashes, 1, "Rotation should replace the session row, not add one")
	expectedHash := sha256.Sum256([]byte(newCookie.Value))
	assert.Equal(t, hex.EncodeToString(expectedHash[:]), hashes[0],
		"The surviving row must be the SHA-256 hash of the rotated cookie")

	statusResp := getWithBearer(t, "/api/auth/status", newAccessToken)
	defer statusResp.Body.Close()
	require.Equal(t, http.StatusOK, statusResp.StatusCode, "Status with the refreshed JWT should succeed")

	// Replaying the already-rotated cookie fails cleanly.
	replayResp := postWithCookie(t, "/api/auth/refresh", oldCookie)
	defer replayResp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, replayResp.StatusCode, "Replaying the old cookie must fail")
	var replayEnvelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.NewDecoder(replayResp.Body).Decode(&replayEnvelope), "Should decode replay error envelope")
	assert.Equal(t, "AUTH_REFRESH_INVALID", replayEnvelope.Error.Code, "Replay must report AUTH_REFRESH_INVALID")
	assert.Len(t, sessionTokenHashes(t, db, userID), 1, "A rejected replay must not disturb the rotated row")

	// Device 2's independent session survives device 1's rotation and
	// refreshes fine — multi-device, per-device sessions.
	device2 := suite.CookieJarClient()
	secondLogin := postJSONWithClient(t, device2, "/api/auth/login", map[string]string{
		"username": username,
		"password": password,
	})
	require.Equal(t, http.StatusOK, secondLogin.StatusCode, "Second device should log in")
	require.NoError(t, secondLogin.Body.Close(), "Should close second login body")
	require.Len(t, sessionTokenHashes(t, db, userID), 2, "Second device should add its own session row")

	secondRefresh := postJSONWithClient(t, device2, "/api/auth/refresh", map[string]string{})
	require.Equal(t, http.StatusOK, secondRefresh.StatusCode, "Second device should refresh independently")
	require.NoError(t, secondRefresh.Body.Close(), "Should close second refresh body")
	assert.Len(t, sessionTokenHashes(t, db, userID), 2, "Second device's rotation should replace its own row only")
}
