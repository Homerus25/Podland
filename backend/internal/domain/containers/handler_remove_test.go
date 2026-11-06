package containers

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

// TestContainerHandler_BulkRemove_Success pins the bulk-remove contract: the
// body's ids and force flag reach the engine, and the answer is 200 with
// one success entry per requested id, in request order.
func TestContainerHandler_BulkRemove_Success(t *testing.T) {
	var gotIDs []string
	var gotForce bool
	engine := &fakeContainerEngine{
		removeFn: func(id string, force bool) error {
			gotIDs = append(gotIDs, id)
			gotForce = force
			return nil
		},
	}

	w := testutils.BulkRemoveRequest(newTestHandler(engine).BulkRemove, "containers", `{"ids":["ctr-a","ctr-b"],"force":true}`)

	assert.Equal(t, http.StatusOK, w.Code, "bulk remove answers 200 even when entries fail")
	assert.Equal(t, []string{"ctr-a", "ctr-b"}, gotIDs, "every requested id must reach the engine")
	assert.True(t, gotForce, "the body's force flag must reach the engine")
	got := testutils.DecodeBulkRemoveReport(t, w)
	require.Len(t, got, 2, "one entry per requested id")
	assert.Equal(t, "ctr-a", got[0].ID)
	assert.True(t, got[0].Success)
	assert.Nil(t, got[0].Error)
	assert.Equal(t, "ctr-b", got[1].ID)
	assert.True(t, got[1].Success)
}

// TestContainerHandler_BulkRemove_RunningRefusalKeepsBatchGoing pins the
// refusal entry: a running container without force answers a
// CONTAINER_RUNNING entry carrying podman's text, and the other ids in the
// same request still process. The error shape is podman's real one — the
// compat handler answers a 500 whose message is the running-removal
// refusal, not a 409 — so this exercises the conflict phrase-match end to
// end.
func TestContainerHandler_BulkRemove_RunningRefusalKeepsBatchGoing(t *testing.T) {
	const refusal = "cannot remove container 3f2e1d0c as it is running - running or paused containers cannot be removed without force: container state improper"
	engine := &fakeContainerEngine{
		removeFn: func(id string, force bool) error {
			if id == "running-ctr" && !force {
				return &errorhandling.ErrorModel{
					ResponseCode: http.StatusInternalServerError,
					Message:      refusal,
					Because:      "container state improper",
				}
			}
			return nil
		},
	}

	w := testutils.BulkRemoveRequest(newTestHandler(engine).BulkRemove, "containers", `{"ids":["stopped-a","running-ctr","stopped-b"]}`)

	assert.Equal(t, http.StatusOK, w.Code, "a refused entry must not change the status code")
	got := testutils.DecodeBulkRemoveReport(t, w)
	require.Len(t, got, 3)
	assert.True(t, got[0].Success, "the item before the refusal must still be removed")
	require.NotNil(t, got[1].Error, "the running container must answer a refused entry")
	assert.False(t, got[1].Success)
	assert.Equal(t, "CONTAINER_RUNNING", got[1].Error.Code)
	assert.Contains(t, got[1].Error.Message, refusal, "podman's refusal text must reach the client")
	assert.True(t, got[2].Success, "the item after the refusal must still be removed")
}

// TestContainerHandler_BulkRemove_ForceRemovesRunning pins that force=true in
// the body removes the running containers the unforced pass refused.
func TestContainerHandler_BulkRemove_ForceRemovesRunning(t *testing.T) {
	var gotForce bool
	engine := &fakeContainerEngine{
		removeFn: func(id string, force bool) error {
			gotForce = force
			if !force {
				return &errorhandling.ErrorModel{ResponseCode: http.StatusConflict, Message: "container is running"}
			}
			return nil
		},
	}

	w := testutils.BulkRemoveRequest(newTestHandler(engine).BulkRemove, "containers", `{"ids":["running-ctr"],"force":true}`)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, gotForce, "force=true must reach the engine")
	got := testutils.DecodeBulkRemoveReport(t, w)
	require.Len(t, got, 1)
	assert.True(t, got[0].Success, "a forced removal of a running container succeeds")
	assert.Nil(t, got[0].Error)
}

// TestContainerHandler_BulkRemove_MissingIDIsNotFoundEntry pins the vanished
// container branch: a missing id answers a CONTAINER_NOT_FOUND entry and
// the rest of the batch still processes.
func TestContainerHandler_BulkRemove_MissingIDIsNotFoundEntry(t *testing.T) {
	engine := &fakeContainerEngine{
		removeFn: func(id string, force bool) error {
			if id == "ghost-ctr" {
				return &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such container"}
			}
			return nil
		},
	}

	w := testutils.BulkRemoveRequest(newTestHandler(engine).BulkRemove, "containers", `{"ids":["ghost-ctr","real-ctr"]}`)

	assert.Equal(t, http.StatusOK, w.Code)
	got := testutils.DecodeBulkRemoveReport(t, w)
	require.Len(t, got, 2)
	require.NotNil(t, got[0].Error)
	assert.False(t, got[0].Success)
	assert.Equal(t, "CONTAINER_NOT_FOUND", got[0].Error.Code)
	assert.True(t, got[1].Success, "a missing id must not abort the batch")
}

// --- Route wiring: the containers subtree gates bulk remove at write level ---

func bulkRemoveAccessRouter(hostRepo *testutils.MockHostRepository, accessRepo *testutils.MockHostAccessRepository, handler *ContainerHandler) http.Handler {
	hostAccess := appmiddleware.RequireHostAccess(hostRepo, accessRepo)
	requireWrite := appmiddleware.RequireHostPermission(hostRepo, accessRepo, models.PermissionWrite)
	r := http.NewServeMux()
	r.Handle("POST /api/hosts/{id}/containers/remove", hostAccess(requireWrite(http.HandlerFunc(handler.BulkRemove))))
	return r
}

// TestContainerHandler_BulkRemove_WriteGate pins the write gate through the
// real middleware: a read-level grantee is denied with 403 before the
// engine is ever reached, while write- and admin-level grantees pass.
func TestContainerHandler_BulkRemove_WriteGate(t *testing.T) {
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
				removeFn: func(string, bool) error {
					called = true
					return nil
				},
			}
			router := bulkRemoveAccessRouter(hostRepo, accessRepo, newTestHandler(engine))

			w := testutils.BulkRemoveRequestAsUser(router, "containers", "grantee-1", `{"ids":["ctr-a"]}`)

			assert.Equal(t, tt.expectStatus, w.Result().StatusCode, "level %s must answer %d", tt.level, tt.expectStatus)
			assert.Equal(t, tt.expectCalled, called, "the engine must run=%v for level %s — the gate fires before any podman call", tt.expectCalled, tt.level)
			if tt.expectStatus == http.StatusForbidden {
				assert.Contains(t, w.Body.String(), "ACCESS_DENIED", "the deny must carry the ACCESS_DENIED code")
			}
		})
	}
}
