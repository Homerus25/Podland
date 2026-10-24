package pods

import (
	"net/http"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	runtimespec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
	"go.podman.io/podman/v6/pkg/specgen"
)

// createSpec serves a create request through the real service over the
// fake engine and returns the pod spec the service built, requiring a
// clean call so a failed create fails the test loudly.
func createSpec(t *testing.T, request PodCreateRequest) *specgen.PodSpecGenerator {
	t.Helper()
	var gotSpec *specgen.PodSpecGenerator
	engine := &fakePodEngine{
		createFn: func(spec entities.PodSpec) (*entities.PodCreateReport, error) {
			gotSpec = &spec.PodSpecGen
			return &entities.PodCreateReport{Id: "pod-123"}, nil
		},
	}
	service := NewPodService(fakeEngineProvider{engine: engine})

	_, err := service.Create(t.Context(), "user", "machine", request)

	require.NoError(t, err)
	require.NotNil(t, gotSpec, "the engine must receive the built spec")
	return gotSpec
}

// TestPodService_Create_MapsNameLabelsNetworks pins the basic create
// mapping: name and labels straight through, each requested network one
// entry in the spec's network map with empty per-network options.
func TestPodService_Create_MapsNameLabelsNetworks(t *testing.T) {
	spec := createSpec(t, PodCreateRequest{
		Name:     "web-pod",
		Labels:   map[string]string{"app": "web"},
		Networks: []string{"backend", "app-net"},
	})

	assert.Equal(t, "web-pod", spec.Name)
	assert.Equal(t, map[string]string{"app": "web"}, spec.Labels)
	require.Len(t, spec.Networks, 2)
	_, hasBackend := spec.Networks["backend"]
	_, hasAppNet := spec.Networks["app-net"]
	assert.True(t, hasBackend, "the backend network must be in the spec's network map")
	assert.True(t, hasAppNet, "the app-net network must be in the spec's network map")
	assert.Equal(t, spec.Networks["backend"], spec.Networks["app-net"],
		"per-network options must be empty for every requested network")
}

// TestPodService_Create_MapsVolumesAndMounts pins the storage mapping:
// named volumes and bind mounts in the same shapes as the container
// create request map onto the pod spec's storage config.
func TestPodService_Create_MapsVolumesAndMounts(t *testing.T) {
	spec := createSpec(t, PodCreateRequest{
		Name: "web-pod",
		Volumes: []VolumeRequest{
			{Name: "web-data", Destination: "/usr/share/nginx/html", Mode: "ro"},
			{Name: "web-cache", Destination: "/var/cache/nginx"},
		},
		Mounts: []MountRequest{
			{Source: "/host/nginx.conf", Destination: "/etc/nginx/nginx.conf", Mode: "ro"},
			{Source: "/host/html", Destination: "/usr/share/nginx/html"},
		},
	})

	require.Len(t, spec.Volumes, 2)
	assert.Equal(t, &specgen.NamedVolume{Name: "web-data", Dest: "/usr/share/nginx/html", Options: []string{"ro"}}, spec.Volumes[0])
	assert.Equal(t, &specgen.NamedVolume{Name: "web-cache", Dest: "/var/cache/nginx", Options: []string{"rw"}}, spec.Volumes[1])

	assert.Equal(t, []runtimespec.Mount{
		{Type: "bind", Source: "/host/nginx.conf", Destination: "/etc/nginx/nginx.conf", Options: []string{"rbind", "ro"}},
		{Type: "bind", Source: "/host/html", Destination: "/usr/share/nginx/html", Options: []string{"rbind", "rw"}},
	}, spec.Mounts)
}

// TestPodService_Create_MapsCPULimit pins the cpus mapping: quota =
// cpus x 100000, period 100000.
func TestPodService_Create_MapsCPULimit(t *testing.T) {
	spec := createSpec(t, PodCreateRequest{Name: "web-pod", CPUs: 1.5})

	require.NotNil(t, spec.ResourceLimits, "a cpus limit must produce resource limits")
	require.NotNil(t, spec.ResourceLimits.CPU)
	require.NotNil(t, spec.ResourceLimits.CPU.Period)
	require.NotNil(t, spec.ResourceLimits.CPU.Quota)
	assert.Equal(t, uint64(100000), *spec.ResourceLimits.CPU.Period)
	assert.Equal(t, int64(150000), *spec.ResourceLimits.CPU.Quota)
}

// TestPodService_Create_MapsMemoryLimit pins the memory mapping: the
// value parses through podman's units parser into a byte limit.
func TestPodService_Create_MapsMemoryLimit(t *testing.T) {
	spec := createSpec(t, PodCreateRequest{Name: "web-pod", Memory: "512m"})

	require.NotNil(t, spec.ResourceLimits, "a memory limit must produce resource limits")
	require.NotNil(t, spec.ResourceLimits.Memory)
	require.NotNil(t, spec.ResourceLimits.Memory.Limit)
	assert.Equal(t, int64(512*1024*1024), *spec.ResourceLimits.Memory.Limit)
}

// TestPodService_Create_NoLimitsLeavesResourceLimitsNil pins the
// default: a request without cpus and memory leaves the spec's resource
// limits unset.
func TestPodService_Create_NoLimitsLeavesResourceLimitsNil(t *testing.T) {
	spec := createSpec(t, PodCreateRequest{Name: "web-pod"})

	assert.Nil(t, spec.ResourceLimits, "a request without limits must leave the spec's resource limits unset")
}

// TestPodService_Create_UnparseableMemoryAnswersValidationFailed pins
// the documented 400: an unparseable memory value is a
// VALIDATION_FAILED, not a 500.
func TestPodService_Create_UnparseableMemoryAnswersValidationFailed(t *testing.T) {
	engine := &fakePodEngine{}
	service := NewPodService(fakeEngineProvider{engine: engine})

	_, err := service.Create(t.Context(), "user", "machine", PodCreateRequest{Name: "web-pod", Memory: "not-a-size"})

	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr, "an unparseable memory value must answer an AppError")
	assert.Equal(t, "VALIDATION_FAILED", appErr.Code)
	assert.Equal(t, http.StatusBadRequest, appErr.Status)
}

// TestPodService_Create_PodmanClientRejectionAnswersInvalidRequest pins
// the create classification: a podman-side 4xx rejection (duplicate name,
// invalid name format) surfaces as 400 INVALID_REQUEST carrying podman's
// message.
func TestPodService_Create_PodmanClientRejectionAnswersInvalidRequest(t *testing.T) {
	engine := &fakePodEngine{
		createFn: func(entities.PodSpec) (*entities.PodCreateReport, error) {
			return nil, &errorhandling.ErrorModel{ResponseCode: 409, Because: `name "web-pod" is in use`}
		},
	}
	service := NewPodService(fakeEngineProvider{engine: engine})

	_, err := service.Create(t.Context(), "user", "machine", PodCreateRequest{Name: "web-pod"})

	require.Error(t, err)
	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr, "podman-side 4xx rejections must map to an AppError")
	assert.Equal(t, "INVALID_REQUEST", appErr.Code)
	assert.NotEmpty(t, appErr.Message, "the message must carry podman's refusal text")
}
