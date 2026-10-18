package containers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.podman.io/podman/v6/libpod/define"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// detailInspectReport builds an inspect report carrying the detail fields
// the container detail DTO maps beyond the basics: entrypoint, secrets,
// named volume mounts (alongside a bind mount that must be filtered out of
// Volumes but not out of Mounts), exposed ports, and the attached networks
// (inserted unsorted, so the mapping's sort is pinned). The top-level Image
// field carries the image ID (sha256:…), distinct from Config.Image, which
// carries the image name.
func detailInspectReport() *entities.ContainerInspectReport {
	return &entities.ContainerInspectReport{InspectContainerData: &define.InspectContainerData{
		ID:    "container-123",
		Name:  "/web",
		Image: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		Config: &define.InspectContainerConfig{
			Image:      "docker.io/library/nginx:latest",
			Entrypoint: []string{"/docker-entrypoint.sh"},
			Secrets: []*define.InspectSecret{
				{Name: "db-password", ID: "secret-1", UID: 1000, GID: 1001, Mode: 0o400},
			},
			ExposedPorts: map[string]struct{}{
				"8080/tcp": {},
				"53/udp":   {},
			},
		},
		Mounts: []define.InspectMount{
			{
				Type:        "volume",
				Name:        "web-data",
				Source:      "/var/lib/containers/storage/volumes/web-data/_data",
				Destination: "/usr/share/nginx/html",
				RW:          true,
			},
			{
				Type:        "bind",
				Source:      "/host/nginx.conf",
				Destination: "/etc/nginx/nginx.conf",
				RW:          false,
			},
		},
		NetworkSettings: &define.InspectNetworkSettings{
			Networks: map[string]*define.InspectAdditionalNetwork{
				"backend": {},
				"app-net": {},
			},
		},
	}}
}

// TestFromPodmanInspectContainer_MapsDetailFields pins the mapping of the
// detail fields the container detail page needs: Entrypoint from
// Config.Entrypoint, Secrets from Config.Secrets, Volumes filtered from
// Mounts down to named volume mounts, ExposedPorts from
// Config.ExposedPorts, Mounts as --mount-style strings for every mount,
// and Networks as the sorted names of the attached networks.
func TestFromPodmanInspectContainer_MapsDetailFields(t *testing.T) {
	got := FromPodmanInspectContainer(detailInspectReport())

	// The image name and the image ID are distinct fields: Config.Image
	// carries the name the container was created from, the report's
	// top-level Image carries the image ID the detail page's image
	// cross-link targets.
	assert.Equal(t, "docker.io/library/nginx:latest", got.Image)
	assert.Equal(t, "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", got.ImageID)

	assert.Equal(t, []string{"/docker-entrypoint.sh"}, got.Entrypoint)

	assert.Equal(t, []ContainerSecretResponse{
		{Name: "db-password", UID: 1000, GID: 1001, Mode: 0o400},
	}, got.Secrets)

	assert.Equal(t, []ContainerVolumeResponse{
		{
			Name:        "web-data",
			Source:      "/var/lib/containers/storage/volumes/web-data/_data",
			Destination: "/usr/share/nginx/html",
			RW:          true,
		},
	}, got.Volumes, "only the volume-typed mount is kept; the bind mount is filtered out")

	assert.Equal(t, []ExposedPortResponse{
		{ContainerPort: 53, Protocol: "udp"},
		{ContainerPort: 8080, Protocol: "tcp"},
	}, got.ExposedPorts, "exposed ports are sorted by container port for a stable response")

	assert.Equal(t, []string{
		"type=volume,source=/var/lib/containers/storage/volumes/web-data/_data,destination=/usr/share/nginx/html",
		"type=bind,source=/host/nginx.conf,destination=/etc/nginx/nginx.conf",
	}, got.Mounts, "every mount maps to a --mount-style string, in report order — named volumes and binds alike")

	assert.Equal(t, []string{"app-net", "backend"}, got.Networks, "attached networks are sorted by name for a stable response")
}

// TestFromPodmanInspectContainer_EmptyDetailFieldsStayNil pins the empty
// case: a report whose Config carries no entrypoint, secrets, or exposed
// ports, whose Mounts hold no named volume, and whose NetworkSettings is
// absent yields nil detail fields, so the JSON response omits them. The
// Mounts row still maps the bind mount — it lists every mount, not just
// named volumes.
func TestFromPodmanInspectContainer_EmptyDetailFieldsStayNil(t *testing.T) {
	report := &entities.ContainerInspectReport{InspectContainerData: &define.InspectContainerData{
		ID:     "container-123",
		Name:   "/web",
		Config: &define.InspectContainerConfig{Image: "nginx:latest"},
		Mounts: []define.InspectMount{
			{Type: "bind", Source: "/host/nginx.conf", Destination: "/etc/nginx/nginx.conf"},
		},
	}}

	got := FromPodmanInspectContainer(report)

	assert.Empty(t, got.ImageID, "a report without a top-level Image maps to an empty image ID")
	assert.Nil(t, got.Entrypoint)
	assert.Nil(t, got.Secrets)
	assert.Nil(t, got.Volumes, "a mounts list without volume-typed entries maps to nil")
	assert.Nil(t, got.ExposedPorts)
	assert.Nil(t, got.Networks, "a report without network settings maps to nil")
	assert.Equal(t, []string{
		"type=bind,source=/host/nginx.conf,destination=/etc/nginx/nginx.conf",
	}, got.Mounts, "the mounts row lists every mount, not just named volumes")
}

// TestFromPodmanInspectContainer_NilConfigLeavesDetailFieldsNil pins the
// nil-Config case: the detail fields stay nil and the mapping does not
// panic.
func TestFromPodmanInspectContainer_NilConfigLeavesDetailFieldsNil(t *testing.T) {
	report := &entities.ContainerInspectReport{InspectContainerData: &define.InspectContainerData{
		ID:   "container-123",
		Name: "/web",
	}}

	got := FromPodmanInspectContainer(report)

	assert.Nil(t, got.Entrypoint)
	assert.Nil(t, got.Secrets)
	assert.Nil(t, got.Volumes)
	assert.Nil(t, got.ExposedPorts)
	assert.Nil(t, got.Mounts)
	assert.Nil(t, got.Networks)
}

// TestFromPodmanInspectContainer_NilReportReturnsEmptyResponse pins the
// nil-report guard: the mapping returns the zero DTO.
func TestFromPodmanInspectContainer_NilReportReturnsEmptyResponse(t *testing.T) {
	got := FromPodmanInspectContainer(nil)

	assert.Equal(t, ContainerGetResponse{}, got)
}

// TestParseExposedPortKey_RangeExpands pins the range expansion: a key
// like "2000-2002/udp" expands to each port in the range, so no exposed
// port is silently dropped (spec 301 review finding).
func TestParseExposedPortKey_RangeExpands(t *testing.T) {
	ports, protocol, ok := parseExposedPortKey("2000-2002/udp")
	assert.True(t, ok)
	assert.Equal(t, "udp", protocol)
	assert.Equal(t, []int{2000, 2001, 2002}, ports)
}

// TestParseExposedPortKey_SinglePort pins the single-port case: a bare
// port key returns a one-element slice.
func TestParseExposedPortKey_SinglePort(t *testing.T) {
	ports, protocol, ok := parseExposedPortKey("8080/tcp")
	assert.True(t, ok)
	assert.Equal(t, "tcp", protocol)
	assert.Equal(t, []int{8080}, ports)
}

// TestParseExposedPortKey_NoProtocolDefaultsToTCP pins the default
// protocol: a key without a protocol suffix defaults to tcp.
func TestParseExposedPortKey_NoProtocolDefaultsToTCP(t *testing.T) {
	ports, protocol, ok := parseExposedPortKey("9090")
	assert.True(t, ok)
	assert.Equal(t, "tcp", protocol)
	assert.Equal(t, []int{9090}, ports)
}

// TestParseExposedPortKey_InvalidKeys pins the rejection cases: a
// non-numeric port, an inverted range, and an oversized range all
// return ok=false.
func TestParseExposedPortKey_InvalidKeys(t *testing.T) {
	for _, key := range []string{"abc/tcp", "2002-2000/tcp", "1-5000/tcp"} {
		_, _, ok := parseExposedPortKey(key)
		assert.False(t, ok, "key %q should be rejected", key)
	}
}

// TestMapExposedPorts_RangeExpands pins the end-to-end mapping: a
// range key in Config.ExposedPorts expands to one ExposedPortResponse
// per port in the range, sorted by container port.
func TestMapExposedPorts_RangeExpands(t *testing.T) {
	got := mapExposedPorts(map[string]struct{}{
		"2000-2002/udp": {},
		"8080/tcp":      {},
	})
	assert.Equal(t, []ExposedPortResponse{
		{ContainerPort: 2000, Protocol: "udp"},
		{ContainerPort: 2001, Protocol: "udp"},
		{ContainerPort: 2002, Protocol: "udp"},
		{ContainerPort: 8080, Protocol: "tcp"},
	}, got)
}
