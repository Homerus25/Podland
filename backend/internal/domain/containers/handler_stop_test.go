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

// TestContainerHandler_Stop_Success pins the stop contract through the
// real service: 200 with the stopped container's ID, its inspect-resolved
// name, and the stopped state.
func TestContainerHandler_Stop_Success(t *testing.T) {
	var gotID string
	engine := &fakeContainerEngine{
		stopFn: func(id string) ([]*entities.StopReport, error) {
			gotID = id
			return []*entities.StopReport{{Id: id}}, nil
		},
		inspectFn: func(id string) ([]*entities.ContainerInspectReport, []error, error) {
			return []*entities.ContainerInspectReport{{
				InspectContainerData: &define.InspectContainerData{Name: "/test-container"},
			}}, nil, nil
		},
	}

	req := httptest.NewRequest("POST", "/api/machines/test-machine/containers/container-123/stop", nil).
		WithContext(authedContext())

	w := httptest.NewRecorder()
	r := chi.NewRouter()
	r.Post("/api/machines/{id}/containers/{containerId}/stop", newTestHandler(engine).Stop)
	r.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code, "Should return 200 on success")
	assert.Equal(t, "container-123", gotID, "the URL param must be the stopped container")
	var got ContainerStopResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "container-123", got.ID)
	assert.Equal(t, "test-container", got.Name, "the inspect-resolved name must be reported")
	assert.Equal(t, "stopped", got.State)
	assert.NotEmpty(t, got.Timestamp)
}
