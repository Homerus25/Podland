package volumes

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// deleteRequest serves DELETE
// /api/machines/{id}/volumes/{volumeName}{query} through the handler
// with an authenticated test identity.
func deleteRequest(handler http.HandlerFunc, volumeName, query string) *httptest.ResponseRecorder {
	return testutils.Perform(handler, testutils.Request{
		Method:    http.MethodDelete,
		Path:      "/api/machines/test-machine/volumes/" + volumeName + query,
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-machine", "volumeName": volumeName},
	})
}

// TestVolumeHandler_Delete_Success pins the delete contract: 204 with an
// empty body, and the force flag forwarded as parsed (default false).
func TestVolumeHandler_Delete_Success(t *testing.T) {
	for _, tc := range []struct {
		query      string
		wantForce  bool
		queryLabel string
	}{
		{query: "", wantForce: false, queryLabel: "default"},
		{query: "?force=true", wantForce: true, queryLabel: "force=true"},
		{query: "?force=false", wantForce: false, queryLabel: "force=false"},
	} {
		t.Run(tc.queryLabel, func(t *testing.T) {
			mockService := new(MockVolumeService)
			mockService.On("Delete", mock.Anything, "test-user", "test-machine", "my-volume", tc.wantForce).Return(nil)
			handler := NewVolumeHandler(mockService)

			w := deleteRequest(handler.Delete, "my-volume", tc.query)

			assert.Equal(t, 204, w.Code, "delete of an unused volume should return 204")
			assert.Empty(t, w.Body.String(), "204 response must have an empty body")
			mockService.AssertExpectations(t)
		})
	}
}

// TestVolumeHandler_Delete_InvalidForceFlag pins force-flag parsing: a
// non-boolean force value is a client error before the service is called.
func TestVolumeHandler_Delete_InvalidForceFlag(t *testing.T) {
	handler := NewVolumeHandler(new(MockVolumeService))

	w := deleteRequest(handler.Delete, "my-volume", "?force=yes")

	assert.Equal(t, 400, w.Code)
	assert.Contains(t, w.Body.String(), "INVALID_REQUEST")
}

// TestVolumeHandler_Delete_NotFound pins that an unknown volume name gets
// the same clean 404 as inspect.
func TestVolumeHandler_Delete_NotFound(t *testing.T) {
	mockService := new(MockVolumeService)
	mockService.On("Delete", mock.Anything, "test-user", "test-machine", "no-such-volume", false).
		Return(errors.ErrVolumeNotFound)
	handler := NewVolumeHandler(mockService)

	w := deleteRequest(handler.Delete, "no-such-volume", "")

	assert.Equal(t, 404, w.Code)
	assert.Contains(t, w.Body.String(), "VOLUME_NOT_FOUND")
}

// TestVolumeHandler_Delete_InUse pins the distinct in-use code: deleting
// an in-use volume without force surfaces as 409 VOLUME_IN_USE carrying
// podman's message, so the UI can explain what happened.
func TestVolumeHandler_Delete_InUse(t *testing.T) {
	inUseErr := errors.ErrVolumeInUse.WithMessage("volume is in use by container abc")
	mockService := new(MockVolumeService)
	mockService.On("Delete", mock.Anything, "test-user", "test-machine", "my-volume", false).Return(inUseErr)
	handler := NewVolumeHandler(mockService)

	w := deleteRequest(handler.Delete, "my-volume", "")

	assert.Equal(t, 409, w.Code)
	assert.Contains(t, w.Body.String(), "VOLUME_IN_USE")
	assert.Contains(t, w.Body.String(), "volume is in use by container abc", "podman's message must be carried")
}

func TestVolumeHandler_Delete_ServiceError(t *testing.T) {
	mockService := new(MockVolumeService)
	mockService.On("Delete", mock.Anything, "test-user", "test-machine", "my-volume", false).
		Return(fmt.Errorf("boom"))
	handler := NewVolumeHandler(mockService)

	w := deleteRequest(handler.Delete, "my-volume", "")

	assert.Equal(t, 500, w.Code)
}

func TestVolumeHandler_Delete_MissingVolumeName(t *testing.T) {
	handler := NewVolumeHandler(new(MockVolumeService))
	w := deleteRequest(handler.Delete, "", "")
	assert.Equal(t, 400, w.Code, "missing volume name must be rejected")
}

// TestMapInUseError pins the in-use mapping: podman's 409 ErrorModel and
// its plain "in use" text both map to VOLUME_IN_USE, while other errors
// stay unmapped (wrapped 5xx at the handler).
func TestMapInUseError(t *testing.T) {
	appErr := mapInUseError(&errorhandling.ErrorModel{ResponseCode: http.StatusConflict, Message: "volume in use"})
	require.NotNil(t, appErr, "podman 409 must map to the in-use client error")
	assert.Equal(t, http.StatusConflict, appErr.Status)
	assert.Equal(t, "VOLUME_IN_USE", appErr.Code)
	assert.Equal(t, "volume in use", appErr.Message)

	appErr = mapInUseError(fmt.Errorf("volume is in use by a container"))
	require.NotNil(t, appErr, "plain in-use text must map to the in-use client error")
	assert.Equal(t, http.StatusConflict, appErr.Status)

	assert.Nil(t, mapInUseError(&errorhandling.ErrorModel{ResponseCode: http.StatusNotFound}), "404 must not map to in-use")
	assert.Nil(t, mapInUseError(assert.AnError), "unrelated errors must not map to in-use")
}
