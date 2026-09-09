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

// listRequest performs a GET /api/machines/{id}/volumes through the
// handler with an authenticated test identity.
func listRequest(list http.HandlerFunc) *httptest.ResponseRecorder {
	return testutils.Perform(list, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/machines/test-machine/volumes",
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-machine"},
	})
}

// TestVolumeHandler_List_Success pins the list contract: 200, JSON array,
// and the DTO fields the API promises (name, driver, createdAt, anonymous,
// labels, options, mountpoint, size when reported).
func TestVolumeHandler_List_Success(t *testing.T) {
	createdAt := "2026-08-30T10:00:00Z"
	reports := []*entities.VolumeListReport{
		{VolumeConfigResponse: namedVolumeFixture(createdAt)},
		{VolumeConfigResponse: anonymousVolumeFixture()},
	}
	engine := &fakeVolumeEngine{
		listFn: func(entities.VolumeListOptions) ([]*entities.VolumeListReport, error) {
			return reports, nil
		},
	}

	w := listRequest(newTestHandler(engine).List)

	assert.Equal(t, 200, w.Code, "list should return 200")

	volumes := decodeVolumes(t, w.Body.String())
	assert.Len(t, volumes, 2, "named and anonymous volumes must both appear")

	named := volumes[0]
	assert.Equal(t, "my-volume", named.Name)
	assert.Equal(t, "local", named.Driver)
	assert.Equal(t, createdAt, named.CreatedAt)
	assert.False(t, named.Anonymous)
	assert.Equal(t, "/var/lib/containers/storage/volumes/my-volume/_data", named.Mountpoint)
	assert.Equal(t, map[string]string{"app": "test"}, named.Labels)
	assert.Equal(t, map[string]string{"type": "tmpfs"}, named.Options)
	assert.NotContains(t, w.Body.String(), `"size":`,
		"size must be omitted when podman does not report it")

	anonymous := volumes[1]
	assert.True(t, anonymous.Anonymous, "anonymous volumes must be flagged")
	assert.NotEmpty(t, anonymous.Name)
}

func decodeVolumes(t *testing.T, body string) []VolumeResponse {
	t.Helper()
	var out []VolumeResponse
	require.NoError(t, json.Unmarshal([]byte(body), &out), "response must decode as a JSON array of volumes")
	return out
}

// jsonUnmarshalSlice decodes a JSON array response body.
func jsonUnmarshalSlice(data []byte, out any) error {
	return json.Unmarshal(data, out)
}
