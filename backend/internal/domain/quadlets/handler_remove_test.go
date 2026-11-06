package quadlets

import (
	"net/http"
	"testing"
	"time"

	appmiddleware "github.com/Homerus25/podland/backend/internal/middleware"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// stoppedQuadlets answers the ritual's list call with the named quadlets,
// all in a non-running state.
func stoppedQuadlets(names ...string) func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
	return func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
		list := make([]*entities.ListQuadlet, 0, len(names))
		for _, name := range names {
			list = append(list, quadletFixture(name, "/etc/containers/systemd/"+name, "stopped"))
		}
		return list, nil
	}
}

// TestQuadletHandler_BulkRemove_Success pins the bulk-remove contract: the
// body's ids and force flag reach the engine as ONE batch call, and the
// answer is 200 with one success entry per requested id, in request order.
// The ritual lists before it removes, so listFn is armed too.
func TestQuadletHandler_BulkRemove_Success(t *testing.T) {
	var gotNames []string
	var gotForce bool
	var calls int
	engine := &fakeQuadletEngine{
		listFn: stoppedQuadlets("web.container", "db.container"),
		removeAllFn: func(names []string, force bool) (*QuadletBatchRemoveReport, error) {
			calls++
			gotNames = names
			gotForce = force
			return &QuadletBatchRemoveReport{Removed: names}, nil
		},
	}

	w := testutils.BulkRemoveRequest(newTestHandler(engine).BulkRemove, "quadlets", `{"ids":["web.container","db.container"],"force":true}`)

	assert.Equal(t, http.StatusOK, w.Code, "bulk remove answers 200 even when entries fail")
	assert.Equal(t, 1, calls, "the whole batch must cross as one podman call")
	assert.Equal(t, []string{"web.container", "db.container"}, gotNames, "every requested id must reach the engine")
	assert.True(t, gotForce, "the body's force flag must reach the engine")
	got := testutils.DecodeBulkRemoveReport(t, w)
	require.Len(t, got, 2, "one entry per requested id")
	assert.Equal(t, "web.container", got[0].ID)
	assert.True(t, got[0].Success)
	assert.Nil(t, got[0].Error)
	assert.Equal(t, "db.container", got[1].ID)
	assert.True(t, got[1].Success)
}

// TestQuadletHandler_BulkRemove_StopsRunningResourceFirst pins the silent
// stop=true default: a running quadlet selected without force is stopped
// through the lifecycle seam and then removed — never refused — and the
// request has no way to turn that off (a `stop` field in the body is not
// part of the contract and is ignored).
func TestQuadletHandler_BulkRemove_StopsRunningResourceFirst(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "no stop field", body: `{"ids":["my-container.container"]}`},
		{name: "a stop field is not part of the contract and is ignored", body: `{"ids":["my-container.container"],"stop":false}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			removed := false
			engine := &fakeQuadletEngine{
				listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
					return runningContainerFixture(), nil
				},
				removeAllFn: func(names []string, force bool) (*QuadletBatchRemoveReport, error) {
					removed = true
					assert.False(t, force, "the silent stop must not turn into a force")
					return &QuadletBatchRemoveReport{Removed: names}, nil
				},
			}

			w := testutils.BulkRemoveRequest(newTestHandler(engine).BulkRemove, "quadlets", tt.body)

			assert.Equal(t, http.StatusOK, w.Code)
			assert.True(t, removed, "a running quadlet must be stopped and removed, not refused")
			got := testutils.DecodeBulkRemoveReport(t, w)
			require.Len(t, got, 1)
			assert.True(t, got[0].Success, "stop=true is the silent default, so a running quadlet removes cleanly")
			assert.Nil(t, got[0].Error)
		})
	}
}

// TestQuadletHandler_BulkRemove_FailedQuadletKeepsBatchGoing pins the
// failed entry: a quadlet the batch call reports as failed answers a
// generic INTERNAL_ERROR entry — podman's batch report names the failed
// quadlet but carries no cause (the adapter owns that fact) — and the
// quadlets the call removed still answer success.
func TestQuadletHandler_BulkRemove_FailedQuadletKeepsBatchGoing(t *testing.T) {
	engine := &fakeQuadletEngine{
		listFn: stoppedQuadlets("free-a.container", "busy.container", "free-b.container"),
		removeAllFn: func(names []string, force bool) (*QuadletBatchRemoveReport, error) {
			assert.False(t, force)
			return &QuadletBatchRemoveReport{
				Removed: []string{"free-a.container", "free-b.container"},
				Failed:  []string{"busy.container"},
			}, nil
		},
	}

	w := testutils.BulkRemoveRequest(newTestHandler(engine).BulkRemove, "quadlets", `{"ids":["free-a.container","busy.container","free-b.container"]}`)

	assert.Equal(t, http.StatusOK, w.Code, "a failed entry must not change the status code")
	got := testutils.DecodeBulkRemoveReport(t, w)
	require.Len(t, got, 3)
	assert.True(t, got[0].Success, "a quadlet the call removed must answer success")
	require.NotNil(t, got[1].Error, "the failed quadlet must answer a failed entry")
	assert.False(t, got[1].Success)
	assert.Equal(t, "INTERNAL_ERROR", got[1].Error.Code,
		"the batch report carries no cause, so the entry answers the generic code")
	assert.True(t, got[2].Success, "a quadlet the call removed must answer success")
}

// TestQuadletHandler_BulkRemove_ForceRemovesRunning pins that force=true
// in the body reaches the batch call unforced-stop-free: the running
// resource is neither stopped nor refused, the call removes it.
func TestQuadletHandler_BulkRemove_ForceRemovesRunning(t *testing.T) {
	var gotForce bool
	engine := &fakeQuadletEngine{
		listFn: stoppedQuadlets("busy.container"),
		removeAllFn: func(names []string, force bool) (*QuadletBatchRemoveReport, error) {
			gotForce = force
			return &QuadletBatchRemoveReport{Removed: names}, nil
		},
	}

	w := testutils.BulkRemoveRequest(newTestHandler(engine).BulkRemove, "quadlets", `{"ids":["busy.container"],"force":true}`)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, gotForce, "force=true must reach the engine")
	got := testutils.DecodeBulkRemoveReport(t, w)
	require.Len(t, got, 1)
	assert.True(t, got[0].Success, "a forced removal of a running quadlet succeeds")
	assert.Nil(t, got[0].Error)
}

// TestQuadletHandler_BulkRemove_MissingQuadletIsNotFoundEntry pins the
// vanished quadlet branch: an id the ritual's list does not report answers
// a QUADLET_NOT_FOUND entry and the quadlets the call removed still
// answer success.
func TestQuadletHandler_BulkRemove_MissingQuadletIsNotFoundEntry(t *testing.T) {
	engine := &fakeQuadletEngine{
		listFn: stoppedQuadlets("real.container"),
		removeAllFn: func(names []string, force bool) (*QuadletBatchRemoveReport, error) {
			return &QuadletBatchRemoveReport{Removed: names}, nil
		},
	}

	w := testutils.BulkRemoveRequest(newTestHandler(engine).BulkRemove, "quadlets", `{"ids":["ghost.container","real.container"]}`)

	assert.Equal(t, http.StatusOK, w.Code)
	got := testutils.DecodeBulkRemoveReport(t, w)
	require.Len(t, got, 2)
	require.NotNil(t, got[0].Error)
	assert.False(t, got[0].Success)
	assert.Equal(t, "QUADLET_NOT_FOUND", got[0].Error.Code)
	assert.True(t, got[1].Success, "a missing id must not fail the located ones")
}

// --- Route wiring: the quadlets subtree gates bulk remove at write level ---

func bulkRemoveAccessRouter(hostRepo *testutils.MockHostRepository, accessRepo *testutils.MockHostAccessRepository, handler *QuadletHandler) http.Handler {
	hostAccess := appmiddleware.RequireHostAccess(hostRepo, accessRepo)
	requireWrite := appmiddleware.RequireHostPermission(hostRepo, accessRepo, models.PermissionWrite)
	r := http.NewServeMux()
	r.Handle("POST /api/hosts/{id}/quadlets/remove", hostAccess(requireWrite(http.HandlerFunc(handler.BulkRemove))))
	return r
}

// TestQuadletHandler_BulkRemove_WriteGate pins the write gate through the
// real middleware: a read-level grantee is denied with 403 before the
// engine is ever reached, while write- and admin-level grantees pass.
func TestQuadletHandler_BulkRemove_WriteGate(t *testing.T) {
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
			engine := &fakeQuadletEngine{
				listFn: stoppedQuadlets("web.container"),
				removeAllFn: func(names []string, force bool) (*QuadletBatchRemoveReport, error) {
					called = true
					return &QuadletBatchRemoveReport{Removed: names}, nil
				},
			}
			router := bulkRemoveAccessRouter(hostRepo, accessRepo, newTestHandler(engine))

			w := testutils.BulkRemoveRequestAsUser(router, "quadlets", "grantee-1", `{"ids":["web.container"]}`)

			assert.Equal(t, tt.expectStatus, w.Result().StatusCode, "level %s must answer %d", tt.level, tt.expectStatus)
			assert.Equal(t, tt.expectCalled, called, "the engine must run=%v for level %s — the gate fires before any podman call", tt.expectCalled, tt.level)
			if tt.expectStatus == http.StatusForbidden {
				assert.Contains(t, w.Body.String(), "ACCESS_DENIED", "the deny must carry the ACCESS_DENIED code")
			}
		})
	}
}
