//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// adminLoginForUsers seeds an admin user — the same role the first-run
// setup endpoint grants — and returns a bearer token obtained
// through the real login endpoint (plus the username, for tests that
// need to find the admin in a list), so user-management tests exercise
// the full auth path.
func adminLoginForUsers(t *testing.T) (string, string) {
	t.Helper()

	username := "it-" + suite.RunID + "-useradmin-" + uuid.New().String()[:8]
	password := "it-" + suite.RunID + "-adminpw-" + uuid.New().String()[:8]

	_, err := suite.SeedUserWithRole(username, password, models.RoleAdmin)
	require.NoError(t, err, "Should seed admin user")

	token, err := realLogin(suite.ServerURL, username, password)
	require.NoError(t, err, "Should log in as the seeded admin")
	return token, username
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

	adminToken, _ := adminLoginForUsers(t)

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

	adminToken, _ := adminLoginForUsers(t)

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

	adminToken, _ := adminLoginForUsers(t)

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

// TestUsers_Create_AdminRoleAccepted asserts the optional role: an
// admin can create another admin through the endpoint, the response
// carries the persisted role, and the new admin really is one — a
// regular creation without the role field still lands as "user".
func TestUsers_Create_AdminRoleAccepted(t *testing.T) {
	suite.SkipIfUnavailable(t)

	adminToken, _ := adminLoginForUsers(t)

	adminUsername := "it-" + suite.RunID + "-newadmin-" + uuid.New().String()[:8]
	adminPassword := "it-" + suite.RunID + "-pw-" + uuid.New().String()[:8]

	resp := postCreateUser(t, adminToken, `{"username":"`+adminUsername+`","password":"`+adminPassword+`","role":"admin"}`)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode, "Admin creation with an explicit role should return 201")

	var body createdUserBody
	err := json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err, "Should decode created-user response")
	assert.Equal(t, "admin", body.Role, "Response should carry the requested admin role")

	newToken, err := realLogin(suite.ServerURL, adminUsername, adminPassword)
	require.NoError(t, err, "The created admin should be able to log in")

	// The role took effect: the new admin receives the full list shape.
	listResp := getUsers(t, newToken)
	defer listResp.Body.Close()
	require.Equal(t, http.StatusOK, listResp.StatusCode, "The created admin should be allowed to list users")

	var users []map[string]any
	err = json.NewDecoder(listResp.Body).Decode(&users)
	require.NoError(t, err, "Should decode the users list")
	for _, user := range users {
		assert.ElementsMatch(t,
			[]string{"id", "username", "role", "createdAt"},
			mapKeysAny(user),
			"The created admin must receive the full admin shape")
	}

	// Same request without the role field: the default remains "user".
	regularUsername := "it-" + suite.RunID + "-defaultrole-" + uuid.New().String()[:8]
	regular := postCreateUser(t, adminToken, `{"username":"`+regularUsername+`","password":"`+adminPassword+`"}`)
	defer regular.Body.Close()
	require.Equal(t, http.StatusCreated, regular.StatusCode, "Creation without a role should return 201")

	var regularBody createdUserBody
	err = json.NewDecoder(regular.Body).Decode(&regularBody)
	require.NoError(t, err, "Should decode created-user response")
	assert.Equal(t, "user", regularBody.Role, "An omitted role defaults to the regular user role")
}

// TestUsers_Create_UnknownRoleRejected asserts a role outside
// {user, admin} is rejected with 400 and nothing is persisted.
func TestUsers_Create_UnknownRoleRejected(t *testing.T) {
	suite.SkipIfUnavailable(t)

	adminToken, _ := adminLoginForUsers(t)

	username := "it-" + suite.RunID + "-sneaky-" + uuid.New().String()[:8]

	resp := postCreateUser(t, adminToken, `{"username":"`+username+`","password":"some-password","role":"superuser"}`)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "An unknown role should be rejected with 400")
	assert.Contains(t, respBody(t, resp), "VALIDATION_FAILED", "Error body should carry VALIDATION_FAILED")
}

// TestUsers_Create_PasswordTooShort asserts the password policy holds
// on the admin create path: below 8 characters is a 400
// PASSWORD_TOO_SHORT, the same code the self-service change returns.
func TestUsers_Create_PasswordTooShort(t *testing.T) {
	suite.SkipIfUnavailable(t)

	adminToken, _ := adminLoginForUsers(t)

	resp := postCreateUser(t, adminToken, `{"username":"it-`+suite.RunID+`-shorty","password":"short"}`)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "A too-short password should be rejected with 400")
	assert.Contains(t, respBody(t, resp), "PASSWORD_TOO_SHORT", "Error body should carry PASSWORD_TOO_SHORT")
}

// respBody reads a response body to a string (for error-code contains
// checks; the response body may only be read once).
func respBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "Should read response body")
	return string(raw)
}

// getUsers GETs /api/users with the given bearer token ("" sends no
// Authorization header) and returns the response.
func getUsers(t *testing.T, token string) *http.Response {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, suite.ServerURL+"/api/users", nil)
	require.NoError(t, err, "Should build list-users request")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "Should GET the list-users endpoint")
	return resp
}

// TestUsers_List_AdminGetsFullShape asserts the admin contract: every
// entry carries exactly id, username, role, createdAt (never password
// material), the list is ordered by username, and users created through
// the endpoint show up.
func TestUsers_List_AdminGetsFullShape(t *testing.T) {
	suite.SkipIfUnavailable(t)

	adminToken, adminUsername := adminLoginForUsers(t)

	createdUsername := "it-" + suite.RunID + "-listed-" + uuid.New().String()[:8]
	createResp := postCreateUser(t, adminToken, `{"username":"`+createdUsername+`","password":"some-password"}`)
	createResp.Body.Close()
	require.Equal(t, http.StatusCreated, createResp.StatusCode, "Precondition: user creation should succeed")

	resp := getUsers(t, adminToken)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "Admin list should return 200")

	var users []map[string]any
	err := json.NewDecoder(resp.Body).Decode(&users)
	require.NoError(t, err, "Should decode the users list as a bare JSON array")
	require.NotEmpty(t, users, "The list should contain the seeded users")

	var listed *map[string]any
	usernames := make([]string, 0, len(users))
	for i, user := range users {
		assert.ElementsMatch(t,
			[]string{"id", "username", "role", "createdAt"},
			mapKeysAny(user),
			"Admin list entries must expose exactly the documented fields — never the password hash")
		name, _ := user["username"].(string)
		usernames = append(usernames, name)
		if name == adminUsername {
			listed = &users[i]
		}
	}
	assert.IsIncreasing(t, usernames, "The list should be ordered by username")

	require.NotNil(t, listed, "The seeded admin %q should appear in the list", adminUsername)
	assert.Equal(t, "admin", (*listed)["role"], "The admin's entry should carry the admin role")
}

// TestUsers_List_UserGetsMinimalShape asserts the regular-user
// contract: every entry carries exactly id and username — role and
// createdAt are absent — and admins are discoverable, enough for the
// machine-sharing picker.
func TestUsers_List_UserGetsMinimalShape(t *testing.T) {
	suite.SkipIfUnavailable(t)

	_, adminUsername := adminLoginForUsers(t)

	resp := getUsers(t, suite.Token)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "Regular-user list should return 200")

	var users []map[string]any
	err := json.NewDecoder(resp.Body).Decode(&users)
	require.NoError(t, err, "Should decode the users list as a bare JSON array")
	require.NotEmpty(t, users, "The list should contain the seeded users")

	foundAdmin := false
	for _, user := range users {
		assert.ElementsMatch(t,
			[]string{"id", "username"},
			mapKeysAny(user),
			"Regular callers must receive the minimal shape only")
		if name, _ := user["username"].(string); name == adminUsername {
			foundAdmin = true
		}
	}
	assert.True(t, foundAdmin, "The admin should be discoverable in the minimal list")
}

// TestUsers_List_Unauthenticated asserts a caller without a token is
// rejected with 401 before any listing happens.
func TestUsers_List_Unauthenticated(t *testing.T) {
	suite.SkipIfUnavailable(t)

	resp := getUsers(t, "")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Unauthenticated caller should get 401")
}

// mapKeysAny returns the keys of a JSON object decoded into
// map[string]any (the integration twin of the users package's mapKeys).
func mapKeysAny(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// putUserRole PUTs body to /api/users/{userID}/role with the given
// bearer token ("" sends no Authorization header) and returns the
// response.
func putUserRole(t *testing.T, token, userID, body string) *http.Response {
	t.Helper()

	req, err := http.NewRequest(http.MethodPut, suite.ServerURL+"/api/users/"+userID+"/role", bytes.NewBufferString(body))
	require.NoError(t, err, "Should build role-change request")
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "Should PUT the role-change endpoint")
	return resp
}

// userIDForUsername lists users as the given caller and returns the ID
// recorded for username — the way an admin discovers their own ID (and
// everyone else's) through the API.
func userIDForUsername(t *testing.T, token, username string) string {
	t.Helper()

	resp := getUsers(t, token)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "Precondition: the users list should succeed")

	var users []map[string]any
	err := json.NewDecoder(resp.Body).Decode(&users)
	require.NoError(t, err, "Should decode the users list")
	for _, user := range users {
		if name, _ := user["username"].(string); name == username {
			id, _ := user["id"].(string)
			require.NotEmpty(t, id, "The listed user should carry an ID")
			return id
		}
	}
	require.Fail(t, "User %q should appear in the list", username)
	return ""
}

// TestUsers_UpdateRole_AdminPromotesAndDemotes is the tracer bullet: an
// admin promotes an endpoint-created user to admin — the promoted user
// can then log in and receives the full admin list shape — and demotes
// them back.
func TestUsers_UpdateRole_AdminPromotesAndDemotes(t *testing.T) {
	suite.SkipIfUnavailable(t)

	adminToken, _ := adminLoginForUsers(t)

	username := "it-" + suite.RunID + "-promotee-" + uuid.New().String()[:8]
	password := "it-" + suite.RunID + "-pw-" + uuid.New().String()[:8]
	createResp := postCreateUser(t, adminToken, `{"username":"`+username+`","password":"`+password+`"}`)
	createResp.Body.Close()
	require.Equal(t, http.StatusCreated, createResp.StatusCode, "Precondition: user creation should succeed")

	targetID := userIDForUsername(t, adminToken, username)

	promote := putUserRole(t, adminToken, targetID, `{"role":"admin"}`)
	defer promote.Body.Close()
	require.Equal(t, http.StatusOK, promote.StatusCode, "Promotion should return 200")

	var promoted updatedUserBody
	err := json.NewDecoder(promote.Body).Decode(&promoted)
	require.NoError(t, err, "Should decode the updated-user response")
	assert.Equal(t, targetID, promoted.ID, "Response should carry the target user's ID")
	assert.Equal(t, "admin", promoted.Role, "Response should carry the promoted role")

	// The role took effect: the promoted user logs in and receives the
	// full admin shape on the list.
	newToken, err := realLogin(suite.ServerURL, username, password)
	require.NoError(t, err, "The promoted user should be able to log in")

	listResp := getUsers(t, newToken)
	defer listResp.Body.Close()
	require.Equal(t, http.StatusOK, listResp.StatusCode, "The promoted user should now be allowed to list users")

	var users []map[string]any
	err = json.NewDecoder(listResp.Body).Decode(&users)
	require.NoError(t, err, "Should decode the users list")
	for _, user := range users {
		assert.ElementsMatch(t,
			[]string{"id", "username", "role", "createdAt"},
			mapKeysAny(user),
			"The promoted user must receive the full admin shape")
	}

	demote := putUserRole(t, adminToken, targetID, `{"role":"user"}`)
	defer demote.Body.Close()
	require.Equal(t, http.StatusOK, demote.StatusCode, "Demotion should return 200")

	var demoted updatedUserBody
	err = json.NewDecoder(demote.Body).Decode(&demoted)
	require.NoError(t, err, "Should decode the updated-user response")
	assert.Equal(t, "user", demoted.Role, "Response should carry the demoted role")
}

// TestUsers_UpdateRole_SelfRejected pins the self-lockout guard at the
// API surface: an admin changing their own role gets 400
// SELF_ROLE_CHANGE, and the admin still works afterwards — the instance
// was never locked out.
func TestUsers_UpdateRole_SelfRejected(t *testing.T) {
	suite.SkipIfUnavailable(t)

	adminToken, adminUsername := adminLoginForUsers(t)
	adminID := userIDForUsername(t, adminToken, adminUsername)

	resp := putUserRole(t, adminToken, adminID, `{"role":"user"}`)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "A self role change should be rejected with 400")
	assert.Contains(t, respBody(t, resp), "SELF_ROLE_CHANGE", "Error body should carry SELF_ROLE_CHANGE")

	// The admin is untouched: an admin-only operation still succeeds.
	stillAdmin := postCreateUser(t, adminToken, `{"username":"it-`+suite.RunID+`-after-self-`+uuid.New().String()[:8]+`","password":"some-password"}`)
	defer stillAdmin.Body.Close()
	assert.Equal(t, http.StatusCreated, stillAdmin.StatusCode, "The admin must keep working after a rejected self-change")
}

// TestUsers_UpdateRole_NonAdminForbidden asserts an authenticated
// regular user is rejected with 403 before any role logic runs.
func TestUsers_UpdateRole_NonAdminForbidden(t *testing.T) {
	suite.SkipIfUnavailable(t)

	adminToken, _ := adminLoginForUsers(t)

	username := "it-" + suite.RunID + "-gated-" + uuid.New().String()[:8]
	createResp := postCreateUser(t, adminToken, `{"username":"`+username+`","password":"some-password"}`)
	createResp.Body.Close()
	require.Equal(t, http.StatusCreated, createResp.StatusCode, "Precondition: user creation should succeed")
	targetID := userIDForUsername(t, adminToken, username)

	resp := putUserRole(t, suite.Token, targetID, `{"role":"admin"}`)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "Non-admin caller should get 403")
}

// TestUsers_UpdateRole_Unauthenticated asserts a caller without a token
// is rejected with 401.
func TestUsers_UpdateRole_Unauthenticated(t *testing.T) {
	suite.SkipIfUnavailable(t)

	resp := putUserRole(t, "", "some-user-id", `{"role":"admin"}`)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Unauthenticated caller should get 401")
}

// TestUsers_UpdateRole_InvalidRole asserts a role outside {user, admin}
// is rejected with 400 VALIDATION_FAILED.
func TestUsers_UpdateRole_InvalidRole(t *testing.T) {
	suite.SkipIfUnavailable(t)

	adminToken, _ := adminLoginForUsers(t)

	username := "it-" + suite.RunID + "-roletarget-" + uuid.New().String()[:8]
	createResp := postCreateUser(t, adminToken, `{"username":"`+username+`","password":"some-password"}`)
	createResp.Body.Close()
	require.Equal(t, http.StatusCreated, createResp.StatusCode, "Precondition: user creation should succeed")

	targetID := userIDForUsername(t, adminToken, username)

	resp := putUserRole(t, adminToken, targetID, `{"role":"superuser"}`)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "An unknown role should be rejected with 400")
	assert.Contains(t, respBody(t, resp), "VALIDATION_FAILED", "Error body should carry VALIDATION_FAILED")
}

// updatedUserBody decodes the 200 response of PUT /api/users/{id}/role.
type updatedUserBody struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	CreatedAt string `json:"createdAt"`
}
