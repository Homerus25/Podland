//go:build integration

package integration

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "github.com/mattn/go-sqlite3"
)

// openSuiteDB opens a second connection to the suite's SQLite database
// so tests can assert on persisted state directly. The App keeps its
// own connection (WAL mode); this read-side handle sees every committed
// row.
func openSuiteDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite3", suite.DBPath)
	require.NoError(t, err, "Should open suite database")
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// loginWithCookies performs a real POST /api/auth/login and returns the
// raw response, so tests can inspect Set-Cookie (the default client and
// realLogin only surface the JSON body).
func loginWithCookies(t *testing.T, username, password string) *http.Response {
	t.Helper()

	reqBody, err := json.Marshal(map[string]string{
		"username": username,
		"password": password,
	})
	require.NoError(t, err, "Should marshal login request")

	resp, err := http.Post(suite.ServerURL+"/api/auth/login", "application/json", bytes.NewReader(reqBody))
	require.NoError(t, err, "Should POST to login endpoint")
	return resp
}

// refreshCookieFrom extracts the refresh_token cookie from a login
// response, failing the test when it is missing.
func refreshCookieFrom(t *testing.T, resp *http.Response) *http.Cookie {
	t.Helper()

	for _, cookie := range resp.Cookies() {
		if cookie.Name == "refresh_token" {
			return cookie
		}
	}
	t.Fatal("login response should set a refresh_token cookie")
	return nil
}

// sessionTokenHashes queries the sessions table for the given user's
// stored token hashes, ordered by creation time.
func sessionTokenHashes(t *testing.T, db *sql.DB, userID string) []string {
	t.Helper()

	rows, err := db.Query(`SELECT token_hash FROM sessions WHERE user_id = ? ORDER BY created_at, id`, userID)
	require.NoError(t, err, "Should query sessions for user")
	defer rows.Close()

	var hashes []string
	for rows.Next() {
		var hash string
		require.NoError(t, rows.Scan(&hash), "Should scan token hash")
		hashes = append(hashes, hash)
	}
	require.NoError(t, rows.Err(), "Should iterate session rows")
	return hashes
}

// TestAuth_Login_SetsRefreshCookie asserts the real server's login
// response sets the refresh cookie with every security attribute the
// spec demands: httpOnly, Secure, SameSite=Strict, Path=/api/auth.
func TestAuth_Login_SetsRefreshCookie(t *testing.T) {
	suite.SkipIfUnavailable(t)

	username := "it-" + suite.RunID + "-cookieuser"
	password := "it-" + suite.RunID + "-cookiepw-" + uuid.New().String()[:8]
	_, err := suite.SeedUserWithRole(username, password, "user")
	require.NoError(t, err, "Should seed cookie-test user")

	resp := loginWithCookies(t, username, password)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "Login should succeed")

	cookie := refreshCookieFrom(t, resp)
	assert.NotEmpty(t, cookie.Value, "Refresh cookie should carry a token")
	assert.True(t, cookie.HttpOnly, "Refresh cookie must be httpOnly")
	assert.True(t, cookie.Secure, "Refresh cookie must be Secure")
	assert.Equal(t, http.SameSiteStrictMode, cookie.SameSite, "Refresh cookie must be SameSite=Strict")
	assert.Equal(t, "/api/auth", cookie.Path, "Refresh cookie must be scoped to /api/auth")

	var body struct {
		Token string `json:"token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body), "Should decode login body")
	assert.NotEmpty(t, body.Token, "Login body should still carry the access JWT")
	assert.NotContains(t, body.Token, cookie.Value, "Access JWT and refresh token must be unrelated values")
}

// TestAuth_Login_PersistsOnlyHashedSessionRow asserts the database
// effect of a login: exactly one session row for that user, storing the
// SHA-256 hex hash of the cookie's refresh token — never the plaintext.
func TestAuth_Login_PersistsOnlyHashedSessionRow(t *testing.T) {
	suite.SkipIfUnavailable(t)

	username := "it-" + suite.RunID + "-hashuser"
	password := "it-" + suite.RunID + "-hashpw-" + uuid.New().String()[:8]
	userID, err := suite.SeedUserWithRole(username, password, "user")
	require.NoError(t, err, "Should seed hash-test user")

	resp := loginWithCookies(t, username, password)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "Login should succeed")

	cookie := refreshCookieFrom(t, resp)

	db := openSuiteDB(t)
	hashes := sessionTokenHashes(t, db, userID)
	require.Len(t, hashes, 1, "Login should persist exactly one session row for the user")

	expectedHash := sha256.Sum256([]byte(cookie.Value))
	assert.Equal(t, hex.EncodeToString(expectedHash[:]), hashes[0],
		"Stored row must be the SHA-256 hex hash of the cookie's refresh token")
	assert.NotEqual(t, cookie.Value, hashes[0], "Stored row must not be the plaintext token")
}

// TestAuth_Login_EveryLoginAddsSessionRow asserts the multi-session
// model end-to-end: two logins of the same user leave two distinct
// hashed rows, so a second device keeps its own session.
func TestAuth_Login_EveryLoginAddsSessionRow(t *testing.T) {
	suite.SkipIfUnavailable(t)

	username := "it-" + suite.RunID + "-multidevice"
	password := "it-" + suite.RunID + "-multipw-" + uuid.New().String()[:8]
	userID, err := suite.SeedUserWithRole(username, password, "user")
	require.NoError(t, err, "Should seed multi-device user")

	first := loginWithCookies(t, username, password)
	firstCookie := refreshCookieFrom(t, first)
	require.NoError(t, first.Body.Close(), "Should close first login body")

	second := loginWithCookies(t, username, password)
	secondCookie := refreshCookieFrom(t, second)
	require.NoError(t, second.Body.Close(), "Should close second login body")

	assert.NotEqual(t, firstCookie.Value, secondCookie.Value, "Each login should receive a distinct refresh token")

	db := openSuiteDB(t)
	hashes := sessionTokenHashes(t, db, userID)
	assert.Len(t, hashes, 2, "Two logins should leave two session rows (multi-session)")
	for _, stored := range hashes {
		assert.NotEqual(t, firstCookie.Value, stored, "No row may store a plaintext token")
		assert.NotEqual(t, secondCookie.Value, stored, "No row may store a plaintext token")
	}
}
