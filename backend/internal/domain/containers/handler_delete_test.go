package containers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
)

func newTestHandler(engine *fakeContainerEngine) *ContainerHandler {
	return NewContainerHandler(NewContainerService(fakeEngineProvider{engine: engine}))
}

// deleteRequest serves DELETE
// /api/machines/{id}/containers/{containerId}{query} through the handler
// with an authenticated test identity.
func deleteRequest(handler http.HandlerFunc, containerID, query string) *httptest.ResponseRecorder {
	return testutils.Perform(handler, testutils.Request{
		Method:    http.MethodDelete,
		Path:      "/api/machines/test-machine/containers/" + containerID + query,
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-machine", "containerId": containerID},
	})
}

// TestContainerHandler_Delete_Success tests successful container deletion,
// with the force flag reaching the machine parsed.
func TestContainerHandler_Delete_Success(t *testing.T) {
	var gotID string
	var gotForce bool
	engine := &fakeContainerEngine{
		removeFn: func(id string, force bool) error {
			gotID, gotForce = id, force
			return nil
		},
	}

	w := deleteRequest(newTestHandler(engine).Delete, "container-123", "")

	assert.Equal(t, http.StatusNoContent, w.Code, "Should return 204 on success")
	assert.Equal(t, "container-123", gotID)
	assert.False(t, gotForce, "a missing force param must delete unforced")
}

// TestContainerHandler_Delete_Force tests deletion with force query parameter
func TestContainerHandler_Delete_Force(t *testing.T) {
	var gotForce bool
	engine := &fakeContainerEngine{
		removeFn: func(id string, force bool) error {
			gotForce = force
			return nil
		},
	}

	w := deleteRequest(newTestHandler(engine).Delete, "container-123", "?force=true")

	assert.Equal(t, http.StatusNoContent, w.Code, "Should return 204 on success with force=true")
	assert.True(t, gotForce, "force=true must reach the machine")
}
