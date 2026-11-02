package users

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/domain/auth"
	"github.com/Homerus25/podland/backend/internal/failurebudget"
	appmiddleware "github.com/Homerus25/podland/backend/internal/middleware"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// newUsersTestRouter wires the users routes exactly like internal/app:
// JWT middleware (the list endpoint gated by nothing but that; create,
// role change, and password reset behind the admin gate), against a
// mock user repository holding one admin and one regular user and a
// mock session repository the password reset revokes through. It
// returns the router, the auth service so tests can mint bearer
// tokens, and the session repository so tests can seed and count
// sessions.
//
// The router (and the postUsersAs/putUserRoleAs/putUserPasswordAs/
// getUsersAs helpers driving it) is hand-rolled instead of riding
// testutils.Perform on purpose: the behaviour under test includes the
// real middleware chain — JWT plus RequireAdmin's repo lookup — and
// Perform invokes a single handler func, so it cannot build that
// chain. The auth package's handler tests can stay on Perform because
// they pin handler behaviour below the middleware.
//
// The admin carries a real bcrypt hash of known-admin-password so the
// password-reset tests can re-authenticate the acting admin.
func newUsersTestRouter(t *testing.T) (http.Handler, *auth.AuthService, *testutils.MockSessionRepository) {
	return newUsersTestRouterWithBudget(t, nil)
}

// newUsersTestRouterWithBudget is newUsersTestRouter with the reset
// endpoint's credential-failure budget wired into the service and the
// handler, the way internal/app wires one failurebudget.Surface value
// into both. A nil surface builds the plain unwired router.
func newUsersTestRouterWithBudget(t *testing.T, budget *failurebudget.Surface) (http.Handler, *auth.AuthService, *testutils.MockSessionRepository) {
	t.Helper()

	authService, err := auth.NewTestAuthService()
	require.NoError(t, err, "Should create auth service without error")

	adminHash, err := bcrypt.GenerateFromPassword([]byte("known-admin-password"), bcrypt.DefaultCost)
	require.NoError(t, err, "Should hash the admin's password")

	repo := testutils.NewMockUserRepository()
	repo.AddUser(&models.User{ID: "admin-1", Username: "root", Role: models.RoleAdmin, PasswordHash: string(adminHash), CreatedAt: time.Now()})
	repo.AddUser(&models.User{ID: "user-1", Username: "pleb", Role: models.RoleUser, CreatedAt: time.Now()})
	sessionRepo := testutils.NewMockSessionRepository()

	userService := NewUserService(repo, sessionRepo)
	userHandler := NewUserHandler(userService)
	if budget != nil {
		userService = userService.WithFailureBudget(budget)
		userHandler = userHandler.WithFailureBudget(budget)
	}

	jwtAuth := appmiddleware.JWTMiddleware(authService)
	requireAdmin := appmiddleware.RequireAdmin(repo)
	router := http.NewServeMux()
	router.Handle("GET /api/users", jwtAuth(http.HandlerFunc(userHandler.List)))
	router.Handle("POST /api/users", jwtAuth(requireAdmin(http.HandlerFunc(userHandler.Create))))
	router.Handle("PUT /api/users/{userId}/role", jwtAuth(requireAdmin(http.HandlerFunc(userHandler.UpdateRole))))
	router.Handle("PUT /api/users/{userId}/password", jwtAuth(requireAdmin(http.HandlerFunc(userHandler.ResetPassword))))

	return router, authService, sessionRepo
}

// postUsersAs posts the given JSON body to /api/users with a bearer
// token minted for userID (empty string sends no Authorization header).
func postUsersAs(t *testing.T, router http.Handler, authService *auth.AuthService, userID, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if userID != "" {
		token, err := auth.CreateValidTestToken(authService, userID)
		require.NoError(t, err, "Should mint a test token")
		req.Header.Set("Authorization", "Bearer "+token)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// TestUserHandler_Create_AdminCreatesUser asserts the happy path: an
// authenticated admin POSTs username + password and gets 201 with the
// created user — id, username, role "user", createdAt — and nothing
// else (in particular no password material).
func TestUserHandler_Create_AdminCreatesUser(t *testing.T) {
	router, authService, _ := newUsersTestRouter(t)

	w := postUsersAs(t, router, authService, "admin-1", `{"username":"alice","password":"wonderland"}`)

	resp := w.Result()
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode, "Admin creation should return 201")

	var body map[string]any
	err := json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err, "Should decode the created-user response")

	assert.Equal(t, "alice", body["username"], "Response should carry the new username")
	assert.Equal(t, "user", body["role"], "Created users get the regular user role")
	assert.NotEmpty(t, body["id"], "Response should carry the new user's ID")
	assert.NotEmpty(t, body["createdAt"], "Response should carry the creation timestamp")

	assert.ElementsMatch(t,
		[]string{"id", "username", "role", "createdAt"},
		mapKeys(body),
		"Response must expose exactly the documented fields — never the password hash")
}

// TestUserHandler_Create_AdminRoleAccepted asserts the body's optional
// role: an admin may create another admin, and the response carries the
// persisted role.
func TestUserHandler_Create_AdminRoleAccepted(t *testing.T) {
	router, authService, _ := newUsersTestRouter(t)

	w := postUsersAs(t, router, authService, "admin-1", `{"username":"boss","password":"wonderland","role":"admin"}`)

	resp := w.Result()
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode, "Admin creation with an explicit role should return 201")

	var body map[string]any
	err := json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err, "Should decode the created-user response")
	assert.Equal(t, "admin", body["role"], "Response should carry the requested admin role")
}

// TestUserHandler_Create_UnknownRoleRejected asserts a role outside
// {user, admin} is rejected with 400 VALIDATION_FAILED and persists
// nothing (the duplicate attempt afterwards confirms no row landed).
func TestUserHandler_Create_UnknownRoleRejected(t *testing.T) {
	router, authService, _ := newUsersTestRouter(t)

	w := postUsersAs(t, router, authService, "admin-1", `{"username":"sneaky","password":"wonderland","role":"superuser"}`)

	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "An unknown role should be rejected with 400")
	assert.Equal(t, "VALIDATION_FAILED", testutils.DecodeErrorEnvelope(t, resp).Error.Code, "Error body should carry VALIDATION_FAILED")

	dup := postUsersAs(t, router, authService, "admin-1", `{"username":"sneaky","password":"wonderland"}`)
	defer dup.Result().Body.Close()
	assert.Equal(t, http.StatusCreated, dup.Result().StatusCode, "The rejected attempt must not have persisted a row")
}

// TestUserHandler_Create_PasswordTooShort asserts the password policy
// holds on the admin create path: below 8 characters is a 400
// PASSWORD_TOO_SHORT, the same code the self-service change returns.
func TestUserHandler_Create_PasswordTooShort(t *testing.T) {
	router, authService, _ := newUsersTestRouter(t)

	w := postUsersAs(t, router, authService, "admin-1", `{"username":"shorty","password":"short"}`)

	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "A too-short password should be rejected with 400")
	assert.Equal(t, "PASSWORD_TOO_SHORT", testutils.DecodeErrorEnvelope(t, resp).Error.Code, "Error body should carry PASSWORD_TOO_SHORT")
}

// TestUserHandler_Create_Unauthenticated asserts a caller without a
// token is rejected by the JWT middleware with 401 before the admin
// gate or handler ever run.
func TestUserHandler_Create_Unauthenticated(t *testing.T) {
	router, authService, _ := newUsersTestRouter(t)

	w := postUsersAs(t, router, authService, "", `{"username":"alice","password":"wonderland"}`)

	assert.Equal(t, http.StatusUnauthorized, w.Result().StatusCode, "Missing token should be rejected with 401")
}

// TestUserHandler_Create_NonAdminForbidden asserts an authenticated
// caller with the regular user role gets 403 ADMIN_REQUIRED.
func TestUserHandler_Create_NonAdminForbidden(t *testing.T) {
	router, authService, _ := newUsersTestRouter(t)

	w := postUsersAs(t, router, authService, "user-1", `{"username":"alice","password":"wonderland"}`)

	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "Non-admin should be rejected with 403")
	assert.Equal(t, "ADMIN_REQUIRED", testutils.DecodeErrorEnvelope(t, resp).Error.Code, "Error body should carry the ADMIN_REQUIRED code")
}

// TestUserHandler_Create_MissingFields asserts missing or empty
// username/password is rejected with 400 VALIDATION_FAILED.
func TestUserHandler_Create_MissingFields(t *testing.T) {
	router, authService, _ := newUsersTestRouter(t)

	for name, body := range map[string]string{
		"missing username": `{"password":"wonderland"}`,
		"missing password": `{"username":"alice"}`,
		"empty username":   `{"username":"","password":"wonderland"}`,
		"empty password":   `{"username":"alice","password":""}`,
	} {
		t.Run(name, func(t *testing.T) {
			w := postUsersAs(t, router, authService, "admin-1", body)

			resp := w.Result()
			defer resp.Body.Close()
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Missing fields should be rejected with 400")
			assert.Equal(t, "VALIDATION_FAILED", testutils.DecodeErrorEnvelope(t, resp).Error.Code, "Error body should carry VALIDATION_FAILED")
		})
	}
}

// TestUserHandler_Create_DuplicateUsername asserts creating a username
// that already exists — here one seeded straight into the repository —
// surfaces the service's USERNAME_TAKEN as 409.
func TestUserHandler_Create_DuplicateUsername(t *testing.T) {
	router, authService, _ := newUsersTestRouter(t)

	w := postUsersAs(t, router, authService, "admin-1", `{"username":"pleb","password":"wonderland"}`)

	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusConflict, resp.StatusCode, "Duplicate username should be rejected with 409")
	assert.Equal(t, "USERNAME_TAKEN", testutils.DecodeErrorEnvelope(t, resp).Error.Code, "Error body should carry USERNAME_TAKEN")
}

// getUsersAs GETs /api/users with a bearer token minted for userID
// (empty string sends no Authorization header).
func getUsersAs(t *testing.T, router http.Handler, authService *auth.AuthService, userID string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	if userID != "" {
		token, err := auth.CreateValidTestToken(authService, userID)
		require.NoError(t, err, "Should mint a test token")
		req.Header.Set("Authorization", "Bearer "+token)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// decodeUsersList decodes the response body as a bare JSON array of
// user objects — the shape the envelope writes for list endpoints
// (machines List is the precedent).
func decodeUsersList(t *testing.T, w *httptest.ResponseRecorder) []map[string]any {
	t.Helper()

	var list []map[string]any
	err := json.NewDecoder(w.Body).Decode(&list)
	require.NoError(t, err, "Should decode the users list as a bare JSON array")
	return list
}

// TestUserHandler_List_AdminGetsFullFields asserts an authenticated
// admin GETs 200 with every user carrying exactly id, username, role,
// createdAt — never the password hash.
func TestUserHandler_List_AdminGetsFullFields(t *testing.T) {
	router, authService, _ := newUsersTestRouter(t)

	w := getUsersAs(t, router, authService, "admin-1")

	resp := w.Result()
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "Admin list should return 200")

	list := decodeUsersList(t, w)
	require.Len(t, list, 2, "Every user should be listed")

	for _, user := range list {
		assert.ElementsMatch(t,
			[]string{"id", "username", "role", "createdAt"},
			mapKeys(user),
			"Admin list entries must expose exactly the documented fields — never the password hash")
	}
}

// TestUserHandler_List_UserGetsMinimalFields asserts an authenticated
// regular user GETs 200 with every user carrying exactly id and
// username — role and createdAt are absent from the shape entirely.
func TestUserHandler_List_UserGetsMinimalFields(t *testing.T) {
	router, authService, _ := newUsersTestRouter(t)

	w := getUsersAs(t, router, authService, "user-1")

	resp := w.Result()
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "Regular-user list should return 200")

	list := decodeUsersList(t, w)
	require.Len(t, list, 2, "Every user should be listed — the admin included")

	for _, user := range list {
		assert.ElementsMatch(t,
			[]string{"id", "username"},
			mapKeys(user),
			"Regular callers must receive the minimal shape only")
	}
}

// TestUserHandler_List_Unauthenticated asserts a caller without a token
// is rejected by the JWT middleware with 401 before the handler runs.
func TestUserHandler_List_Unauthenticated(t *testing.T) {
	router, authService, _ := newUsersTestRouter(t)

	w := getUsersAs(t, router, authService, "")

	assert.Equal(t, http.StatusUnauthorized, w.Result().StatusCode, "Missing token should be rejected with 401")
}

// mapKeys returns the keys of a JSON object decoded into map[string]any.
func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
