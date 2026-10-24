package pods

import (
	"fmt"
	"testing"
	"time"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/libpod/define"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// podInspectFixture builds a PodInspectReport like podman's pod inspect
// answer: identity, state, created, labels, restart policy, mounts, the
// container summaries, and the infra config's networks.
func podInspectFixture(id, name, state string) *entities.PodInspectReport {
	return &entities.PodInspectReport{
		InspectPodData: &define.InspectPodData{
			ID:            id,
			Name:          name,
			State:         state,
			Created:       time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC),
			Labels:        map[string]string{"app": "web"},
			RestartPolicy: "always",
			Mounts: []define.InspectMount{
				{Type: "volume", Name: "web-data", Source: "/var/lib/podman/volumes/web-data/_data", Destination: "/data", RW: true},
				{Type: "bind", Source: "/host/html", Destination: "/usr/share/nginx/html", RW: true},
			},
			Containers: []define.InspectPodContainerInfo{
				{ID: "ctr-b", Name: "db", State: "running"},
				{ID: "ctr-a", Name: "web", State: "running"},
			},
			InfraConfig: &define.InspectPodInfraConfig{
				Networks: []string{"podman"},
			},
		},
	}
}

// TestPodService_Get_ResolvesSingleReport pins the detail service
// behaviour: the single inspect report resolves through the shared
// inspect kit and passes through to the handler's mapper.
func TestPodService_Get_ResolvesSingleReport(t *testing.T) {
	report := podInspectFixture("pod-a", "web", "Running")
	var gotName string
	engine := &fakePodEngine{
		inspectFn: func(nameOrID string) ([]*entities.PodInspectReport, []error, error) {
			gotName = nameOrID
			return []*entities.PodInspectReport{report}, nil, nil
		},
	}
	service := NewPodService(fakeEngineProvider{engine: engine})

	got, err := service.Get(t.Context(), "user", "machine", "web")

	require.NoError(t, err)
	assert.Equal(t, "web", gotName, "the URL-param pod must reach the machine")
	assert.Equal(t, report, got)
}

// TestPodService_Get_MissingPodAnswers404 pins the documented 404: the
// tunnel's per-item not-found answer classifies into ErrPodNotFound.
func TestPodService_Get_MissingPodAnswers404(t *testing.T) {
	engine := &fakePodEngine{
		inspectFn: func(nameOrID string) ([]*entities.PodInspectReport, []error, error) {
			return nil, []error{fmt.Errorf("no such pod %q", nameOrID)}, nil
		},
	}
	service := NewPodService(fakeEngineProvider{engine: engine})

	_, err := service.Get(t.Context(), "user", "machine", "ghost-pod")

	require.ErrorIs(t, err, appErrors.ErrPodNotFound)
}

// TestPodService_Inspect_ReturnsRawReport pins the raw inspect service
// behaviour: podman's report crosses unmapped.
func TestPodService_Inspect_ReturnsRawReport(t *testing.T) {
	report := podInspectFixture("pod-a", "web", "Running")
	engine := &fakePodEngine{
		inspectFn: func(nameOrID string) ([]*entities.PodInspectReport, []error, error) {
			return []*entities.PodInspectReport{report}, nil, nil
		},
	}
	service := NewPodService(fakeEngineProvider{engine: engine})

	got, err := service.Inspect(t.Context(), "user", "machine", "web")

	require.NoError(t, err)
	assert.Equal(t, report, got, "the raw inspect report must cross unmapped")
}

// TestFromPodmanInspectPod_MapsAllFields pins the detail mapping: name,
// ID, state, created (RFC3339), labels, restart policy, mounts
// (--mount-style strings), containers (ID, name, state), and networks
// from the infra config.
func TestFromPodmanInspectPod_MapsAllFields(t *testing.T) {
	got := FromPodmanInspectPod(podInspectFixture("pod-a", "web", "Running"))

	assert.Equal(t, "pod-a", got.ID)
	assert.Equal(t, "web", got.Name)
	assert.Equal(t, "Running", got.State)
	assert.Equal(t, "2026-10-24T12:00:00Z", got.CreatedAt)
	assert.Equal(t, map[string]string{"app": "web"}, got.Labels)
	assert.Equal(t, "always", got.RestartPolicy)
	assert.Equal(t, []string{
		"type=volume,source=/var/lib/podman/volumes/web-data/_data,destination=/data",
		"type=bind,source=/host/html,destination=/usr/share/nginx/html",
	}, got.Mounts)
	assert.Equal(t, []PodContainerRef{
		{ID: "ctr-a", Name: "web", State: "running"},
		{ID: "ctr-b", Name: "db", State: "running"},
	}, got.Containers, "containers map with ID, name, and state, ordered by ID")
	assert.Equal(t, []string{"podman"}, got.Networks, "networks resolve from the infra config")
}

// TestFromPodmanInspectPod_NilReport pins the nil case: the zero DTO
// with empty containers.
func TestFromPodmanInspectPod_NilReport(t *testing.T) {
	got := FromPodmanInspectPod(nil)

	assert.Empty(t, got.ID)
	assert.Empty(t, got.Containers)
}
