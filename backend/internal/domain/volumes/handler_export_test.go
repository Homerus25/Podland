package volumes

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	apperrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// exportRequest serves GET
// /api/machines/{id}/volumes/{volumeName}/export through the handler
// with an authenticated test identity.
func exportRequest(export http.HandlerFunc, machineID, volumeName string) *httptest.ResponseRecorder {
	return testutils.Perform(export, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/machines/" + machineID + "/volumes/" + volumeName + "/export",
		UserID:    "test-user",
		URLParams: map[string]string{"id": machineID, "volumeName": volumeName},
	})
}

// TestVolumeHandler_Export_Success pins the export contract: 200 with
// tar headers carrying the volume's filename, and the streamed bytes
// passed through unbuffered to the response.
func TestVolumeHandler_Export_Success(t *testing.T) {
	mockService := new(MockVolumeService)
	mockService.On("Export", mock.Anything, "test-user", "test-machine", "my-volume", mock.Anything).
		Return(nil).Run(func(args mock.Arguments) {
		w := args.Get(4).(io.Writer)
		_, _ = w.Write([]byte("tar-bytes"))
	})
	handler := NewVolumeHandler(mockService)

	w := exportRequest(handler.Export, "test-machine", "my-volume")

	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "application/x-tar", w.Header().Get("Content-Type"))
	assert.Equal(t, `attachment; filename="my-volume.tar"`, w.Header().Get("Content-Disposition"))
	assert.Equal(t, "tar-bytes", w.Body.String(), "streamed bytes must reach the response")
}

// TestVolumeHandler_Export_NotFound pins that an unknown volume name
// gets the standard 404 envelope — no tar headers may leak into it.
func TestVolumeHandler_Export_NotFound(t *testing.T) {
	mockService := new(MockVolumeService)
	mockService.On("Export", mock.Anything, "test-user", "test-machine", "no-such-volume", mock.Anything).
		Return(apperrors.ErrVolumeNotFound)
	handler := NewVolumeHandler(mockService)

	w := exportRequest(handler.Export, "test-machine", "no-such-volume")

	assert.Equal(t, 404, w.Code)
	assert.Contains(t, w.Body.String(), "VOLUME_NOT_FOUND")
	assert.Contains(t, w.Body.String(), "requestId")
	assert.NotEqual(t, "application/x-tar", w.Header().Get("Content-Type"),
		"error response must not keep the tar content type")
}

func TestVolumeHandler_Export_ServiceError(t *testing.T) {
	mockService := new(MockVolumeService)
	mockService.On("Export", mock.Anything, "test-user", "test-machine", "my-volume", mock.Anything).
		Return(assert.AnError)
	handler := NewVolumeHandler(mockService)

	w := exportRequest(handler.Export, "test-machine", "my-volume")

	assert.Equal(t, 500, w.Code)
}

func TestVolumeHandler_Export_MissingVolumeName(t *testing.T) {
	handler := NewVolumeHandler(new(MockVolumeService))
	w := exportRequest(handler.Export, "test-machine", "")
	assert.Equal(t, 400, w.Code)
}

func TestVolumeHandler_Export_MissingMachineID(t *testing.T) {
	handler := NewVolumeHandler(new(MockVolumeService))
	w := exportRequest(handler.Export, "", "my-volume")
	assert.Equal(t, 400, w.Code)
}
