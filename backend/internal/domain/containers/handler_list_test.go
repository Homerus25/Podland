package containers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	netTypes "go.podman.io/common/libnetwork/types"
	"go.podman.io/podman/v6/pkg/domain/entities"
	define "go.podman.io/podman/v6/pkg/ps/define"
)

// listRequest serves GET /api/hosts/{id}/containers{query} through
// the handler with an authenticated test identity.
func listRequest(list http.HandlerFunc, query string) *httptest.ResponseRecorder {
	return testutils.Perform(list, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/hosts/test-host/containers" + query,
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-host"},
	})
}

// TestContainerHandler_List_Success pins the list contract through the
// real service: 200 with the count, the mapped container fields, and the
// timestamp envelope.
func TestContainerHandler_List_Success(t *testing.T) {
	testContainers := []entities.ListContainer{
		{
			ID:       "container-1",
			Names:    []string{"test-container-1"},
			Image:    "test-image:latest",
			State:    "running",
			Status:   "Up 5 minutes",
			Restarts: 0,
		},
		{
			ID:       "container-2",
			Names:    []string{"test-container-2"},
			Image:    "another-image:v1",
			State:    "exited",
			Status:   "Exited (0) 2 hours ago",
			Restarts: 2,
			Exited:   true,
			ExitCode: 0,
		},
	}
	engine := &fakeContainerEngine{
		listFn: func(entities.ContainerListOptions) ([]entities.ListContainer, error) {
			return testContainers, nil
		},
	}

	rr := listRequest(newTestHandler(engine).List, "?all=true")

	assert.Equal(t, http.StatusOK, rr.Code, "Should return 200 OK for successful request")
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"), "Should return JSON content type")

	var result ContainerListResult
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &result), "Should be able to parse JSON response")

	assert.Equal(t, 2, result.Total, "Should return correct container count")
	assert.Equal(t, 2, len(result.Containers), "Should return correct number of containers")
	assert.NotEmpty(t, result.Timestamp, "Should include timestamp")

	assert.Equal(t, "container-1", result.Containers[0].ID, "Should map container ID correctly")
	assert.Equal(t, []string{"test-container-1"}, result.Containers[0].Names, "Should map container names correctly")
	assert.Equal(t, "test-image:latest", result.Containers[0].Image, "Should map container image correctly")
	assert.Equal(t, "running", result.Containers[0].State, "Should map container state correctly")
}

// TestContainerHandler_List_QueryParameters tests query parameter parsing
func TestContainerHandler_List_QueryParameters(t *testing.T) {
	// Test different query parameter combinations
	testCases := []struct {
		name         string
		query        string
		expectedOpts entities.ContainerListOptions
		description  string
	}{
		{
			name:         "all parameter",
			query:        "?all=true",
			expectedOpts: entities.ContainerListOptions{All: true},
			description:  "Should parse all parameter correctly",
		},
		{
			name:         "size parameter",
			query:        "?size=true",
			expectedOpts: entities.ContainerListOptions{Size: true},
			description:  "Should parse size parameter correctly",
		},
		{
			name:         "namespace parameter",
			query:        "?namespace=true",
			expectedOpts: entities.ContainerListOptions{Namespace: true},
			description:  "Should parse namespace parameter correctly",
		},
		{
			name:  "multiple parameters",
			query: "?all=true&size=true&last=5",
			expectedOpts: entities.ContainerListOptions{
				All:  true,
				Size: true,
				Last: 5,
			},
			description: "Should parse multiple parameters correctly",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var gotOpts entities.ContainerListOptions
			engine := &fakeContainerEngine{
				listFn: func(opts entities.ContainerListOptions) ([]entities.ListContainer, error) {
					gotOpts = opts
					return []entities.ListContainer{}, nil
				},
			}

			rr := listRequest(newTestHandler(engine).List, tc.query)

			assert.Equal(t, http.StatusOK, rr.Code, tc.description)
			assert.Equal(t, tc.expectedOpts, gotOpts, "the parsed query options must reach the host")
		})
	}
}

// TestContainerHandler_List_EmptyResponse tests handling of empty container lists
func TestContainerHandler_List_EmptyResponse(t *testing.T) {
	engine := &fakeContainerEngine{
		listFn: func(entities.ContainerListOptions) ([]entities.ListContainer, error) {
			return []entities.ListContainer{}, nil
		},
	}

	rr := listRequest(newTestHandler(engine).List, "")

	assert.Equal(t, http.StatusOK, rr.Code, "Should return 200 OK for empty container list")

	var result ContainerListResult
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &result), "Should be able to parse empty response")

	assert.Equal(t, 0, result.Total, "Should return 0 total containers")
	assert.Equal(t, 0, len(result.Containers), "Should return empty container array")
	assert.NotEmpty(t, result.Timestamp, "Should still include timestamp")
}

// TestContainerHandler_List_ContainerWithOptionalFields tests handling of containers with optional fields
func TestContainerHandler_List_ContainerWithOptionalFields(t *testing.T) {
	// Container with various optional fields
	testContainer := entities.ListContainer{
		ID:       "container-with-options",
		Names:    []string{"test-container"},
		Image:    "test-image:latest",
		ImageID:  "sha256:abc123",
		Command:  []string{"/bin/sh", "-c", "echo hello"},
		State:    "running",
		Status:   "Up 10 minutes",
		Restarts: 3,
		Exited:   false,
		ExitCode: 0,
		Labels: map[string]string{
			"app":     "test",
			"env":     "development",
			"version": "1.0",
		},
		Ports: []netTypes.PortMapping{
			{
				ContainerPort: 8080,
				HostPort:      8080,
				Protocol:      "tcp",
				HostIP:        "0.0.0.0",
			},
		},
		Size: &define.ContainerSize{
			RootFsSize: 1024,
			RwSize:     512,
		},
		Networks: []string{"bridge", "host"},
		Mounts:   []string{"/var/lib/docker", "/tmp"},
	}

	engine := &fakeContainerEngine{
		listFn: func(entities.ContainerListOptions) ([]entities.ListContainer, error) {
			return []entities.ListContainer{testContainer}, nil
		},
	}

	rr := listRequest(newTestHandler(engine).List, "")

	assert.Equal(t, http.StatusOK, rr.Code, "Should return 200 OK")

	var result ContainerListResult
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &result), "Should be able to parse response")

	assert.Equal(t, 1, result.Total, "Should return 1 container")
	container := result.Containers[0]

	// Verify all optional fields are mapped correctly
	assert.Equal(t, "sha256:abc123", container.ImageID, "Should map image ID")
	assert.Equal(t, []string{"/bin/sh", "-c", "echo hello"}, container.Command, "Should map command")
	assert.Equal(t, map[string]string{"app": "test", "env": "development", "version": "1.0"}, container.Labels, "Should map labels")
	assert.Equal(t, 1, len(container.Ports), "Should map ports")
	assert.Equal(t, 8080, container.Ports[0].ContainerPort, "Should map container port")
	assert.Equal(t, "tcp", container.Ports[0].Protocol, "Should map protocol")
	assert.NotNil(t, container.Size, "Should map size")
	assert.Equal(t, int64(1024), container.Size.RootFsSize, "Should map root FS size")
	assert.Equal(t, []string{"bridge", "host"}, container.Networks, "Should map networks")
	assert.Equal(t, []string{"/var/lib/docker", "/tmp"}, container.Mounts, "Should map mounts")
}
