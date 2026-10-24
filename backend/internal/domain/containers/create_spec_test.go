package containers

import (
	"testing"

	runtimespec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	netTypes "go.podman.io/common/libnetwork/types"
	"go.podman.io/podman/v6/pkg/specgen"
)

// TestBuildCreateSpec pins the create request → specgen mapping as
// direct builder table tests (the pure per-domain create builder
// settlement, backend-conventions): request in, populated spec out, no
// error — every wire-documented guard is decode-level, owned by
// web.Decode and the request's struct tags, so the builder carries no
// error vocabulary of its own. No handler, no fake engine, no HTTP
// ride: each case builds the request, calls the builder, asserts the
// spec.
func TestBuildCreateSpec(t *testing.T) {
	cases := []struct {
		name    string
		request ContainerCreateRequest
		check   func(t *testing.T, spec *specgen.SpecGenerator)
	}{
		{
			name: "core fields map straight through",
			request: ContainerCreateRequest{
				Name:        "test-container",
				Image:       "nginx:latest",
				Command:     []string{"/bin/sh", "-c", "echo hello"},
				WorkingDir:  "/srv",
				Environment: map[string]string{"ENV1": "value1"},
				Labels:      map[string]string{"app": "test"},
			},
			check: func(t *testing.T, spec *specgen.SpecGenerator) {
				assert.Equal(t, "nginx:latest", spec.Image, "the request image must reach the spec")
				assert.Equal(t, "test-container", spec.Name, "the request name must reach the spec")
				assert.Equal(t, []string{"/bin/sh", "-c", "echo hello"}, spec.Command, "the request command must reach the spec")
				assert.Equal(t, "/srv", spec.WorkDir, "the request working directory must reach the spec")
				assert.Equal(t, map[string]string{"ENV1": "value1"}, spec.Env, "the request environment must reach the spec")
				assert.Equal(t, map[string]string{"app": "test"}, spec.Labels, "the request labels must reach the spec")
			},
		},
		{
			name: "port bindings map with the protocol defaulting to tcp",
			request: ContainerCreateRequest{
				Image: "nginx:latest",
				PortBindings: []PortBindingRequest{
					{HostPort: 8080, ContainerPort: 80, Protocol: "udp"},
					{HostPort: 9090, ContainerPort: 90},
				},
			},
			check: func(t *testing.T, spec *specgen.SpecGenerator) {
				assert.Equal(t, []netTypes.PortMapping{
					{HostPort: 8080, ContainerPort: 80, Protocol: "udp"},
					{HostPort: 9090, ContainerPort: 90, Protocol: "tcp"},
				}, spec.PortMappings, "port bindings must reach the spec's port mappings, protocol defaulting to tcp")
			},
		},
		{
			name: "mounts map as bind mounts with rbind and the mode defaulting to rw",
			request: ContainerCreateRequest{
				Image: "nginx:latest",
				Mounts: []MountRequest{
					{Source: "/host/nginx.conf", Destination: "/etc/nginx/nginx.conf", Mode: "ro"},
					{Source: "/host/html", Destination: "/usr/share/nginx/html"},
				},
			},
			check: func(t *testing.T, spec *specgen.SpecGenerator) {
				assert.Equal(t, []runtimespec.Mount{
					{Type: "bind", Source: "/host/nginx.conf", Destination: "/etc/nginx/nginx.conf", Options: []string{"rbind", "ro"}},
					{Type: "bind", Source: "/host/html", Destination: "/usr/share/nginx/html", Options: []string{"rbind", "rw"}},
				}, spec.Mounts, "mounts must reach the spec as bind mounts with rbind and the requested mode, defaulting to rw")
			},
		},
		{
			name: "mount flags map as z/Z/U alongside rbind and the mode",
			request: ContainerCreateRequest{
				Image: "nginx:latest",
				Mounts: []MountRequest{
					{Source: "/host/ssl", Destination: "/etc/ssl", SelinuxPrivate: true, ChownUser: true},
					{Source: "/host/html", Destination: "/usr/share/nginx/html", Mode: "ro", SelinuxShared: true},
				},
			},
			check: func(t *testing.T, spec *specgen.SpecGenerator) {
				assert.Equal(t, []runtimespec.Mount{
					{Type: "bind", Source: "/host/ssl", Destination: "/etc/ssl", Options: []string{"rbind", "rw", "Z", "U"}},
					{Type: "bind", Source: "/host/html", Destination: "/usr/share/nginx/html", Options: []string{"rbind", "ro", "z"}},
				}, spec.Mounts, "the mount flags must reach the spec's mount options as z/Z/U alongside rbind and the mode")
			},
		},
		{
			name:    "restart policy maps straight through",
			request: ContainerCreateRequest{Image: "nginx:latest", RestartPolicy: "unless-stopped"},
			check: func(t *testing.T, spec *specgen.SpecGenerator) {
				assert.Equal(t, "unless-stopped", spec.RestartPolicy, "the request restart policy must reach the spec")
			},
		},
		{
			name:    "remove maps to the spec's Remove pointer",
			request: ContainerCreateRequest{Image: "nginx:latest", Remove: boolPtr(true)},
			check: func(t *testing.T, spec *specgen.SpecGenerator) {
				require.NotNil(t, spec.Remove, "the request remove flag must reach the spec's Remove pointer")
				assert.True(t, *spec.Remove)
			},
		},
		{
			name:    "privileged sets the spec's pointer only when asked",
			request: ContainerCreateRequest{Image: "nginx:latest", Privileged: true},
			check: func(t *testing.T, spec *specgen.SpecGenerator) {
				require.NotNil(t, spec.Privileged, "a privileged request must reach the spec's Privileged pointer")
				assert.True(t, *spec.Privileged, "the spec must be privileged when the request asks for it")
			},
		},
		{
			name:    "capAdd maps verbatim",
			request: ContainerCreateRequest{Image: "nginx:latest", CapAdd: []string{"NET_ADMIN", "SYS_TIME"}},
			check: func(t *testing.T, spec *specgen.SpecGenerator) {
				assert.Equal(t, []string{"NET_ADMIN", "SYS_TIME"}, spec.CapAdd, "the requested capabilities must reach the spec verbatim")
			},
		},
		{
			name:    "user maps straight through",
			request: ContainerCreateRequest{Image: "nginx:latest", User: "1000:1000"},
			check: func(t *testing.T, spec *specgen.SpecGenerator) {
				assert.Equal(t, "1000:1000", spec.User, "the request user must reach the spec")
			},
		},
		{
			name:    "timezone maps straight through",
			request: ContainerCreateRequest{Image: "nginx:latest", Timezone: "Europe/Berlin"},
			check: func(t *testing.T, spec *specgen.SpecGenerator) {
				assert.Equal(t, "Europe/Berlin", spec.Timezone, "the request timezone must reach the spec")
			},
		},
		{
			name:    "networks map to one empty-options entry each",
			request: ContainerCreateRequest{Image: "nginx:latest", Networks: []string{"backend", "app-net"}},
			check: func(t *testing.T, spec *specgen.SpecGenerator) {
				assert.Equal(t, map[string]netTypes.PerNetworkOptions{
					"backend": {},
					"app-net": {},
				}, spec.Networks, "each requested network must become one entry in the spec's network map")
			},
		},
		{
			name:    "pod maps as the free string podman resolves",
			request: ContainerCreateRequest{Image: "nginx:latest", Pod: "my-pod"},
			check: func(t *testing.T, spec *specgen.SpecGenerator) {
				assert.Equal(t, "my-pod", spec.Pod, "the requested pod must reach the spec's Pod field")
			},
		},
		{
			name: "volumes map with the mode as the mount option",
			request: ContainerCreateRequest{
				Image: "nginx:latest",
				Volumes: []VolumeRequest{
					{Name: "web-data", Destination: "/usr/share/nginx/html", Mode: "ro"},
					{Name: "web-cache", Destination: "/var/cache/nginx"},
				},
			},
			check: func(t *testing.T, spec *specgen.SpecGenerator) {
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
			name: "secrets map with the target defaulting to /run/secrets/<source>",
			request: ContainerCreateRequest{
				Image: "nginx:latest",
				Secrets: []SecretRequest{
					{Source: "db-password"},
					{Source: "api-key", Target: "/etc/secrets/api-key"},
				},
			},
			check: func(t *testing.T, spec *specgen.SpecGenerator) {
				assert.Equal(t, []specgen.Secret{
					{Source: "db-password", Target: "/run/secrets/db-password"},
					{Source: "api-key", Target: "/etc/secrets/api-key"},
				}, spec.Secrets, "secrets must reach the spec with the target defaulting to /run/secrets/<source>")
			},
		},
		{
			name: "devices map by host path, a differing container path also gets a bind mount",
			request: ContainerCreateRequest{
				Image: "nginx:latest",
				Devices: []DeviceRequest{
					{HostPath: "/dev/dri", ContainerPath: "/dev/dri"},
					{HostPath: "/dev/video0", ContainerPath: "/dev/video"},
				},
			},
			check: func(t *testing.T, spec *specgen.SpecGenerator) {
				assert.Equal(t, []runtimespec.LinuxDevice{
					{Path: "/dev/dri"},
					{Path: "/dev/video0"},
				}, spec.Devices, "each device must reach the spec's device list with its host path")

				assert.Equal(t, []runtimespec.Mount{
					{Type: "bind", Source: "/dev/video0", Destination: "/dev/video", Options: []string{"rbind"}},
				}, spec.Mounts, "a device whose container path differs from its host path must also get a bind mount")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec := buildCreateSpec(tc.request)
			require.NotNil(t, spec, "the builder must return a populated spec")
			tc.check(t, spec)
		})
	}
}

// TestBuildCreateSpec_DefaultsLeaveTheSpecUnset pins the defaults: a
// request carrying only the required image leaves the spec's Pod field
// empty (the container is created outside any pod), the Privileged
// pointer unset (an unprivileged container), and the Remove pointer
// unset.
func TestBuildCreateSpec_DefaultsLeaveTheSpecUnset(t *testing.T) {
	spec := buildCreateSpec(ContainerCreateRequest{Image: "nginx:latest"})

	assert.Empty(t, spec.Pod, "a request without a pod must leave the spec's Pod field empty")
	assert.Nil(t, spec.Privileged, "a request without privileged must leave the spec unprivileged")
	assert.False(t, spec.IsPrivileged(), "the spec must not be privileged when the request omits the flag")
	assert.Nil(t, spec.Remove, "a request without remove must leave the spec's Remove pointer unset")
	assert.Empty(t, spec.CapAdd, "a request without capAdd must leave the spec's CapAdd empty")
}

// boolPtr is the test-local address-of helper for the request's pointer
// fields.
func boolPtr(b bool) *bool {
	return &b
}
