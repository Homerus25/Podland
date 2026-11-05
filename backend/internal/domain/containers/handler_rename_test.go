package containers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// renameRequest serves POST
// /api/hosts/{id}/containers/{containerId}/rename through the handler
// with an authenticated test identity.
func renameRequest(handler http.HandlerFunc, containerID, body string) *httptest.ResponseRecorder {
	return testutils.Perform(handler, testutils.Request{
		Method:    http.MethodPost,
		Path:      "/api/hosts/test-host/containers/" + containerID + "/rename",
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-host", "containerId": containerID},
		Body:      strings.NewReader(body),
	})
}

// TestContainerHandler_Rename_Success pins the rename contract through
// the real service: 200 with the same detail DTO the GET answers, now
// carrying the new name.
func TestContainerHandler_Rename_Success(t *testing.T) {
	var gotID, gotName string
	engine := &fakeContainerEngine{
		renameFn: func(id, newName string) error {
			gotID, gotName = id, newName
			return nil
		},
		inspectFn: renamedInspect("renamed"),
	}

	w := renameRequest(newTestHandler(engine).Rename, "container-123", `{"name":"renamed"}`)

	require.Equal(t, http.StatusOK, w.Code, "Should return 200 on success")
	assert.Equal(t, "container-123", gotID, "the URL param must be the renamed container")
	assert.Equal(t, "renamed", gotName, "the body must carry the new name")
	var got ContainerGetResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "renamed", got.Name, "the response is the detail DTO with the new name")
	assert.Equal(t, "container-123", got.ID)
	assert.Equal(t, "running", got.State)
}

// TestContainerHandler_Rename_EmptyNameAnswers400 pins the documented
// 400 through the shared decode vocabulary: an empty name fails the
// request's struct tag and answers VALIDATION_FAILED, never reaching the
// engine.
func TestContainerHandler_Rename_EmptyNameAnswers400(t *testing.T) {
	// The engine stays unarmed: a decode failure must not acquire the
	// lease, and an unexpected call panics.
	engine := &fakeContainerEngine{}

	w := renameRequest(newTestHandler(engine).Rename, "container-123", `{"name":""}`)

	require.Equal(t, http.StatusBadRequest, w.Code)
	var got appErrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "VALIDATION_FAILED", got.Error.Code)
}

// TestContainerHandler_Rename_MalformedBodyAnswers400 pins the decode
// half of the shared vocabulary: an unparseable body answers
// INVALID_REQUEST.
func TestContainerHandler_Rename_MalformedBodyAnswers400(t *testing.T) {
	engine := &fakeContainerEngine{}

	w := renameRequest(newTestHandler(engine).Rename, "container-123", `{"name":`)

	require.Equal(t, http.StatusBadRequest, w.Code)
	var got appErrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "INVALID_REQUEST", got.Error.Code)
	assert.Equal(t, "Invalid request body", got.Error.Message)
}

// TestContainerHandler_Rename_NameCollisionAnswers409 pins the
// documented 409 through the HTTP surface: a name already in use
// surfaces as the CONTAINER_ALREADY_EXISTS envelope, not a 500.
func TestContainerHandler_Rename_NameCollisionAnswers409(t *testing.T) {
	engine := &fakeContainerEngine{
		renameFn: func(string, string) error {
			return &errorhandling.ErrorModel{
				ResponseCode: http.StatusInternalServerError,
				Message:      "renaming container abc: UNIQUE constraint failed: ContainerConfig.Name",
			}
		},
		// inspectFn stays unarmed: the collision predicate answers first.
	}

	w := renameRequest(newTestHandler(engine).Rename, "container-123", `{"name":"taken"}`)

	require.Equal(t, http.StatusConflict, w.Code, "a taken name must answer 409, not 500")
	var got appErrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "CONTAINER_ALREADY_EXISTS", got.Error.Code)
	assert.Equal(t, "Container with this name already exists", got.Error.Message)
}

// TestContainerHandler_Rename_MissingContainerAnswers404 pins the
// documented 404 through the HTTP surface.
func TestContainerHandler_Rename_MissingContainerAnswers404(t *testing.T) {
	engine := &fakeContainerEngine{
		renameFn: func(string, string) error {
			return &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such container"}
		},
	}

	w := renameRequest(newTestHandler(engine).Rename, "container-123", `{"name":"renamed"}`)

	require.Equal(t, http.StatusNotFound, w.Code, "a missing container must answer 404, not 500")
	var got appErrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "CONTAINER_NOT_FOUND", got.Error.Code)
	assert.Equal(t, "Container not found", got.Error.Message)
}

// TestContainerHandler_Rename_FailureAnswers500 pins that an unrelated
// rename failure is classified through the HTTP surface: a failed
// rename must not answer 200 with a stale detail DTO.
func TestContainerHandler_Rename_FailureAnswers500(t *testing.T) {
	engine := &fakeContainerEngine{
		renameFn: func(string, string) error { return assert.AnError },
		// inspectFn stays unarmed: an unrelated failure must not probe.
	}

	w := renameRequest(newTestHandler(engine).Rename, "container-123", `{"name":"free"}`)

	require.Equal(t, http.StatusInternalServerError, w.Code, "a failed rename must not answer 200")
	var got appErrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "INTERNAL_ERROR", got.Error.Code)
}

// TestContainerHandler_Rename_MissingContainerIDAnswers400 pins the
// route-param guard: an empty containerId answers the shared
// MISSING_CONTAINER_ID 400 before any decode or engine call.
func TestContainerHandler_Rename_MissingContainerIDAnswers400(t *testing.T) {
	engine := &fakeContainerEngine{}

	w := renameRequest(newTestHandler(engine).Rename, "", `{"name":"renamed"}`)

	require.Equal(t, http.StatusBadRequest, w.Code)
	var got appErrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "MISSING_CONTAINER_ID", got.Error.Code)
}
