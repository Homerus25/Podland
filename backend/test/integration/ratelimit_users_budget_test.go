//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUsersResetPasswordFailureBudget_LocksAdmin is the reset surface's
// credential-failure budget demo (issue 187): with the shared budget
// ceiling raised out of the way, wrong-admin-password resets for one
// admin spend that admin's budget and then lock even the CORRECT admin
// password out with 429 RATE_LIMIT_EXCEEDED and the X-RateLimit
// headers; a successful reset beforehand spends nothing; and another
// admin resetting the same target is unaffected — the budget is keyed
// on the acting admin.
func TestUsersResetPasswordFailureBudget_LocksAdmin(t *testing.T) {
	suite.SkipIfUnavailable(t)

	const budget = 3
	application, serverURL := bootFailureBudgetApp(t, budget)

	password := "it-" + suite.RunID + "-fbrpw-" + uuid.New().String()[:8]
	adminUser := "it-" + suite.RunID + "-fbr-admin"
	_, err := application.SeedUserWithRole(adminUser, password, models.RoleAdmin)
	require.NoError(t, err, "Should seed the resetting admin")
	otherAdminUser := "it-" + suite.RunID + "-fbr-admin2"
	_, err = application.SeedUserWithRole(otherAdminUser, password, models.RoleAdmin)
	require.NoError(t, err, "Should seed the untouched admin")
	targetUser := "it-" + suite.RunID + "-fbr-target"
	targetID, err := application.SeedUserWithRole(targetUser, password, models.RoleUser)
	require.NoError(t, err, "Should seed the reset target")

	adminToken, err := realLogin(serverURL, adminUser, password)
	require.NoError(t, err, "Should log in as the resetting admin")

	putReset := func(bearerToken, adminPassword string) *http.Response {
		t.Helper()
		reqBody, err := json.Marshal(map[string]string{"adminPassword": adminPassword, "newPassword": "brand-new-pw"})
		require.NoError(t, err, "Should marshal reset body")
		req, err := http.NewRequest(http.MethodPut, serverURL+"/api/users/"+targetID+"/password", bytes.NewReader(reqBody))
		require.NoError(t, err, "Should build reset request")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+bearerToken)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err, "Should PUT the reset endpoint")
		return resp
	}

	// A successful reset first: it must not consume any budget.
	successResp := putReset(adminToken, password)
	require.Equal(t, http.StatusOK, successResp.StatusCode, "The pre-budget reset with the correct admin password should succeed")
	require.NoError(t, successResp.Body.Close(), "Should close the 200 body")

	// Spend the budget with wrong-admin-password attempts: all in-budget
	// 401s.
	for i := range budget {
		resp := putReset(adminToken, "not-the-admins-password")
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode,
			"Wrong-admin-password attempt %d within the budget should reach the handler as 401", i+1)
		require.NoError(t, resp.Body.Close(), "Should close the 401 body")
	}

	// Locked out: the CORRECT admin password answers 429 with the
	// budget's headers, not the reset payload.
	lockedResp := putReset(adminToken, password)
	defer lockedResp.Body.Close()
	require.Equal(t, http.StatusTooManyRequests, lockedResp.StatusCode,
		"The locked-out reset should answer 429 even with the correct admin password")
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

	// Keyed on the acting admin: the second admin's budget is untouched,
	// so their reset of the same target goes through.
	otherToken, err := realLogin(serverURL, otherAdminUser, password)
	require.NoError(t, err, "Should log in as the second admin")
	otherResp := putReset(otherToken, password)
	defer otherResp.Body.Close()
	require.Equal(t, http.StatusOK, otherResp.StatusCode,
		"One admin's failures must never touch another admin's budget")
}
