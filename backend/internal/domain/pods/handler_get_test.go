package pods

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/libpod/define"
	"go.podman.io/podman/v6/pkg/domain/entities"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/testutils"
)

// getRequest serves GET /api/machines/{id}/pods/{podId} through the
// handler with an authenticated test identity.
func getRequest(get http.HandlerFunc, podID string) *httptest.ResponseRecorder {
	return testutils.Perform(get, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/machines/test-machine/pods/" + podID,
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-machine", "podId": podID},
	})
}

// TestPodHandler_Get_Success pins the detail contract: 200 with the
// mapped PodDetailResponse as the top-level JSON body — name, id,
// state, createdAt (RFC3339), labels, restart policy, mounts as
// --mount-style strings, containers ordered by ID, and the infra
// config's networks.
func TestPodHandler_Get_Success(t *testing.T) {
	report := podInspectFixture("pod-a", "web", "Running")
	var inspectedName string
	engine := &fakePodEngine{
		inspectFn: func(nameOrID string) ([]*entities.PodInspectReport, []error, error) {
			inspectedName = nameOrID
			return []*entities.PodInspectReport{report}, nil, nil
		},
	}

	w := getRequest(newTestHandler(engine).Get, "web")

	require.Equal(t, http.StatusOK, w.Code, "get should return 200")
	assert.Equal(t, "web", inspectedName, "the URL-param pod must reach the machine")

	var body PodDetailResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), "the top-level body must be the detail DTO")
	assert.Equal(t, PodDetailResponse{
		ID:            "pod-a",
		Name:          "web",
		State:         "Running",
		CreatedAt:     "2026-10-24T12:00:00Z",
		Labels:        map[string]string{"app": "web"},
		RestartPolicy: "always",
		Mounts: []string{
			"type=volume,source=/var/lib/podman/volumes/web-data/_data,destination=/data",
			"type=bind,source=/host/html,destination=/usr/share/nginx/html",
		},
		Containers: []PodContainerRef{
			{ID: "ctr-a", Name: "web", State: "running"},
			{ID: "ctr-b", Name: "db", State: "running"},
		},
		Networks: []string{"podman"},
	}, body)
}

// TestPodHandler_Get_MissingPodAnswers404 pins the documented 404
// envelope end to end: the real service classifies the tunnel's
// per-item not-found answer and the web envelope maps the sentinel.
func TestPodHandler_Get_MissingPodAnswers404(t *testing.T) {
	engine := &fakePodEngine{
		inspectFn: func(nameOrID string) ([]*entities.PodInspectReport, []error, error) {
			return nil, []error{fmt.Errorf("unable to find pod %q: no such pod", nameOrID)}, nil
		},
	}

	w := getRequest(newTestHandler(engine).Get, "ghost-pod")

	require.Equal(t, http.StatusNotFound, w.Code, "a missing pod must answer 404")
	var body appErrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "test-request-id", body.RequestID)
	assert.Equal(t, "POD_NOT_FOUND", body.Error.Code)
	assert.Equal(t, "Pod not found", body.Error.Message)
}

// TestPodHandler_Get_MissingPodID pins the URL-param guard: a request
// without the podId route param answers 400 MISSING_POD_ID before the
// engine is reached.
func TestPodHandler_Get_MissingPodID(t *testing.T) {
	engine := &fakePodEngine{} // inspectFn unarmed: the guard must fail first

	w := testutils.Perform(newTestHandler(engine).Get, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/machines/test-machine/pods/",
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-machine"},
	})

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "MISSING_POD_ID", testutils.DecodeErrorEnvelope(t, w.Result()).Error.Code)
}

// inspectRequest serves GET
// /api/machines/{id}/pods/{podId}/inspect through the handler with an
// authenticated test identity.
func inspectRequest(inspect http.HandlerFunc, podID string) *httptest.ResponseRecorder {
	return testutils.Perform(inspect, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/machines/test-machine/pods/" + podID + "/inspect",
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-machine", "podId": podID},
	})
}

// TestPodHandler_Inspect_ReturnsRawReportAsTopLevelBody pins the raw
// inspect contract: 200 with podman's inspect report as the top-level
// JSON body, unmapped.
func TestPodHandler_Inspect_ReturnsRawReportAsTopLevelBody(t *testing.T) {
	report := podInspectFixture("pod-a", "web", "Running")
	engine := &fakePodEngine{
		inspectFn: func(nameOrID string) ([]*entities.PodInspectReport, []error, error) {
			return []*entities.PodInspectReport{report}, nil, nil
		},
	}

	w := inspectRequest(newTestHandler(engine).Inspect, "web")

	require.Equal(t, http.StatusOK, w.Code)
	var body entities.PodInspectReport
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), "the top-level body must be podman's raw report")
	require.NotNil(t, body.InspectPodData)
	assert.Equal(t, "pod-a", body.ID)
	assert.Equal(t, "web", body.Name)
}

// TestPodHandler_Inspect_SurfacesInfraPortMappings pins the portMappings
// acceptance criterion (spec #402 ticket #405): the raw inspect passthrough
// carries the infra container's published port mappings verbatim — the
// mappings create requests plant surface in the pod's inspect data with
// no mapping layer in between.
func TestPodHandler_Inspect_SurfacesInfraPortMappings(t *testing.T) {
	report := podInspectFixture("pod-a", "web", "Running")
	report.InspectPodData.InfraConfig.PortBindings = map[string][]define.InspectHostPort{
		"80/tcp": {{HostIP: "", HostPort: "8080"}},
		"53/udp": {{HostIP: "127.0.0.1", HostPort: "53"}},
	}
	engine := &fakePodEngine{
		inspectFn: func(nameOrID string) ([]*entities.PodInspectReport, []error, error) {
			return []*entities.PodInspectReport{report}, nil, nil
		},
	}

	w := inspectRequest(newTestHandler(engine).Inspect, "web")

	require.Equal(t, http.StatusOK, w.Code)
	var body entities.PodInspectReport
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), "the top-level body must be podman's raw report")
	require.NotNil(t, body.InspectPodData)
	require.NotNil(t, body.InspectPodData.InfraConfig)
	assert.Equal(t,
		map[string][]define.InspectHostPort{
			"80/tcp": {{HostIP: "", HostPort: "8080"}},
			"53/udp": {{HostIP: "127.0.0.1", HostPort: "53"}},
		},
		body.InspectPodData.InfraConfig.PortBindings,
		"the infra container's published port mappings must cross verbatim",
	)
}

// TestPodHandler_Inspect_MissingPodAnswers404 pins the documented 404
// envelope on the raw-inspect route: the real service classifies the
// tunnel's per-item not-found answer and the web envelope maps the
// sentinel.
func TestPodHandler_Inspect_MissingPodAnswers404(t *testing.T) {
	engine := &fakePodEngine{
		inspectFn: func(nameOrID string) ([]*entities.PodInspectReport, []error, error) {
			return nil, []error{fmt.Errorf("unable to find pod %q: no such pod", nameOrID)}, nil
		},
	}

	w := inspectRequest(newTestHandler(engine).Inspect, "ghost-pod")

	require.Equal(t, http.StatusNotFound, w.Code, "a missing pod must answer 404")
	var body appErrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "test-request-id", body.RequestID)
	assert.Equal(t, "POD_NOT_FOUND", body.Error.Code)
	assert.Equal(t, "Pod not found", body.Error.Message)
}
