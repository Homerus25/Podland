package networks

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
	"go.podman.io/common/libnetwork/types"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// inspectRequest serves GET
// /api/machines/{id}/networks/{networkName}/inspect through the handler
// with an authenticated test identity.
func inspectRequest(inspect http.HandlerFunc, networkName string) *httptest.ResponseRecorder {
	return testutils.Perform(inspect, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/machines/test-machine/networks/" + networkName + "/inspect",
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-machine", "networkName": networkName},
	})
}

// networkInspectFixture builds a raw network inspect report like
// podman's inspect answer: the embedded network plus the attached
// containers.
func networkInspectFixture(name string) entities.NetworkInspectReport {
	return entities.NetworkInspectReport{
		Network: types.Network{
			Name:    name,
			ID:      "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
			Driver:  "bridge",
			Created: mustParseNetworkTime("2026-09-01T12:30:00Z"),
			Labels:  map[string]string{"app": "frontend"},
		},
		Containers: map[string]entities.NetworkContainerInfo{
			"ctr-a": {Name: "web"},
		},
	}
}

// TestNetworkHandler_Inspect_Success pins the inspect contract: 200
// with the raw network inspect report as the top-level JSON body — no
// DTO mapping — and the URL-param network reaching the machine.
func TestNetworkHandler_Inspect_Success(t *testing.T) {
	report := networkInspectFixture("frontend")
	var inspectedName string
	engine := &fakeNetworkEngine{
		inspectFn: func(name string) ([]entities.NetworkInspectReport, []error, error) {
			inspectedName = name
			return []entities.NetworkInspectReport{report}, nil, nil
		},
	}

	w := inspectRequest(newTestHandler(engine).Inspect, "frontend")

	require.Equal(t, http.StatusOK, w.Code, "inspect should return 200")
	assert.Equal(t, "frontend", inspectedName, "the URL-param network must reach the machine")

	var body entities.NetworkInspectReport
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), "the top-level body must be the raw inspect report")
	assert.Equal(t, report.Name, body.Name)
	assert.Equal(t, report.ID, body.ID)
	assert.Equal(t, report.Driver, body.Driver)
	assert.True(t, report.Created.Equal(body.Created), "the creation timestamp must survive the round trip")
	assert.Equal(t, report.Labels, body.Labels)
	require.Contains(t, body.Containers, "ctr-a", "the attached containers must be part of the raw report")
	assert.Equal(t, "web", body.Containers["ctr-a"].Name)
}

// TestNetworkHandler_Inspect_MissingNetworkAnswers404 pins the
// documented 404 envelope end to end: the real service classifies the
// tunnel's per-item not-found answer and the web envelope maps the
// sentinel.
func TestNetworkHandler_Inspect_MissingNetworkAnswers404(t *testing.T) {
	engine := &fakeNetworkEngine{
		inspectFn: func(name string) ([]entities.NetworkInspectReport, []error, error) {
			return nil, []error{fmt.Errorf("no such network %q", name)}, nil
		},
	}

	w := inspectRequest(newTestHandler(engine).Inspect, "ghost-net")

	require.Equal(t, http.StatusNotFound, w.Code, "a missing network must answer 404")
	var body appErrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "test-request-id", body.RequestID)
	assert.Equal(t, "NETWORK_NOT_FOUND", body.Error.Code)
	assert.Equal(t, "Network not found", body.Error.Message)
}

// TestNetworkHandler_Inspect_MissingNetworkName pins the URL-param
// guard: a request without the networkName route param answers 400
// MISSING_NETWORK_NAME before the engine is reached.
func TestNetworkHandler_Inspect_MissingNetworkName(t *testing.T) {
	engine := &fakeNetworkEngine{} // inspectFn unarmed: the guard must fail first

	w := testutils.Perform(newTestHandler(engine).Inspect, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/machines/test-machine/networks//inspect",
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-machine"},
	})

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "MISSING_NETWORK_NAME", testutils.DecodeErrorEnvelope(t, w.Result()).Error.Code)
}

// networksAccessRouter mirrors app.go's mounting of the networks
// inspect route: GET /api/machines/{id}/networks/{networkName}/inspect
// rides the RequireMachineAccess subtree at read level (no requireWrite).
func networksAccessRouter(machineRepo *testutils.MockMachineRepository, accessRepo *testutils.MockMachineAccessRepository, service *NetworkService) http.Handler {
	r := http.NewServeMux()
	r.Handle("GET /api/machines/{id}/networks/{networkName}/inspect",
		appmiddleware.RequireMachineAccess(machineRepo, accessRepo)(http.HandlerFunc(NewNetworkHandler(service).Inspect)))
	return r
}

// requestAsUser builds a request carrying the given user ID in the
// context, standing in for JWTMiddleware.
func requestAsUser(method, target, userID string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	return req.WithContext(contextutils.SetUserID(req.Context(), userID))
}

// TestNetworkHandler_Inspect_ReadLevelGranteeReachesRoute pins the
// route mounting: GET /api/machines/{id}/networks/{networkName}/inspect
// rides the RequireMachineAccess subtree at read level (no requireWrite),
// so a read-level grantee reaches the handler.
func TestNetworkHandler_Inspect_ReadLevelGranteeReachesRoute(t *testing.T) {
	machineRepo := testutils.NewMockMachineRepository()
	machineRepo.AddMachine(&models.Machine{ID: "machine-1", Name: "machine-one", OwnerID: "owner-1"})
	accessRepo := testutils.NewMockMachineAccessRepository()
	accessRepo.AddAccess("machine-1", "reader-1", models.PermissionRead, time.Now())
	engine := &fakeNetworkEngine{
		inspectFn: func(name string) ([]entities.NetworkInspectReport, []error, error) {
			return []entities.NetworkInspectReport{networkInspectFixture("frontend")}, nil, nil
		},
	}
	router := networksAccessRouter(machineRepo, accessRepo, NewNetworkService(fakeEngineProvider{engine: engine}))

	w := httptest.NewRecorder()
	router.ServeHTTP(w, requestAsUser(http.MethodGet, "/api/machines/machine-1/networks/frontend/inspect", "reader-1"))

	assert.Equal(t, http.StatusOK, w.Result().StatusCode, "a read-level grantee must reach the raw inspect endpoint")
}
