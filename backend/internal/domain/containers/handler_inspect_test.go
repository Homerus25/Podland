package containers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	appmiddleware "github.com/Homerus25/podland/backend/internal/middleware"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/libpod/define"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// inspectRequest serves GET
// /api/hosts/{id}/containers/{containerId}/inspect through the handler
// with an authenticated test identity.
func inspectRequest(handler http.HandlerFunc, containerID string) *httptest.ResponseRecorder {
	return testutils.Perform(handler, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/hosts/test-host/containers/" + containerID + "/inspect",
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-host", "containerId": containerID},
	})
}

// TestContainerHandler_Inspect_ReturnsRawReportAsTopLevelBody pins the
// endpoint's contract: the raw podman inspect report is the top-level
// JSON body, with no DTO mapping — the Get DTO would trim the leading
// slash from the container's name, the raw report keeps it.
func TestContainerHandler_Inspect_ReturnsRawReportAsTopLevelBody(t *testing.T) {
	report := &entities.ContainerInspectReport{InspectContainerData: &define.InspectContainerData{
		ID:   "container-123",
		Name: "/web",
	}}
	engine := &fakeContainerEngine{
		inspectFn: func(string) ([]*entities.ContainerInspectReport, []error, error) {
			return []*entities.ContainerInspectReport{report}, nil, nil
		},
	}

	w := inspectRequest(newTestHandler(engine).Inspect, "container-123")

	require.Equal(t, http.StatusOK, w.Code)
	var body entities.ContainerInspectReport
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.NotNil(t, body.InspectContainerData)
	assert.Equal(t, "container-123", body.ID)
	assert.Equal(t, "/web", body.Name, "the raw report keeps podman's leading slash; the Get DTO would trim it")
}

// TestContainerHandler_Inspect_MissingContainerAnswers404 pins the
// documented 404 envelope end to end: the real service classifies the
// tunnel's per-item not-found answer and the web envelope maps the
// sentinel.
func TestContainerHandler_Inspect_MissingContainerAnswers404(t *testing.T) {
	engine := &fakeContainerEngine{
		inspectFn: func(id string) ([]*entities.ContainerInspectReport, []error, error) {
			return nil, []error{fmt.Errorf("no such container %q", id)}, nil
		},
	}

	w := inspectRequest(newTestHandler(engine).Inspect, "container-123")

	require.Equal(t, http.StatusNotFound, w.Code, "a missing container must answer 404")
	var body appErrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "test-request-id", body.RequestID)
	assert.Equal(t, "CONTAINER_NOT_FOUND", body.Error.Code)
	assert.Equal(t, "Container not found", body.Error.Message)
}

// TestContainerHandler_Inspect_MissingContainerIDAnswers400 pins the
// URL-param guard: a request without the containerId route param answers
// the missing-param 400 before the service (and engine) is reached.
func TestContainerHandler_Inspect_MissingContainerIDAnswers400(t *testing.T) {
	engine := &fakeContainerEngine{
		inspectFn: func(string) ([]*entities.ContainerInspectReport, []error, error) {
			panic("engine must not be reached when the param guard refuses")
		},
	}

	w := testutils.Perform(newTestHandler(engine).Inspect, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/hosts/test-host/containers//inspect",
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-host"},
	})

	require.Equal(t, http.StatusBadRequest, w.Code, "a missing containerId must answer 400")
	var body appErrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "MISSING_CONTAINER_ID", body.Error.Code)
	assert.Equal(t, "Container ID is required", body.Error.Message)
}

// containersAccessRouter mirrors app.go's mounting of the containers
// inspect route: GET /api/hosts/{id}/containers/{containerId}/inspect
// rides the RequireHostAccess subtree at read level (no requireWrite).
func containersAccessRouter(hostRepo *testutils.MockHostRepository, accessRepo *testutils.MockHostAccessRepository, service *ContainerService) http.Handler {
	r := http.NewServeMux()
	r.Handle("GET /api/hosts/{id}/containers/{containerId}/inspect",
		appmiddleware.RequireHostAccess(hostRepo, accessRepo)(http.HandlerFunc(NewContainerHandler(service).Inspect)))
	return r
}

// requestAsUser builds a request carrying the given user ID in the
// context, standing in for JWTMiddleware.
func requestAsUser(method, target, userID string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	return req.WithContext(contextutils.SetUserID(req.Context(), userID))
}

// TestContainerHandler_Inspect_ReadLevelGranteeReachesRoute pins the
// route mounting: GET /api/hosts/{id}/containers/{containerId}/inspect
// rides the RequireHostAccess subtree at read level (no requireWrite),
// so a read-level grantee reaches the handler.
func TestContainerHandler_Inspect_ReadLevelGranteeReachesRoute(t *testing.T) {
	hostRepo := testutils.NewMockHostRepository()
	hostRepo.AddHost(&models.Host{ID: "host-1", Name: "host-one", OwnerID: "owner-1"})
	accessRepo := testutils.NewMockHostAccessRepository()
	accessRepo.AddAccess("host-1", "reader-1", models.PermissionRead, time.Now())
	engine := &fakeContainerEngine{
		inspectFn: func(string) ([]*entities.ContainerInspectReport, []error, error) {
			return []*entities.ContainerInspectReport{{InspectContainerData: &define.InspectContainerData{ID: "c-1", Name: "/web"}}}, nil, nil
		},
	}
	router := containersAccessRouter(hostRepo, accessRepo, NewContainerService(fakeEngineProvider{engine: engine}))

	w := httptest.NewRecorder()
	router.ServeHTTP(w, requestAsUser(http.MethodGet, "/api/hosts/host-1/containers/c-1/inspect", "reader-1"))

	assert.Equal(t, http.StatusOK, w.Result().StatusCode, "a read-level grantee must reach the raw inspect endpoint")
}
