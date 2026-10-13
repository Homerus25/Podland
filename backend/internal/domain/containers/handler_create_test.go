package containers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/specgen"
)

// createRequest serves POST /api/machines/{id}/containers with the given
// JSON body through the handler with an authenticated test identity.
func createRequest(create http.HandlerFunc, body string) *httptest.ResponseRecorder {
	return testutils.Perform(create, testutils.Request{
		Method:    http.MethodPost,
		Path:      "/api/machines/test-machine/containers",
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-machine"},
		Body:      strings.NewReader(body),
	})
}

// TestContainerHandler_Create_Success pins the create contract through the
// real service: 201 with the created container's ID, name, state, and
// image, and the request decoded into the machine's spec.
func TestContainerHandler_Create_Success(t *testing.T) {
	requestBody := `{
		"name": "test-container",
		"image": "nginx:latest",
		"command": ["/bin/sh", "-c", "echo hello"],
		"environment": {
			"ENV1": "value1"
		},
		"labels": {
			"app": "test"
		}
	}`

	var gotImage string
	var gotName string
	var gotCommand []string
	var gotEnv map[string]string
	var gotLabels map[string]string
	engine := &fakeContainerEngine{
		createFn: func(spec *specgen.SpecGenerator) (*entities.ContainerCreateReport, error) {
			gotImage = spec.Image
			gotName = spec.Name
			gotCommand = spec.Command
			gotEnv = spec.Env
			gotLabels = spec.Labels
			return &entities.ContainerCreateReport{Id: "container-123"}, nil
		},
	}

	rr := createRequest(newTestHandler(engine).Create, requestBody)

	assert.Equal(t, http.StatusCreated, rr.Code, "Should return 201 Created for successful container creation")
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"), "Should return JSON content type")

	assert.Equal(t, "nginx:latest", gotImage, "the request image must reach the machine's spec")
	assert.Equal(t, "test-container", gotName, "the request name must reach the machine's spec")
	assert.Equal(t, []string{"/bin/sh", "-c", "echo hello"}, gotCommand, "the request command must reach the machine's spec")
	assert.Equal(t, map[string]string{"ENV1": "value1"}, gotEnv, "the request environment must reach the machine's spec")
	assert.Equal(t, map[string]string{"app": "test"}, gotLabels, "the request labels must reach the machine's spec")

	var result ContainerCreateResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &result), "Should be able to parse JSON response")
	assert.Equal(t, "container-123", result.ID, "Should return correct container ID")
	assert.Equal(t, "test-container", result.Name, "Should return correct container name")
	assert.Equal(t, "created", result.State, "Should return correct container state")
	assert.Equal(t, "nginx:latest", result.Image, "Should return correct container image")
	assert.NotEmpty(t, result.CreatedAt, "Should include creation timestamp")
}

// TestContainerHandler_Create_MissingImage pins the validation guard: a
// request without an image is refused with 400 before the machine is asked
// (the engine stays unarmed and would panic on a call).
func TestContainerHandler_Create_MissingImage(t *testing.T) {
	engine := &fakeContainerEngine{}

	rr := createRequest(newTestHandler(engine).Create, `{"name":"x"}`)

	assert.Equal(t, http.StatusBadRequest, rr.Code, "a create without an image must be refused with 400")
	assert.Contains(t, rr.Body.String(), "VALIDATION_FAILED")
}
