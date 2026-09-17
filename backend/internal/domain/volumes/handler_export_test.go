package volumes

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"go.podman.io/podman/v6/pkg/domain/entities"
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
	engine := &fakeVolumeEngine{
		inspectFn: func(string) (*entities.VolumeInspectReport, error) {
			return inspectFixture("my-volume"), nil
		},
		exportFn: func(_ string, w io.Writer) error {
			_, err := w.Write([]byte("tar-bytes"))
			return err
		},
	}

	w := exportRequest(newTestHandler(engine).Export, "test-machine", "my-volume")

	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "application/x-tar", w.Header().Get("Content-Type"))
	assert.Equal(t, `attachment; filename="my-volume.tar"`, w.Header().Get("Content-Disposition"))
	assert.Equal(t, "tar-bytes", w.Body.String(), "streamed bytes must reach the response")
}

// TestVolumeHandler_Export_NotFound pins that an unknown volume name
// gets the standard 404 envelope — no tar headers may leak into it.
func TestVolumeHandler_Export_NotFound(t *testing.T) {
	engine := &fakeVolumeEngine{
		inspectFn: func(string) (*entities.VolumeInspectReport, error) {
			return nil, notFoundErr()
		},
		// exportFn stays unarmed: it must never be reached for a
		// missing volume.
	}

	w := exportRequest(newTestHandler(engine).Export, "test-machine", "no-such-volume")

	assert.Equal(t, 404, w.Code)
	assert.Contains(t, w.Body.String(), "VOLUME_NOT_FOUND")
	assert.Contains(t, w.Body.String(), "requestId")
	assert.NotEqual(t, "application/x-tar", w.Header().Get("Content-Type"),
		"error response must not keep the tar content type")
}
