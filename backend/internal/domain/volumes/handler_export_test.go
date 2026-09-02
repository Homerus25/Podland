package volumes

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	apperrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

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

	w := recordExportRequest(handler.Export, "test-machine", "my-volume")

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

	w := recordExportRequest(handler.Export, "test-machine", "no-such-volume")

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

	w := recordExportRequest(handler.Export, "test-machine", "my-volume")

	assert.Equal(t, 500, w.Code)
}

func TestVolumeHandler_Export_MissingVolumeName(t *testing.T) {
	handler := NewVolumeHandler(new(MockVolumeService))
	w := recordExportRequest(handler.Export, "test-machine", "")
	assert.Equal(t, 400, w.Code)
}

// recordExportRequest serves GET
// /api/machines/{id}/volumes/{volumeName}/export through the given
// handler with an authenticated test identity.
func recordExportRequest(export http.HandlerFunc, machineID, volumeName string) *httptest.ResponseRecorder {
	ctx := contextutils.SetUserID(contextutils.SetRequestID(context.Background(), "test-request-id"), "test-user")
	req := httptest.NewRequest(http.MethodGet, "/api/machines/"+machineID+"/volumes/"+volumeName+"/export", nil).WithContext(ctx)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", machineID)
	rctx.URLParams.Add("volumeName", volumeName)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	export(w, req)
	return w
}

func TestVolumeHandler_Export_MissingMachineID(t *testing.T) {
	handler := NewVolumeHandler(new(MockVolumeService))
	w := recordExportRequest(handler.Export, "", "my-volume")
	assert.Equal(t, 400, w.Code)
}
