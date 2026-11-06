package containers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// TestFromPodmanContainerStats_MapsRawValues pins the stats DTO mapping:
// every field rides the DTO as a raw value (no formatting server-side),
// and the network byte counters are summed over the container's
// interfaces — two interfaces contribute to both directions.
func TestFromPodmanContainerStats_MapsRawValues(t *testing.T) {
	stats := define.ContainerStats{
		ContainerID: "container-123",
		Name:        "web",
		CPU:         12.5,
		MemUsage:    1048576,
		MemLimit:    2147483648,
		MemPerc:     0.05,
		Network: map[string]define.ContainerNetworkStats{
			"eth0": {RxBytes: 1000, TxBytes: 500},
			"eth1": {RxBytes: 200, TxBytes: 300},
		},
		BlockInput:  4096,
		BlockOutput: 8192,
		PIDs:        7,
	}

	got := FromPodmanContainerStats(stats)

	assert.Equal(t, ContainerStatsResponse{
		ID:              "container-123",
		Name:            "web",
		CPUPercent:      12.5,
		MemoryUsage:     1048576,
		MemoryLimit:     2147483648,
		MemoryPercent:   0.05,
		NetworkBytesIn:  1200,
		NetworkBytesOut: 800,
		BlockBytesIn:    4096,
		BlockBytesOut:   8192,
		PIDs:            7,
	}, got)
}

// TestFromPodmanContainerStats_NoInterfacesSumToZero pins the empty
// network case: a container without interface stats maps to zero byte
// counters, not a panic.
func TestFromPodmanContainerStats_NoInterfacesSumToZero(t *testing.T) {
	got := FromPodmanContainerStats(define.ContainerStats{ContainerID: "container-123"})

	assert.Zero(t, got.NetworkBytesIn)
	assert.Zero(t, got.NetworkBytesOut)
}

// TestToContainerStatsResult_Envelope pins the plural wrapper: the list
// envelope carries one DTO per stats entry, the entry count as total,
// and a timestamp — and an empty snapshot maps to an empty array,
// never null.
func TestToContainerStatsResult_Envelope(t *testing.T) {
	got := ToContainerStatsResult([]define.ContainerStats{
		{ContainerID: "container-123", Name: "web"},
		{ContainerID: "container-456", Name: "db"},
	})

	require.Len(t, got.Containers, 2)
	assert.Equal(t, "container-123", got.Containers[0].ID)
	assert.Equal(t, "container-456", got.Containers[1].ID)
	assert.Equal(t, 2, got.Total)
	assert.NotEmpty(t, got.Timestamp, "the envelope carries a timestamp")
}

// TestToContainerStatsResult_EmptySnapshotIsLegal pins the legal empty
// answer: nothing running maps to an empty (non-nil) container array.
func TestToContainerStatsResult_EmptySnapshotIsLegal(t *testing.T) {
	got := ToContainerStatsResult(nil)

	require.NotNil(t, got.Containers, "an empty snapshot must map to an empty array, not null")
	assert.Empty(t, got.Containers)
	assert.Zero(t, got.Total)
}

// TestFromPodmanInspectContainer_ReportWithoutEmbeddedInspectData pins
// the latent nil-panic's absence: a report without embedded inspect
// data (podman's report embeds it as a pointer) maps to the empty
// detail response instead of panicking on the promoted field access.
func TestFromPodmanInspectContainer_ReportWithoutEmbeddedInspectData(t *testing.T) {
	assert.Equal(t, ContainerGetResponse{}, FromPodmanInspectContainer(&entities.ContainerInspectReport{}))
}

// TestDetailReaders_DelegateToLifecycleReaders pins the detail DTO's
// name/state/exited delegation to the lifecycle module's nil-safe
// readers (lifecycle.go): a report without embedded inspect data
// resolves through them instead of panicking — the name falls back to
// the report's ID (empty here), the state resolves to created, and
// exited stays false.
func TestDetailReaders_DelegateToLifecycleReaders(t *testing.T) {
	report := &entities.ContainerInspectReport{}

	assert.Equal(t, "", getContainerName(report), "the name resolver must fall back, not panic")
	assert.Equal(t, "created", getContainerState(report))
	assert.False(t, isContainerExited(report))
}

// TestIsContainerExited_RunningReportWithNonzeroExitCodeIsNotExited pins
// the precedence isContainerExited's delegation rides: the state
// resolver answers running before it ever reads the exit code, so a
// running report is not exited even when it carries a nonzero exit
// code. Real podman reports never combine the two (podman resets the
// exit code when a container starts), which is what makes the
// delegation equivalent to the old exit-code-only check for real
// reports; this pin documents the edge's precedence explicitly, so a
// future resolver reorder that flips it fails here first.
func TestIsContainerExited_RunningReportWithNonzeroExitCodeIsNotExited(t *testing.T) {
	report := &entities.ContainerInspectReport{InspectContainerData: &define.InspectContainerData{
		State: &define.InspectContainerState{Running: true, ExitCode: 137},
	}}

	assert.Equal(t, "running", getContainerState(report))
	assert.False(t, isContainerExited(report), "a running report is not exited, whatever exit code it carries")
}
