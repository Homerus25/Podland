package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourorg/podland/backend/internal/contextutils"
	"github.com/yourorg/podland/backend/internal/models"
	"github.com/yourorg/podland/backend/internal/testutils"
)

// requestAsUser builds a request whose context carries userID, the way
// JWTMiddleware does for authenticated requests downstream.
func requestAsUser(method, target, userID string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	return req.WithContext(contextutils.SetUserID(req.Context(), userID))
}

// seedUserWithRole adds a user with the given id and role to the mock
// repository so RequireAdmin's per-request role lookup finds them.
func seedUserWithRole(t *testing.T, repo *testutils.MockUserRepository, id, role string) {
	t.Helper()
	repo.AddUser(&models.User{
		ID:        id,
		Username:  "user-" + id,
		Role:      role,
		CreatedAt: time.Now(),
	})
}

// TestRequireAdmin_AdminPassesThrough asserts a caller whose persisted
// role is admin reaches the downstream handler.
func TestRequireAdmin_AdminPassesThrough(t *testing.T) {
	repo := testutils.NewMockUserRepository()
	seedUserWithRole(t, repo, "admin-1", models.RoleAdmin)

	var observed string
	handler := RequireAdmin(repo)(observedUserIDHandler(&observed))

	req := requestAsUser(http.MethodPost, "/api/users", "admin-1")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode, "Admin request should reach the handler")
	assert.Equal(t, "admin-1", observed, "User ID should survive into the downstream handler")
}

// TestRequireAdmin_RegularUserForbidden asserts an authenticated caller
// with the regular user role is rejected with 403 ADMIN_REQUIRED and the
// downstream handler never runs.
func TestRequireAdmin_RegularUserForbidden(t *testing.T) {
	repo := testutils.NewMockUserRepository()
	seedUserWithRole(t, repo, "user-1", models.RoleUser)

	var observed string
	handler := RequireAdmin(repo)(observedUserIDHandler(&observed))

	req := requestAsUser(http.MethodPost, "/api/users", "user-1")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Result().StatusCode, "Non-admin should be rejected with 403")
	assert.Empty(t, observed, "Handler must not run for a non-admin")
	assert.Contains(t, w.Body.String(), "ADMIN_REQUIRED", "Error body should carry the ADMIN_REQUIRED code")
}

// TestRequireAdmin_UnknownUserForbidden asserts a valid token whose user
// no longer exists is rejected rather than defaulted through.
func TestRequireAdmin_UnknownUserForbidden(t *testing.T) {
	repo := testutils.NewMockUserRepository()

	var observed string
	handler := RequireAdmin(repo)(observedUserIDHandler(&observed))

	req := requestAsUser(http.MethodPost, "/api/users", "deleted-user")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Result().StatusCode, "Unknown user should be rejected with 403")
	assert.Empty(t, observed, "Handler must not run for an unknown user")
}

// TestRequireAdmin_MissingUserIDUnauthorized asserts a request that
// never went through JWTMiddleware surfaces as 401 — an unauthenticated
// caller — rather than 403, so a miswired route cannot mask the auth
// contract.
func TestRequireAdmin_MissingUserIDUnauthorized(t *testing.T) {
	repo := testutils.NewMockUserRepository()

	var observed string
	handler := RequireAdmin(repo)(observedUserIDHandler(&observed))

	req := httptest.NewRequest(http.MethodPost, "/api/users", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Result().StatusCode, "Request without a user ID is unauthenticated: 401")
	assert.Empty(t, observed, "Handler must not run without a user ID")
}

// TestRequireAdmin_RepoErrorIs500 asserts a failing role lookup surfaces
// as 500 INTERNAL_ERROR, not a silent allow/deny guess.
func TestRequireAdmin_RepoErrorIs500(t *testing.T) {
	repo := testutils.NewMockUserRepositoryWithErrors(nil, assert.AnError, nil, nil, nil, nil)

	var observed string
	handler := RequireAdmin(repo)(observedUserIDHandler(&observed))

	req := requestAsUser(http.MethodPost, "/api/users", "user-1")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Result().StatusCode, "Repository failure should surface as 500")
	require.Empty(t, observed, "Handler must not run when the role lookup fails")
}
