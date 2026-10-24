package pods

import (
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	runtimespec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	netTypes "go.podman.io/common/libnetwork/types"
	"go.podman.io/podman/v6/pkg/specgen"
)

// TestBuildCreateSpec pins the create request → specgen mapping as
// direct builder table tests (the pure per-domain create builder
// settlement, backend-conventions): request in, populated pod specgen
// out, plus the memory-parse error — the builder's only error branch.
// No handler, no fake engine, no HTTP ride: each case builds the
// request, calls the builder, asserts the spec.
func TestBuildCreateSpec(t *testing.T) {
	cases := []struct {
		name    string
		request PodCreateRequest
		check   func(t *testing.T, spec *specgen.PodSpecGenerator)
	}{
		{
			name: "name and labels map straight through",
			request: PodCreateRequest{
				Name:   "web-pod",
				Labels: map[string]string{"app": "web"},
			},
			check: func(t *testing.T, spec *specgen.PodSpecGenerator) {
				assert.Equal(t, "web-pod", spec.Name, "the request name must reach the spec")
				assert.Equal(t, map[string]string{"app": "web"}, spec.Labels, "the request labels must reach the spec")
			},
		},
		{
			name:    "networks map to one empty-options entry each",
			request: PodCreateRequest{Name: "web-pod", Networks: []string{"backend", "app-net"}},
			check: func(t *testing.T, spec *specgen.PodSpecGenerator) {
				assert.Equal(t, map[string]netTypes.PerNetworkOptions{
					"backend": {},
					"app-net": {},
				}, spec.Networks, "each requested network must become one entry in the spec's network map")
			},
		},
		{
			name: "volumes map with the mode as the mount option",
			request: PodCreateRequest{
				Name: "web-pod",
				Volumes: []VolumeRequest{
					{Name: "web-data", Destination: "/usr/share/nginx/html", Mode: "ro"},
					{Name: "web-cache", Destination: "/var/cache/nginx"},
				},
			},
			check: func(t *testing.T, spec *specgen.PodSpecGenerator) {
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
			},
		},
		{
			name: "mounts map as bind mounts with rbind and the mode defaulting to rw",
			request: PodCreateRequest{
				Name: "web-pod",
				Mounts: []MountRequest{
					{Source: "/host/nginx.conf", Destination: "/etc/nginx/nginx.conf", Mode: "ro"},
					{Source: "/host/html", Destination: "/usr/share/nginx/html"},
				},
			},
			check: func(t *testing.T, spec *specgen.PodSpecGenerator) {
				assert.Equal(t, []runtimespec.Mount{
					{Type: "bind", Source: "/host/nginx.conf", Destination: "/etc/nginx/nginx.conf", Options: []string{"rbind", "ro"}},
					{Type: "bind", Source: "/host/html", Destination: "/usr/share/nginx/html", Options: []string{"rbind", "rw"}},
				}, spec.Mounts, "mounts must reach the spec as bind mounts with rbind and the requested mode, defaulting to rw")
			},
		},
		{
			name: "mount flags map as z/Z/U alongside rbind and the mode",
			request: PodCreateRequest{
				Name: "web-pod",
				Mounts: []MountRequest{
					{Source: "/host/ssl", Destination: "/etc/ssl", SelinuxPrivate: true, ChownUser: true},
					{Source: "/host/html", Destination: "/usr/share/nginx/html", Mode: "ro", SelinuxShared: true},
				},
			},
			check: func(t *testing.T, spec *specgen.PodSpecGenerator) {
				assert.Equal(t, []runtimespec.Mount{
					{Type: "bind", Source: "/host/ssl", Destination: "/etc/ssl", Options: []string{"rbind", "rw", "Z", "U"}},
					{Type: "bind", Source: "/host/html", Destination: "/usr/share/nginx/html", Options: []string{"rbind", "ro", "z"}},
				}, spec.Mounts, "the mount flags must reach the spec's mount options as z/Z/U alongside rbind and the mode")
			},
		},
		{
			name:    "cpus maps onto the CPU limit (quota = cpus x 100000, period 100000)",
			request: PodCreateRequest{Name: "web-pod", CPUs: 1.5},
			check: func(t *testing.T, spec *specgen.PodSpecGenerator) {
				require.NotNil(t, spec.ResourceLimits, "a cpus limit must produce resource limits")
				require.NotNil(t, spec.ResourceLimits.CPU)
				require.NotNil(t, spec.ResourceLimits.CPU.Period)
				require.NotNil(t, spec.ResourceLimits.CPU.Quota)
				assert.Equal(t, uint64(100000), *spec.ResourceLimits.CPU.Period)
				assert.Equal(t, int64(150000), *spec.ResourceLimits.CPU.Quota)
			},
		},
		{
			name:    "memory parses through podman's units parser into a byte limit",
			request: PodCreateRequest{Name: "web-pod", Memory: "512m"},
			check: func(t *testing.T, spec *specgen.PodSpecGenerator) {
				require.NotNil(t, spec.ResourceLimits, "a memory limit must produce resource limits")
				require.NotNil(t, spec.ResourceLimits.Memory)
				require.NotNil(t, spec.ResourceLimits.Memory.Limit)
				assert.Equal(t, int64(512*1024*1024), *spec.ResourceLimits.Memory.Limit)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec, err := buildCreateSpec(tc.request)
			require.NoError(t, err)
			require.NotNil(t, spec, "the builder must return a populated spec")
			tc.check(t, spec)
		})
	}
}

// TestBuildCreateSpec_NoLimitsLeavesResourceLimitsUnset pins the
// default: a request without cpus and memory leaves the spec's resource
// limits unset.
func TestBuildCreateSpec_NoLimitsLeavesResourceLimitsUnset(t *testing.T) {
	spec, err := buildCreateSpec(PodCreateRequest{Name: "web-pod"})

	require.NoError(t, err)
	assert.Nil(t, spec.ResourceLimits, "a request without limits must leave the spec's resource limits unset")
}

// TestBuildCreateSpec_UnparseableMemoryAnswersValidationFailed pins the
// builder's only error branch: an unparseable memory value answers the
// documented 400 VALIDATION_FAILED.
func TestBuildCreateSpec_UnparseableMemoryAnswersValidationFailed(t *testing.T) {
	spec, err := buildCreateSpec(PodCreateRequest{Name: "web-pod", Memory: "not-a-size"})

	assert.Nil(t, spec, "an unparseable memory value must not produce a spec")
	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr, "an unparseable memory value must answer an AppError")
	assert.Equal(t, "VALIDATION_FAILED", appErr.Code)
}
