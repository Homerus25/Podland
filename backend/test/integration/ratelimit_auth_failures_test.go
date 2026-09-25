//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Homerus25/podland/backend/internal/app"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bootFailureBudgetApp boots a second in-process backend whose
// credential-failure budget is tiny while the per-IP auth cap is raised
// out of the way, so the failure budget answers FIRST (issue 186) and
// its 429 path can be exercised for real — real DB, real handlers, real
// middleware limiter. Tear-down mirrors the suite's own LIFO order.
func bootFailureBudgetApp(t *testing.T, budget int) (*app.App, string) {
	t.Helper()

	cfg, err := buildTestConfig(t.TempDir())
	require.NoError(t, err, "Should build the second app's config")
	cfg.RateLimit.AuthRequestsPerMinute = 100000 // per-IP cap out of the way: this slice pins the budget answering first
	cfg.RateLimit.FailureBudget = budget
	cfg.RateLimit.FailureWindowMinutes = 15

	application, err := app.New(cfg)
	require.NoError(t, err, "Should boot the app with failure budget %d", budget)

	srv := httptest.NewServer(application.Handler())
	t.Cleanup(func() { _ = application.Close() })
	t.Cleanup(srv.Close)
	return application, srv.URL
}

// TestAuthFailureBudget_LocksUsernameBeforePerIPCap is the
// credential-failure budget demo (issue 186): with the per-IP cap
// raised out of the way, wrong-password logins for one name spend its
// budget and then lock even the CORRECT password out with 429
// RATE_LIMIT_EXCEEDED and the X-RateLimit headers; another name is
// unaffected; and a refresh session minted before the lockout keeps
// working — the budget bars fresh logins, it does not evict sessions.
func TestAuthFailureBudget_LocksUsernameBeforePerIPCap(t *testing.T) {
	suite.SkipIfUnavailable(t)

	const budget = 3
	application, serverURL := bootFailureBudgetApp(t, budget)

	password := "it-" + suite.RunID + "-fbpw-" + uuid.New().String()[:8]
	lockedUser := "it-" + suite.RunID + "-fb-locked"
	otherUser := "it-" + suite.RunID + "-fb-other"
	_, err := application.SeedUser(lockedUser, password)
	require.NoError(t, err, "Should seed the locked-out target")
	_, err = application.SeedUser(otherUser, password)
	require.NoError(t, err, "Should seed the untouched user")

	client := &http.Client{}
	postLogin := func(username, pass string) *http.Response {
		t.Helper()
		reqBody, err := json.Marshal(map[string]string{"username": username, "password": pass})
		require.NoError(t, err, "Should marshal login body")
		resp, err := client.Post(serverURL+"/api/auth/login", "application/json", bytes.NewReader(reqBody))
		require.NoError(t, err, "Should POST login for %s", username)
		return resp
	}

	// First a working login: its refresh session proves the lockout
	// does not evict sessions.
	loginResp := postLogin(lockedUser, password)
	require.Equal(t, http.StatusOK, loginResp.StatusCode, "The pre-lockout login should succeed")
	var refreshCookie *http.Cookie
	for _, cookie := range loginResp.Cookies() {
		if cookie.Name == "refresh_token" {
			refreshCookie = cookie
		}
	}
	require.NotNil(t, refreshCookie, "The login should set the refresh cookie")
	require.NoError(t, loginResp.Body.Close(), "Should close the login body")

	// Spend the budget with wrong-password attempts: all in-budget 401s.
	for i := range budget {
		resp := postLogin(lockedUser, "wrong-password")
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode,
			"Wrong-password attempt %d within the budget should reach the handler as 401", i+1)
		require.NoError(t, resp.Body.Close(), "Should close the 401 body")
	}

	// Locked out: the CORRECT password answers 429 with the budget's
	// headers, not the login payload.
	lockedResp := postLogin(lockedUser, password)
	defer lockedResp.Body.Close()
	require.Equal(t, http.StatusTooManyRequests, lockedResp.StatusCode,
		"The locked-out login should answer 429 even with the correct password")
	assert.Equal(t, "3", lockedResp.Header.Get("X-RateLimit-Limit"),
		"The 429 should advertise the configured failure budget")
	assert.Equal(t, "0", lockedResp.Header.Get("X-RateLimit-Remaining"),
		"The 429 should report no remaining budget")
	assert.NotEmpty(t, lockedResp.Header.Get("X-RateLimit-Reset"),
		"The 429 should report when the window resets")
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.NewDecoder(lockedResp.Body).Decode(&envelope), "Should decode the 429 error envelope")
	assert.Equal(t, "RATE_LIMIT_EXCEEDED", envelope.Error.Code, "The 429 should carry the rate-limit error code")

	// Another name's budget is untouched: it logs in fine.
	otherResp := postLogin(otherUser, password)
	require.Equal(t, http.StatusOK, otherResp.StatusCode,
		"One name's failures must never touch another name's budget")
	require.NoError(t, otherResp.Body.Close(), "Should close the other login's body")

	// The session minted before the lockout keeps working: refresh
	// rotates it while fresh logins for the name stay locked out.
	refreshReq, err := http.NewRequest(http.MethodPost, serverURL+"/api/auth/refresh", bytes.NewReader([]byte("{}")))
	require.NoError(t, err, "Should build the refresh request")
	refreshReq.AddCookie(refreshCookie)
	refreshResp, err := client.Do(refreshReq)
	require.NoError(t, err, "Should POST refresh")
	defer refreshResp.Body.Close()
	assert.Equal(t, http.StatusOK, refreshResp.StatusCode,
		"An active refresh session must keep working while its username is login-locked")
}

// TestAuthFailureBudget_ChangePasswordLocksCaller pins the password
// change's half of the budget demo over HTTP (issue 184; the lockout
// itself is unit-pinned at the service seam): with the shared budget
// ceiling raised out of the way, wrong-current-password changes for one
// caller spend that caller's budget and then lock even the CORRECT
// current password out with 429 RATE_LIMIT_EXCEEDED and the
// X-RateLimit headers. Login — the other endpoint on the same limiter
// instance — keeps working throughout: the endpoint-namespaced keys
// (login vs change-password) give each endpoint its own budget inside
// the shared instance.
func TestAuthFailureBudget_ChangePasswordLocksCaller(t *testing.T) {
	suite.SkipIfUnavailable(t)

	const budget = 3
	application, serverURL := bootFailureBudgetApp(t, budget)

	password := "it-" + suite.RunID + "-fbcpw-" + uuid.New().String()[:8]
	caller := "it-" + suite.RunID + "-fb-change"
	_, err := application.SeedUser(caller, password)
	require.NoError(t, err, "Should seed the changing caller")

	token, err := realLogin(serverURL, caller, password)
	require.NoError(t, err, "Should log in as the changing caller")

	postChange := func(currentPassword, newPassword string) *http.Response {
		t.Helper()
		reqBody, err := json.Marshal(map[string]string{"currentPassword": currentPassword, "newPassword": newPassword})
		require.NoError(t, err, "Should marshal change body")
		req, err := http.NewRequest(http.MethodPost, serverURL+"/api/auth/password", bytes.NewReader(reqBody))
		require.NoError(t, err, "Should build change request")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err, "Should POST the change endpoint")
		return resp
	}

	// Spend the budget with wrong-current-password attempts: all
	// in-budget 401s, and the stored password never changes.
	for i := range budget {
		resp := postChange("not-the-current-password", "brand-new-pw-9")
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode,
			"Wrong-current-password attempt %d within the budget should reach the handler as 401", i+1)
		require.NoError(t, resp.Body.Close(), "Should close the 401 body")
	}

	// Locked out: the CORRECT current password answers 429 with the
	// budget's headers, not the change payload.
	lockedResp := postChange(password, "brand-new-pw-9")
	defer lockedResp.Body.Close()
	require.Equal(t, http.StatusTooManyRequests, lockedResp.StatusCode,
		"The locked-out change should answer 429 even with the correct current password")
	assert.Equal(t, "3", lockedResp.Header.Get("X-RateLimit-Limit"),
		"The 429 should advertise the configured failure budget")
	assert.Equal(t, "0", lockedResp.Header.Get("X-RateLimit-Remaining"),
		"The 429 should report no remaining budget")
	assert.NotEmpty(t, lockedResp.Header.Get("X-RateLimit-Reset"),
		"The 429 should report when the window resets")
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.NewDecoder(lockedResp.Body).Decode(&envelope), "Should decode the 429 error envelope")
	assert.Equal(t, "RATE_LIMIT_EXCEEDED", envelope.Error.Code, "The 429 should carry the rate-limit error code")

	// The budget's other endpoint is untouched: the same caller's fresh
	// LOGIN still goes through — the shared instance's key spaces are
	// disjoint, so spending one endpoint's budget never locks the other.
	loginResp, err := realLogin(serverURL, caller, password)
	require.NoError(t, err, "The spent change-password budget must not lock the same caller's login")
	require.NotEmpty(t, loginResp, "The isolation login should return a token")
}

// TestAuthFailureBudget_UnknownUsernameSpendsItsBudget pins the
// enumeration brake over HTTP (issue 184): bogus logins for a name that
// does not exist spend that name's budget — the 401s are
// indistinguishable from wrong-password misses — and once spent,
// further logins for that same unknown name answer 429 with the
// X-RateLimit headers. A seeded name's budget is untouched: the key is
// the claimed username, and one name's spending never bleeds into
// another's.
func TestAuthFailureBudget_UnknownUsernameSpendsItsBudget(t *testing.T) {
	suite.SkipIfUnavailable(t)

	const budget = 3
	application, serverURL := bootFailureBudgetApp(t, budget)

	password := "it-" + suite.RunID + "-fbupw-" + uuid.New().String()[:8]
	ghost := "it-" + suite.RunID + "-fb-ghost"
	knownUser := "it-" + suite.RunID + "-fb-known"
	_, err := application.SeedUser(knownUser, password)
	require.NoError(t, err, "Should seed the known user")

	client := &http.Client{}
	postLogin := func(username, pass string) *http.Response {
		t.Helper()
		reqBody, err := json.Marshal(map[string]string{"username": username, "password": pass})
		require.NoError(t, err, "Should marshal login body")
		resp, err := client.Post(serverURL+"/api/auth/login", "application/json", bytes.NewReader(reqBody))
		require.NoError(t, err, "Should POST login for %s", username)
		return resp
	}

	// Spend the unknown name's budget: every in-budget attempt is a 401,
	// identical in shape to a wrong-password miss.
	for i := range budget {
		resp := postLogin(ghost, "whatever-password")
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode,
			"Unknown-username attempt %d within the budget should stay an indistinguishable 401", i+1)
		require.NoError(t, resp.Body.Close(), "Should close the 401 body")
	}

	// Locked out: the same unknown name now answers 429 with the
	// budget's headers — mixing unknown and known names cannot skirt
	// the lockout.
	lockedResp := postLogin(ghost, "whatever-password")
	defer lockedResp.Body.Close()
	require.Equal(t, http.StatusTooManyRequests, lockedResp.StatusCode,
		"The spent unknown-name budget should lock further logins for that name")
	assert.Equal(t, "3", lockedResp.Header.Get("X-RateLimit-Limit"),
		"The 429 should advertise the configured failure budget")
	assert.Equal(t, "0", lockedResp.Header.Get("X-RateLimit-Remaining"),
		"The 429 should report no remaining budget")
	assert.NotEmpty(t, lockedResp.Header.Get("X-RateLimit-Reset"),
		"The 429 should report when the window resets")
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.NewDecoder(lockedResp.Body).Decode(&envelope), "Should decode the 429 error envelope")
	assert.Equal(t, "RATE_LIMIT_EXCEEDED", envelope.Error.Code, "The 429 should carry the rate-limit error code")

	// The known name's budget is untouched by ghost's spending.
	knownResp := postLogin(knownUser, password)
	require.Equal(t, http.StatusOK, knownResp.StatusCode,
		"The unknown name's failures must never touch a known name's budget")
	require.NoError(t, knownResp.Body.Close(), "Should close the known login's body")
}
