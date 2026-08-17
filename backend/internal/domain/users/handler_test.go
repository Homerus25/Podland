package users

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourorg/podland/backend/internal/domain/auth"
	appmiddleware "github.com/yourorg/podland/backend/internal/middleware"
	"github.com/yourorg/podland/backend/internal/models"
	"github.com/yourorg/podland/backend/internal/testutils"
)

// newUsersTestRouter wires POST /api/users exactly like internal/app:
// JWT middleware, then the admin gate, then the handler — against a
// mock repository holding one admin and one regular user. It returns
// the router plus the auth service so tests can mint bearer tokens.
func newUsersTestRouter(t *testing.T) (http.Handler, *auth.AuthService) {
	t.Helper()

	authService, err := auth.NewTestAuthService()
	require.NoError(t, err, "Should create auth service without error")

	repo := testutils.NewMockUserRepository()
	repo.AddUser(&models.User{ID: "admin-1", Username: "root", Role: models.RoleAdmin, CreatedAt: time.Now()})
	repo.AddUser(&models.User{ID: "user-1", Username: "pleb", Role: models.RoleUser, CreatedAt: time.Now()})

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Group(func(protected chi.Router) {
			protected.Use(appmiddleware.JWTMiddleware(authService))
			protected.With(appmiddleware.RequireAdmin(repo)).Post("/users", NewUserHandler(NewUserService(repo)).Create)
		})
	})

	return router, authService
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
	router, authService := newUsersTestRouter(t)

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

// TestUserHandler_Create_Unauthenticated asserts a caller without a
// token is rejected by the JWT middleware with 401 before the admin
// gate or handler ever run.
func TestUserHandler_Create_Unauthenticated(t *testing.T) {
	router, authService := newUsersTestRouter(t)

	w := postUsersAs(t, router, authService, "", `{"username":"alice","password":"wonderland"}`)

	assert.Equal(t, http.StatusUnauthorized, w.Result().StatusCode, "Missing token should be rejected with 401")
}

// TestUserHandler_Create_NonAdminForbidden asserts an authenticated
// caller with the regular user role gets 403 ADMIN_REQUIRED.
func TestUserHandler_Create_NonAdminForbidden(t *testing.T) {
	router, authService := newUsersTestRouter(t)

	w := postUsersAs(t, router, authService, "user-1", `{"username":"alice","password":"wonderland"}`)

	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "Non-admin should be rejected with 403")
	assert.Contains(t, w.Body.String(), "ADMIN_REQUIRED", "Error body should carry the ADMIN_REQUIRED code")
}

// TestUserHandler_Create_MissingFields asserts missing or empty
// username/password is rejected with 400 VALIDATION_FAILED.
func TestUserHandler_Create_MissingFields(t *testing.T) {
	router, authService := newUsersTestRouter(t)

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
			assert.Contains(t, w.Body.String(), "VALIDATION_FAILED", "Error body should carry VALIDATION_FAILED")
		})
	}
}

// TestUserHandler_Create_MalformedBody asserts a body that is not valid
// JSON is rejected with 400 INVALID_REQUEST.
func TestUserHandler_Create_MalformedBody(t *testing.T) {
	router, authService := newUsersTestRouter(t)

	w := postUsersAs(t, router, authService, "admin-1", `{"username":`)

	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Malformed body should be rejected with 400")
	assert.Contains(t, w.Body.String(), "INVALID_REQUEST", "Error body should carry INVALID_REQUEST")
}

// TestUserHandler_Create_DuplicateUsername asserts creating a username
// that already exists — here one seeded straight into the repository —
// surfaces the service's USERNAME_TAKEN as 409.
func TestUserHandler_Create_DuplicateUsername(t *testing.T) {
	router, authService := newUsersTestRouter(t)

	w := postUsersAs(t, router, authService, "admin-1", `{"username":"pleb","password":"wonderland"}`)

	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusConflict, resp.StatusCode, "Duplicate username should be rejected with 409")
	assert.Contains(t, w.Body.String(), "USERNAME_TAKEN", "Error body should carry USERNAME_TAKEN")
}

// mapKeys returns the keys of a JSON object decoded into map[string]any.
func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
