package containers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	appmiddleware "github.com/Homerus25/podland/backend/internal/middleware"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities/reports"
)

// pruneRequest serves POST /api/hosts/{id}/containers/prune through
// the handler with an authenticated test identity.
func pruneRequest(prune http.HandlerFunc) *httptest.ResponseRecorder {
	return testutils.Perform(prune, testutils.Request{
		Method:    http.MethodPost,
		Path:      "/api/hosts/test-host/containers/prune",
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-host"},
	})
}

// TestContainerHandler_Prune_Success pins the prune contract: 200 with one
// report entry per removed container, ID and reclaimed size mapped, and
// per-container errors carried in the entry.
func TestContainerHandler_Prune_Success(t *testing.T) {
	prunes := []*reports.PruneReport{
		{Id: "stopped-a", Size: 4096},
		{Id: "stopped-b", Size: 0, Err: fmt.Errorf("container is running")},
	}
	engine := &fakeContainerEngine{
		pruneFn: func() ([]*reports.PruneReport, error) {
			return prunes, nil
		},
	}

	w := pruneRequest(newTestHandler(engine).Prune)

	assert.Equal(t, 200, w.Code, "prune should return 200")
	var got []ContainerPruneResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got), "response must decode as a report array")
	require.Len(t, got, 2, "each removed container must have a report entry")
	assert.Equal(t, "stopped-a", got[0].ID)
	assert.Equal(t, uint64(4096), got[0].Size)
	assert.Empty(t, got[0].Error)
	assert.Equal(t, "stopped-b", got[1].ID)
	assert.Equal(t, "container is running", got[1].Error, "per-container error must be carried")
}

// TestContainerHandler_Prune_EmptyReport pins that pruning with nothing to
// remove still returns 200 with an empty JSON array (not nil/null).
func TestContainerHandler_Prune_EmptyReport(t *testing.T) {
	engine := &fakeContainerEngine{
		pruneFn: func() ([]*reports.PruneReport, error) {
			return []*reports.PruneReport{}, nil
		},
	}

	w := pruneRequest(newTestHandler(engine).Prune)

	assert.Equal(t, 200, w.Code)
	var got []ContainerPruneResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.NotNil(t, got, "response must decode as an empty array, not null")
	assert.Empty(t, got)
}

// TestFromPodmanContainerPruneReports_NilEntries pins that nil reports are
// skipped rather than marshalled as empty entries.
func TestFromPodmanContainerPruneReports_NilEntries(t *testing.T) {
	got := FromPodmanContainerPruneReports([]*reports.PruneReport{nil, {Id: "x", Size: 1}})
	require.Len(t, got, 1)
	assert.Equal(t, "x", got[0].ID)
}

// --- Route wiring: the containers subtree gates prune at write level ---
//
// These tests mount the containers routes on a ServeMux exactly the way
// internal/app/app.go does (RequireHostAccess wrapping the sub-mux,
// RequireHostPermission(write) on the literal POST /prune, the
// literal winning over the /{containerId} wildcard by ServeMux
// precedence) so the 403 semantics and the literal-route win the ACs
// promise are exercised through the real middleware.

func pruneAccessRouter(hostRepo *testutils.MockHostRepository, accessRepo *testutils.MockHostAccessRepository, handler *ContainerHandler) http.Handler {
	hostAccess := appmiddleware.RequireHostAccess(hostRepo, accessRepo)
	requireWrite := appmiddleware.RequireHostPermission(hostRepo, accessRepo, models.PermissionWrite)
	r := http.NewServeMux()
	r.Handle("GET /api/hosts/{id}/containers", hostAccess(http.HandlerFunc(handler.List)))
	// The literal /prune route must beat the /{containerId} wildcard
	// or prune calls get swallowed as a container named "prune".
	r.Handle("POST /api/hosts/{id}/containers/prune", hostAccess(requireWrite(http.HandlerFunc(handler.Prune))))
	r.Handle("GET /api/hosts/{id}/containers/{containerId}", hostAccess(http.HandlerFunc(handler.Get)))
	return r
}

func pruneRequestAsUser(router http.Handler, userID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/hosts/host-1/containers/prune", nil)
	req = req.WithContext(contextutils.SetUserID(req.Context(), userID))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// TestContainerHandler_Prune_WriteGate pins the write gate through the
// real middleware: a read-level grantee is denied with 403 before the
// engine is ever reached, while write- and admin-level grantees pass.
func TestContainerHandler_Prune_WriteGate(t *testing.T) {
	tests := []struct {
		name         string
		level        string
		expectStatus int
		expectCalled bool
	}{
		{name: "read-level grantee is denied", level: models.PermissionRead, expectStatus: http.StatusForbidden, expectCalled: false},
		{name: "write-level grantee passes", level: models.PermissionWrite, expectStatus: http.StatusOK, expectCalled: true},
		{name: "admin-level grantee passes", level: models.PermissionAdmin, expectStatus: http.StatusOK, expectCalled: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hostRepo := testutils.NewMockHostRepository()
			hostRepo.AddHost(&models.Host{ID: "host-1", Name: "host-one", OwnerID: "owner-1"})
			accessRepo := testutils.NewMockHostAccessRepository()
			accessRepo.AddAccess("host-1", "grantee-1", tt.level, time.Now())
			called := false
			engine := &fakeContainerEngine{
				pruneFn: func() ([]*reports.PruneReport, error) {
					called = true
					return []*reports.PruneReport{{Id: "stopped-a", Size: 4096}}, nil
				},
			}
			router := pruneAccessRouter(hostRepo, accessRepo, newTestHandler(engine))

			w := pruneRequestAsUser(router, "grantee-1")

			assert.Equal(t, tt.expectStatus, w.Result().StatusCode, "level %s must answer %d", tt.level, tt.expectStatus)
			assert.Equal(t, tt.expectCalled, called, "the engine must run=%v for level %s — the gate fires before any podman call", tt.expectCalled, tt.level)
			if tt.expectStatus == http.StatusForbidden {
				assert.Contains(t, w.Body.String(), "ACCESS_DENIED", "the deny must carry the ACCESS_DENIED code")
			}
		})
	}
}

// TestContainerHandler_Prune_LiteralRouteWins pins that the literal
// /prune route beats the /{containerId} subtree: with both mounted the
// way app.go mounts them, POST /containers/prune reaches the prune
// handler (a swallowed route would answer 404/405, not a prune report).
func TestContainerHandler_Prune_LiteralRouteWins(t *testing.T) {
	hostRepo := testutils.NewMockHostRepository()
	hostRepo.AddHost(&models.Host{ID: "host-1", Name: "host-one", OwnerID: "owner-1"})
	accessRepo := testutils.NewMockHostAccessRepository()
	accessRepo.AddAccess("host-1", "grantee-1", models.PermissionWrite, time.Now())
	engine := &fakeContainerEngine{
		pruneFn: func() ([]*reports.PruneReport, error) {
			return []*reports.PruneReport{{Id: "stopped-a", Size: 4096}}, nil
		},
	}
	router := pruneAccessRouter(hostRepo, accessRepo, newTestHandler(engine))

	w := pruneRequestAsUser(router, "grantee-1")

	assert.Equal(t, http.StatusOK, w.Result().StatusCode, "the literal /prune route must win over the {containerId} subtree")
	var got []ContainerPruneResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got), "the prune handler must answer the report array")
	require.Len(t, got, 1)
	assert.Equal(t, "stopped-a", got[0].ID)
}
