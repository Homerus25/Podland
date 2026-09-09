package containers

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/libpod/define"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// TestContainerHandler_Start_Success pins the start contract through the
// real service: 200 with the started container's ID, its inspect-resolved
// name, and the running state.
func TestContainerHandler_Start_Success(t *testing.T) {
	var gotID string
	engine := &fakeContainerEngine{
		startFn: func(id string) ([]*entities.ContainerStartReport, error) {
			gotID = id
			return []*entities.ContainerStartReport{{Id: id}}, nil
		},
		inspectFn: func(id string) ([]*entities.ContainerInspectReport, []error, error) {
			return []*entities.ContainerInspectReport{{
				InspectContainerData: &define.InspectContainerData{Name: "/test-container"},
			}}, nil, nil
		},
	}

	req := httptest.NewRequest("POST", "/api/machines/test-machine/containers/container-123/start", nil).
		WithContext(authedContext())

	w := httptest.NewRecorder()
	r := chi.NewRouter()
	r.Post("/api/machines/{id}/containers/{containerId}/start", newTestHandler(engine).Start)
	r.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code, "Should return 200 on success")
	assert.Equal(t, "container-123", gotID, "the URL param must be the started container")
	var got ContainerStartResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "container-123", got.ID)
	assert.Equal(t, "test-container", got.Name, "the inspect-resolved name must be reported")
	assert.Equal(t, "running", got.State)
	assert.NotEmpty(t, got.Timestamp)
}
