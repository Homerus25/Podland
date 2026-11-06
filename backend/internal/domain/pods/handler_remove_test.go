package pods

import (
	"net/http"
	"testing"
	"time"

	appmiddleware "github.com/Homerus25/podland/backend/internal/middleware"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// TestPodHandler_BulkRemove_Success pins the bulk-remove contract: the
// body's ids and force flag reach the engine, and the answer is 200 with
// one success entry per requested id, in request order.
func TestPodHandler_BulkRemove_Success(t *testing.T) {
	var gotIDs []string
	var gotForce bool
	engine := &fakePodEngine{
		removeFn: func(nameOrID string, force bool) error {
			gotIDs = append(gotIDs, nameOrID)
			gotForce = force
			return nil
		},
	}

	w := testutils.BulkRemoveRequest(newTestHandler(engine).BulkRemove, "pods", `{"ids":["pod-a","pod-b"],"force":true}`)

	assert.Equal(t, http.StatusOK, w.Code, "bulk remove answers 200 even when entries fail")
	assert.Equal(t, []string{"pod-a", "pod-b"}, gotIDs, "every requested id must reach the engine")
	assert.True(t, gotForce, "the body's force flag must reach the engine")
	got := testutils.DecodeBulkRemoveReport(t, w)
	require.Len(t, got, 2, "one entry per requested id")
	assert.Equal(t, "pod-a", got[0].ID)
	assert.True(t, got[0].Success)
	assert.Nil(t, got[0].Error)
	assert.Equal(t, "pod-b", got[1].ID)
	assert.True(t, got[1].Success)
}

// TestPodHandler_BulkRemove_StateConflictKeepsBatchGoing pins the refusal
// entry: a pod whose state refuses the removal without force answers a
// POD_STATE_CONFLICT entry carrying podman's text, and the other ids in
// the same request still process.
func TestPodHandler_BulkRemove_StateConflictKeepsBatchGoing(t *testing.T) {
	const refusal = "pod web has running containers, cannot be removed without force"
	engine := &fakePodEngine{
		removeFn: func(nameOrID string, force bool) error {
			if nameOrID == "web" && !force {
				return &errorhandling.ErrorModel{
					ResponseCode: http.StatusConflict,
					Message:      refusal,
				}
			}
			return nil
		},
	}

	w := testutils.BulkRemoveRequest(newTestHandler(engine).BulkRemove, "pods", `{"ids":["free-a","web","free-b"]}`)

	assert.Equal(t, http.StatusOK, w.Code, "a refused entry must not change the status code")
	got := testutils.DecodeBulkRemoveReport(t, w)
	require.Len(t, got, 3)
	assert.True(t, got[0].Success, "the pod before the refusal must still be removed")
	require.NotNil(t, got[1].Error, "the conflicting pod must answer a refused entry")
	assert.False(t, got[1].Success)
	assert.Equal(t, "POD_STATE_CONFLICT", got[1].Error.Code)
	assert.Contains(t, got[1].Error.Message, refusal, "podman's refusal text must reach the client")
	assert.True(t, got[2].Success, "the pod after the refusal must still be removed")
}

// TestPodHandler_BulkRemove_ForceRemovesRunning pins that force=true in
// the body removes the pods the unforced pass refused.
func TestPodHandler_BulkRemove_ForceRemovesRunning(t *testing.T) {
	var gotForce bool
	engine := &fakePodEngine{
		removeFn: func(nameOrID string, force bool) error {
			gotForce = force
			if !force {
				return &errorhandling.ErrorModel{ResponseCode: http.StatusConflict, Message: "pod has running containers"}
			}
			return nil
		},
	}

	w := testutils.BulkRemoveRequest(newTestHandler(engine).BulkRemove, "pods", `{"ids":["web"],"force":true}`)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, gotForce, "force=true must reach the engine")
	got := testutils.DecodeBulkRemoveReport(t, w)
	require.Len(t, got, 1)
	assert.True(t, got[0].Success, "a forced removal of a running pod succeeds")
	assert.Nil(t, got[0].Error)
}

// TestPodHandler_BulkRemove_MissingPodIsNotFoundEntry pins the vanished
// pod branch: a missing id answers a POD_NOT_FOUND entry and the rest of
// the batch still processes.
func TestPodHandler_BulkRemove_MissingPodIsNotFoundEntry(t *testing.T) {
	engine := &fakePodEngine{
		removeFn: func(nameOrID string, force bool) error {
			if nameOrID == "ghost" {
				return &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such pod"}
			}
			return nil
		},
	}

	w := testutils.BulkRemoveRequest(newTestHandler(engine).BulkRemove, "pods", `{"ids":["ghost","real"]}`)

	assert.Equal(t, http.StatusOK, w.Code)
	got := testutils.DecodeBulkRemoveReport(t, w)
	require.Len(t, got, 2)
	require.NotNil(t, got[0].Error)
	assert.False(t, got[0].Success)
	assert.Equal(t, "POD_NOT_FOUND", got[0].Error.Code)
	assert.True(t, got[1].Success, "a missing id must not abort the batch")
}

// --- Route wiring: the pods subtree gates bulk remove at write level ---

func bulkRemoveAccessRouter(hostRepo *testutils.MockHostRepository, accessRepo *testutils.MockHostAccessRepository, handler *PodHandler) http.Handler {
	hostAccess := appmiddleware.RequireHostAccess(hostRepo, accessRepo)
	requireWrite := appmiddleware.RequireHostPermission(hostRepo, accessRepo, models.PermissionWrite)
	r := http.NewServeMux()
	// The literal /remove route must beat the /{podId} wildcard or
	// remove calls get swallowed as a pod named "remove".
	r.Handle("POST /api/hosts/{id}/pods/remove", hostAccess(requireWrite(http.HandlerFunc(handler.BulkRemove))))
	r.Handle("GET /api/hosts/{id}/pods/{podId}", hostAccess(http.HandlerFunc(handler.Get)))
	return r
}

// TestPodHandler_BulkRemove_WriteGate pins the write gate through the real
// middleware: a read-level grantee is denied with 403 before the engine is
// ever reached, while write- and admin-level grantees pass.
func TestPodHandler_BulkRemove_WriteGate(t *testing.T) {
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
			engine := &fakePodEngine{
				removeFn: func(string, bool) error {
					called = true
					return nil
				},
			}
			router := bulkRemoveAccessRouter(hostRepo, accessRepo, newTestHandler(engine))

			w := testutils.BulkRemoveRequestAsUser(router, "pods", "grantee-1", `{"ids":["web"]}`)

			assert.Equal(t, tt.expectStatus, w.Result().StatusCode, "level %s must answer %d", tt.level, tt.expectStatus)
			assert.Equal(t, tt.expectCalled, called, "the engine must run=%v for level %s — the gate fires before any podman call", tt.expectCalled, tt.level)
			if tt.expectStatus == http.StatusForbidden {
				assert.Contains(t, w.Body.String(), "ACCESS_DENIED", "the deny must carry the ACCESS_DENIED code")
			}
		})
	}
}

// TestPodHandler_BulkRemove_LiteralRouteWins pins that the literal /remove
// route beats the /{podId} subtree: with both mounted the way app.go
// mounts them, POST /pods/remove reaches the bulk handler (a swallowed
// route would answer 404/405, not a report).
func TestPodHandler_BulkRemove_LiteralRouteWins(t *testing.T) {
	hostRepo := testutils.NewMockHostRepository()
	hostRepo.AddHost(&models.Host{ID: "host-1", Name: "host-one", OwnerID: "owner-1"})
	accessRepo := testutils.NewMockHostAccessRepository()
	accessRepo.AddAccess("host-1", "grantee-1", models.PermissionWrite, time.Now())
	engine := &fakePodEngine{
		removeFn: func(string, bool) error { return nil },
	}
	router := bulkRemoveAccessRouter(hostRepo, accessRepo, newTestHandler(engine))

	w := testutils.BulkRemoveRequestAsUser(router, "pods", "grantee-1", `{"ids":["web"]}`)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode, "the literal /remove route must win over the {podId} subtree")
	got := testutils.DecodeBulkRemoveReport(t, w)
	require.Len(t, got, 1)
	assert.Equal(t, "web", got[0].ID)
}
