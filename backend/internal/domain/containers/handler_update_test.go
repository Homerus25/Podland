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
	"go.podman.io/podman/v6/pkg/domain/entities"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// updateRequest serves POST
// /api/machines/{id}/containers/{containerId}/update through the handler
// with an authenticated test identity.
func updateRequest(handler http.HandlerFunc, containerID, body string) *httptest.ResponseRecorder {
	return testutils.Perform(handler, testutils.Request{
		Method:    http.MethodPost,
		Path:      "/api/machines/test-machine/containers/" + containerID + "/update",
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-machine", "containerId": containerID},
		Body:      strings.NewReader(body),
	})
}

// TestContainerHandler_Update_Success pins the update contract through
// the real service: 200 with the same detail DTO the GET answers, now
// carrying the new limits, and the request's fields reaching the engine.
func TestContainerHandler_Update_Success(t *testing.T) {
	var gotID string
	var gotOpts *entities.ContainerUpdateOptions
	engine := &fakeContainerEngine{
		updateFn: func(id string, opts *entities.ContainerUpdateOptions) error {
			gotID, gotOpts = id, opts
			return nil
		},
		inspectFn: updatedInspect(),
	}

	w := updateRequest(newTestHandler(engine).Update, "container-123",
		`{"cpus":1.5,"memory":"512m","healthcheck":{"cmd":"curl -f http://localhost/health","interval":"30s","retries":3}}`)

	require.Equal(t, http.StatusOK, w.Code, "Should return 200 on success")
	assert.Equal(t, "container-123", gotID, "the URL param must be the updated container")
	require.NotNil(t, gotOpts)
	require.NotNil(t, gotOpts.Resources)
	require.NotNil(t, gotOpts.Resources.CPU)
	assert.Equal(t, int64(150000), *gotOpts.Resources.CPU.Quota, "the body's cpus must reach the engine")
	require.NotNil(t, gotOpts.Resources.Memory)
	assert.Equal(t, int64(512*1024*1024), *gotOpts.Resources.Memory.Limit, "the body's memory must reach the engine")
	changed := gotOpts.ChangedHealthCheckConfiguration
	require.NotNil(t, changed, "the body's healthcheck must reach the engine")
	require.NotNil(t, changed.HealthCmd)
	assert.Equal(t, "curl -f http://localhost/health", *changed.HealthCmd)
	require.NotNil(t, changed.HealthInterval)
	assert.Equal(t, "30s", *changed.HealthInterval)
	require.NotNil(t, changed.HealthRetries)
	assert.Equal(t, uint(3), *changed.HealthRetries)

	var got ContainerGetResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "container-123", got.ID, "the response is the refreshed detail DTO")
}

// TestContainerHandler_Update_PatchOmitsUnchanged pins the patch
// semantics through the HTTP surface: a request that omits a field
// leaves that aspect unchanged — the engine sees no resource limits and
// no healthcheck change for the omitted kinds.
func TestContainerHandler_Update_PatchOmitsUnchanged(t *testing.T) {
	var gotOpts *entities.ContainerUpdateOptions
	engine := &fakeContainerEngine{
		updateFn: func(id string, opts *entities.ContainerUpdateOptions) error {
			gotOpts = opts
			return nil
		},
		inspectFn: updatedInspect(),
	}

	w := updateRequest(newTestHandler(engine).Update, "container-123", `{"memory":"1g"}`)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, gotOpts)
	assert.Nil(t, gotOpts.Resources.CPU, "an omitted cpus must leave the CPU limit unchanged")
	require.NotNil(t, gotOpts.Resources.Memory)
	assert.Nil(t, gotOpts.ChangedHealthCheckConfiguration, "an omitted healthcheck must leave the healthcheck unchanged")
}

// TestContainerHandler_Update_MalformedBodyAnswers400 pins the decode
// half of the shared vocabulary: an unparseable body answers
// INVALID_REQUEST.
func TestContainerHandler_Update_MalformedBodyAnswers400(t *testing.T) {
	// The engine stays unarmed: a decode failure must not acquire the
	// lease, and an unexpected call panics.
	engine := &fakeContainerEngine{}

	w := updateRequest(newTestHandler(engine).Update, "container-123", `{"cpus":`)

	require.Equal(t, http.StatusBadRequest, w.Code)
	var got appErrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "INVALID_REQUEST", got.Error.Code)
	assert.Equal(t, "Invalid request body", got.Error.Message)
}

// TestContainerHandler_Update_InvalidMemoryAnswers400 pins the
// documented 400 through the HTTP surface: an unparseable memory value
// answers VALIDATION_FAILED with a message naming the field, never
// reaching the engine.
func TestContainerHandler_Update_InvalidMemoryAnswers400(t *testing.T) {
	engine := &fakeContainerEngine{}

	w := updateRequest(newTestHandler(engine).Update, "container-123", `{"memory":"not-a-size"}`)

	require.Equal(t, http.StatusBadRequest, w.Code)
	var got appErrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "VALIDATION_FAILED", got.Error.Code)
	assert.Contains(t, got.Error.Message, "memory", "the message must name the field")
}

// TestContainerHandler_Update_InvalidDurationAnswers400 pins the
// documented 400 through the HTTP surface: a malformed healthcheck
// duration answers VALIDATION_FAILED with a message naming the field.
func TestContainerHandler_Update_InvalidDurationAnswers400(t *testing.T) {
	engine := &fakeContainerEngine{}

	w := updateRequest(newTestHandler(engine).Update, "container-123", `{"healthcheck":{"interval":"not-a-duration"}}`)

	require.Equal(t, http.StatusBadRequest, w.Code)
	var got appErrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "VALIDATION_FAILED", got.Error.Code)
	assert.Contains(t, got.Error.Message, "interval", "the message must name the field")
}

// TestContainerHandler_Update_NonPositiveCpusAnswers400 pins the
// documented 400 through the HTTP surface: a cpus value that is not a
// positive number fails the request's struct tag at decode time
// (web.Decode, per the create-builder settlement) and answers
// VALIDATION_FAILED, never reaching the engine.
func TestContainerHandler_Update_NonPositiveCpusAnswers400(t *testing.T) {
	engine := &fakeContainerEngine{}

	w := updateRequest(newTestHandler(engine).Update, "container-123", `{"cpus":-1.5}`)

	require.Equal(t, http.StatusBadRequest, w.Code)
	var got appErrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "VALIDATION_FAILED", got.Error.Code)
	assert.Contains(t, got.Error.Message, "CPUs", "the validator's message must name the field")
}

// TestContainerHandler_Update_DisableFlagExclusiveAnswers400 pins the
// documented 400 through the HTTP surface: the disable flag combined
// with any other healthcheck field answers VALIDATION_FAILED without
// calling podman.
func TestContainerHandler_Update_DisableFlagExclusiveAnswers400(t *testing.T) {
	// The engine stays unarmed: the exclusivity guard must answer
	// before any engine call, and an unexpected call panics.
	engine := &fakeContainerEngine{}

	w := updateRequest(newTestHandler(engine).Update, "container-123",
		`{"healthcheck":{"disabled":true,"cmd":"true"}}`)

	require.Equal(t, http.StatusBadRequest, w.Code)
	var got appErrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "VALIDATION_FAILED", got.Error.Code)
	assert.Contains(t, got.Error.Message, "healthcheck", "the message must name the field group")
}

// TestContainerHandler_Update_MissingContainerAnswers404 pins the
// documented 404 through the HTTP surface: the same not-found envelope
// as start/stop.
func TestContainerHandler_Update_MissingContainerAnswers404(t *testing.T) {
	engine := &fakeContainerEngine{
		updateFn: func(string, *entities.ContainerUpdateOptions) error {
			return &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such container"}
		},
		// inspectFn stays unarmed: the not-found predicate answers first.
	}

	w := updateRequest(newTestHandler(engine).Update, "container-123", `{"memory":"512m"}`)

	require.Equal(t, http.StatusNotFound, w.Code, "a missing container must answer 404, not 500")
	var got appErrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "CONTAINER_NOT_FOUND", got.Error.Code)
	assert.Equal(t, "Container not found", got.Error.Message)
}

// TestContainerHandler_Update_FailureAnswers500 pins that an unrelated
// update failure is classified through the HTTP surface: a failed
// update must not answer 200 with a stale detail DTO.
func TestContainerHandler_Update_FailureAnswers500(t *testing.T) {
	engine := &fakeContainerEngine{
		updateFn: func(string, *entities.ContainerUpdateOptions) error { return assert.AnError },
		// inspectFn stays unarmed: an unrelated failure must not probe.
	}

	w := updateRequest(newTestHandler(engine).Update, "container-123", `{"memory":"512m"}`)

	require.Equal(t, http.StatusInternalServerError, w.Code, "a failed update must not answer 200")
	var got appErrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "INTERNAL_ERROR", got.Error.Code)
}

// TestContainerHandler_Update_MissingContainerIDAnswers400 pins the
// route-param guard: an empty containerId answers the shared
// MISSING_CONTAINER_ID 400 before any decode or engine call.
func TestContainerHandler_Update_MissingContainerIDAnswers400(t *testing.T) {
	engine := &fakeContainerEngine{}

	w := updateRequest(newTestHandler(engine).Update, "", `{"memory":"512m"}`)

	require.Equal(t, http.StatusBadRequest, w.Code)
	var got appErrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "MISSING_CONTAINER_ID", got.Error.Code)
}
