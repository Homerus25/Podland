package containers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

func newTestHandler(engine *fakeContainerEngine) *ContainerHandler {
	return NewContainerHandler(NewContainerService(fakeEngineProvider{engine: engine}))
}

// deleteRequest serves DELETE
// /api/hosts/{id}/containers/{containerId}{query} through the handler
// with an authenticated test identity.
func deleteRequest(handler http.HandlerFunc, containerID, query string) *httptest.ResponseRecorder {
	return testutils.Perform(handler, testutils.Request{
		Method:    http.MethodDelete,
		Path:      "/api/hosts/test-host/containers/" + containerID + query,
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-host", "containerId": containerID},
	})
}

// TestContainerHandler_Delete_Success tests successful container deletion,
// with the force flag reaching the host parsed.
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
	assert.True(t, gotForce, "force=true must reach the host")
}

// TestContainerHandler_Delete_RunningRefusal pins the running-container
// refusal through the HTTP surface: 409 CONTAINER_RUNNING carrying podman's
// refusal text. The error shape is podman's real one — the compat handler
// answers a 500 whose message is the running-removal refusal, not a 409 — so
// this exercises the conflict phrase-match end to end, not just the
// response-code shortcut.
func TestContainerHandler_Delete_RunningRefusal(t *testing.T) {
	const refusal = "cannot remove container 3f2e1d0c as it is running - running or paused containers cannot be removed without force: container state improper"
	engine := &fakeContainerEngine{
		removeFn: func(string, bool) error {
			return &errorhandling.ErrorModel{
				ResponseCode: http.StatusInternalServerError,
				Message:      refusal,
				Because:      "container state improper",
			}
		},
	}

	w := deleteRequest(newTestHandler(engine).Delete, "container-123", "")
	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusConflict, resp.StatusCode)
	envelope := testutils.DecodeErrorEnvelope(t, resp)
	assert.Equal(t, "CONTAINER_RUNNING", envelope.Error.Code)
	assert.Contains(t, envelope.Error.Message, refusal,
		"podman's refusal text must reach the client")
}

// TestContainerHandler_Delete_NotFound pins the vanished-container branch
// through the HTTP surface: 404 CONTAINER_NOT_FOUND.
func TestContainerHandler_Delete_NotFound(t *testing.T) {
	engine := &fakeContainerEngine{
		removeFn: func(string, bool) error {
			return &errorhandling.ErrorModel{
				ResponseCode: http.StatusNotFound,
				Message:      "no such container",
			}
		},
	}

	w := deleteRequest(newTestHandler(engine).Delete, "ghost", "")
	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Equal(t, "CONTAINER_NOT_FOUND", testutils.DecodeErrorEnvelope(t, resp).Error.Code)
}
