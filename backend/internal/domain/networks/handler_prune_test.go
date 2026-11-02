package networks

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	appmiddleware "github.com/Homerus25/podland/backend/internal/middleware"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// pruneRequest serves POST /api/machines/{id}/networks/prune through
// the handler with an authenticated test identity.
func pruneRequest(prune http.HandlerFunc) *httptest.ResponseRecorder {
	return testutils.Perform(prune, testutils.Request{
		Method:    http.MethodPost,
		Path:      "/api/machines/test-machine/networks/prune",
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-machine"},
	})
}

// TestNetworkHandler_Prune_Success pins the prune contract: 200 with
// one report entry per removed network, name-keyed (podman's network
// prune report carries no size), per-network errors carried in the
// entry.
func TestNetworkHandler_Prune_Success(t *testing.T) {
	engine := &fakeNetworkEngine{
		pruneFn: func() ([]*entities.NetworkPruneReport, error) {
			return []*entities.NetworkPruneReport{
				{Name: "unused-a"},
				{Name: "unused-b", Error: fmt.Errorf("network is in use")},
			}, nil
		},
	}

	w := pruneRequest(newTestHandler(engine).Prune)

	assert.Equal(t, 200, w.Code, "prune should return 200")
	var got []NetworkPruneResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got), "response must decode as a report array")
	require.Len(t, got, 2, "each removed network must have a report entry")
	assert.Equal(t, "unused-a", got[0].Name)
	assert.Empty(t, got[0].Error)
	assert.Equal(t, "unused-b", got[1].Name)
	assert.Equal(t, "network is in use", got[1].Error, "per-network error must be carried")
}

// TestNetworkHandler_Prune_EmptyReport pins that pruning with nothing
// to remove still returns 200 with an empty JSON array (not nil/null).
func TestNetworkHandler_Prune_EmptyReport(t *testing.T) {
	engine := &fakeNetworkEngine{
		pruneFn: func() ([]*entities.NetworkPruneReport, error) {
			return []*entities.NetworkPruneReport{}, nil
		},
	}

	w := pruneRequest(newTestHandler(engine).Prune)

	assert.Equal(t, 200, w.Code)
	var got []NetworkPruneResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.NotNil(t, got, "response must decode as an empty array, not null")
	assert.Empty(t, got)
}

// TestFromPodmanPruneReports_NilEntries pins that nil reports are
// skipped rather than marshalled as empty entries.
func TestFromPodmanPruneReports_NilEntries(t *testing.T) {
	got := FromPodmanPruneReports([]*entities.NetworkPruneReport{nil, {Name: "x"}})
	require.Len(t, got, 1)
	assert.Equal(t, "x", got[0].Name)
}

// TestFromPodmanPruneReports_Empty pins that pruning with nothing to
// remove still yields an empty array, not nil.
func TestFromPodmanPruneReports_Empty(t *testing.T) {
	got := FromPodmanPruneReports(nil)

	assert.NotNil(t, got)
	assert.Empty(t, got)
}

// --- Route wiring: the networks subtree gates prune at write level ---
//
// These tests mount the networks routes on a chi subtree exactly the way
// internal/app/app.go does (RequireMachineAccess at /{id},
// RequireMachinePermission(write) on the literal POST /prune, the
// literal registered ahead of the {networkName} subtree) so the 403
// semantics and the literal-route win the ACs promise are exercised
// through the real middleware.

func pruneAccessRouter(machineRepo *testutils.MockMachineRepository, accessRepo *testutils.MockMachineAccessRepository, handler *NetworkHandler) http.Handler {
	machineAccess := appmiddleware.RequireMachineAccess(machineRepo, accessRepo)
	requireWrite := appmiddleware.RequireMachinePermission(machineRepo, accessRepo, models.PermissionWrite)
	r := http.NewServeMux()
	// The literal /prune route must beat the /{networkName} subtree or
	// prune calls get swallowed as a network named "prune".
	r.Handle("POST /api/machines/{id}/networks/prune", machineAccess(requireWrite(http.HandlerFunc(handler.Prune))))
	r.Handle("GET /api/machines/{id}/networks", machineAccess(http.HandlerFunc(handler.List)))
	r.Handle("GET /api/machines/{id}/networks/{networkName}", machineAccess(http.HandlerFunc(handler.Get)))
	return r
}

func pruneRequestAsUser(router http.Handler, userID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/machines/machine-1/networks/prune", nil)
	req = req.WithContext(contextutils.SetUserID(req.Context(), userID))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// TestNetworkHandler_Prune_WriteGate pins the write gate through the
// real middleware: a read-level grantee is denied with 403 before the
// engine is ever reached, while write- and admin-level grantees pass.
func TestNetworkHandler_Prune_WriteGate(t *testing.T) {
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
			machineRepo := testutils.NewMockMachineRepository()
			machineRepo.AddMachine(&models.Machine{ID: "machine-1", Name: "machine-one", OwnerID: "owner-1"})
			accessRepo := testutils.NewMockMachineAccessRepository()
			accessRepo.AddAccess("machine-1", "grantee-1", tt.level, time.Now())
			called := false
			engine := &fakeNetworkEngine{
				pruneFn: func() ([]*entities.NetworkPruneReport, error) {
					called = true
					return []*entities.NetworkPruneReport{{Name: "unused-a"}}, nil
				},
			}
			router := pruneAccessRouter(machineRepo, accessRepo, newTestHandler(engine))

			w := pruneRequestAsUser(router, "grantee-1")

			assert.Equal(t, tt.expectStatus, w.Result().StatusCode, "level %s must answer %d", tt.level, tt.expectStatus)
			assert.Equal(t, tt.expectCalled, called, "the engine must run=%v for level %s — the gate fires before any podman call", tt.expectCalled, tt.level)
			if tt.expectStatus == http.StatusForbidden {
				assert.Contains(t, w.Body.String(), "ACCESS_DENIED", "the deny must carry the ACCESS_DENIED code")
			}
		})
	}
}

// TestNetworkHandler_Prune_LiteralRouteWins pins that the literal
// /prune route beats the /{networkName} subtree: with both mounted the
// way app.go mounts them, POST /networks/prune reaches the prune
// handler (a swallowed route would answer 404/405, not a prune report).
func TestNetworkHandler_Prune_LiteralRouteWins(t *testing.T) {
	machineRepo := testutils.NewMockMachineRepository()
	machineRepo.AddMachine(&models.Machine{ID: "machine-1", Name: "machine-one", OwnerID: "owner-1"})
	accessRepo := testutils.NewMockMachineAccessRepository()
	accessRepo.AddAccess("machine-1", "grantee-1", models.PermissionWrite, time.Now())
	engine := &fakeNetworkEngine{
		pruneFn: func() ([]*entities.NetworkPruneReport, error) {
			return []*entities.NetworkPruneReport{{Name: "unused-a"}}, nil
		},
	}
	router := pruneAccessRouter(machineRepo, accessRepo, newTestHandler(engine))

	w := pruneRequestAsUser(router, "grantee-1")

	assert.Equal(t, http.StatusOK, w.Result().StatusCode, "the literal /prune route must win over the {networkName} subtree")
	var got []NetworkPruneResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got), "the prune handler must answer the report array")
	require.Len(t, got, 1)
	assert.Equal(t, "unused-a", got[0].Name)
}
