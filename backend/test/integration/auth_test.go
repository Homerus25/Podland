//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

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