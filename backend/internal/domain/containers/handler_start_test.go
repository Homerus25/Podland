package containers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/libpod/define"
	"go.podman.io/podman/v6/pkg/domain/entities"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// startRequest serves POST
// /api/hosts/{id}/containers/{containerId}/start through the handler
// with an authenticated test identity.
func startRequest(handler http.HandlerFunc, containerID string) *httptest.ResponseRecorder {
	return testutils.Perform(handler, testutils.Request{
		Method:    http.MethodPost,
		Path:      "/api/hosts/test-host/containers/" + containerID + "/start",
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-host", "containerId": containerID},
	})
}

// TestContainerHandler_Start_Success pins the start contract through the
// real service: 200 with the started container's ID, its inspect-resolved
// name, and the running state.
func TestContainerHandler_Start_Success(t *testing.T) {
	var gotID string
	engine := &fakeContainerEngine{
		inspectFn: func(id string) ([]*entities.ContainerInspectReport, []error, error) {
			return []*entities.ContainerInspectReport{{
				InspectContainerData: &define.InspectContainerData{
					Name:  "/test-container",
					State: &define.InspectContainerState{Status: "exited", Running: false},
				},
			}}, nil, nil
		},
		startFn: func(id string) error {
			gotID = id
			return nil
		},
	}

	w := startRequest(newTestHandler(engine).Start, "container-123")

	require.Equal(t, 200, w.Code, "Should return 200 on success")
	assert.Equal(t, "container-123", gotID, "the URL param must be the started container")
	var got ContainerStartResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "container-123", got.ID)
	assert.Equal(t, "test-container", got.Name, "the inspect-resolved name must be reported")
	assert.Equal(t, "running", got.State)
	assert.NotEmpty(t, got.Timestamp)
}

// TestContainerHandler_Start_AlreadyRunningAnswers400 pins the
// documented 400 terminal end to end: starting an already-running
// container surfaces as the CONTAINER_ALREADY_RUNNING envelope, not a
// 200 "running" and not a 500.
func TestContainerHandler_Start_AlreadyRunningAnswers400(t *testing.T) {
	engine := &fakeContainerEngine{
		inspectFn: func(id string) ([]*entities.ContainerInspectReport, []error, error) {
			return []*entities.ContainerInspectReport{{
				InspectContainerData: &define.InspectContainerData{
					Name:  "/test-container",
					State: &define.InspectContainerState{Status: "running", Running: true},
				},
			}}, nil, nil
		},
		// startFn stays unarmed: the precondition must answer
		// before the action call, and an unexpected call panics.
	}

	w := startRequest(newTestHandler(engine).Start, "container-123")

	require.Equal(t, http.StatusBadRequest, w.Code, "an already-running container must answer 400, not 200")
	var got appErrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "test-request-id", got.RequestID)
	assert.Equal(t, "CONTAINER_ALREADY_RUNNING", got.Error.Code)
	assert.Equal(t, "Container is already running", got.Error.Message)
}

// TestContainerHandler_Start_MissingContainerAnswers404 pins the
// documented 404 through the HTTP surface: podman's not-found answer
// surfaces as the CONTAINER_NOT_FOUND envelope, not a 500.
func TestContainerHandler_Start_MissingContainerAnswers404(t *testing.T) {
	engine := &fakeContainerEngine{
		inspectFn: func(id string) ([]*entities.ContainerInspectReport, []error, error) {
			return []*entities.ContainerInspectReport{{
				InspectContainerData: &define.InspectContainerData{
					Name:  "/test-container",
					State: &define.InspectContainerState{Status: "exited", Running: false},
				},
			}}, nil, nil
		},
		startFn: func(string) error {
			return &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such container"}
		},
	}

	w := startRequest(newTestHandler(engine).Start, "container-123")

	require.Equal(t, http.StatusNotFound, w.Code, "a missing container must answer 404, not 500")
	var got appErrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "CONTAINER_NOT_FOUND", got.Error.Code)
	assert.Equal(t, "Container not found", got.Error.Message)
}

// TestContainerHandler_Start_FailureAnswers500 pins that a failed start
// is classified through the HTTP surface — a failed start must not
// answer state "running".
func TestContainerHandler_Start_FailureAnswers500(t *testing.T) {
	engine := &fakeContainerEngine{
		inspectFn: func(id string) ([]*entities.ContainerInspectReport, []error, error) {
			return []*entities.ContainerInspectReport{{
				InspectContainerData: &define.InspectContainerData{
					Name:  "/test-container",
					State: &define.InspectContainerState{Status: "exited", Running: false},
				},
			}}, nil, nil
		},
		startFn: func(string) error { return assert.AnError },
	}

	w := startRequest(newTestHandler(engine).Start, "container-123")

	require.Equal(t, http.StatusInternalServerError, w.Code, "a failed start must not answer state running")
	var got appErrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "INTERNAL_ERROR", got.Error.Code)
}
