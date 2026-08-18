package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourorg/podland/backend/internal/models"
	"github.com/yourorg/podland/backend/internal/testutils"
)

// machineAccessRouter mounts RequireMachineAccess on a chi subtree the
// same way app.go does (With(...).Route("/{id}", ...)), so the tests
// exercise the real URL-param extraction, not a hand-built context.
// The leaf handler records the user ID it observed, proving both
// pass-through and that the handler never runs on rejection.
func machineAccessRouter(machineRepo *testutils.MockMachineRepository, accessRepo *testutils.MockMachineAccessRepository, observed *string) http.Handler {
	r := chi.NewRouter()
	r.Route("/api/machines", func(r chi.Router) {
		r.With(RequireMachineAccess(machineRepo, accessRepo)).Route("/{id}", func(r chi.Router) {
			r.Get("/containers", observedUserIDHandler(observed).ServeHTTP)
		})
	})
	return r
}

// seedMachine adds a machine owned by ownerID to the mock repository.
func seedMachine(t *testing.T, repo *testutils.MockMachineRepository, id, ownerID string) {
	t.Helper()
	repo.AddMachine(&models.Machine{
		ID:      id,
		Name:    "machine-" + id,
		OwnerID: ownerID,
	})
}

// TestRequireMachineAccess_UserWithAccessPassesThrough asserts a caller
// holding a machine_access row (here the owner's admin grant, as
// MachineService.Create seeds at registration) reaches the downstream
// handler with its user ID intact.
func TestRequireMachineAccess_UserWithAccessPassesThrough(t *testing.T) {
	machineRepo := testutils.NewMockMachineRepository()
	accessRepo := testutils.NewMockMachineAccessRepository()
	seedMachine(t, machineRepo, "machine-1", "owner-1")
	accessRepo.AddAccess("machine-1", "owner-1", models.PermissionAdmin, time.Now())

	var observed string
	router := machineAccessRouter(machineRepo, accessRepo, &observed)

	req := requestAsUser(http.MethodGet, "/api/machines/machine-1/containers", "owner-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode, "Caller with access should reach the handler")
	assert.Equal(t, "owner-1", observed, "User ID should survive into the downstream handler")
}

// TestRequireMachineAccess_UserWithoutAccessForbidden asserts an
// authenticated caller with no machine_access row is rejected with 403
// ACCESS_DENIED and the downstream handler never runs.
func TestRequireMachineAccess_UserWithoutAccessForbidden(t *testing.T) {
	machineRepo := testutils.NewMockMachineRepository()
	accessRepo := testutils.NewMockMachineAccessRepository()
	seedMachine(t, machineRepo, "machine-1", "owner-1")

	var observed string
	router := machineAccessRouter(machineRepo, accessRepo, &observed)

	req := requestAsUser(http.MethodGet, "/api/machines/machine-1/containers", "stranger-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Result().StatusCode, "Caller without access should be rejected with 403")
	assert.Empty(t, observed, "Handler must not run for a caller without access")
	assert.Contains(t, w.Body.String(), "ACCESS_DENIED", "Error body should carry the ACCESS_DENIED code")
}

// TestRequireMachineAccess_UnknownMachineNotFound asserts an unknown
// machine ID is rejected with 404 MACHINE_NOT_FOUND — the same answer
// the machines domain gives — rather than leaking a 500 from the
// downstream services.
func TestRequireMachineAccess_UnknownMachineNotFound(t *testing.T) {
	machineRepo := testutils.NewMockMachineRepository()
	accessRepo := testutils.NewMockMachineAccessRepository()

	var observed string
	router := machineAccessRouter(machineRepo, accessRepo, &observed)

	req := requestAsUser(http.MethodGet, "/api/machines/no-such-machine/containers", "owner-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Result().StatusCode, "Unknown machine should be rejected with 404")
	assert.Empty(t, observed, "Handler must not run for an unknown machine")
	assert.Contains(t, w.Body.String(), "MACHINE_NOT_FOUND", "Error body should carry the MACHINE_NOT_FOUND code")
}

// TestRequireMachineAccess_MissingUserIDUnauthorized asserts a request
// that never went through JWTMiddleware surfaces as 401 — an
// unauthenticated caller — rather than 403, so a miswired route cannot
// mask the auth contract (mirrors RequireAdmin).
func TestRequireMachineAccess_MissingUserIDUnauthorized(t *testing.T) {
	machineRepo := testutils.NewMockMachineRepository()
	accessRepo := testutils.NewMockMachineAccessRepository()
	seedMachine(t, machineRepo, "machine-1", "owner-1")

	var observed string
	router := machineAccessRouter(machineRepo, accessRepo, &observed)

	req := httptest.NewRequest(http.MethodGet, "/api/machines/machine-1/containers", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Result().StatusCode, "Request without a user ID is unauthenticated: 401")
	assert.Empty(t, observed, "Handler must not run without a user ID")
}

// TestRequireMachineAccess_MachineRepoErrorIs500 asserts a failing
// machine lookup surfaces as 500 INTERNAL_ERROR, not a silent
// allow/deny guess.
func TestRequireMachineAccess_MachineRepoErrorIs500(t *testing.T) {
	machineRepo := testutils.NewMockMachineRepositoryWithErrors(nil, assert.AnError, nil, nil, nil)
	accessRepo := testutils.NewMockMachineAccessRepository()

	var observed string
	router := machineAccessRouter(machineRepo, accessRepo, &observed)

	req := requestAsUser(http.MethodGet, "/api/machines/machine-1/containers", "owner-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Result().StatusCode, "Machine lookup failure should surface as 500")
	require.Empty(t, observed, "Handler must not run when the machine lookup fails")
}

// TestRequireMachineAccess_AccessRepoErrorIs500 asserts a failing
// access probe surfaces as 500 INTERNAL_ERROR rather than defaulting
// either way.
func TestRequireMachineAccess_AccessRepoErrorIs500(t *testing.T) {
	machineRepo := testutils.NewMockMachineRepository()
	accessRepo := testutils.NewMockMachineAccessRepositoryWithErrors(nil, nil, nil, nil, nil, nil, assert.AnError)
	seedMachine(t, machineRepo, "machine-1", "owner-1")

	var observed string
	router := machineAccessRouter(machineRepo, accessRepo, &observed)

	req := requestAsUser(http.MethodGet, "/api/machines/machine-1/containers", "owner-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Result().StatusCode, "Access probe failure should surface as 500")
	require.Empty(t, observed, "Handler must not run when the access probe fails")
}
