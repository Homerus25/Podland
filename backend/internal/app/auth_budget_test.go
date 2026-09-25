package app_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Homerus25/podland/backend/internal/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestApp_Handler_LoginFailureBudget_LocksUsername pins the production
// failure-budget wiring (issue 186): the shared middleware limiter is
// checked by the auth service before verification and spent on failure,
// so once the budget is gone even the correct password answers 429 with
// the X-RateLimit trio — while another username's budget is untouched.
// The auth package's own tests cover the check/spend contract over a
// fake (middleware imports auth, so its real limiter cannot be wired in
// a unit test there); this test exercises the real limiter end to end.
func TestApp_Handler_LoginFailureBudget_LocksUsername(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.RateLimit.FailureBudget = 2
	cfg.RateLimit.FailureWindowMinutes = 15

	a, err := app.New(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = a.Close() })

	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	baseURL := srv.URL

	_, err = a.SeedUser("locked-target", "right-password-123")
	require.NoError(t, err, "Should seed the locked-out target")
	_, err = a.SeedUser("other-target", "right-password-123")
	require.NoError(t, err, "Should seed the untouched user")

	// Two wrong-password logins spend the budget; both stay 401s.
	for i := range 2 {
		resp, raw := postJSON(t, baseURL, "/api/auth/login",
			`{"username":"locked-target","password":"wrong"}`)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode,
			"Failure %d within the budget should stay a 401, body: %s", i+1, raw)
	}

	// The budget is spent: the CORRECT password is locked out with the
	// rate-limit envelope and the username budget's headers.
	resp, raw := postJSON(t, baseURL, "/api/auth/login",
		`{"username":"locked-target","password":"right-password-123"}`)
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode,
		"The locked-out login should answer 429 even with the correct password, body: %s", raw)
	var body errorBody
	require.NoError(t, json.Unmarshal(raw, &body), "decode 429 body")
	assert.Equal(t, "RATE_LIMIT_EXCEEDED", body.Error.Code, "The 429 should carry the rate-limit code")
	assert.Equal(t, "2", resp.Header.Get("X-RateLimit-Limit"), "The 429 should advertise the configured budget")
	assert.Equal(t, "0", resp.Header.Get("X-RateLimit-Remaining"), "The 429 should report no remaining budget")
	assert.NotEmpty(t, resp.Header.Get("X-RateLimit-Reset"), "The 429 should report the window reset")

	// Another username's budget is untouched: it logs in fine.
	okResp, _ := postJSON(t, baseURL, "/api/auth/login",
		`{"username":"other-target","password":"right-password-123"}`)
	assert.Equal(t, http.StatusOK, okResp.StatusCode,
		"The locked-out name's failures must not touch another name's budget")
}
