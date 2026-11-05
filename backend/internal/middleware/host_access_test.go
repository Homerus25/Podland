package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hostAccessRouter mounts RequireHostAccess on a ServeMux subtree
// the same way app.go does (the middleware wrapping the sub-mux), so
// the tests exercise the real URL-param extraction, not a hand-built
// context. The leaf handler records the user ID it observed, proving
// both pass-through and that the handler never runs on rejection.
func hostAccessRouter(hostRepo *testutils.MockHostRepository, accessRepo *testutils.MockHostAccessRepository, observed *string) http.Handler {
	r := http.NewServeMux()
	r.Handle("GET /api/hosts/{id}/containers",
		RequireHostAccess(hostRepo, accessRepo)(observedUserIDHandler(observed)))
	return r
}

// seedHost adds a host owned by ownerID to the mock repository.
func seedHost(t *testing.T, repo *testutils.MockHostRepository, id, ownerID string) {
	t.Helper()
	repo.AddHost(&models.Host{
		ID:      id,
		Name:    "host-" + id,
		OwnerID: ownerID,
	})
}

// TestRequireHostAccess_UserWithAccessPassesThrough asserts a caller
// holding a host_access row (here the owner's admin grant, as
// HostService.Create seeds at registration) reaches the downstream
// handler with its user ID intact.
func TestRequireHostAccess_UserWithAccessPassesThrough(t *testing.T) {
	hostRepo := testutils.NewMockHostRepository()
	accessRepo := testutils.NewMockHostAccessRepository()
	seedHost(t, hostRepo, "host-1", "owner-1")
	accessRepo.AddAccess("host-1", "owner-1", models.PermissionAdmin, time.Now())

	var observed string
	router := hostAccessRouter(hostRepo, accessRepo, &observed)

	req := requestAsUser(http.MethodGet, "/api/hosts/host-1/containers", "owner-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode, "Caller with access should reach the handler")
	assert.Equal(t, "owner-1", observed, "User ID should survive into the downstream handler")
}

// TestRequireHostAccess_UserWithoutAccessForbidden asserts an
// authenticated caller with no host_access row is rejected with 403
// ACCESS_DENIED and the downstream handler never runs.
func TestRequireHostAccess_UserWithoutAccessForbidden(t *testing.T) {
	hostRepo := testutils.NewMockHostRepository()
	accessRepo := testutils.NewMockHostAccessRepository()
	seedHost(t, hostRepo, "host-1", "owner-1")

	var observed string
	router := hostAccessRouter(hostRepo, accessRepo, &observed)

	req := requestAsUser(http.MethodGet, "/api/hosts/host-1/containers", "stranger-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Result().StatusCode, "Caller without access should be rejected with 403")
	assert.Empty(t, observed, "Handler must not run for a caller without access")
	assert.Contains(t, w.Body.String(), "ACCESS_DENIED", "Error body should carry the ACCESS_DENIED code")
}

// TestRequireHostAccess_UnknownHostNotFound asserts an unknown
// host ID is rejected with 404 HOST_NOT_FOUND — the same answer
// the hosts domain gives — rather than leaking a 500 from the
// downstream services.
func TestRequireHostAccess_UnknownHostNotFound(t *testing.T) {
	hostRepo := testutils.NewMockHostRepository()
	accessRepo := testutils.NewMockHostAccessRepository()

	var observed string
	router := hostAccessRouter(hostRepo, accessRepo, &observed)

	req := requestAsUser(http.MethodGet, "/api/hosts/no-such-host/containers", "owner-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Result().StatusCode, "Unknown host should be rejected with 404")
	assert.Empty(t, observed, "Handler must not run for an unknown host")
	assert.Contains(t, w.Body.String(), "HOST_NOT_FOUND", "Error body should carry the HOST_NOT_FOUND code")
}

// TestRequireHostAccess_MissingUserIDUnauthorized asserts a request
// that never went through JWTMiddleware surfaces as 401 — an
// unauthenticated caller — rather than 403, so a miswired route cannot
// mask the auth contract (mirrors RequireAdmin).
func TestRequireHostAccess_MissingUserIDUnauthorized(t *testing.T) {
	hostRepo := testutils.NewMockHostRepository()
	accessRepo := testutils.NewMockHostAccessRepository()
	seedHost(t, hostRepo, "host-1", "owner-1")

	var observed string
	router := hostAccessRouter(hostRepo, accessRepo, &observed)

	req := httptest.NewRequest(http.MethodGet, "/api/hosts/host-1/containers", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Result().StatusCode, "Request without a user ID is unauthenticated: 401")
	assert.Empty(t, observed, "Handler must not run without a user ID")
}

// TestRequireHostAccess_HostRepoErrorIs500 asserts a failing
// host lookup surfaces as 500 INTERNAL_ERROR, not a silent
// allow/deny guess.
func TestRequireHostAccess_HostRepoErrorIs500(t *testing.T) {
	hostRepo := testutils.NewMockHostRepositoryWithErrors(nil, assert.AnError, nil, nil, nil)
	accessRepo := testutils.NewMockHostAccessRepository()

	var observed string
	router := hostAccessRouter(hostRepo, accessRepo, &observed)

	req := requestAsUser(http.MethodGet, "/api/hosts/host-1/containers", "owner-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Result().StatusCode, "Host lookup failure should surface as 500")
	require.Empty(t, observed, "Handler must not run when the host lookup fails")
}

// TestRequireHostAccess_AccessRepoErrorIs500 asserts a failing
// access probe surfaces as 500 INTERNAL_ERROR rather than defaulting
// either way.
func TestRequireHostAccess_AccessRepoErrorIs500(t *testing.T) {
	hostRepo := testutils.NewMockHostRepository()
	accessRepo := testutils.NewMockHostAccessRepositoryWithErrors(nil, nil, nil, nil, nil, nil, assert.AnError)
	seedHost(t, hostRepo, "host-1", "owner-1")

	var observed string
	router := hostAccessRouter(hostRepo, accessRepo, &observed)

	req := requestAsUser(http.MethodGet, "/api/hosts/host-1/containers", "owner-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Result().StatusCode, "Access probe failure should surface as 500")
	require.Empty(t, observed, "Handler must not run when the access probe fails")
}

// ---------------------------------------------------------------------------
// RequireHostPermission
// ---------------------------------------------------------------------------

// hostPermissionRouter mounts RequireHostPermission on a ServeMux
// subtree the same way app.go wires a mutating route (RequireHost-
// Access at the subtree level, RequireHostPermission(write) on the
// route), so the tests exercise the real URL-param extraction.
func hostPermissionRouter(hostRepo *testutils.MockHostRepository, accessRepo *testutils.MockHostAccessRepository, observed *string) http.Handler {
	r := http.NewServeMux()
	r.Handle("POST /api/hosts/{id}/containers/no-such-container/start",
		RequireHostAccess(hostRepo, accessRepo)(
			RequireHostPermission(hostRepo, accessRepo, models.PermissionWrite)(
				observedUserIDHandler(observed))))
	return r
}

// permissionTestCase seeds one host plus one user at the given
// permission level and returns a router gating a write route, ready to
// be exercised as that user.
func permissionTestCase(t *testing.T, userID, permission string, observed *string) http.Handler {
	t.Helper()
	hostRepo := testutils.NewMockHostRepository()
	accessRepo := testutils.NewMockHostAccessRepository()
	seedHost(t, hostRepo, "host-1", "owner-1")
	if permission != "" {
		accessRepo.AddAccess("host-1", userID, permission, time.Now())
	}
	return hostPermissionRouter(hostRepo, accessRepo, observed)
}

// TestRequireHostPermission_ReadUserForbiddenOnWriteRoute pins the
// gate's one 403 envelope shape (the level-comparison table itself is
// owned by the permission package's suite): a below-write caller gets
// 403 ACCESS_DENIED with the handler never running.
func TestRequireHostPermission_ReadUserForbiddenOnWriteRoute(t *testing.T) {
	var observed string
	router := permissionTestCase(t, "reader-1", models.PermissionRead, &observed)

	req := requestAsUser(http.MethodPost, "/api/hosts/host-1/containers/no-such-container/start", "reader-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Result().StatusCode, "read-level caller must be rejected with 403 on a write route")
	assert.Empty(t, observed, "Handler must not run for a read-level caller")
	assert.Contains(t, w.Body.String(), "ACCESS_DENIED", "Error body should carry the ACCESS_DENIED code")
}

// TestRequireHostPermission_UnknownHostNotFound asserts an
// unknown host ID is rejected with 404 HOST_NOT_FOUND — the same
// answer RequireHostAccess and the hosts domain give.
func TestRequireHostPermission_UnknownHostNotFound(t *testing.T) {
	hostRepo := testutils.NewMockHostRepository()
	accessRepo := testutils.NewMockHostAccessRepository()

	var observed string
	router := hostPermissionRouter(hostRepo, accessRepo, &observed)

	req := requestAsUser(http.MethodPost, "/api/hosts/no-such-host/containers/no-such-container/start", "writer-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Result().StatusCode, "Unknown host should be rejected with 404")
	assert.Empty(t, observed, "Handler must not run for an unknown host")
	assert.Contains(t, w.Body.String(), "HOST_NOT_FOUND", "Error body should carry the HOST_NOT_FOUND code")
}

// TestRequireHostPermission_PermissionFetchErrorIs500 asserts a
// failing permission lookup surfaces as 500 INTERNAL_ERROR rather than
// a silent allow/deny guess.
func TestRequireHostPermission_PermissionFetchErrorIs500(t *testing.T) {
	hostRepo := testutils.NewMockHostRepository()
	accessRepo := testutils.NewMockHostAccessRepository().WithGetPermissionError(assert.AnError)
	seedHost(t, hostRepo, "host-1", "owner-1")
	// The subtree-level RequireHostAccess must pass so the request
	// reaches the permission gate whose lookup then fails.
	accessRepo.AddAccess("host-1", "owner-1", models.PermissionAdmin, time.Now())

	var observed string
	router := hostPermissionRouter(hostRepo, accessRepo, &observed)

	req := requestAsUser(http.MethodPost, "/api/hosts/host-1/containers/no-such-container/start", "owner-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Result().StatusCode, "Permission lookup failure should surface as 500")
	require.Empty(t, observed, "Handler must not run when the permission lookup fails")
}
