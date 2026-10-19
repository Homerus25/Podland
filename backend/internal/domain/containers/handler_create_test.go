package containers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/testutils"
	runtimespec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	netTypes "go.podman.io/common/libnetwork/types"
	"go.podman.io/podman/v6/pkg/domain/entities"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
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
	assert.Equal(t, "Image not found", got.Error.Message)
}

// createSpec serves a create request through the handler over the fake
// engine and returns the spec the service built, requiring a 201 so a
// failed create fails the test loudly.
func createSpec(t *testing.T, body string) *specgen.SpecGenerator {
	t.Helper()
	var gotSpec *specgen.SpecGenerator
	engine := &fakeContainerEngine{
		createFn: func(spec *specgen.SpecGenerator) (*entities.ContainerCreateReport, error) {
			gotSpec = spec
			return &entities.ContainerCreateReport{Id: "container-123"}, nil
		},
	}

	rr := createRequest(newTestHandler(engine).Create, body)

	require.Equal(t, http.StatusCreated, rr.Code, "create should succeed: %s", rr.Body.String())
	require.NotNil(t, gotSpec, "the engine must receive the built spec")
	return gotSpec
}

// TestContainerHandler_Create_MapsPortBindings pins the portBindings
// mapping: each binding reaches the spec's PortMappings with its host
// and container port, and a binding without a protocol defaults to tcp.
func TestContainerHandler_Create_MapsPortBindings(t *testing.T) {
	spec := createSpec(t, `{
		"image": "nginx:latest",
		"portBindings": [
			{"hostPort": 8080, "containerPort": 80, "protocol": "udp"},
			{"hostPort": 9090, "containerPort": 90}
		]
	}`)

	assert.Equal(t, []netTypes.PortMapping{
		{HostPort: 8080, ContainerPort: 80, Protocol: "udp"},
		{HostPort: 9090, ContainerPort: 90, Protocol: "tcp"},
	}, spec.PortMappings, "port bindings must reach the spec's port mappings, protocol defaulting to tcp")
}

// TestContainerHandler_Create_MapsMounts pins the mounts mapping: each
// bind mount reaches the spec's Mounts as a bind mount with rbind
// propagation and the requested access mode (rw when omitted).
func TestContainerHandler_Create_MapsMounts(t *testing.T) {
	spec := createSpec(t, `{
		"image": "nginx:latest",
		"mounts": [
			{"source": "/host/nginx.conf", "destination": "/etc/nginx/nginx.conf", "mode": "ro"},
			{"source": "/host/html", "destination": "/usr/share/nginx/html"}
		]
	}`)

	assert.Equal(t, []runtimespec.Mount{
		{Type: "bind", Source: "/host/nginx.conf", Destination: "/etc/nginx/nginx.conf", Options: []string{"rbind", "ro"}},
		{Type: "bind", Source: "/host/html", Destination: "/usr/share/nginx/html", Options: []string{"rbind", "rw"}},
	}, spec.Mounts, "mounts must reach the spec as bind mounts with rbind and the requested mode, defaulting to rw")
}

// TestContainerHandler_Create_MapsRestartPolicy pins the restartPolicy
// mapping straight through to the spec.
func TestContainerHandler_Create_MapsRestartPolicy(t *testing.T) {
	spec := createSpec(t, `{"image": "nginx:latest", "restartPolicy": "unless-stopped"}`)

	assert.Equal(t, "unless-stopped", spec.RestartPolicy, "the request restart policy must reach the spec")
}

// TestContainerHandler_Create_MapsRemove pins the remove mapping: the
// request's remove flag reaches the spec's Remove pointer.
func TestContainerHandler_Create_MapsRemove(t *testing.T) {
	spec := createSpec(t, `{"image": "nginx:latest", "remove": true}`)

	require.NotNil(t, spec.Remove, "the request remove flag must reach the spec's Remove pointer")
	assert.True(t, *spec.Remove)
}

// TestContainerHandler_Create_MapsUser pins the user mapping straight
// through to the spec.
func TestContainerHandler_Create_MapsUser(t *testing.T) {
	spec := createSpec(t, `{"image": "nginx:latest", "user": "1000:1000"}`)

	assert.Equal(t, "1000:1000", spec.User, "the request user must reach the spec")
}

// TestContainerHandler_Create_MapsTimezone pins the timezone mapping
// straight through to the spec.
func TestContainerHandler_Create_MapsTimezone(t *testing.T) {
	spec := createSpec(t, `{"image": "nginx:latest", "timezone": "Europe/Berlin"}`)

	assert.Equal(t, "Europe/Berlin", spec.Timezone, "the request timezone must reach the spec")
}

// TestContainerHandler_Create_MapsNetworks pins the networks mapping:
// each requested network becomes one entry in the spec's Networks map
// with empty per-network options.
func TestContainerHandler_Create_MapsNetworks(t *testing.T) {
	spec := createSpec(t, `{"image": "nginx:latest", "networks": ["backend", "app-net"]}`)

	assert.Equal(t, map[string]netTypes.PerNetworkOptions{
		"backend": {},
		"app-net": {},
	}, spec.Networks, "each requested network must become one entry in the spec's network map")
}

// TestContainerHandler_Create_MapsVolumes pins the volumes mapping:
// each named volume reaches the spec's Volumes with its name,
// destination, and access mode as the mount option (rw when omitted).
func TestContainerHandler_Create_MapsVolumes(t *testing.T) {
	spec := createSpec(t, `{
		"image": "nginx:latest",
		"volumes": [
			{"name": "web-data", "destination": "/usr/share/nginx/html", "mode": "ro"},
			{"name": "web-cache", "destination": "/var/cache/nginx"}
		]
	}`)

	require.Len(t, spec.Volumes, 2, "each requested volume must reach the spec's volumes")
	assert.Equal(t, &specgen.NamedVolume{
		Name:    "web-data",
		Dest:    "/usr/share/nginx/html",
		Options: []string{"ro"},
	}, spec.Volumes[0])
	assert.Equal(t, &specgen.NamedVolume{
		Name:    "web-cache",
		Dest:    "/var/cache/nginx",
		Options: []string{"rw"},
	}, spec.Volumes[1], "a volume without a mode maps with the rw default as its option")
}

// TestContainerHandler_Create_MapsSecrets pins the secrets mapping: the
// secret's source and target reach the spec's Secrets, with the target
// defaulting to /run/secrets/<source> when the request omits it.
func TestContainerHandler_Create_MapsSecrets(t *testing.T) {
	spec := createSpec(t, `{
		"image": "nginx:latest",
		"secrets": [
			{"source": "db-password"},
			{"source": "api-key", "target": "/etc/secrets/api-key"}
		]
	}`)

	assert.Equal(t, []specgen.Secret{
		{Source: "db-password", Target: "/run/secrets/db-password"},
		{Source: "api-key", Target: "/etc/secrets/api-key"},
	}, spec.Secrets, "secrets must reach the spec with the target defaulting to /run/secrets/<source>")
}

// TestContainerHandler_Create_MapsDevices pins the devices mapping: each
// device reaches the spec's Devices with its host path, and a device
// whose container path differs from its host path also gets a bind
// mount at the container path.
func TestContainerHandler_Create_MapsDevices(t *testing.T) {
	spec := createSpec(t, `{
		"image": "nginx:latest",
		"devices": [
			{"hostPath": "/dev/dri", "containerPath": "/dev/dri"},
			{"hostPath": "/dev/video0", "containerPath": "/dev/video"}
		]
	}`)

	assert.Equal(t, []runtimespec.LinuxDevice{
		{Path: "/dev/dri"},
		{Path: "/dev/video0"},
	}, spec.Devices, "each device must reach the spec's device list with its host path")

	assert.Equal(t, []runtimespec.Mount{
		{Type: "bind", Source: "/dev/video0", Destination: "/dev/video", Options: []string{"rbind"}},
	}, spec.Mounts, "a device whose container path differs from its host path must also get a bind mount")
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
