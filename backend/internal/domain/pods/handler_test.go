package pods

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"

	"github.com/Homerus25/podland/backend/internal/testutils"
)

// listRequest serves GET /api/hosts/{id}/pods through the handler
// with an authenticated test identity.
func listRequest(list http.HandlerFunc) *httptest.ResponseRecorder {
	return testutils.Perform(list, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/hosts/test-host/pods",
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-host"},
	})
}

// TestPodHandler_List_Success pins the list contract: 200 with the
// wrapped PodListResult — pods, total, timestamp — and the DTO fields
// the API promises (name, state, createdAt, containerCount, networks,
// labels).
func TestPodHandler_List_Success(t *testing.T) {
	created := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	reports := []*entities.ListPodsReport{
		podListFixture("pod-a", "web", "Running", created, 2, []string{"podman"}, map[string]string{"app": "web"}),
		podListFixture("pod-b", "db", "Exited", created, 1, nil, nil),
	}
	engine := &fakePodEngine{
		listFn: func(opts entities.PodPSOptions) ([]*entities.ListPodsReport, error) {
			return reports, nil
		},
	}

	w := listRequest(newTestHandler(engine).List)

	require.Equal(t, http.StatusOK, w.Code, "list should return 200")
	var body PodListResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), "the top-level body must be the list result")
	assert.Equal(t, 2, body.Total)
	require.Len(t, body.Pods, 2, "every pod podman reports must appear")

	web := body.Pods[0]
	assert.Equal(t, "pod-a", web.ID)
	assert.Equal(t, "web", web.Name)
	assert.Equal(t, "Running", web.State)
	assert.Equal(t, "2026-10-24T12:00:00Z", web.CreatedAt)
	assert.Equal(t, 2, web.ContainerCount)
	assert.Equal(t, []string{"podman"}, web.Networks)
	assert.Equal(t, map[string]string{"app": "web"}, web.Labels)
	assert.NotEmpty(t, body.Timestamp)
}

// TestPodHandler_List_Empty pins the empty-list contract: 200 with an
// empty pods array, not null.
func TestPodHandler_List_Empty(t *testing.T) {
	engine := &fakePodEngine{
		listFn: func(opts entities.PodPSOptions) ([]*entities.ListPodsReport, error) {
			return nil, nil
		},
	}

	w := listRequest(newTestHandler(engine).List)

	require.Equal(t, http.StatusOK, w.Code)
	var body PodListResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.NotNil(t, body.Pods, "pods must decode as an empty array, not null")
	assert.Empty(t, body.Pods)
	assert.Equal(t, 0, body.Total)
}
