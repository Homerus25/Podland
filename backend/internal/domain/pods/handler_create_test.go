package pods

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/specgen"

	"github.com/Homerus25/podland/backend/internal/testutils"
)

// createRequest serves POST /api/machines/{id}/pods with the given JSON
// body through the handler with an authenticated test identity.
func createRequest(create http.HandlerFunc, body string) *httptest.ResponseRecorder {
	return testutils.Perform(create, testutils.Request{
		Method:    http.MethodPost,
		Path:      "/api/machines/test-machine/pods",
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-machine"},
		Body:      strings.NewReader(body),
	})
}

// TestPodHandler_Create_Success pins the create contract through the
// real service: 201 with the created pod's ID, name, state, and
// creation timestamp, and the request decoded into the machine's spec.
func TestPodHandler_Create_Success(t *testing.T) {
	var gotSpec *specgen.PodSpecGenerator
	engine := &fakePodEngine{
		createFn: func(spec entities.PodSpec) (*entities.PodCreateReport, error) {
			gotSpec = &spec.PodSpecGen
			return &entities.PodCreateReport{Id: "pod-123"}, nil
		},
	}

	w := createRequest(newTestHandler(engine).Create, `{"name":"web-pod","networks":["podman"],"labels":{"app":"web"}}`)

	require.Equal(t, http.StatusCreated, w.Code, "create should return 201")
	require.NotNil(t, gotSpec, "the request must reach the machine's spec")
	assert.Equal(t, "web-pod", gotSpec.Name)

	var body PodCreateResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), "the top-level body must be the create DTO")
	assert.Equal(t, "pod-123", body.ID)
	assert.Equal(t, "web-pod", body.Name)
	assert.Equal(t, "created", body.State)
	assert.NotEmpty(t, body.CreatedAt)
}

// TestPodHandler_Create_MissingName pins the validation gate: a create
// without a name is refused with 400 before the machine is asked (the
// engine stays unarmed and would panic on a call).
func TestPodHandler_Create_MissingName(t *testing.T) {
	engine := &fakePodEngine{}

	w := createRequest(newTestHandler(engine).Create, `{}`)

	assert.Equal(t, http.StatusBadRequest, w.Code, "a create without a name must be refused with 400")
	assert.Contains(t, w.Body.String(), "VALIDATION_FAILED")
}

// TestPodHandler_Create_MountFlagsMutuallyExclusive pins the z+Z guard:
// a mount carrying both selinuxShared and selinuxPrivate is refused
// with 400 VALIDATION_FAILED before the machine is asked (the engine
// stays unarmed and would panic on a call).
func TestPodHandler_Create_MountFlagsMutuallyExclusive(t *testing.T) {
	engine := &fakePodEngine{}

	w := createRequest(newTestHandler(engine).Create, `{
		"name": "web-pod",
		"mounts": [{"source": "/host/html", "destination": "/usr/share/nginx/html", "selinuxShared": true, "selinuxPrivate": true}]
	}`)

	assert.Equal(t, http.StatusBadRequest, w.Code, "a mount with both selinuxShared and selinuxPrivate must be refused with 400")
	assert.Contains(t, w.Body.String(), "VALIDATION_FAILED")
}

// TestPodHandler_Create_InvalidJSON pins the decode gate: a body that
// does not parse as JSON answers 400 INVALID_REQUEST.
func TestPodHandler_Create_InvalidJSON(t *testing.T) {
	engine := &fakePodEngine{}

	w := createRequest(newTestHandler(engine).Create, `not json`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "INVALID_REQUEST")
}
