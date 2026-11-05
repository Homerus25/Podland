package volumes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// getRequest serves a GET /api/hosts/{id}/volumes/{volumeName}
// through the handler with an authenticated test identity.
func getRequest(get http.HandlerFunc, volumeName string) *httptest.ResponseRecorder {
	return testutils.Perform(get, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/hosts/test-host/volumes/" + volumeName,
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-host", "volumeName": volumeName},
	})
}

// TestVolumeHandler_Get_Success pins the inspect contract: 200 with the
// volume's full configuration resolved by exact name.
func TestVolumeHandler_Get_Success(t *testing.T) {
	cfg := namedVolumeFixture("2026-08-30T10:00:00Z")
	report := &entities.VolumeInspectReport{VolumeConfigResponse: &cfg}
	var gotName string
	engine := &fakeVolumeEngine{
		inspectFn: func(name string) (*entities.VolumeInspectReport, error) {
			gotName = name
			return report, nil
		},
	}

	w := getRequest(newTestHandler(engine).Get, "my-volume")

	assert.Equal(t, 200, w.Code, "inspect should return 200")
	assert.Equal(t, "my-volume", gotName, "the URL param must be the inspected name")

	var got VolumeInspectResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got), "response must decode as a volume config")
	assert.Equal(t, "my-volume", got.Name)
	assert.Equal(t, "local", got.Driver)
	assert.Equal(t, "/var/lib/containers/storage/volumes/my-volume/_data", got.Mountpoint)
	assert.Equal(t, map[string]string{"app": "test"}, got.Labels)
	assert.Equal(t, map[string]string{"type": "tmpfs"}, got.Options)
	assert.Equal(t, "2026-08-30T10:00:00Z", got.CreatedAt)
}

// TestVolumeHandler_Get_EmptyLabelsOptions_AlwaysPresentOnWire pins the
// documented wire contract: labels and options are always present in the
// inspect response, {} when the volume has none. The frontend drawer
// renders both maps unconditionally, so an omitted key crashes the page.
func TestVolumeHandler_Get_EmptyLabelsOptions_AlwaysPresentOnWire(t *testing.T) {
	cfg := anonymousVolumeFixture()
	report := &entities.VolumeInspectReport{VolumeConfigResponse: &cfg}
	engine := &fakeVolumeEngine{
		inspectFn: func(name string) (*entities.VolumeInspectReport, error) {
			return report, nil
		},
	}

	w := getRequest(newTestHandler(engine).Get, cfg.Name)

	assert.Equal(t, 200, w.Code, "inspect should return 200")
	assert.Contains(t, w.Body.String(), `"labels":{}`,
		"labels must be present as {} when the volume has none")
	assert.Contains(t, w.Body.String(), `"options":{}`,
		"options must be present as {} when the volume has none")
}
