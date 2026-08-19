//go:build integration

package integration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAuth_Logout_RevokesOnlyTheLoggingOutDevice is the demoable
// lifecycle of the logout feature: two devices log in with separate
// cookie jars, device A logs out (row revoked, cookie cleared, response
// shape unchanged), device A's refresh now fails while device B's still
// succeeds — revocation is per-device, and logout stays idempotent.
func TestAuth_Logout_RevokesOnlyTheLoggingOutDevice(t *testing.T) {
	suite.SkipIfUnavailable(t)

	username := "it-" + suite.RunID + "-logoutuser"
	password := "it-" + suite.RunID + "-logoutpw-" + uuid.New().String()[:8]
	userID, err := suite.SeedUserWithRole(username, password, "user")
	require.NoError(t, err, "Should seed logout-test user")
	db := openSuiteDB(t)

	// Device A and device B log in with separate cookie jars — two
	// independent sessions, browser-style.
	deviceA := suite.CookieJarClient()
	loginA := postJSONWithClient(t, deviceA, "/api/auth/login", map[string]string{
		"username": username,
		"password": password,
	})
	require.Equal(t, http.StatusOK, loginA.StatusCode, "Device A should log in")
	cookieA := refreshCookieFrom(t, loginA)
	require.NoError(t, loginA.Body.Close(), "Should close device A login body")

	deviceB := suite.CookieJarClient()
	loginB := postJSONWithClient(t, deviceB, "/api/auth/login", map[string]string{
		"username": username,
		"password": password,
	})
	require.Equal(t, http.StatusOK, loginB.StatusCode, "Device B should log in")
	cookieB := refreshCookieFrom(t, loginB)
	require.NoError(t, loginB.Body.Close(), "Should close device B login body")
	require.Len(t, sessionTokenHashes(t, db, userID), 2, "Two devices should leave two session rows")

	// Device A logs out: the response keeps the historical shape, and
	// the refresh cookie is cleared — same attributes, already expired.
	logoutResp := postJSONWithClient(t, deviceA, "/api/auth/logout", map[string]string{})
	defer logoutResp.Body.Close()
	require.Equal(t, http.StatusOK, logoutResp.StatusCode, "Logout should succeed")
	var logoutBody map[string]any
	require.NoError(t, json.NewDecoder(logoutResp.Body).Decode(&logoutBody), "Should decode logout body")
	require.Len(t, logoutBody, 1, "Logout body shape must not change")
	assert.Equal(t, "Logged out successfully", logoutBody["message"], "Logout body message must not change")

	cleared := refreshCookieFrom(t, logoutResp)
	assert.Empty(t, cleared.Value, "The clearing cookie must carry no token")
	assert.True(t, cleared.HttpOnly, "The clearing cookie must stay httpOnly")
	assert.True(t, cleared.Secure, "The clearing cookie must stay Secure")
	assert.Equal(t, http.SameSiteStrictMode, cleared.SameSite, "The clearing cookie must stay SameSite=Strict")
	assert.Equal(t, "/api/auth", cleared.Path, "The clearing cookie must keep the /api/auth path")
	expired := cleared.MaxAge < 0 || !cleared.Expires.After(time.Now())
	assert.True(t, expired, "The clearing cookie must be already expired (MaxAge=%d, Expires=%v)", cleared.MaxAge, cleared.Expires)

	// Revocation is per-session: only device A's row is gone; device
	// B's row is the one that survives.
	hashes := sessionTokenHashes(t, db, userID)
	require.Len(t, hashes, 1, "Logout should delete only the logging-out device's row")
	expectedBHash := sha256.Sum256([]byte(cookieB.Value))
	assert.Equal(t, hex.EncodeToString(expectedBHash[:]), hashes[0],
		"The surviving row must be device B's session")

	// Device A can no longer refresh — neither with the stale cookie
	// value nor through its jar, which the clearing cookie emptied.
	staleRefresh := postWithCookie(t, "/api/auth/refresh", cookieA)
	defer staleRefresh.Body.Close()
	require.Equal(t, http.StatusUnauthorized, staleRefresh.StatusCode, "Device A's revoked cookie must fail to refresh")
	var staleEnvelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.NewDecoder(staleRefresh.Body).Decode(&staleEnvelope), "Should decode stale refresh error")
	assert.Equal(t, "AUTH_REFRESH_INVALID", staleEnvelope.Error.Code, "Revoked cookie must report AUTH_REFRESH_INVALID")

	jarRefresh := postJSONWithClient(t, deviceA, "/api/auth/refresh", map[string]string{})
	defer jarRefresh.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, jarRefresh.StatusCode, "Device A's jar-held cookie must be gone or invalid after logout")

	// Logout stays idempotent: a second logout (now cookie-less) still
	// succeeds and does not disturb device B's session.
	secondLogout := postJSONWithClient(t, deviceA, "/api/auth/logout", map[string]string{})
	defer secondLogout.Body.Close()
	require.Equal(t, http.StatusOK, secondLogout.StatusCode, "A cookie-less logout must still succeed")

	// Device B's session survives untouched: its refresh mints a new
	// access JWT and rotates its own row.
	refreshB := postJSONWithClient(t, deviceB, "/api/auth/refresh", map[string]string{})
	defer refreshB.Body.Close()
	require.Equal(t, http.StatusOK, refreshB.StatusCode, "Device B should still refresh after device A logged out")
	decodeTokenBody(t, refreshB)
	assert.Len(t, sessionTokenHashes(t, db, userID), 1, "Device B's refresh should rotate its own row, not add one")
}
