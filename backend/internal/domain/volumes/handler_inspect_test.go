package volumes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// getRequest serves a GET /api/machines/{id}/volumes/{volumeName}
// through the handler with an authenticated test identity.
func getRequest(get http.HandlerFunc, volumeName string) *httptest.ResponseRecorder {
	return testutils.Perform(get, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/machines/test-machine/volumes/" + volumeName,
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-machine", "volumeName": volumeName},
	})
}

// TestVolumeHandler_Get_Success pins the inspect contract: 200 with the
// volume's full configuration resolved by exact name.
func TestVolumeHandler_Get_Success(t *testing.T) {
	cfg := namedVolumeFixture("2026-08-30T10:00:00Z")
	report := &entities.VolumeInspectReport{VolumeConfigResponse: &cfg}

	mockService := new(MockVolumeService)
	mockService.On("Get", mock.Anything, "test-user", "test-machine", "my-volume").Return(report, nil)
	handler := NewVolumeHandler(mockService)

	w := getRequest(handler.Get, "my-volume")

	assert.Equal(t, 200, w.Code, "inspect should return 200")

	var got VolumeInspectResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got), "response must decode as a volume config")
	assert.Equal(t, "my-volume", got.Name)
	assert.Equal(t, "local", got.Driver)
	assert.Equal(t, "/var/lib/containers/storage/volumes/my-volume/_data", got.Mountpoint)
	assert.Equal(t, map[string]string{"app": "test"}, got.Labels)
	assert.Equal(t, map[string]string{"type": "tmpfs"}, got.Options)
	assert.Equal(t, "2026-08-30T10:00:00Z", got.CreatedAt)
}

// TestVolumeHandler_Get_NotFound pins the distinct not-found error code:
// an unknown volume name must surface as 404 VOLUME_NOT_FOUND in the
// standard envelope, so stale UI state is diagnosable.
func TestVolumeHandler_Get_NotFound(t *testing.T) {
	mockService := new(MockVolumeService)
	mockService.On("Get", mock.Anything, "test-user", "test-machine", "no-such-volume").
		Return((*entities.VolumeInspectReport)(nil), errors.ErrVolumeNotFound)
	handler := NewVolumeHandler(mockService)

	w := getRequest(handler.Get, "no-such-volume")

	assert.Equal(t, 404, w.Code)
	assert.Contains(t, w.Body.String(), "VOLUME_NOT_FOUND")
	assert.Contains(t, w.Body.String(), "requestId")
}

// TestVolumeHandler_Get_ConnectionError pins that connection failures
// surface as 5xx via the standard envelope.
func TestVolumeHandler_Get_ConnectionError(t *testing.T) {
	mockService := new(MockVolumeService)
	mockService.On("Get", mock.Anything, "test-user", "test-machine", "my-volume").
		Return((*entities.VolumeInspectReport)(nil), assert.AnError)
	handler := NewVolumeHandler(mockService)

	w := getRequest(handler.Get, "my-volume")

	assert.Equal(t, 500, w.Code)
	assert.Contains(t, w.Body.String(), "requestId")
}

func TestVolumeHandler_Get_MissingVolumeName(t *testing.T) {
	handler := NewVolumeHandler(new(MockVolumeService))
	w := getRequest(handler.Get, "")
	assert.Equal(t, 400, w.Code, "missing volume name must be rejected")
}
