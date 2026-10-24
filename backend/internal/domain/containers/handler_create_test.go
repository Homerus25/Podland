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
	"go.podman.io/podman/v6/pkg/specgen"
)

// The create handler suite pins only what the wire owns: the 201
// response contract, the decode/validation 400s, the two 404 forks,
// and the unknown-capability error surfacing. The request → specgen
// mapping is pinned as direct builder table tests (create_spec_test.go)
// — no handler, no fake engine, no HTTP ride.

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

// TestContainerHandler_Create_Success pins the 201 contract: a decoded
// create request answers 201 with the created container's ID, name,
// state, and image.
func TestContainerHandler_Create_Success(t *testing.T) {
	engine := &fakeContainerEngine{
		createFn: func(*specgen.SpecGenerator) (*entities.ContainerCreateReport, error) {
			return &entities.ContainerCreateReport{Id: "container-123"}, nil
		},
	}

	rr := createRequest(newTestHandler(engine).Create, `{
		"name": "test-container",
		"image": "nginx:latest"
	}`)

	assert.Equal(t, http.StatusCreated, rr.Code, "Should return 201 Created for successful container creation")
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"), "Should return JSON content type")

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

// TestContainerHandler_Create_MissingPodAnswers404 pins the documented
// 404 through the HTTP surface: podman's not-found answer on a create
// that requested a pod surfaces as the POD_NOT_FOUND envelope.
func TestContainerHandler_Create_MissingPodAnswers404(t *testing.T) {
	engine := &fakeContainerEngine{
		createFn: func(*specgen.SpecGenerator) (*entities.ContainerCreateReport, error) {
			return nil, &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such pod"}
		},
	}

	rr := createRequest(newTestHandler(engine).Create, `{"image": "nginx:latest", "pod": "missing-pod"}`)

	require.Equal(t, http.StatusNotFound, rr.Code, "a create against a missing pod must answer 404, not 500")
	var got appErrors.ErrorResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	assert.Equal(t, "POD_NOT_FOUND", got.Error.Code)
	// The classification is ambiguous (podman's not-found covers the
	// image, the pod, anything in the spec), so the message carries
	// podman's own not-found text as the disambiguator.
	assert.Equal(t, "no such pod", got.Error.Message)
}

// TestContainerHandler_Create_MissingImageAnswers404 pins the documented
// 404 through the HTTP surface: podman's not-found answer on create (a
// missing image) surfaces as the IMAGE_NOT_FOUND envelope, not a 500.
func TestContainerHandler_Create_MissingImageAnswers404(t *testing.T) {
	engine := &fakeContainerEngine{
		createFn: func(*specgen.SpecGenerator) (*entities.ContainerCreateReport, error) {
			return nil, &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such image"}
		},
	}

	rr := createRequest(newTestHandler(engine).Create, `{"image": "missing:latest"}`)

	require.Equal(t, http.StatusNotFound, rr.Code, "a create against a missing image must answer 404, not 500")
	var got appErrors.ErrorResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	assert.Equal(t, "IMAGE_NOT_FOUND", got.Error.Code)
	assert.Equal(t, "no such image", got.Error.Message)
}

// TestContainerHandler_Create_MountFlagsMutuallyExclusive pins the z+Z
// guard: a mount carrying both selinuxShared and selinuxPrivate is
// refused with 400 VALIDATION_FAILED before the machine is asked (the
// engine stays unarmed and would panic on a call).
func TestContainerHandler_Create_MountFlagsMutuallyExclusive(t *testing.T) {
	engine := &fakeContainerEngine{}

	rr := createRequest(newTestHandler(engine).Create, `{
		"image": "nginx:latest",
		"mounts": [{"source": "/host/html", "destination": "/usr/share/nginx/html", "selinuxShared": true, "selinuxPrivate": true}]
	}`)

	assert.Equal(t, http.StatusBadRequest, rr.Code, "a mount with both selinuxShared and selinuxPrivate must be refused with 400")
	assert.Contains(t, rr.Body.String(), "VALIDATION_FAILED")
}

// TestContainerHandler_Create_UnknownCapabilitySurfacesError pins that an
// unknown capability is not silently ignored: podman refuses the create
// and the refusal surfaces through the error envelope, not a 201.
func TestContainerHandler_Create_UnknownCapabilitySurfacesError(t *testing.T) {
	engine := &fakeContainerEngine{
		createFn: func(spec *specgen.SpecGenerator) (*entities.ContainerCreateReport, error) {
			if len(spec.CapAdd) > 0 {
				return nil, &errorhandling.ErrorModel{
					ResponseCode: http.StatusBadRequest,
					Message:      "unknown capability \"NOT_A_CAP\"",
				}
			}
			return &entities.ContainerCreateReport{Id: "container-123"}, nil
		},
	}

	rr := createRequest(newTestHandler(engine).Create, `{"image": "nginx:latest", "capAdd": ["NOT_A_CAP"]}`)

	require.Equal(t, http.StatusInternalServerError, rr.Code, "podman's refusal of an unknown capability must surface, not silently succeed")
	var got appErrors.ErrorResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	assert.Equal(t, "INTERNAL_ERROR", got.Error.Code, "the refusal must surface through the error envelope")
}

// TestContainerHandler_Create_Validation pins the validation guards on
// the request fields: an invalid restart policy, a device or volume
// missing a required sub-field, a secret without a source, an invalid
// mode, and an out-of-range port are all refused with 400 before the
// machine is asked (the engine stays unarmed and would panic on a call).
func TestContainerHandler_Create_Validation(t *testing.T) {
	cases := map[string]string{
		"invalid restart policy":        `{"image": "nginx:latest", "restartPolicy": "sometimes"}`,
		"device missing host path":      `{"image": "nginx:latest", "devices": [{"containerPath": "/dev/dri"}]}`,
		"device missing container path": `{"image": "nginx:latest", "devices": [{"hostPath": "/dev/dri"}]}`,
		"volume missing name":           `{"image": "nginx:latest", "volumes": [{"destination": "/data"}]}`,
		"volume missing destination":    `{"image": "nginx:latest", "volumes": [{"name": "data"}]}`,
		"volume invalid mode":           `{"image": "nginx:latest", "volumes": [{"name": "data", "destination": "/data", "mode": "z"}]}`,
		"secret missing source":         `{"image": "nginx:latest", "secrets": [{"target": "/run/secrets/x"}]}`,
		"mount invalid mode":            `{"image": "nginx:latest", "mounts": [{"source": "/a", "destination": "/b", "mode": "z"}]}`,
		"port binding out of range":     `{"image": "nginx:latest", "portBindings": [{"hostPort": 70000, "containerPort": 80}]}`,
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			engine := &fakeContainerEngine{}

			rr := createRequest(newTestHandler(engine).Create, body)

			assert.Equal(t, http.StatusBadRequest, rr.Code, "an invalid create request must be refused with 400")
			assert.Contains(t, rr.Body.String(), "VALIDATION_FAILED")
		})
	}
}
