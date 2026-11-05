// First-run setup contract tests (issue 179), pinned at the same seam
// as app_test.go: the full backend built through app.New over a temp
// SQLite database with generated keys, served under httptest.NewServer —
// real HTTP, real SQLite, no mocks. The round trip, the probe flip, the
// concurrency guard, and the rate-limit group membership are all
// exercised through the wire the frontend and scripted deployments use.
package app_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Homerus25/podland/backend/internal/app"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// statusBody mirrors the GET /api/auth/status response shape the tests
// assert on.
type statusBody struct {
	Authenticated bool   `json:"authenticated"`
	NeedsSetup    bool   `json:"needsSetup"`
	UserID        string `json:"userId"`
	Role          string `json:"role"`
}

// loginBody mirrors the login payload (login, setup success, refresh,
// password change all share the shape).
type loginBody struct {
	Token    string `json:"token"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

// errorBody mirrors the structured error envelope.
type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// startTestApp brings a fresh backend up under httptest.NewServer —
// newTestConfig supplies the temp database and generated keys — and
// returns its base URL.
func startTestApp(t *testing.T) string {
	t.Helper()

	a, err := app.New(newTestConfig(t))
	require.NoError(t, err)
	t.Cleanup(func() { _ = a.Close() })

	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)

	return srv.URL
}

// getStatus performs an anonymous GET /api/auth/status and decodes the
// body.
func getStatus(t *testing.T, baseURL string) (int, statusBody) {
	t.Helper()

	resp, err := http.Get(baseURL + "/api/auth/status")
	require.NoError(t, err, "GET /api/auth/status must connect")
	defer resp.Body.Close()

	var body statusBody
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body), "decode status body")
	return resp.StatusCode, body
}

// postJSON performs an unauthenticated POST with a JSON body. It
// returns the response (status line and headers reachable) with the
// body already read.
func postJSON(t *testing.T, baseURL, path, body string) (*http.Response, []byte) {
	t.Helper()

	resp, err := http.Post(baseURL+path, "application/json", strings.NewReader(body))
	require.NoError(t, err, "POST %s must connect", path)
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read %s response body", path)
	return resp, raw
}

// TestApp_Handler_Setup_RoundTripOnFreshInstance pins the whole
// first-run story over real HTTP: a zero-user instance reports
// needs-setup true to an anonymous probe; the setup endpoint creates
// exactly one admin and signs the caller in (login payload + refresh
// cookie); the probe then flips to false; the new credentials log in;
// and a second setup attempt is refused with SETUP_COMPLETED.
func TestApp_Handler_Setup_RoundTripOnFreshInstance(t *testing.T) {
	baseURL := startTestApp(t)

	// Fresh instance: the anonymous probe reports setup needed.
	probeStatus, probe := getStatus(t, baseURL)
	require.Equal(t, http.StatusOK, probeStatus)
	assert.False(t, probe.Authenticated)
	assert.True(t, probe.NeedsSetup, "a zero-user instance must report needsSetup true")

	// Setup: create the first admin, get signed in on the spot.
	setupResp, setupRaw := postJSON(t, baseURL, "/api/setup/user",
		`{"username":"bootstrap-admin","password":"first-password-123"}`)
	require.Equal(t, http.StatusOK, setupResp.StatusCode,
		"setup on a fresh instance must succeed, body: %s", setupRaw)

	var setup loginBody
	require.NoError(t, json.Unmarshal(setupRaw, &setup), "decode setup body")
	assert.Equal(t, "bootstrap-admin", setup.Username)
	assert.Equal(t, models.RoleAdmin, setup.Role, "the first user is always an admin")
	assert.NotEmpty(t, setup.Token, "setup must hand back an access token")

	// The refresh cookie rides along with login's exact attributes.
	cookies := setupResp.Cookies()
	require.Len(t, cookies, 1, "setup must set exactly the refresh cookie")
	refresh := cookies[0]
	assert.Equal(t, "refresh_token", refresh.Name)
	assert.True(t, refresh.HttpOnly)
	assert.True(t, refresh.Secure)
	assert.Equal(t, http.SameSiteStrictMode, refresh.SameSite)
	assert.Equal(t, "/api/auth", refresh.Path, "the refresh cookie shares the login/refresh scope")
	assert.NotEmpty(t, refresh.Value)

	// The access token works: presenting it, the probe reports the
	// caller authenticated as an admin.
	authedReq, err := http.NewRequest(http.MethodGet, baseURL+"/api/auth/status", nil)
	require.NoError(t, err)
	authedReq.Header.Set("Authorization", "Bearer "+setup.Token)
	authedResp, err := http.DefaultClient.Do(authedReq)
	require.NoError(t, err)
	var authed statusBody
	require.NoError(t, json.NewDecoder(authedResp.Body).Decode(&authed))
	authedResp.Body.Close()
	require.Equal(t, http.StatusOK, authedResp.StatusCode)
	assert.True(t, authed.Authenticated)
	assert.Equal(t, models.RoleAdmin, authed.Role)
	assert.False(t, authed.NeedsSetup, "an authenticated caller never sees the setup flag")

	// The probe flips: setup is no longer needed.
	flipStatus, flip := getStatus(t, baseURL)
	require.Equal(t, http.StatusOK, flipStatus)
	assert.False(t, flip.Authenticated)
	assert.False(t, flip.NeedsSetup, "after setup the instance is configured")

	// The chosen credentials log in.
	loginResp, loginRaw := postJSON(t, baseURL, "/api/auth/login",
		`{"username":"bootstrap-admin","password":"first-password-123"}`)
	require.Equal(t, http.StatusOK, loginResp.StatusCode, "login with the setup credentials must succeed, body: %s", loginRaw)

	// Setup never returns: a second attempt is a conflict.
	againResp, againRaw := postJSON(t, baseURL, "/api/setup/user",
		`{"username":"too-late","password":"second-password-123"}`)
	require.Equal(t, http.StatusConflict, againResp.StatusCode)
	var again errorBody
	require.NoError(t, json.Unmarshal(againRaw, &again))
	assert.Equal(t, "SETUP_COMPLETED", again.Error.Code)
}

// TestApp_Handler_Setup_ConfiguredInstanceRefuses pins the configured
// instance's side: needs-setup false, setup refused with 409
// SETUP_COMPLETED, and no side effects — the refused user does not
// exist and the existing admin is untouched.
func TestApp_Handler_Setup_ConfiguredInstanceRefuses(t *testing.T) {
	a, err := app.New(newTestConfig(t))
	require.NoError(t, err)
	t.Cleanup(func() { _ = a.Close() })

	_, err = a.SeedUserWithRole("existing-admin", "existing-password-123", models.RoleAdmin)
	require.NoError(t, err, "seed the configured instance's admin")

	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	baseURL := srv.URL

	probeStatus, probe := getStatus(t, baseURL)
	require.Equal(t, http.StatusOK, probeStatus)
	assert.False(t, probe.NeedsSetup, "an instance with users is configured")

	setupResp, setupRaw := postJSON(t, baseURL, "/api/setup/user",
		`{"username":"late-arriver","password":"never-stored-123"}`)
	require.Equal(t, http.StatusConflict, setupResp.StatusCode, "body: %s", setupRaw)
	var body errorBody
	require.NoError(t, json.Unmarshal(setupRaw, &body))
	assert.Equal(t, "SETUP_COMPLETED", body.Error.Code)

	// No side effects: the refused user was never created...
	loginLateResp, _ := postJSON(t, baseURL, "/api/auth/login",
		`{"username":"late-arriver","password":"never-stored-123"}`)
	assert.Equal(t, http.StatusUnauthorized, loginLateResp.StatusCode)

	// ...and the existing admin still logs in.
	loginAdminResp, loginAdminRaw := postJSON(t, baseURL, "/api/auth/login",
		`{"username":"existing-admin","password":"existing-password-123"}`)
	require.Equal(t, http.StatusOK, loginAdminResp.StatusCode, "body: %s", loginAdminRaw)
}

// TestApp_Handler_Setup_ConcurrentRequestsExactlyOneWinner pins the
// atomicity contract: two setup requests fired concurrently against a
// fresh instance decide exactly once — one 200, one 409
// SETUP_COMPLETED — and afterwards exactly one user exists, with the
// admin role (proven over the wire: the winner's credentials log in as
// admin, the loser's credentials do not exist, and setup is closed).
func TestApp_Handler_Setup_ConcurrentRequestsExactlyOneWinner(t *testing.T) {
	baseURL := startTestApp(t)

	type outcome struct {
		status int
		body   []byte
	}

	fire := func(username string, out chan<- outcome, start <-chan struct{}) {
		<-start
		resp, raw := postJSON(t, baseURL, "/api/setup/user",
			fmt.Sprintf(`{"username":%q,"password":"concurrent-password-123"}`, username))
		out <- outcome{status: resp.StatusCode, body: raw}
	}

	results := make(chan outcome, 2)
	start := make(chan struct{})
	go fire("first-visitor", results, start)
	go fire("second-visitor", results, start)
	close(start)

	first, second := <-results, <-results
	statuses := []int{first.status, second.status}
	assert.Contains(t, statuses, http.StatusOK, "exactly one request must win")
	assert.Contains(t, statuses, http.StatusConflict, "the loser must be refused")

	var winnerUsername string
	for _, r := range []outcome{first, second} {
		if r.status == http.StatusOK {
			var won loginBody
			require.NoError(t, json.Unmarshal(r.body, &won), "decode winner body: %s", r.body)
			assert.Equal(t, models.RoleAdmin, won.Role, "the winner created an admin")
			winnerUsername = won.Username
		} else {
			var lost errorBody
			require.NoError(t, json.Unmarshal(r.body, &lost), "decode loser body: %s", r.body)
			assert.Equal(t, "SETUP_COMPLETED", lost.Error.Code,
				"the loser must learn setup was completed, not a generic error")
		}
	}
	require.NotEmpty(t, winnerUsername)

	// Exactly one user: the winner's credentials log in as admin.
	winnerResp, winnerRaw := postJSON(t, baseURL, "/api/auth/login",
		fmt.Sprintf(`{"username":%q,"password":"concurrent-password-123"}`, winnerUsername))
	require.Equal(t, http.StatusOK, winnerResp.StatusCode, "body: %s", winnerRaw)
	var won loginBody
	require.NoError(t, json.Unmarshal(winnerRaw, &won))
	assert.Equal(t, models.RoleAdmin, won.Role)

	// The loser's username never made it into the database...
	loserUsername := "second-visitor"
	if winnerUsername == "second-visitor" {
		loserUsername = "first-visitor"
	}
	loserResp, _ := postJSON(t, baseURL, "/api/auth/login",
		fmt.Sprintf(`{"username":%q,"password":"concurrent-password-123"}`, loserUsername))
	assert.Equal(t, http.StatusUnauthorized, loserResp.StatusCode)

	// ...and the instance is configured: setup is closed.
	_, closed := getStatus(t, baseURL)
	assert.False(t, closed.NeedsSetup)
}

// TestApp_Handler_Setup_ValidationFailuresLeaveInstanceFresh pins that
// validation failures reuse the creation path's existing error codes
// and write nothing — the instance still needs setup afterwards.
func TestApp_Handler_Setup_ValidationFailuresLeaveInstanceFresh(t *testing.T) {
	baseURL := startTestApp(t)

	cases := []struct {
		name       string
		body       string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "password below the policy",
			body:       `{"username":"someone","password":"short"}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "PASSWORD_TOO_SHORT",
		},
		{
			name:       "missing username",
			body:       `{"password":"valid-password-123"}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "VALIDATION_FAILED",
		},
		{
			name:       "missing password",
			body:       `{"username":"someone"}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "VALIDATION_FAILED",
		},
		{
			name:       "malformed JSON",
			body:       `{"username":`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_REQUEST",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, raw := postJSON(t, baseURL, "/api/setup/user", tc.body)
			require.Equal(t, tc.wantStatus, resp.StatusCode, "body: %s", raw)
			var body errorBody
			require.NoError(t, json.Unmarshal(raw, &body))
			assert.Equal(t, tc.wantCode, body.Error.Code)
		})
	}

	// Nothing was written: the instance still needs setup.
	_, fresh := getStatus(t, baseURL)
	assert.True(t, fresh.NeedsSetup, "failed setup attempts must leave the instance fresh")
}

// TestApp_Handler_Setup_SharesPerIPAuthRateLimitGroup pins the limiter
// wiring (issue 179): the status probe, the setup endpoint, and login
// draw from ONE per-IP budget, while routes outside the auth group are
// unaffected by it.
func TestApp_Handler_Setup_SharesPerIPAuthRateLimitGroup(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.RateLimit.AuthRequestsPerMinute = 1 // one request per IP per window

	a, err := app.New(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = a.Close() })

	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	baseURL := srv.URL

	// First auth-group request: allowed, budget spent.
	probeStatus, probe := getStatus(t, baseURL)
	require.Equal(t, http.StatusOK, probeStatus)
	assert.True(t, probe.NeedsSetup)

	// Second: the setup endpoint shares the probe's empty budget...
	setupResp, _ := postJSON(t, baseURL, "/api/setup/user",
		`{"username":"rate-limited","password":"never-reaches-123"}`)
	assert.Equal(t, http.StatusTooManyRequests, setupResp.StatusCode,
		"setup must share the per-IP auth rate-limit group")

	// ...and so does login, third in the same window.
	loginResp, _ := postJSON(t, baseURL, "/api/auth/login",
		`{"username":"x","password":"y"}`)
	assert.Equal(t, http.StatusTooManyRequests, loginResp.StatusCode)

	// A route outside the auth group is untouched by that budget.
	healthResp, err := http.Get(baseURL + "/api/health")
	require.NoError(t, err)
	defer healthResp.Body.Close()
	assert.Equal(t, http.StatusOK, healthResp.StatusCode,
		"the auth budget must not throttle routes outside the group")
}

// TestApp_Handler_ProtectedRoutesStayGatedDuringSetup pins that a
// user-less instance still gates every protected route: no valid token
// can exist in setup state, so the gates answer 401 exactly as on a
// configured instance.
func TestApp_Handler_ProtectedRoutesStayGatedDuringSetup(t *testing.T) {
	baseURL := startTestApp(t)

	_, status := getStatus(t, baseURL)
	require.True(t, status.NeedsSetup, "precondition: the instance is in setup state")

	hostsResp, err := http.Get(baseURL + "/api/hosts")
	require.NoError(t, err)
	defer hostsResp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, hostsResp.StatusCode)

	usersResp, _ := postJSON(t, baseURL, "/api/users",
		`{"username":"someone","password":"whatever-123"}`)
	assert.Equal(t, http.StatusUnauthorized, usersResp.StatusCode)
}
