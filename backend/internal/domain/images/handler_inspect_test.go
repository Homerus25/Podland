package images

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
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/inspect"
)

// inspectRequest serves GET /api/machines/{id}/images/{imageId}/inspect
// through the handler with an authenticated test identity.
func inspectRequest(handler http.HandlerFunc, imageID string) *httptest.ResponseRecorder {
	return testutils.Perform(handler, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/machines/test-machine/images/" + imageID + "/inspect",
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-machine", "imageId": imageID},
	})
}

// TestImageHandler_Inspect_SuccessAnswersRawReport pins the inspect
// contract through the real service: 200 with the raw podman inspect
// report as the top-level JSON body — no DTO mapping (the podman field
// names, capital "Id" included, reach the wire untouched).
func TestImageHandler_Inspect_SuccessAnswersRawReport(t *testing.T) {
	report := &entities.ImageInspectReport{ImageData: &inspect.ImageData{
		ID:           "sha256:abc123",
		RepoTags:     []string{"nginx:latest"},
		Os:           "linux",
		Architecture: "amd64",
	}}
	engine := &fakeImageEngine{
		inspectFn: func(string) ([]*entities.ImageInspectReport, []error, error) {
			return []*entities.ImageInspectReport{report}, nil, nil
		},
	}

	w := inspectRequest(newTestHandler(engine).Inspect, "sha256:abc123")

	require.Equal(t, http.StatusOK, w.Code, "inspect must answer 200")
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "sha256:abc123", body["Id"], "the raw report's podman field names must reach the wire (no DTO mapping)")
	assert.Equal(t, []any{"nginx:latest"}, body["RepoTags"])
	assert.Equal(t, "linux", body["Os"])
	assert.Equal(t, "amd64", body["Architecture"])
}

// TestImageHandler_Inspect_MissingImageAnswers404 pins the documented
// 404 envelope end to end: the real service classifies the tunnel's
// per-item not-found answer and the web envelope maps the sentinel.
func TestImageHandler_Inspect_MissingImageAnswers404(t *testing.T) {
	engine := &fakeImageEngine{
		inspectFn: func(id string) ([]*entities.ImageInspectReport, []error, error) {
			return nil, []error{fmt.Errorf("no such image %q", id)}, nil
		},
	}

	w := inspectRequest(newTestHandler(engine).Inspect, "image-123")

	require.Equal(t, http.StatusNotFound, w.Code, "a missing image must answer 404")
	var body appErrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "test-request-id", body.RequestID)
	assert.Equal(t, "IMAGE_NOT_FOUND", body.Error.Code)
	assert.Equal(t, "Image not found", body.Error.Message)
}

// TestImageHandler_Inspect_MissingParamAnswers400 pins the URL-param
// guard: an empty imageId answers 400 with the explicit code.
func TestImageHandler_Inspect_MissingParamAnswers400(t *testing.T) {
	handler := newTestHandler(&fakeImageEngine{})

	w := testutils.Perform(handler.Inspect, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/machines/test-machine/images//inspect",
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-machine"},
	})

	require.Equal(t, http.StatusBadRequest, w.Code)
	var body appErrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "MISSING_IMAGE_ID", body.Error.Code)
}

// imagesAccessRouter mirrors app.go's mounting of the images inspect
// route: GET /api/machines/{id}/images/{imageId}/inspect rides the
// RequireMachineAccess subtree at read level (no requireWrite).
func imagesAccessRouter(machineRepo *testutils.MockMachineRepository, accessRepo *testutils.MockMachineAccessRepository, service *ImageService) http.Handler {
	r := http.NewServeMux()
	r.Handle("GET /api/machines/{id}/images/{imageId}/inspect",
		appmiddleware.RequireMachineAccess(machineRepo, accessRepo)(http.HandlerFunc(NewImageHandler(service).Inspect)))
	return r
}

// requestAsUser builds a request carrying the given user ID in the
// context, standing in for JWTMiddleware.
func requestAsUser(method, target, userID string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	return req.WithContext(contextutils.SetUserID(req.Context(), userID))
}

// TestImageHandler_Inspect_ReadLevelGranteeReachesRoute pins the route
// mounting: GET /api/machines/{id}/images/{imageId}/inspect rides the
// RequireMachineAccess subtree at read level (no requireWrite), so a
// read-level grantee reaches the handler.
func TestImageHandler_Inspect_ReadLevelGranteeReachesRoute(t *testing.T) {
	machineRepo := testutils.NewMockMachineRepository()
	machineRepo.AddMachine(&models.Machine{ID: "machine-1", Name: "machine-one", OwnerID: "owner-1"})
	accessRepo := testutils.NewMockMachineAccessRepository()
	accessRepo.AddAccess("machine-1", "reader-1", models.PermissionRead, time.Now())
	engine := &fakeImageEngine{
		inspectFn: func(string) ([]*entities.ImageInspectReport, []error, error) {
			return []*entities.ImageInspectReport{{ImageData: &inspect.ImageData{ID: "sha256:abc123"}}}, nil, nil
		},
	}
	router := imagesAccessRouter(machineRepo, accessRepo, NewImageService(fakeEngineProvider{engine: engine}))

	w := httptest.NewRecorder()
	router.ServeHTTP(w, requestAsUser(http.MethodGet, "/api/machines/machine-1/images/sha256:abc123/inspect", "reader-1"))

	assert.Equal(t, http.StatusOK, w.Result().StatusCode, "a read-level grantee must reach the raw inspect endpoint")
}
