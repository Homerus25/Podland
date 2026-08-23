//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/Homerus25/podland/backend/internal/app"
)

// bootAuthLimitedApp boots a second in-process backend whose per-IP auth
// limiter has a tiny budget, so the 429 path can be exercised for real
// (real DB, real handlers, real middleware) without poisoning the shared
// suite server, whose limits are raised out of the way. Tear-down mirrors
// the suite's own LIFO order: server close → app.Close → remove temp
// files.
func bootAuthLimitedApp(t *testing.T, authLimit int) (*app.App, string) {
	t.Helper()

	cfg, tempDirs, err := buildTestConfig()
	require.NoError(t, err, "Should build the second app's config")
	cfg.RateLimit.AuthRequestsPerMinute = authLimit

	application, err := app.New(cfg)
	require.NoError(t, err, "Should boot the app with auth limit %d", authLimit)

	srv := httptest.NewServer(application.Handler())
	t.Cleanup(func() {
		for _, d := range tempDirs {
			_ = os.RemoveAll(d)
		}
	})
	t.Cleanup(func() { _ = application.Close() })
	t.Cleanup(srv.Close)
	return application, srv.URL
}

// TestAuthRateLimit_LoginAndRefreshSharePerIPBudget is the brute-force
// protection demo: with a low configured limit, repeated login attempts
// from one client eventually receive 429 with the structured error
// envelope and rate-limit headers, the exhausted budget also covers
// refresh (same IP-keyed group), and logout is NOT limited.
func TestAuthRateLimit_LoginAndRefreshSharePerIPBudget(t *testing.T) {
	suite.SkipIfUnavailable(t)

	const limit = 3
	application, serverURL := bootAuthLimitedApp(t, limit)

	username := "it-" + suite.RunID + "-rl"
	password := "it-" + suite.RunID + "-rlpw-" + uuid.New().String()[:8]
	_, err := application.SeedUser(username, password)
	require.NoError(t, err, "Should seed a user in the limited app's own DB")

	client := &http.Client{}

	// Hammer login with a wrong password: in-budget attempts reach the
	// handler (401 INVALID_CREDENTIALS), then the per-IP budget runs out.
	var rateLimitedResp *http.Response
	for i := 0; i < 10 && rateLimitedResp == nil; i++ {
		reqBody, err := json.Marshal(map[string]string{"username": username, "password": "wrong"})
		require.NoError(t, err, "Should marshal login body")
		resp, err := client.Post(serverURL+"/api/auth/login", "application/json", bytes.NewReader(reqBody))
		require.NoError(t, err, "Should POST login attempt %d", i+1)

		if resp.StatusCode == http.StatusTooManyRequests {
			rateLimitedResp = resp
			break
		}
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode,
			"Login attempt %d within the budget should reach the handler as 401", i+1)
		require.NoError(t, resp.Body.Close(), "Should close body")
	}
	require.NotNil(t, rateLimitedResp, "Repeated login attempts should eventually receive 429")

	assert.Equal(t, "3", rateLimitedResp.Header.Get("X-RateLimit-Limit"), "429 should advertise the configured limit")
	assert.Equal(t, "0", rateLimitedResp.Header.Get("X-RateLimit-Remaining"), "429 should report no remaining budget")
	assert.NotEmpty(t, rateLimitedResp.Header.Get("X-RateLimit-Reset"), "429 should report when the window resets")
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.NewDecoder(rateLimitedResp.Body).Decode(&envelope), "Should decode the 429 error envelope")
	assert.Equal(t, "RATE_LIMIT_EXCEEDED", envelope.Error.Code, "429 should carry the rate-limit error code")
	require.NoError(t, rateLimitedResp.Body.Close(), "Should close the 429 body")

	// Refresh shares login's IP-keyed group: with the budget exhausted,
	// it must 429 too — not its usual cookie-less 401.
	refreshResp, err := client.Post(serverURL+"/api/auth/refresh", "application/json", bytes.NewReader([]byte("{}")))
	require.NoError(t, err, "Should POST refresh")
	defer refreshResp.Body.Close()
	assert.Equal(t, http.StatusTooManyRequests, refreshResp.StatusCode, "Refresh should share login's per-IP budget")

	// Logout is deliberately outside the group: still answered normally.
	logoutResp, err := client.Post(serverURL+"/api/auth/logout", "application/json", bytes.NewReader([]byte("{}")))
	require.NoError(t, err, "Should POST logout")
	defer logoutResp.Body.Close()
	assert.Equal(t, http.StatusOK, logoutResp.StatusCode, "Logout must not be rate limited")
}
