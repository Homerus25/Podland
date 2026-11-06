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
	"go.podman.io/podman/v6/pkg/domain/entities"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// lifecycleHandlerRow is one action declaration under test at the HTTP
// seam: the handler arm, the route suffix, the fake engine's arm, the
// inspect answers whose state passes and fails the action's gate, and
// the per-action wire pins (the 200 state string, the 400 envelope).
// The expected values come from the doku contract, not from the
// declarations under test.
type lifecycleHandlerRow struct {
	name  string
	route string
	// handle is the handler arm the row exercises.
	handle func(h *ContainerHandler) http.HandlerFunc
	// arm arms the fake engine's action method.
	arm func(e *fakeContainerEngine, fn func(id string) error)
	// passingInspect is the pre-action inspect answer whose state
	// satisfies the action's gate.
	passingInspect func(string) ([]*entities.ContainerInspectReport, []error, error)
	// failingInspect is the answer whose state fails the gate; the
	// engine's action arm stays unarmed for it, so an action call
	// after the gate fails the test loudly.
	failingInspect func(string) ([]*entities.ContainerInspectReport, []error, error)
	// postState is the state string the 200 response stamps for this
	// action (running for start/restart, stopped for stop).
	postState string
	// wrongStateCode and wrongStateMessage are the 400 envelope the
	// gate's sentinel maps to.
	wrongStateCode    string
	wrongStateMessage string
}

func lifecycleHandlerTable() []lifecycleHandlerRow {
	return []lifecycleHandlerRow{
		{
			name:              "start",
			route:             "start",
			handle:            func(h *ContainerHandler) http.HandlerFunc { return h.Start },
			arm:               func(e *fakeContainerEngine, fn func(id string) error) { e.startFn = fn },
			passingInspect:    lifecycleInspectAnswer("/test-container", lifecycleStoppedState),
			failingInspect:    lifecycleInspectAnswer("/test-container", lifecycleRunningState),
			postState:         "running",
			wrongStateCode:    "CONTAINER_ALREADY_RUNNING",
			wrongStateMessage: "Container is already running",
		},
		{
			name:              "stop",
			route:             "stop",
			handle:            func(h *ContainerHandler) http.HandlerFunc { return h.Stop },
			arm:               func(e *fakeContainerEngine, fn func(id string) error) { e.stopFn = fn },
			passingInspect:    lifecycleInspectAnswer("/test-container", lifecycleRunningState),
			failingInspect:    lifecycleInspectAnswer("/test-container", lifecycleStoppedState),
			postState:         "stopped",
			wrongStateCode:    "CONTAINER_NOT_RUNNING",
			wrongStateMessage: "Container is not running",
		},
		{
			name:              "restart",
			route:             "restart",
			handle:            func(h *ContainerHandler) http.HandlerFunc { return h.Restart },
			arm:               func(e *fakeContainerEngine, fn func(id string) error) { e.restartFn = fn },
			passingInspect:    lifecycleInspectAnswer("/test-container", lifecycleRunningState),
			failingInspect:    lifecycleInspectAnswer("/test-container", lifecycleStoppedState),
			postState:         "running",
			wrongStateCode:    "CONTAINER_NOT_RUNNING",
			wrongStateMessage: "Container is not running",
		},
	}
}

// lifecycleRequest serves POST
// /api/hosts/{id}/containers/{containerId}/{route} through the handler
// with an authenticated test identity.
func lifecycleRequest(handler http.HandlerFunc, route, containerID string) *httptest.ResponseRecorder {
	return testutils.Perform(handler, testutils.Request{
		Method:    http.MethodPost,
		Path:      "/api/hosts/test-host/containers/" + containerID + "/" + route,
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-host", "containerId": containerID},
	})
}

// TestContainerHandler_Lifecycle_Success pins the 200 contract through
// the real service: the started/stopped/restarted container's ID, its
// inspect-resolved name, the action's post-action state string, and a
// stamped timestamp.
func TestContainerHandler_Lifecycle_Success(t *testing.T) {
	for _, row := range lifecycleHandlerTable() {
		t.Run(row.name, func(t *testing.T) {
			var gotID string
			engine := &fakeContainerEngine{inspectFn: row.passingInspect}
			row.arm(engine, func(id string) error {
				gotID = id
				return nil
			})

			w := lifecycleRequest(row.handle(newTestHandler(engine)), row.route, "container-123")

			require.Equal(t, 200, w.Code, "Should return 200 on success")
			assert.Equal(t, "container-123", gotID, "the URL param must be the acted-on container")
			var got ContainerLifecycleResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
			assert.Equal(t, "container-123", got.ID)
			assert.Equal(t, "test-container", got.Name, "the inspect-resolved name must be reported")
			assert.Equal(t, row.postState, got.State)
			assert.NotEmpty(t, got.Timestamp)
		})
	}
}

// TestContainerHandler_Lifecycle_WrongStateAnswers400 pins the
// documented 400 terminal end to end: the gate's sentinel surfaces as
// the action's wrong-state envelope, not a lying 200 and not a 500.
// The engine's action arm stays unarmed: an unexpected call panics.
func TestContainerHandler_Lifecycle_WrongStateAnswers400(t *testing.T) {
	for _, row := range lifecycleHandlerTable() {
		t.Run(row.name, func(t *testing.T) {
			engine := &fakeContainerEngine{inspectFn: row.failingInspect}

			w := lifecycleRequest(row.handle(newTestHandler(engine)), row.route, "container-123")

			require.Equal(t, http.StatusBadRequest, w.Code, "the wrong state must answer 400, not 200")
			var got appErrors.ErrorResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
			assert.Equal(t, "test-request-id", got.RequestID)
			assert.Equal(t, row.wrongStateCode, got.Error.Code)
			assert.Equal(t, row.wrongStateMessage, got.Error.Message)
		})
	}
}

// TestContainerHandler_Lifecycle_MissingContainerAnswers404 pins the
// documented 404 through the HTTP surface: podman's not-found answer
// on the action call surfaces as the CONTAINER_NOT_FOUND envelope,
// not a 500.
func TestContainerHandler_Lifecycle_MissingContainerAnswers404(t *testing.T) {
	for _, row := range lifecycleHandlerTable() {
		t.Run(row.name, func(t *testing.T) {
			engine := &fakeContainerEngine{inspectFn: row.passingInspect}
			row.arm(engine, func(string) error {
				return &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such container"}
			})

			w := lifecycleRequest(row.handle(newTestHandler(engine)), row.route, "container-123")

			require.Equal(t, http.StatusNotFound, w.Code, "a missing container must answer 404, not 500")
			var got appErrors.ErrorResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
			assert.Equal(t, "CONTAINER_NOT_FOUND", got.Error.Code)
			assert.Equal(t, "Container not found", got.Error.Message)
		})
	}
}

// TestContainerHandler_Lifecycle_FailureAnswers500 pins that a failed
// action is classified through the HTTP surface — a failed action must
// not answer a post-action success body.
func TestContainerHandler_Lifecycle_FailureAnswers500(t *testing.T) {
	for _, row := range lifecycleHandlerTable() {
		t.Run(row.name, func(t *testing.T) {
			engine := &fakeContainerEngine{inspectFn: row.passingInspect}
			row.arm(engine, func(string) error { return assert.AnError })

			w := lifecycleRequest(row.handle(newTestHandler(engine)), row.route, "container-123")

			require.Equal(t, http.StatusInternalServerError, w.Code, "a failed action must not answer a success body")
			var got appErrors.ErrorResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
			assert.Equal(t, "INTERNAL_ERROR", got.Error.Code)
		})
	}
}
