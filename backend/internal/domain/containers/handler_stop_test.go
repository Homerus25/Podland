package containers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/libpod/define"
	"go.podman.io/podman/v6/pkg/domain/entities"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// TestContainerHandler_Stop_Success pins the stop contract through the
// real service: 200 with the stopped container's ID, its inspect-resolved
// name, and the stopped state.
func TestContainerHandler_Stop_Success(t *testing.T) {
	var gotID string
	engine := &fakeContainerEngine{
		inspectFn: func(id string) ([]*entities.ContainerInspectReport, []error, error) {
			return []*entities.ContainerInspectReport{{
				InspectContainerData: &define.InspectContainerData{
					Name:  "/test-container",
					State: &define.InspectContainerState{Status: "running", Running: true},
				},
			}}, nil, nil
		},
		stopFn: func(id string) ([]*entities.StopReport, error) {
			gotID = id
			return []*entities.StopReport{{Id: id}}, nil
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

// TestContainerHandler_Stop_NotRunningAnswers400 pins the documented
// 400 terminal end to end: stopping a stopped container surfaces as
// the CONTAINER_NOT_RUNNING envelope, not a lying 200 "stopped" and
// not a 500.
func TestContainerHandler_Stop_NotRunningAnswers400(t *testing.T) {
	engine := &fakeContainerEngine{
		inspectFn: func(id string) ([]*entities.ContainerInspectReport, []error, error) {
			return []*entities.ContainerInspectReport{{
				InspectContainerData: &define.InspectContainerData{
					Name:  "/test-container",
					State: &define.InspectContainerState{Status: "exited", Running: false},
				},
			}}, nil, nil
		},
		// stopFn stays unarmed: the precondition must answer
		// before the action call, and an unexpected call panics.
	}

	req := httptest.NewRequest("POST", "/api/machines/test-machine/containers/container-123/stop", nil).
		WithContext(authedContext())

	w := httptest.NewRecorder()
	r := chi.NewRouter()
	r.Post("/api/machines/{id}/containers/{containerId}/stop", newTestHandler(engine).Stop)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, "a stopped container must answer 400, not 200")
	var got appErrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "test-request-id", got.RequestID)
	assert.Equal(t, "CONTAINER_NOT_RUNNING", got.Error.Code)
	assert.Equal(t, "Container is not running", got.Error.Message)
}

// TestContainerHandler_Stop_MissingContainerAnswers404 pins the
// documented 404 through the HTTP surface: podman's not-found answer
// surfaces as the CONTAINER_NOT_FOUND envelope, not a 500.
func TestContainerHandler_Stop_MissingContainerAnswers404(t *testing.T) {
	engine := &fakeContainerEngine{
		inspectFn: func(id string) ([]*entities.ContainerInspectReport, []error, error) {
			return []*entities.ContainerInspectReport{{
				InspectContainerData: &define.InspectContainerData{
					Name:  "/test-container",
					State: &define.InspectContainerState{Status: "running", Running: true},
				},
			}}, nil, nil
		},
		stopFn: func(string) ([]*entities.StopReport, error) {
			return nil, &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such container"}
		},
	}

	req := httptest.NewRequest("POST", "/api/machines/test-machine/containers/container-123/stop", nil).
		WithContext(authedContext())

	w := httptest.NewRecorder()
	r := chi.NewRouter()
	r.Post("/api/machines/{id}/containers/{containerId}/stop", newTestHandler(engine).Stop)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, "a missing container must answer 404, not 500")
	var got appErrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "CONTAINER_NOT_FOUND", got.Error.Code)
	assert.Equal(t, "Container not found", got.Error.Message)
}

// TestContainerHandler_Stop_PerItemFailureAnswers500 pins that a
// per-item report error is classified through the HTTP surface — a
// failed stop must not answer state "stopped".
func TestContainerHandler_Stop_PerItemFailureAnswers500(t *testing.T) {
	engine := &fakeContainerEngine{
		inspectFn: func(id string) ([]*entities.ContainerInspectReport, []error, error) {
			return []*entities.ContainerInspectReport{{
				InspectContainerData: &define.InspectContainerData{
					Name:  "/test-container",
					State: &define.InspectContainerState{Status: "running", Running: true},
				},
			}}, nil, nil
		},
		stopFn: func(id string) ([]*entities.StopReport, error) {
			return []*entities.StopReport{{Id: id, Err: assert.AnError}}, nil
		},
	}

	req := httptest.NewRequest("POST", "/api/machines/test-machine/containers/container-123/stop", nil).
		WithContext(authedContext())

	w := httptest.NewRecorder()
	r := chi.NewRouter()
	r.Post("/api/machines/{id}/containers/{containerId}/stop", newTestHandler(engine).Stop)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code, "a failed stop must not answer state stopped")
	var got appErrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "INTERNAL_ERROR", got.Error.Code)
}
