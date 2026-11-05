package containers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// getRequest serves GET /api/hosts/{id}/containers/{containerId}
// through the handler with an authenticated test identity.
func getRequest(handler http.HandlerFunc, containerID string) *httptest.ResponseRecorder {
	return testutils.Perform(handler, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/hosts/test-host/containers/" + containerID,
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-host", "containerId": containerID},
	})
}

// TestContainerHandler_Get_MissingContainerAnswers404 pins the documented
// 404 envelope end to end: the real service classifies the tunnel's
// per-item not-found answer and the web envelope maps the sentinel.
func TestContainerHandler_Get_MissingContainerAnswers404(t *testing.T) {
	engine := &fakeContainerEngine{
		inspectFn: func(id string) ([]*entities.ContainerInspectReport, []error, error) {
			return nil, []error{fmt.Errorf("no such container %q", id)}, nil
		},
	}

	w := getRequest(newTestHandler(engine).Get, "container-123")

	require.Equal(t, http.StatusNotFound, w.Code, "a missing container must answer 404")
	var body appErrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "test-request-id", body.RequestID)
	assert.Equal(t, "CONTAINER_NOT_FOUND", body.Error.Code)
	assert.Equal(t, "Container not found", body.Error.Message)
}

// TestContainerHandler_Get_ReturnsExtendedDetailDTO pins the extended
// response shape of GET .../containers/{containerId}: the detail fields
// (entrypoint, secrets, volumes, exposedPorts, mounts, networks) ride the
// DTO end to end, through the real service over the fake engine, under
// their camelCase wire tags.
func TestContainerHandler_Get_ReturnsExtendedDetailDTO(t *testing.T) {
	engine := &fakeContainerEngine{
		inspectFn: func(string) ([]*entities.ContainerInspectReport, []error, error) {
			return []*entities.ContainerInspectReport{detailInspectReport()}, nil, nil
		},
	}

	w := getRequest(newTestHandler(engine).Get, "container-123")

	require.Equal(t, http.StatusOK, w.Code)
	var body ContainerGetResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	// The image ID rides the DTO so the detail page's image cross-link has
	// a target; the image name stays a separate field.
	assert.Equal(t, "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", body.ImageID)
	assert.Equal(t, "docker.io/library/nginx:latest", body.Image)
	assert.Equal(t, []string{"/docker-entrypoint.sh"}, body.Entrypoint)
	assert.Equal(t, []ContainerSecretResponse{
		{Name: "db-password", UID: 1000, GID: 1001, Mode: 0o400},
	}, body.Secrets)
	assert.Equal(t, []ContainerVolumeResponse{
		{
			Name:        "web-data",
			Source:      "/var/lib/containers/storage/volumes/web-data/_data",
			Destination: "/usr/share/nginx/html",
			RW:          true,
		},
	}, body.Volumes)
	assert.Equal(t, []ExposedPortResponse{
		{ContainerPort: 53, Protocol: "udp"},
		{ContainerPort: 8080, Protocol: "tcp"},
	}, body.ExposedPorts)
	assert.Equal(t, []string{
		"type=volume,source=/var/lib/containers/storage/volumes/web-data/_data,destination=/usr/share/nginx/html",
		"type=bind,source=/host/nginx.conf,destination=/etc/nginx/nginx.conf",
	}, body.Mounts)
	assert.Equal(t, []string{"app-net", "backend"}, body.Networks)

	// Pin the wire tags: the raw JSON carries the camelCase keys the
	// detail page consumes.
	var raw map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	assert.Equal(t, "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", raw["imageId"])
	assert.Contains(t, raw, "entrypoint")
	assert.Contains(t, raw, "secrets")
	assert.Contains(t, raw, "volumes")
	assert.Contains(t, raw, "exposedPorts")
	assert.Contains(t, raw, "mounts")
	assert.Contains(t, raw, "networks")

	secrets, ok := raw["secrets"].([]any)
	require.True(t, ok, "secrets must be a JSON array")
	require.Len(t, secrets, 1)
	secret, ok := secrets[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "db-password", secret["name"])
	assert.Equal(t, float64(1000), secret["uid"])
	assert.Equal(t, float64(1001), secret["gid"])
	assert.Equal(t, float64(0o400), secret["mode"])

	volumes, ok := raw["volumes"].([]any)
	require.True(t, ok, "volumes must be a JSON array")
	require.Len(t, volumes, 1)
	volume, ok := volumes[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "web-data", volume["name"])
	assert.Equal(t, "/var/lib/containers/storage/volumes/web-data/_data", volume["source"])
	assert.Equal(t, "/usr/share/nginx/html", volume["destination"])
	assert.Equal(t, true, volume["rw"])

	exposedPorts, ok := raw["exposedPorts"].([]any)
	require.True(t, ok, "exposedPorts must be a JSON array")
	require.Len(t, exposedPorts, 2)
	first, ok := exposedPorts[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(53), first["containerPort"])
	assert.Equal(t, "udp", first["protocol"])

	// mounts and networks are string arrays, not objects: the detail
	// page renders their entries verbatim.
	mounts, ok := raw["mounts"].([]any)
	require.True(t, ok, "mounts must be a JSON array")
	require.Len(t, mounts, 2)
	assert.Equal(t, "type=volume,source=/var/lib/containers/storage/volumes/web-data/_data,destination=/usr/share/nginx/html", mounts[0])
	assert.Equal(t, "type=bind,source=/host/nginx.conf,destination=/etc/nginx/nginx.conf", mounts[1])

	networks, ok := raw["networks"].([]any)
	require.True(t, ok, "networks must be a JSON array")
	require.Len(t, networks, 2)
	assert.Equal(t, "app-net", networks[0])
	assert.Equal(t, "backend", networks[1])
}
