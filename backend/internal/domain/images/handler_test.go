package images

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/yourorg/podland/backend/internal/contextutils"
	appErrors "github.com/yourorg/podland/backend/internal/errors"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// MockImageService is a mock implementation of ImageServiceInterface for testing.
type MockImageService struct {
	mock.Mock
}

func (m *MockImageService) List(ctx context.Context, userID, machineID string, opts entities.ImageListOptions) ([]*entities.ImageSummary, error) {
	args := m.Called(ctx, userID, machineID, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*entities.ImageSummary), args.Error(1)
}

func (m *MockImageService) Delete(ctx context.Context, userID, machineID, imageID string, force bool) error {
	args := m.Called(ctx, userID, machineID, imageID, force)
	return args.Error(0)
}

func (m *MockImageService) Pull(ctx context.Context, userID, machineID, rawImage string, opts entities.ImagePullOptions) (*entities.ImagePullReport, error) {
	args := m.Called(ctx, userID, machineID, rawImage, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entities.ImagePullReport), args.Error(1)
}

// TestImageHandler_List_Success tests successful image listing through the handler.
func TestImageHandler_List_Success(t *testing.T) {
	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	created := int64(1700000000)
	testImages := []*entities.ImageSummary{
		{
			ID:          "sha256:abc123",
			RepoTags:    []string{"nginx:latest"},
			RepoDigests: []string{"nginx@sha256:digest1"},
			ParentId:    "sha256:parent1",
			Created:     created,
			Size:        102400000,
			Os:          "linux",
			Arch:        "amd64",
			Labels:      map[string]string{"maintainer": "NGINX"},
		},
		{
			ID:       "sha256:def456",
			RepoTags: []string{"alpine:3.18"},
			Created:  created,
			Size:     5000000,
		},
	}

	mockService := new(MockImageService)
	opts := entities.ImageListOptions{}
	mockService.On("List", mock.Anything, "test-user", "test-machine", opts).Return(testImages, nil)

	handler := NewImageHandler(mockService, nil)

	req := httptest.NewRequest("GET", "/api/machines/test-machine/images", nil)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()

	r := chi.NewRouter()
	r.Get("/api/machines/{id}/images", handler.List)
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code, "Should return 200 OK")
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var result []ImageListResponse
	err := json.Unmarshal(rr.Body.Bytes(), &result)
	assert.NoError(t, err, "Should be able to parse JSON response")
	assert.Equal(t, 2, len(result), "Should return correct number of images")

	assert.Equal(t, "sha256:abc123", result[0].ID)
	assert.Equal(t, []string{"nginx:latest"}, result[0].RepoTags)
	assert.Equal(t, []string{"nginx@sha256:digest1"}, result[0].RepoDigests)
	assert.Equal(t, "sha256:parent1", result[0].ParentID)
	assert.Equal(t, int64(102400000), result[0].Size)
	assert.Equal(t, "linux", result[0].OS)
	assert.Equal(t, "amd64", result[0].Architecture)
	assert.Equal(t, "NGINX", result[0].Labels["maintainer"])

	assert.Equal(t, "sha256:def456", result[1].ID)
	assert.Empty(t, result[1].ParentID)
	assert.Empty(t, result[1].OS)

	mockService.AssertExpectations(t)
}

// TestImageHandler_List_MissingUserID tests that List returns 401 when user ID is missing.
func TestImageHandler_List_MissingUserID(t *testing.T) {
	mockService := new(MockImageService)
	handler := NewImageHandler(mockService, nil)

	ctx := context.Background()
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	req := httptest.NewRequest("GET", "/api/machines/test-machine/images", nil)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()

	r := chi.NewRouter()
	r.Get("/api/machines/{id}/images", handler.List)
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code, "Should return 401 when user ID is missing")
}

// TestImageHandler_List_MissingMachineID tests that List returns 400 when machine ID is missing.
func TestImageHandler_List_MissingMachineID(t *testing.T) {
	mockService := new(MockImageService)
	handler := NewImageHandler(mockService, nil)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	req := httptest.NewRequest("GET", "/api/machines//images", nil)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()

	r := chi.NewRouter()
	r.Get("/api/machines/{id}/images", handler.List)
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code, "Should return 400 when machine ID is missing")
}

// TestImageHandler_List_ServiceError tests that List returns 500 when the service returns an error.
func TestImageHandler_List_ServiceError(t *testing.T) {
	mockService := new(MockImageService)
	handler := NewImageHandler(mockService, nil)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	opts := entities.ImageListOptions{}
	mockService.On("List", mock.Anything, "test-user", "test-machine", opts).
		Return(([]*entities.ImageSummary)(nil), errors.New("connection refused"))

	req := httptest.NewRequest("GET", "/api/machines/test-machine/images", nil)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()

	r := chi.NewRouter()
	r.Get("/api/machines/{id}/images", handler.List)
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code, "Should return 500 on service error")
	mockService.AssertExpectations(t)
}

// TestImageHandler_List_AppError tests that List responds with the structured error
// when the service returns a known AppError.
func TestImageHandler_List_AppError(t *testing.T) {
	mockService := new(MockImageService)
	handler := NewImageHandler(mockService, nil)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	opts := entities.ImageListOptions{}
	mockService.On("List", mock.Anything, "test-user", "test-machine", opts).
		Return(([]*entities.ImageSummary)(nil), appErrors.ErrConnectionFailed)

	req := httptest.NewRequest("GET", "/api/machines/test-machine/images", nil)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()

	r := chi.NewRouter()
	r.Get("/api/machines/{id}/images", handler.List)
	r.ServeHTTP(rr, req)

	assert.Equal(t, appErrors.ErrConnectionFailed.Status, rr.Code,
		"Should return the AppError's HTTP status")
}

// TestImageHandler_List_AllQueryParam tests that the all query parameter is parsed
// and passed to the service.
func TestImageHandler_List_AllQueryParam(t *testing.T) {
	mockService := new(MockImageService)
	handler := NewImageHandler(mockService, nil)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	opts := entities.ImageListOptions{All: true}
	mockService.On("List", mock.Anything, "test-user", "test-machine", opts).
		Return([]*entities.ImageSummary{}, nil)

	req := httptest.NewRequest("GET", "/api/machines/test-machine/images?all=true", nil)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()

	r := chi.NewRouter()
	r.Get("/api/machines/{id}/images", handler.List)
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	mockService.AssertExpectations(t)
}
