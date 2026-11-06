package networks

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

// TestNetworkHandler_BulkRemove_Success pins the bulk-remove contract: the
// body's names and force flag reach the engine, and the answer is 200 with
// one success entry per requested name, in request order.
func TestNetworkHandler_BulkRemove_Success(t *testing.T) {
	var gotNames []string
	var gotForce bool
	engine := &fakeNetworkEngine{
		removeFn: func(name string, force bool) error {
			gotNames = append(gotNames, name)
			gotForce = force
			return nil
		},
	}

	w := testutils.BulkRemoveRequest(newTestHandler(engine).BulkRemove, "networks", `{"ids":["net-a","net-b"],"force":true}`)

	assert.Equal(t, http.StatusOK, w.Code, "bulk remove answers 200 even when entries fail")
	assert.Equal(t, []string{"net-a", "net-b"}, gotNames, "every requested name must reach the engine")
	assert.True(t, gotForce, "the body's force flag must reach the engine")
	got := testutils.DecodeBulkRemoveReport(t, w)
	require.Len(t, got, 2, "one entry per requested name")
	assert.Equal(t, "net-a", got[0].ID)
	assert.True(t, got[0].Success)
	assert.Nil(t, got[0].Error)
	assert.Equal(t, "net-b", got[1].ID)
	assert.True(t, got[1].Success)
}

// TestNetworkHandler_BulkRemove_InUseRefusalKeepsBatchGoing pins the refusal
// entry: an in-use network without force answers a NETWORK_IN_USE entry
// carrying podman's text, and the other names in the same request still
// process. The error shape is podman's real one — the compat handler
// answers a 500 whose message ends in the ErrNetworkInUse text, not a 409 —
// so this exercises the conflict phrase-match end to end.
func TestNetworkHandler_BulkRemove_InUseRefusalKeepsBatchGoing(t *testing.T) {
	const refusal = `"busy-net" has associated containers with it. Use -f to forcibly delete containers and pods: network is being used`
	engine := &fakeNetworkEngine{
		removeFn: func(name string, force bool) error {
			if name == "busy-net" && !force {
				return &errorhandling.ErrorModel{
					ResponseCode: http.StatusInternalServerError,
					Message:      refusal,
				}
			}
			return nil
		},
	}

	w := testutils.BulkRemoveRequest(newTestHandler(engine).BulkRemove, "networks", `{"ids":["free-a","busy-net","free-b"]}`)

	assert.Equal(t, http.StatusOK, w.Code, "a refused entry must not change the status code")
	got := testutils.DecodeBulkRemoveReport(t, w)
	require.Len(t, got, 3)
	assert.True(t, got[0].Success, "the item before the refusal must still be removed")
	require.NotNil(t, got[1].Error, "the in-use network must answer a refused entry")
	assert.False(t, got[1].Success)
	assert.Equal(t, "NETWORK_IN_USE", got[1].Error.Code)
	assert.Contains(t, got[1].Error.Message, refusal, "podman's refusal text must reach the client")
	assert.True(t, got[2].Success, "the item after the refusal must still be removed")
}

// TestNetworkHandler_BulkRemove_ForceRemovesInUse pins that force=true in the
// body removes the in-use networks the unforced pass refused.
func TestNetworkHandler_BulkRemove_ForceRemovesInUse(t *testing.T) {
	var gotForce bool
	engine := &fakeNetworkEngine{
		removeFn: func(name string, force bool) error {
			gotForce = force
			if !force {
				return &errorhandling.ErrorModel{ResponseCode: http.StatusConflict, Message: "network is in use"}
			}
			return nil
		},
	}

	w := testutils.BulkRemoveRequest(newTestHandler(engine).BulkRemove, "networks", `{"ids":["busy-net"],"force":true}`)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, gotForce, "force=true must reach the engine")
	got := testutils.DecodeBulkRemoveReport(t, w)
	require.Len(t, got, 1)
	assert.True(t, got[0].Success, "a forced removal of an in-use network succeeds")
	assert.Nil(t, got[0].Error)
}

// TestNetworkHandler_BulkRemove_MissingNameIsNotFoundEntry pins the vanished
// network branch: a missing name answers a NETWORK_NOT_FOUND entry and the
// rest of the batch still processes.
func TestNetworkHandler_BulkRemove_MissingNameIsNotFoundEntry(t *testing.T) {
	engine := &fakeNetworkEngine{
		removeFn: func(name string, force bool) error {
			if name == "ghost-net" {
				return &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such network"}
			}
			return nil
		},
	}

	w := testutils.BulkRemoveRequest(newTestHandler(engine).BulkRemove, "networks", `{"ids":["ghost-net","real-net"]}`)

	assert.Equal(t, http.StatusOK, w.Code)
	got := testutils.DecodeBulkRemoveReport(t, w)
	require.Len(t, got, 2)
	require.NotNil(t, got[0].Error)
	assert.False(t, got[0].Success)
	assert.Equal(t, "NETWORK_NOT_FOUND", got[0].Error.Code)
	assert.True(t, got[1].Success, "a missing name must not abort the batch")
}

// --- Route wiring: the networks subtree gates bulk remove at write level ---

func bulkRemoveAccessRouter(hostRepo *testutils.MockHostRepository, accessRepo *testutils.MockHostAccessRepository, handler *NetworkHandler) http.Handler {
	hostAccess := appmiddleware.RequireHostAccess(hostRepo, accessRepo)
	requireWrite := appmiddleware.RequireHostPermission(hostRepo, accessRepo, models.PermissionWrite)
	r := http.NewServeMux()
	r.Handle("POST /api/hosts/{id}/networks/remove", hostAccess(requireWrite(http.HandlerFunc(handler.BulkRemove))))
	return r
}

// TestNetworkHandler_BulkRemove_WriteGate pins the write gate through the real
// middleware: a read-level grantee is denied with 403 before the engine is
// ever reached, while write- and admin-level grantees pass.
func TestNetworkHandler_BulkRemove_WriteGate(t *testing.T) {
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
			engine := &fakeNetworkEngine{
				removeFn: func(string, bool) error {
					called = true
					return nil
				},
			}
			router := bulkRemoveAccessRouter(hostRepo, accessRepo, newTestHandler(engine))

			w := testutils.BulkRemoveRequestAsUser(router, "networks", "grantee-1", `{"ids":["net-a"]}`)

			assert.Equal(t, tt.expectStatus, w.Result().StatusCode, "level %s must answer %d", tt.level, tt.expectStatus)
			assert.Equal(t, tt.expectCalled, called, "the engine must run=%v for level %s — the gate fires before any podman call", tt.expectCalled, tt.level)
			if tt.expectStatus == http.StatusForbidden {
				assert.Contains(t, w.Body.String(), "ACCESS_DENIED", "the deny must carry the ACCESS_DENIED code")
			}
		})
	}
}
