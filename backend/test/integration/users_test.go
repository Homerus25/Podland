//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/Homerus25/podland/backend/internal/models"
)

// adminLoginForUsers seeds an admin user — the same role cmd/initdb
// grants its bootstrap user — and returns a bearer token obtained
// through the real login endpoint, so user-management tests exercise
// the full auth path.
func adminLoginForUsers(t *testing.T) string {
	t.Helper()

	username := "it-" + suite.RunID + "-useradmin-" + uuid.New().String()[:8]
	password := "it-" + suite.RunID + "-adminpw-" + uuid.New().String()[:8]

	_, err := suite.SeedUserWithRole(username, password, models.RoleAdmin)
	require.NoError(t, err, "Should seed admin user")

	token, err := realLogin(suite.ServerURL, username, password)
	require.NoError(t, err, "Should log in as the seeded admin")
	return token
}

// postCreateUser posts body to POST /api/users with the given bearer
// token ("" sends no Authorization header) and returns the response.
func postCreateUser(t *testing.T, token, body string) *http.Response {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, suite.ServerURL+"/api/users", bytes.NewBufferString(body))
	require.NoError(t, err, "Should build create-user request")
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "Should POST to create-user endpoint")
	return resp
}

// createdUserBody decodes the 201 response of POST /api/users.
type createdUserBody struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	CreatedAt string `json:"createdAt"`
}

// TestUsers_Create_AdminCreatesUserWhoCanLogIn is the slice's tracer
// bullet: an admin creates a user through the endpoint, the response
// carries the created user (id, username, role, createdAt — no password
// material), and the new user can immediately log in with the given
// credentials through the real login endpoint.
func TestUsers_Create_AdminCreatesUserWhoCanLogIn(t *testing.T) {
	suite.SkipIfUnavailable(t)

	adminToken := adminLoginForUsers(t)

	username := "it-" + suite.RunID + "-alice-" + uuid.New().String()[:8]
	password := "it-" + suite.RunID + "-pw-" + uuid.New().String()[:8]

	resp := postCreateUser(t, adminToken, `{"username":"`+username+`","password":"`+password+`"}`)
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode, "Admin creation should return 201")

	var body createdUserBody
	err := json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err, "Should decode created-user response")

	assert.Equal(t, username, body.Username, "Response should carry the new username")
	assert.Equal(t, "user", body.Role, "Endpoint-created users get the regular user role")
	assert.NotEmpty(t, body.ID, "Response should carry the new user's ID")
	assert.NotEmpty(t, body.CreatedAt, "Response should carry the creation timestamp")

	newToken, err := realLogin(suite.ServerURL, username, password)
	require.NoError(t, err, "Created user must be able to log in with the given credentials")
	assert.NotEmpty(t, newToken, "Login should return a token")
}

// TestUsers_Create_NonAdminForbidden asserts an authenticated caller
// with the regular user role — the suite's default owner — is rejected
// with 403.
func TestUsers_Create_NonAdminForbidden(t *testing.T) {
	suite.SkipIfUnavailable(t)

	resp := postCreateUser(t, suite.Token, `{"username":"it-`+suite.RunID+`-nope","password":"some-password"}`)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "Non-admin caller should get 403")
}

// TestUsers_Create_Unauthenticated asserts a caller without a token is
// rejected with 401 before any creation logic runs.
func TestUsers_Create_Unauthenticated(t *testing.T) {
	suite.SkipIfUnavailable(t)

	resp := postCreateUser(t, "", `{"username":"it-`+suite.RunID+`-anon","password":"some-password"}`)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Unauthenticated caller should get 401")
}

// TestUsers_Create_DuplicateUsername asserts creating a username that
// already exists — including one created through the endpoint itself —
// is rejected with 409.
func TestUsers_Create_DuplicateUsername(t *testing.T) {
	suite.SkipIfUnavailable(t)

	adminToken := adminLoginForUsers(t)

	username := "it-" + suite.RunID + "-dupe-" + uuid.New().String()[:8]
	body := `{"username":"` + username + `","password":"first-password"}`

	first := postCreateUser(t, adminToken, body)
	defer first.Body.Close()
	require.Equal(t, http.StatusCreated, first.StatusCode, "First creation should succeed")

	second := postCreateUser(t, adminToken, body)
	defer second.Body.Close()
	assert.Equal(t, http.StatusConflict, second.StatusCode, "Duplicate username should get 409")
}

// TestUsers_Create_MissingFields asserts missing or empty fields are
// rejected with 400 before anything is persisted.
func TestUsers_Create_MissingFields(t *testing.T) {
	suite.SkipIfUnavailable(t)

	adminToken := adminLoginForUsers(t)

	for name, body := range map[string]string{
		"missing username": `{"password":"wonderland"}`,
		"empty password":   `{"username":"it-` + suite.RunID + `-fields","password":""}`,
	} {
		t.Run(name, func(t *testing.T) {
			resp := postCreateUser(t, adminToken, body)
			defer resp.Body.Close()
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Invalid payload should get 400")
		})
	}
}
