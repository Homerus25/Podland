package users

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Homerus25/podland/backend/internal/domain/auth"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// putUserRoleAs PUTs the given JSON body to /api/users/{targetID}/role
// with a bearer token minted for callerID (empty string sends no
// Authorization header).
func putUserRoleAs(t *testing.T, router http.Handler, authService *auth.AuthService, callerID, targetID, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPut, "/api/users/"+targetID+"/role", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if callerID != "" {
		token, err := auth.CreateValidTestToken(authService, callerID)
		require.NoError(t, err, "Should mint a test token")
		req.Header.Set("Authorization", "Bearer "+token)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// roleInList lists users as admin-1 and returns the role recorded for
// the user with the given ID — persistence verified through the HTTP
// surface, not the repository.
func roleInList(t *testing.T, router http.Handler, authService *auth.AuthService, userID string) string {
	t.Helper()

	w := getUsersAs(t, router, authService, "admin-1")
	resp := w.Result()
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "Precondition: the admin list should succeed")

	for _, user := range decodeUsersList(t, w) {
		if user["id"] == userID {
			role, _ := user["role"].(string)
			return role
		}
	}
	require.Fail(t, "User %q should appear in the list", userID)
	return ""
}

// TestUserHandler_UpdateRole_AdminPromotesUser asserts the happy path:
// an admin promotes a regular user, gets 200 with the updated user —
// exactly id, username, role, createdAt — and a following list shows
// the promotion took effect.
func TestUserHandler_UpdateRole_AdminPromotesUser(t *testing.T) {
	router, authService, _ := newUsersTestRouter(t)

	w := putUserRoleAs(t, router, authService, "admin-1", "user-1", `{"role":"admin"}`)

	resp := w.Result()
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "Admin role change should return 200")

	var body map[string]any
	err := json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err, "Should decode the updated-user response")
	assert.Equal(t, "user-1", body["id"], "Response should carry the target user's ID")
	assert.Equal(t, "admin", body["role"], "Response should carry the new role")

	assert.ElementsMatch(t,
		[]string{"id", "username", "role", "createdAt"},
		mapKeys(body),
		"Response must expose exactly the documented fields — never the password hash")

	assert.Equal(t, "admin", roleInList(t, router, authService, "user-1"), "The promotion should be persisted")
}

// TestUserHandler_UpdateRole_DemotesAdmin asserts the reverse direction:
// a promoted user can be demoted back, and the list reflects it.
func TestUserHandler_UpdateRole_DemotesAdmin(t *testing.T) {
	router, authService, _ := newUsersTestRouter(t)

	promote := putUserRoleAs(t, router, authService, "admin-1", "user-1", `{"role":"admin"}`)
	defer promote.Result().Body.Close()
	require.Equal(t, http.StatusOK, promote.Result().StatusCode, "Precondition: promotion should succeed")

	w := putUserRoleAs(t, router, authService, "admin-1", "user-1", `{"role":"user"}`)

	resp := w.Result()
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "Demotion should return 200")

	var body map[string]any
	err := json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err, "Should decode the updated-user response")
	assert.Equal(t, "user", body["role"], "Response should carry the demoted role")

	assert.Equal(t, "user", roleInList(t, router, authService, "user-1"), "The demotion should be persisted")
}

// TestUserHandler_UpdateRole_SelfRejected pins the guard at the HTTP
// surface: an admin changing their own role gets 400 SELF_ROLE_CHANGE
// and their role survives untouched — the instance cannot be locked
// out of administration.
func TestUserHandler_UpdateRole_SelfRejected(t *testing.T) {
	router, authService, _ := newUsersTestRouter(t)

	w := putUserRoleAs(t, router, authService, "admin-1", "admin-1", `{"role":"user"}`)

	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "A self role change should be rejected with 400")
	assert.Equal(t, "SELF_ROLE_CHANGE", testutils.DecodeErrorEnvelope(t, resp).Error.Code, "Error body should carry SELF_ROLE_CHANGE")

	assert.Equal(t, "admin", roleInList(t, router, authService, "admin-1"), "The caller's role must be untouched")
}

// TestUserHandler_UpdateRole_UnknownRoleRejected asserts a role outside
// {user, admin} — and an empty one — is rejected with 400
// VALIDATION_FAILED and changes nothing.
func TestUserHandler_UpdateRole_UnknownRoleRejected(t *testing.T) {
	router, authService, _ := newUsersTestRouter(t)

	for name, body := range map[string]string{
		"unknown role": `{"role":"superuser"}`,
		"empty role":   `{"role":""}`,
		"missing role": `{}`,
	} {
		t.Run(name, func(t *testing.T) {
			w := putUserRoleAs(t, router, authService, "admin-1", "user-1", body)

			resp := w.Result()
			defer resp.Body.Close()
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "An invalid role should be rejected with 400")
			assert.Equal(t, "VALIDATION_FAILED", testutils.DecodeErrorEnvelope(t, resp).Error.Code, "Error body should carry VALIDATION_FAILED")
		})
	}

	assert.Equal(t, "user", roleInList(t, router, authService, "user-1"), "The target's role must be untouched")
}

// TestUserHandler_UpdateRole_TargetNotFound asserts changing the role
// of a user that does not exist is answered with 404 USER_NOT_FOUND.
func TestUserHandler_UpdateRole_TargetNotFound(t *testing.T) {
	router, authService, _ := newUsersTestRouter(t)

	w := putUserRoleAs(t, router, authService, "admin-1", "ghost-id", `{"role":"admin"}`)

	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "A missing target user should be rejected with 404")
	assert.Equal(t, "USER_NOT_FOUND", testutils.DecodeErrorEnvelope(t, resp).Error.Code, "Error body should carry USER_NOT_FOUND")
}

// TestUserHandler_UpdateRole_NonAdminForbidden asserts an authenticated
// caller with the regular user role gets 403 ADMIN_REQUIRED.
func TestUserHandler_UpdateRole_NonAdminForbidden(t *testing.T) {
	router, authService, _ := newUsersTestRouter(t)

	w := putUserRoleAs(t, router, authService, "user-1", "admin-1", `{"role":"admin"}`)

	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "Non-admin should be rejected with 403")
	assert.Equal(t, "ADMIN_REQUIRED", testutils.DecodeErrorEnvelope(t, resp).Error.Code, "Error body should carry the ADMIN_REQUIRED code")
}

// TestUserHandler_UpdateRole_Unauthenticated asserts a caller without a
// token is rejected by the JWT middleware with 401 before the admin
// gate or handler ever run.
func TestUserHandler_UpdateRole_Unauthenticated(t *testing.T) {
	router, authService, _ := newUsersTestRouter(t)

	w := putUserRoleAs(t, router, authService, "", "user-1", `{"role":"admin"}`)

	assert.Equal(t, http.StatusUnauthorized, w.Result().StatusCode, "Missing token should be rejected with 401")
}
