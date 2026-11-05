package images

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	appmiddleware "github.com/Homerus25/podland/backend/internal/middleware"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/inspect"
)

// getRequest serves GET /api/hosts/{id}/images/{imageId} through the
// handler with an authenticated test identity.
func getRequest(handler http.HandlerFunc, imageID string) *httptest.ResponseRecorder {
	return testutils.Perform(handler, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/hosts/test-host/images/" + imageID,
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-host", "imageId": imageID},
	})
}

// fullInspectReport is the engine fixture for the detail endpoint: list
// fields on the ImageData, inspect fields on its Config.
func fullInspectReport() *entities.ImageInspectReport {
	created := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	return &entities.ImageInspectReport{ImageData: &inspect.ImageData{
		ID:           "sha256:abc123",
		RepoTags:     []string{"nginx:latest"},
		RepoDigests:  []string{"nginx@sha256:digest1"},
		Created:      &created,
		Size:         102400000,
		Os:           "linux",
		Architecture: "amd64",
		Config: &v1.ImageConfig{
			User:       "nginx",
			Env:        []string{"PATH=/usr/bin"},
			Entrypoint: []string{"/docker-entrypoint.sh"},
			Cmd:        []string{"nginx"},
			Labels:     map[string]string{"maintainer": "NGINX"},
		},
	}}
}

// TestImageHandler_Get_SuccessAnswersDetailDTO pins the detail contract
// through the real service: 200 with the ImageDetailResponse DTO — list
// fields combined with the inspect report's config fields.
func TestImageHandler_Get_SuccessAnswersDetailDTO(t *testing.T) {
	engine := &fakeImageEngine{
		inspectFn: func(string) ([]*entities.ImageInspectReport, []error, error) {
			return []*entities.ImageInspectReport{fullInspectReport()}, nil, nil
		},
	}

	w := getRequest(newTestHandler(engine).Get, "sha256:abc123")

	require.Equal(t, http.StatusOK, w.Code, "get must answer 200")
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var body ImageDetailResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, ImageDetailResponse{
		ID:           "sha256:abc123",
		RepoTags:     []string{"nginx:latest"},
		RepoDigests:  []string{"nginx@sha256:digest1"},
		CreatedAt:    "2026-10-18T12:00:00Z",
		Size:         102400000,
		OS:           "linux",
		Architecture: "amd64",
		User:         "nginx",
		Env:          map[string]string{"PATH": "/usr/bin"},
		Entrypoint:   []string{"/docker-entrypoint.sh"},
		Cmd:          []string{"nginx"},
		Labels:       map[string]string{"maintainer": "NGINX"},
	}, body)
}

// TestImageHandler_Get_MissingImageAnswers404 pins the documented 404
// envelope end to end: the real service classifies the tunnel's
// per-item not-found answer and the web envelope maps the sentinel.
func TestImageHandler_Get_MissingImageAnswers404(t *testing.T) {
	engine := &fakeImageEngine{
		inspectFn: func(id string) ([]*entities.ImageInspectReport, []error, error) {
			return nil, []error{fmt.Errorf("no such image %q", id)}, nil
		},
	}

	w := getRequest(newTestHandler(engine).Get, "image-123")

	require.Equal(t, http.StatusNotFound, w.Code, "a missing image must answer 404")
	var body appErrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "test-request-id", body.RequestID)
	assert.Equal(t, "IMAGE_NOT_FOUND", body.Error.Code)
	assert.Equal(t, "Image not found", body.Error.Message)
}

// TestImageHandler_Get_MissingParamAnswers400 pins the URL-param guard:
// an empty imageId answers 400 with the explicit code.
func TestImageHandler_Get_MissingParamAnswers400(t *testing.T) {
	handler := newTestHandler(&fakeImageEngine{})

	w := testutils.Perform(handler.Get, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/hosts/test-host/images/",
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-host"},
	})

	require.Equal(t, http.StatusBadRequest, w.Code)
	var body appErrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "MISSING_IMAGE_ID", body.Error.Code)
}

// imagesGetAccessRouter mirrors app.go's mounting of the images detail
// route: GET /api/hosts/{id}/images/{imageId} rides the
// RequireHostAccess subtree at read level (no requireWrite).
func imagesGetAccessRouter(hostRepo *testutils.MockHostRepository, accessRepo *testutils.MockHostAccessRepository, service *ImageService) http.Handler {
	r := http.NewServeMux()
	r.Handle("GET /api/hosts/{id}/images/{imageId}",
		appmiddleware.RequireHostAccess(hostRepo, accessRepo)(http.HandlerFunc(NewImageHandler(service).Get)))
	return r
}

// TestImageHandler_Get_ReadLevelGranteeReachesRoute pins the route
// mounting: GET /api/hosts/{id}/images/{imageId} rides the
// RequireHostAccess subtree at read level (no requireWrite), so a
// read-level grantee reaches the handler.
func TestImageHandler_Get_ReadLevelGranteeReachesRoute(t *testing.T) {
	hostRepo := testutils.NewMockHostRepository()
	hostRepo.AddHost(&models.Host{ID: "host-1", Name: "host-one", OwnerID: "owner-1"})
	accessRepo := testutils.NewMockHostAccessRepository()
	accessRepo.AddAccess("host-1", "reader-1", models.PermissionRead, time.Now())
	engine := &fakeImageEngine{
		inspectFn: func(string) ([]*entities.ImageInspectReport, []error, error) {
			return []*entities.ImageInspectReport{fullInspectReport()}, nil, nil
		},
	}
	router := imagesGetAccessRouter(hostRepo, accessRepo, NewImageService(fakeEngineProvider{engine: engine}))

	w := httptest.NewRecorder()
	router.ServeHTTP(w, requestAsUser(http.MethodGet, "/api/hosts/host-1/images/sha256:abc123", "reader-1"))

	assert.Equal(t, http.StatusOK, w.Result().StatusCode, "a read-level grantee must reach the detail endpoint")
}
