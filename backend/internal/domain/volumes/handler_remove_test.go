package volumes

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

// TestVolumeHandler_BulkRemove_Success pins the bulk-remove contract: the
// body's names and force flag reach the engine, and the answer is 200
// with one success entry per requested name, in request order.
func TestVolumeHandler_BulkRemove_Success(t *testing.T) {
	var gotNames []string
	var gotForce bool
	engine := &fakeVolumeEngine{
		removeFn: func(name string, force bool) error {
			gotNames = append(gotNames, name)
			gotForce = force
			return nil
		},
	}

	w := testutils.BulkRemoveRequest(newTestHandler(engine).BulkRemove, "volumes", `{"ids":["vol-a","vol-b"],"force":true}`)

	assert.Equal(t, http.StatusOK, w.Code, "bulk remove answers 200 even when entries fail")
	assert.Equal(t, []string{"vol-a", "vol-b"}, gotNames, "every requested name must reach the engine")
	assert.True(t, gotForce, "the body's force flag must reach the engine")
	got := testutils.DecodeBulkRemoveReport(t, w)
	require.Len(t, got, 2, "one entry per requested name")
	assert.Equal(t, "vol-a", got[0].ID)
	assert.True(t, got[0].Success)
	assert.Nil(t, got[0].Error)
	assert.Equal(t, "vol-b", got[1].ID)
	assert.True(t, got[1].Success)
}

// TestVolumeHandler_BulkRemove_InUseRefusalKeepsBatchGoing pins the refusal
// entry: an in-use volume without force answers a VOLUME_IN_USE entry
// carrying podman's text, and the other names in the same request still
// process.
func TestVolumeHandler_BulkRemove_InUseRefusalKeepsBatchGoing(t *testing.T) {
	const refusal = `volume "busy-vol" is being used by a container: volume is being used`
	engine := &fakeVolumeEngine{
		removeFn: func(name string, force bool) error {
			if name == "busy-vol" && !force {
				return &errorhandling.ErrorModel{
					ResponseCode: http.StatusInternalServerError,
					Message:      refusal,
				}
			}
			return nil
		},
	}

	w := testutils.BulkRemoveRequest(newTestHandler(engine).BulkRemove, "volumes", `{"ids":["free-a","busy-vol","free-b"]}`)

	assert.Equal(t, http.StatusOK, w.Code, "a refused entry must not change the status code")
	got := testutils.DecodeBulkRemoveReport(t, w)
	require.Len(t, got, 3)
	assert.True(t, got[0].Success, "the item before the refusal must still be removed")
	require.NotNil(t, got[1].Error, "the in-use volume must answer a refused entry")
	assert.False(t, got[1].Success)
	assert.Equal(t, "VOLUME_IN_USE", got[1].Error.Code)
	assert.Contains(t, got[1].Error.Message, refusal, "podman's refusal text must reach the client")
	assert.True(t, got[2].Success, "the item after the refusal must still be removed")
}

// TestVolumeHandler_BulkRemove_ForceRemovesInUse pins that force=true in the
// body removes the in-use volumes the unforced pass refused.
func TestVolumeHandler_BulkRemove_ForceRemovesInUse(t *testing.T) {
	var gotForce bool
	engine := &fakeVolumeEngine{
		removeFn: func(name string, force bool) error {
			gotForce = force
			if !force {
				return &errorhandling.ErrorModel{ResponseCode: http.StatusConflict, Message: "volume is in use"}
			}
			return nil
		},
	}

	w := testutils.BulkRemoveRequest(newTestHandler(engine).BulkRemove, "volumes", `{"ids":["busy-vol"],"force":true}`)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, gotForce, "force=true must reach the engine")
	got := testutils.DecodeBulkRemoveReport(t, w)
	require.Len(t, got, 1)
	assert.True(t, got[0].Success, "a forced removal of an in-use volume succeeds")
	assert.Nil(t, got[0].Error)
}

// TestVolumeHandler_BulkRemove_MissingNameIsNotFoundEntry pins the vanished
// volume branch: a missing name answers a VOLUME_NOT_FOUND entry and the
// rest of the batch still processes.
func TestVolumeHandler_BulkRemove_MissingNameIsNotFoundEntry(t *testing.T) {
	engine := &fakeVolumeEngine{
		removeFn: func(name string, force bool) error {
			if name == "ghost-vol" {
				return &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such volume"}
			}
			return nil
		},
	}

	w := testutils.BulkRemoveRequest(newTestHandler(engine).BulkRemove, "volumes", `{"ids":["ghost-vol","real-vol"]}`)

	assert.Equal(t, http.StatusOK, w.Code)
	got := testutils.DecodeBulkRemoveReport(t, w)
	require.Len(t, got, 2)
	require.NotNil(t, got[0].Error)
	assert.False(t, got[0].Success)
	assert.Equal(t, "VOLUME_NOT_FOUND", got[0].Error.Code)
	assert.True(t, got[1].Success, "a missing name must not abort the batch")
}

// --- Route wiring: the volumes subtree gates bulk remove at write level ---

func bulkRemoveAccessRouter(hostRepo *testutils.MockHostRepository, accessRepo *testutils.MockHostAccessRepository, handler *VolumeHandler) http.Handler {
	hostAccess := appmiddleware.RequireHostAccess(hostRepo, accessRepo)
	requireWrite := appmiddleware.RequireHostPermission(hostRepo, accessRepo, models.PermissionWrite)
	r := http.NewServeMux()
	r.Handle("POST /api/hosts/{id}/volumes/remove", hostAccess(requireWrite(http.HandlerFunc(handler.BulkRemove))))
	return r
}

// TestVolumeHandler_BulkRemove_WriteGate pins the write gate through the real
// middleware: a read-level grantee is denied with 403 before the engine is
// ever reached, while write- and admin-level grantees pass.
func TestVolumeHandler_BulkRemove_WriteGate(t *testing.T) {
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
			engine := &fakeVolumeEngine{
				removeFn: func(string, bool) error {
					called = true
					return nil
				},
			}
			router := bulkRemoveAccessRouter(hostRepo, accessRepo, newTestHandler(engine))

			w := testutils.BulkRemoveRequestAsUser(router, "volumes", "grantee-1", `{"ids":["vol-a"]}`)

			assert.Equal(t, tt.expectStatus, w.Result().StatusCode, "level %s must answer %d", tt.level, tt.expectStatus)
			assert.Equal(t, tt.expectCalled, called, "the engine must run=%v for level %s — the gate fires before any podman call", tt.expectCalled, tt.level)
			if tt.expectStatus == http.StatusForbidden {
				assert.Contains(t, w.Body.String(), "ACCESS_DENIED", "the deny must carry the ACCESS_DENIED code")
			}
		})
	}
}
