package images

import (
	"net/http"
	"testing"
	"time"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	appmiddleware "github.com/Homerus25/podland/backend/internal/middleware"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// TestImageHandler_BulkRemove_Success pins the bulk-remove contract: the
// body's ids and force flag reach the engine as ONE batch call, and the
// answer is 200 with one success entry per requested id, in request
// order.
func TestImageHandler_BulkRemove_Success(t *testing.T) {
	var gotIDs []string
	var gotForce bool
	var calls int
	engine := &fakeImageEngine{
		removeAllFn: func(ids []string, force bool) (*entities.ImageRemoveReport, []error) {
			calls++
			gotIDs = ids
			gotForce = force
			return &entities.ImageRemoveReport{Deleted: ids}, nil
		},
	}

	w := testutils.BulkRemoveRequest(newTestHandler(engine).BulkRemove, "images", `{"ids":["sha256:aaa","sha256:bbb"],"force":true}`)

	assert.Equal(t, http.StatusOK, w.Code, "bulk remove answers 200 even when entries fail")
	assert.Equal(t, 1, calls, "the whole batch must cross as one podman call")
	assert.Equal(t, []string{"sha256:aaa", "sha256:bbb"}, gotIDs, "every requested id must reach the engine")
	assert.True(t, gotForce, "the body's force flag must reach the engine")
	got := testutils.DecodeBulkRemoveReport(t, w)
	require.Len(t, got, 2, "one entry per requested id")
	assert.Equal(t, "sha256:aaa", got[0].ID)
	assert.True(t, got[0].Success)
	assert.Nil(t, got[0].Error)
	assert.Equal(t, "sha256:bbb", got[1].ID)
	assert.True(t, got[1].Success)
}

// TestImageHandler_BulkRemove_InUseRefusalKeepsBatchGoing pins the refusal
// entry: an image used by containers without force answers an
// IMAGE_USED_BY_CONTAINERS entry, and the images the batch did remove
// still answer success. The entry mirrors the single-item delete exactly —
// the images refusal answers the bare sentinel, so podman's text does not
// ride it (unlike the domains whose refusal builder threads the cause).
func TestImageHandler_BulkRemove_InUseRefusalKeepsBatchGoing(t *testing.T) {
	engine := &fakeImageEngine{
		removeAllFn: func(ids []string, force bool) (*entities.ImageRemoveReport, []error) {
			assert.False(t, force, "the unforced pass must reach the engine unforced")
			return &entities.ImageRemoveReport{Deleted: []string{"sha256:free-a", "sha256:free-b"}},
				[]error{&errorhandling.ErrorModel{
					ResponseCode: http.StatusConflict,
					Message:      "image is in use by a container",
				}}
		},
	}

	w := testutils.BulkRemoveRequest(newTestHandler(engine).BulkRemove, "images", `{"ids":["sha256:free-a","sha256:busy","sha256:free-b"]}`)

	assert.Equal(t, http.StatusOK, w.Code, "a refused entry must not change the status code")
	got := testutils.DecodeBulkRemoveReport(t, w)
	require.Len(t, got, 3)
	assert.True(t, got[0].Success, "an image the batch removed must answer success")
	require.NotNil(t, got[1].Error, "the in-use image must answer a refused entry")
	assert.False(t, got[1].Success)
	assert.Equal(t, "IMAGE_USED_BY_CONTAINERS", got[1].Error.Code)
	assert.Equal(t, appErrors.ErrImageUsedByContainers.Message, got[1].Error.Message,
		"the entry carries the same message the single-item delete answers")
	assert.True(t, got[2].Success, "an image the batch removed must answer success")
}

// TestImageHandler_BulkRemove_ForceRemovesInUse pins that force=true in
// the body removes the in-use images the unforced pass refused.
func TestImageHandler_BulkRemove_ForceRemovesInUse(t *testing.T) {
	var gotForce bool
	engine := &fakeImageEngine{
		removeAllFn: func(ids []string, force bool) (*entities.ImageRemoveReport, []error) {
			gotForce = force
			return &entities.ImageRemoveReport{Deleted: ids}, nil
		},
	}

	w := testutils.BulkRemoveRequest(newTestHandler(engine).BulkRemove, "images", `{"ids":["sha256:busy"],"force":true}`)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, gotForce, "force=true must reach the engine")
	got := testutils.DecodeBulkRemoveReport(t, w)
	require.Len(t, got, 1)
	assert.True(t, got[0].Success, "a forced removal of an in-use image succeeds")
	assert.Nil(t, got[0].Error)
}

// TestImageHandler_BulkRemove_MissingIDIsNotFoundEntry pins the vanished image
// branch: a missing id answers an IMAGE_NOT_FOUND entry and the images the
// batch did remove still answer success.
func TestImageHandler_BulkRemove_MissingIDIsNotFoundEntry(t *testing.T) {
	engine := &fakeImageEngine{
		removeAllFn: func(ids []string, force bool) (*entities.ImageRemoveReport, []error) {
			return &entities.ImageRemoveReport{Deleted: []string{"sha256:real"}},
				[]error{&errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such image"}}
		},
	}

	w := testutils.BulkRemoveRequest(newTestHandler(engine).BulkRemove, "images", `{"ids":["sha256:ghost","sha256:real"]}`)

	assert.Equal(t, http.StatusOK, w.Code)
	got := testutils.DecodeBulkRemoveReport(t, w)
	require.Len(t, got, 2)
	require.NotNil(t, got[0].Error)
	assert.False(t, got[0].Success)
	assert.Equal(t, "IMAGE_NOT_FOUND", got[0].Error.Code)
	assert.True(t, got[1].Success, "a missing id must not fail the removed ids")
}

// --- Route wiring: the images subtree gates bulk remove at write level ---

func bulkRemoveAccessRouter(hostRepo *testutils.MockHostRepository, accessRepo *testutils.MockHostAccessRepository, handler *ImageHandler) http.Handler {
	hostAccess := appmiddleware.RequireHostAccess(hostRepo, accessRepo)
	requireWrite := appmiddleware.RequireHostPermission(hostRepo, accessRepo, models.PermissionWrite)
	r := http.NewServeMux()
	r.Handle("POST /api/hosts/{id}/images/remove", hostAccess(requireWrite(http.HandlerFunc(handler.BulkRemove))))
	return r
}

// TestImageHandler_BulkRemove_WriteGate pins the write gate through the real
// middleware: a read-level grantee is denied with 403 before the engine is
// ever reached, while write- and admin-level grantees pass.
func TestImageHandler_BulkRemove_WriteGate(t *testing.T) {
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
			engine := &fakeImageEngine{
				removeAllFn: func(ids []string, force bool) (*entities.ImageRemoveReport, []error) {
					called = true
					return &entities.ImageRemoveReport{Deleted: ids}, nil
				},
			}
			router := bulkRemoveAccessRouter(hostRepo, accessRepo, newTestHandler(engine))

			w := testutils.BulkRemoveRequestAsUser(router, "images", "grantee-1", `{"ids":["sha256:aaa"]}`)

			assert.Equal(t, tt.expectStatus, w.Result().StatusCode, "level %s must answer %d", tt.level, tt.expectStatus)
			assert.Equal(t, tt.expectCalled, called, "the engine must run=%v for level %s — the gate fires before any podman call", tt.expectCalled, tt.level)
			if tt.expectStatus == http.StatusForbidden {
				assert.Contains(t, w.Body.String(), "ACCESS_DENIED", "the deny must carry the ACCESS_DENIED code")
			}
		})
	}
}
