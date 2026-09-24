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

// TestAuth_Login_WrongPassword asserts that POST /api/auth/login returns 401
// INVALID_CREDENTIALS when the password is incorrect. This test extends the
// 080-01 harness auth by exercising the real login endpoint with invalid
// credentials.
func TestAuth_Login_WrongPassword(t *testing.T) {
	suite.SkipIfUnavailable(t)

	// Use the suite's seeded username but with a wrong password
	username := "it-" + suite.RunID + "-admin"
	wrongPassword := suite.OwnerUserID + "-wrong-password"

	loginReq := map[string]string{
		"username": username,
		"password": wrongPassword,
	}

	reqBody, err := json.Marshal(loginReq)
	require.NoError(t, err, "Should marshal login request")

	resp, err := http.Post(suite.ServerURL+"/api/auth/login", "application/json", bytes.NewReader(reqBody))
	require.NoError(t, err, "Should POST to login endpoint")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Should return 401 for wrong password")

	var errorResp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		RequestID string `json:"requestId"`
	}
	err = json.NewDecoder(resp.Body).Decode(&errorResp)
	require.NoError(t, err, "Should decode error response")

	assert.Equal(t, "INVALID_CREDENTIALS", errorResp.Error.Code, "Should return INVALID_CREDENTIALS error code")
	assert.Equal(t, "Invalid username or password", errorResp.Error.Message, "Should return correct error message")
	assert.NotEmpty(t, errorResp.RequestID, "Should include request ID")
}

// TestAuth_Login_UnknownUser asserts that POST /api/auth/login returns 401
// INVALID_CREDENTIALS when the username doesn't exist (no user enumeration).
func TestAuth_Login_UnknownUser(t *testing.T) {
	suite.SkipIfUnavailable(t)

	loginReq := map[string]string{
		"username": "nonexistent-user-" + suite.RunID,
		"password": "some-password",
	}

	reqBody, err := json.Marshal(loginReq)
	require.NoError(t, err, "Should marshal login request")

	resp, err := http.Post(suite.ServerURL+"/api/auth/login", "application/json", bytes.NewReader(reqBody))
	require.NoError(t, err, "Should POST to login endpoint")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Should return 401 for unknown user")

	var errorResp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		RequestID string `json:"requestId"`
	}
	err = json.NewDecoder(resp.Body).Decode(&errorResp)
	require.NoError(t, err, "Should decode error response")

	assert.Equal(t, "INVALID_CREDENTIALS", errorResp.Error.Code, "Should return INVALID_CREDENTIALS error code")
	assert.NotEmpty(t, errorResp.RequestID, "Should include request ID")
}

// TestAuth_Login_ResponseCarriesRole asserts the login body reports the
// authenticated user's role: a regular-seeded user sees "user", a
// seeded admin sees "admin".
func TestAuth_Login_ResponseCarriesRole(t *testing.T) {
	suite.SkipIfUnavailable(t)

	seed := func(role string) (string, string) {
		t.Helper()
		username := "it-" + suite.RunID + "-login-" + role + "-" + uuid.New().String()[:8]
		password := "it-" + suite.RunID + "-pw-" + uuid.New().String()[:8]
		_, err := suite.SeedUserWithRole(username, password, role)
		require.NoError(t, err, "Should seed %s-role user", role)
		return username, password
	}

	login := func(username, password string) map[string]any {
		t.Helper()
		reqBody, err := json.Marshal(map[string]string{"username": username, "password": password})
		require.NoError(t, err, "Should marshal login request")
		resp, err := http.Post(suite.ServerURL+"/api/auth/login", "application/json", bytes.NewReader(reqBody))
		require.NoError(t, err, "Should POST to login endpoint")
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "Login should succeed with correct credentials")
		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body), "Should decode login response")
		return body
	}

	adminUser, adminPass := seed(models.RoleAdmin)
	assert.Equal(t, "admin", login(adminUser, adminPass)["role"], "Admin login should report the admin role")

	regularUser, regularPass := seed(models.RoleUser)
	assert.Equal(t, "user", login(regularUser, regularPass)["role"], "Regular-user login should report the user role")
}

// statusResponseBody is the decoded GET /api/auth/status payload shared
// by the role tests below.
type statusResponseBody struct {
	Authenticated bool   `json:"authenticated"`
	UserID        string `json:"userId"`
	Role          string `json:"role"`
}

// TestAuth_Status_DefaultUserSeesUserRole asserts that the suite owner —
// seeded without an explicit role, like every user created before the
// role concept existed — sees the default role "user" in the status
// response (090-00).
func TestAuth_Status_DefaultUserSeesUserRole(t *testing.T) {
	suite.SkipIfUnavailable(t)

	client := suite.AuthenticatedClient()
	resp, err := client.Get(suite.ServerURL + "/api/auth/status")
	require.NoError(t, err, "Should GET status endpoint")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Should return 200 for authenticated user")

	var body statusResponseBody
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err, "Should decode status response")

	assert.True(t, body.Authenticated, "Should report authenticated")
	assert.Equal(t, suite.OwnerUserID, body.UserID, "Should report the logged-in user's ID")
	assert.Equal(t, "user", body.Role, "Default-seeded user should see role user")
}

// TestAuth_Status_AdminSeesAdminRole asserts that a user seeded with the
// admin role — the same role the first-run setup endpoint grants — sees
// "admin" in the status response after a real login (090-00).
func TestAuth_Status_AdminSeesAdminRole(t *testing.T) {
	suite.SkipIfUnavailable(t)

	username := "it-" + suite.RunID + "-superadmin"
	password := "it-" + suite.RunID + "-adminpw-" + uuid.New().String()[:8]
	_, err := suite.SeedUserWithRole(username, password, models.RoleAdmin)
	require.NoError(t, err, "Should seed admin-role user")

	token, err := realLogin(suite.ServerURL, username, password)
	require.NoError(t, err, "Should log in as the seeded admin")

	req, err := http.NewRequest(http.MethodGet, suite.ServerURL+"/api/auth/status", nil)
	require.NoError(t, err, "Should build status request")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "Should GET status endpoint")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Should return 200 for authenticated admin")

	var body statusResponseBody
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err, "Should decode status response")

	assert.True(t, body.Authenticated, "Should report authenticated")
	assert.Equal(t, "admin", body.Role, "Admin-seeded user should see role admin")
}
