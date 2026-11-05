package pods

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
)

// podListFixture builds a ListPodsReport like podman's pod ps answer:
// id, name, status, creation time, networks, labels, and the container
// summaries (infra included).
func podListFixture(id, name, status string, created time.Time, containers int, networks []string, labels map[string]string) *entities.ListPodsReport {
	report := &entities.ListPodsReport{
		Id:       id,
		Name:     name,
		Status:   status,
		Created:  created,
		Networks: networks,
		Labels:   labels,
	}
	for i := 0; i < containers; i++ {
		report.Containers = append(report.Containers, &entities.ListPodContainer{
			Id:    id + "-ctr-" + string(rune('a'+i)),
			Names: name + "-ctr-" + string(rune('a'+i)),
		})
	}
	return report
}

// TestPodService_List_ReturnsPodmanReports pins the list service
// behaviour: the engine's reports pass through untouched, and the op tag
// names the pods domain.
func TestPodService_List_ReturnsPodmanReports(t *testing.T) {
	created := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	reports := []*entities.ListPodsReport{
		podListFixture("pod-a", "web", "Running", created, 2, []string{"podman"}, nil),
	}
	var gotOp string
	engine := &fakePodEngine{
		listFn: func(opts entities.PodPSOptions) ([]*entities.ListPodsReport, error) {
			return reports, nil
		},
	}
	provider := fakeEngineProvider{engine: engine, opRecorder: func(op string) { gotOp = op }}
	service := NewPodService(provider)

	got, err := service.List(t.Context(), "user", "host", entities.PodPSOptions{})

	require.NoError(t, err)
	assert.Equal(t, reports, got, "the engine's reports must pass through untouched")
	assert.Equal(t, "pods.List", gotOp, "the op tag must name the pods domain")
}

// TestPodService_List_AcquireFailureIsThe502Sentinel pins the connection
// seam: an acquisition failure crosses the service untouched (the 502
// CONNECTION_FAILED mapping lives in the seam, never here), and the
// engine is never reached.
func TestPodService_List_AcquireFailureIsThe502Sentinel(t *testing.T) {
	engine := &fakePodEngine{
		listFn: func(entities.PodPSOptions) ([]*entities.ListPodsReport, error) {
			panic("engine must not be reached when acquisition fails")
		},
	}
	service := NewPodService(fakeEngineProvider{engine: engine, err: appErrors.ErrConnectionFailed})

	got, err := service.List(t.Context(), "user", "host", entities.PodPSOptions{})

	require.Error(t, err)
	assert.Nil(t, got)
	assert.ErrorIs(t, err, appErrors.ErrConnectionFailed)
}

// TestFromPodmanListPod_MapsAllFields pins the singular list mapping:
// id, name, state (the report's status), createdAt (RFC3339), container
// count, networks, and labels.
func TestFromPodmanListPod_MapsAllFields(t *testing.T) {
	created := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	report := podListFixture("pod-a", "web", "Running", created, 2, []string{"podman"}, map[string]string{"app": "web"})

	got := FromPodmanListPod(report)

	assert.Equal(t, "pod-a", got.ID)
	assert.Equal(t, "web", got.Name)
	assert.Equal(t, "Running", got.State)
	assert.Equal(t, "2026-10-24T12:00:00Z", got.CreatedAt)
	assert.Equal(t, 2, got.ContainerCount)
	assert.Equal(t, []string{"podman"}, got.Networks)
	assert.Equal(t, map[string]string{"app": "web"}, got.Labels)
}

// TestToPodListResult_MapsEveryReport pins the plural list mapping: one
// DTO row per report, total matching, timestamp stamped.
func TestToPodListResult_MapsEveryReport(t *testing.T) {
	created := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	reports := []*entities.ListPodsReport{
		podListFixture("pod-a", "web", "Running", created, 2, nil, nil),
		podListFixture("pod-b", "db", "Exited", created, 1, nil, nil),
	}

	got := ToPodListResult(reports)

	require.Len(t, got.Pods, 2)
	assert.Equal(t, 2, got.Total)
	assert.Equal(t, "pod-a", got.Pods[0].ID)
	assert.Equal(t, "pod-b", got.Pods[1].ID)
	assert.Equal(t, 2, got.Pods[0].ContainerCount)
	assert.NotEmpty(t, got.Timestamp)
}

// TestToPodListResult_EmptyList pins that an empty list still marshals
// as an empty array, not null.
func TestToPodListResult_EmptyList(t *testing.T) {
	got := ToPodListResult(nil)

	assert.NotNil(t, got.Pods)
	assert.Empty(t, got.Pods)
	assert.Equal(t, 0, got.Total)
}
