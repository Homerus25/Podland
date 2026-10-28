package pods

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"

	"github.com/Homerus25/podland/backend/internal/testutils"
)

// actionRequest serves POST /api/machines/{id}/pods/{podId}/{action}
// through the handler with an authenticated test identity.
func actionRequest(action http.HandlerFunc, podID, actionName string) *httptest.ResponseRecorder {
	return testutils.Perform(action, testutils.Request{
		Method:    http.MethodPost,
		Path:      "/api/machines/test-machine/pods/" + podID + "/" + actionName,
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-machine", "podId": podID},
	})
}

// TestPodHandler_Pause_Success pins the action contract: 200 with the
// pod's ID, the post-action state stamped at mapping time, and a
// timestamp.
func TestPodHandler_Pause_Success(t *testing.T) {
	var gotName string
	engine := &fakePodEngine{
		pauseFn: func(nameOrID string) error {
			gotName = nameOrID
			return nil
		},
	}

	w := actionRequest(newTestHandler(engine).Pause, "web", "pause")

	require.Equal(t, http.StatusOK, w.Code, "pause should return 200")
	assert.Equal(t, "web", gotName, "the URL-param pod must reach the machine")
	var body PodActionResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), "the top-level body must be the action DTO")
	assert.Equal(t, "web", body.ID)
	assert.Equal(t, "paused", body.State)
	assert.NotEmpty(t, body.Timestamp)
}

// TestPodHandler_ActionStates pins each action's stamped post-action
// state: start, unpause and restart answer running, stop answers
// stopped, kill answers exited.
func TestPodHandler_ActionStates(t *testing.T) {
	tests := []struct {
		name      string
		handler   func(h *PodHandler) http.HandlerFunc
		arm       func(e *fakePodEngine)
		wantState string
	}{
		{
			name:      "start",
			handler:   func(h *PodHandler) http.HandlerFunc { return h.Start },
			arm:       func(e *fakePodEngine) { e.startFn = func(string) error { return nil } },
			wantState: "running",
		},
		{
			name:      "stop",
			handler:   func(h *PodHandler) http.HandlerFunc { return h.Stop },
			arm:       func(e *fakePodEngine) { e.stopFn = func(string) error { return nil } },
			wantState: "stopped",
		},
		{
			name:      "unpause",
			handler:   func(h *PodHandler) http.HandlerFunc { return h.Unpause },
			arm:       func(e *fakePodEngine) { e.unpauseFn = func(string) error { return nil } },
			wantState: "running",
		},
		{
			name:      "kill",
			handler:   func(h *PodHandler) http.HandlerFunc { return h.Kill },
			arm:       func(e *fakePodEngine) { e.killFn = func(string) error { return nil } },
			wantState: "exited",
		},
		{
			name:      "restart",
			handler:   func(h *PodHandler) http.HandlerFunc { return h.Restart },
			arm:       func(e *fakePodEngine) { e.restartFn = func(string) error { return nil } },
			wantState: "running",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := &fakePodEngine{}
			tt.arm(engine)

			w := actionRequest(tt.handler(newTestHandler(engine)), "web", tt.name)

			require.Equal(t, http.StatusOK, w.Code)
			var body PodActionResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			assert.Equal(t, "web", body.ID)
			assert.Equal(t, tt.wantState, body.State)
			assert.NotEmpty(t, body.Timestamp)
		})
	}
}

// removeRequest serves DELETE /api/machines/{id}/pods/{podId}{query}
// through the handler with an authenticated test identity.
func removeRequest(remove http.HandlerFunc, podID, query string) *httptest.ResponseRecorder {
	return testutils.Perform(remove, testutils.Request{
		Method:    http.MethodDelete,
		Path:      "/api/machines/test-machine/pods/" + podID + query,
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-machine", "podId": podID},
	})
}

// TestPodHandler_Remove_NoContent pins the remove contract: 204 with an
// empty body, and the force query flag crossing to the machine.
func TestPodHandler_Remove_NoContent(t *testing.T) {
	var gotName string
	var gotForce bool
	engine := &fakePodEngine{
		removeFn: func(nameOrID string, force bool) error {
			gotName, gotForce = nameOrID, force
			return nil
		},
	}

	w := removeRequest(newTestHandler(engine).Remove, "web", "?force=true")

	require.Equal(t, http.StatusNoContent, w.Code, "remove should return 204")
	assert.Equal(t, "web", gotName)
	assert.True(t, gotForce, "the force query flag must reach the machine")
}

// TestPodHandler_Remove_DefaultUnforced pins the force default: a
// missing force param removes unforced.
func TestPodHandler_Remove_DefaultUnforced(t *testing.T) {
	var gotForce bool
	engine := &fakePodEngine{
		removeFn: func(nameOrID string, force bool) error {
			gotForce = force
			return nil
		},
	}

	w := removeRequest(newTestHandler(engine).Remove, "web", "")

	require.Equal(t, http.StatusNoContent, w.Code)
	assert.False(t, gotForce, "a missing force param must remove unforced")
}

// TestPodHandler_Pause_MissingPodID pins the URL-param guard: a request
// without the podId route param answers 400 MISSING_POD_ID before the
// engine is reached.
func TestPodHandler_Pause_MissingPodID(t *testing.T) {
	engine := &fakePodEngine{} // pauseFn unarmed: the guard must fail first

	w := testutils.Perform(newTestHandler(engine).Pause, testutils.Request{
		Method:    http.MethodPost,
		Path:      "/api/machines/test-machine/pods//pause",
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-machine"},
	})

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "MISSING_POD_ID", testutils.DecodeErrorEnvelope(t, w.Result()).Error.Code)
}

// TestPodHandler_Pause_MissingPodAnswers404 pins the documented 404
// envelope end to end on a write route: the real service classifies the
// tunnel's not-found answer and the web envelope maps the sentinel.
func TestPodHandler_Pause_MissingPodAnswers404(t *testing.T) {
	engine := &fakePodEngine{
		pauseFn: func(string) error {
			return &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "unable to find pod"}
		},
	}

	w := actionRequest(newTestHandler(engine).Pause, "ghost-pod", "pause")

	require.Equal(t, http.StatusNotFound, w.Code, "a missing pod must answer 404")
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "POD_NOT_FOUND", body.Error.Code)
}

// pruneRequest serves POST /api/machines/{id}/pods/prune through the
// handler with an authenticated test identity.
func pruneRequest(prune http.HandlerFunc) *httptest.ResponseRecorder {
	return testutils.Perform(prune, testutils.Request{
		Method:    http.MethodPost,
		Path:      "/api/machines/test-machine/pods/prune",
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-machine"},
	})
}

// TestPodHandler_Prune_Success pins the prune contract: 200 with one
// report entry per removed pod, and per-pod errors carried in the
// entry.
func TestPodHandler_Prune_Success(t *testing.T) {
	engine := &fakePodEngine{
		pruneFn: func() ([]*entities.PodPruneReport, error) {
			return []*entities.PodPruneReport{
				{Id: "stopped-a"},
				{Id: "stopped-b", Err: assert.AnError},
			}, nil
		},
	}

	w := pruneRequest(newTestHandler(engine).Prune)

	assert.Equal(t, http.StatusOK, w.Code, "prune should return 200")
	var got []PodPruneResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got), "response must decode as a report array")
	require.Len(t, got, 2, "each removed pod must have a report entry")
	assert.Equal(t, "stopped-a", got[0].ID)
	assert.Empty(t, got[0].Error)
	assert.Equal(t, "stopped-b", got[1].ID)
	assert.NotEmpty(t, got[1].Error, "per-pod error must be carried")
}

// TestPodHandler_Prune_EmptyReport pins that pruning with nothing to
// remove still returns 200 with an empty JSON array (not nil/null).
func TestPodHandler_Prune_EmptyReport(t *testing.T) {
	engine := &fakePodEngine{
		pruneFn: func() ([]*entities.PodPruneReport, error) {
			return nil, nil
		},
	}

	w := pruneRequest(newTestHandler(engine).Prune)

	assert.Equal(t, http.StatusOK, w.Code)
	var got []PodPruneResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.NotNil(t, got, "response must decode as an empty array, not null")
	assert.Empty(t, got)
}
