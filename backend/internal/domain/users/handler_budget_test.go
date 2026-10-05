package users

import (
	"net/http"
	"testing"

	"github.com/Homerus25/podland/backend/internal/failurebudget"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUserHandler_ResetPassword_FailureBudget429CarriesHeaders pins the
// wire contract of the reset surface's lockout: once the acting admin's
// budget is spent, the reset — even with the correct admin password —
// answers 429 RATE_LIMIT_EXCEEDED with the X-RateLimit trio of the
// caller-keyed budget, while in-budget failures stay plain 401s without
// rate-limit headers.
func TestUserHandler_ResetPassword_FailureBudget429CarriesHeaders(t *testing.T) {
	budget := newFakeBudget(1)
	router, authService, _ := newUsersTestRouterWithBudget(t, failurebudget.NewSurface("reset-password", budget))

	// In-budget failure: a plain 401, no rate-limit advertisement.
	w := putUserPasswordAs(t, router, authService, "admin-1", "user-1",
		`{"adminPassword":"not-the-admins-password","newPassword":"brand-new-pw"}`)
	resp := w.Result()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "The in-budget failure should stay a 401")
	assert.Empty(t, resp.Header.Get("X-RateLimit-Limit"), "A plain 401 must not carry rate-limit headers")
	require.NoError(t, resp.Body.Close(), "Should close the 401 body")

	// The budget is spent: even the correct admin password is locked out.
	w = putUserPasswordAs(t, router, authService, "admin-1", "user-1",
		`{"adminPassword":"known-admin-password","newPassword":"brand-new-pw"}`)
	resp = w.Result()
	defer resp.Body.Close()
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "The locked-out reset should answer 429")
	assert.Equal(t, "RATE_LIMIT_EXCEEDED", testutils.DecodeErrorEnvelope(t, resp).Error.Code, "The 429 should carry the rate-limit code")
	assert.Equal(t, "1", resp.Header.Get("X-RateLimit-Limit"), "The 429 should advertise the configured budget")
	assert.Equal(t, "0", resp.Header.Get("X-RateLimit-Remaining"), "The 429 should report no remaining budget")
	assert.Equal(t, "300", resp.Header.Get("X-RateLimit-Reset"), "The 429 should report the window reset in seconds")
}

// TestUserHandler_ResetPassword_UnwiredBudget429CarriesNoHeaders pins
// the optional-dependency contract: without WithFailureBudget the
// handler advertises nothing, whatever the service answers (the
// unwired service never produces a 429, so this pins the handler side
// alone).
func TestUserHandler_ResetPassword_UnwiredBudget429CarriesNoHeaders(t *testing.T) {
	router, authService, _ := newUsersTestRouter(t)

	w := putUserPasswordAs(t, router, authService, "admin-1", "user-1",
		`{"adminPassword":"not-the-admins-password","newPassword":"brand-new-pw"}`)
	resp := w.Result()
	defer resp.Body.Close()

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "The unwired-budget failure should stay a 401")
	assert.Empty(t, resp.Header.Get("X-RateLimit-Limit"), "An unwired budget must not carry rate-limit headers")
}
