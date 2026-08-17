package images

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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

func (m *MockImageService) PullWithProgress(ctx context.Context, userID, machineID, rawImage string, opts entities.ImagePullOptions, progressWriter func(PullProgressEvent)) (*entities.ImagePullReport, error) {
	args := m.Called(ctx, userID, machineID, rawImage, opts, progressWriter)
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

// TestImageHandler_Delete_Success tests successful image deletion
func TestImageHandler_Delete_Success(t *testing.T) {
	mockService := new(MockImageService)
	handler := NewImageHandler(mockService, nil)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	req := httptest.NewRequest("DELETE", "/api/machines/test-machine/images/sha256:abc123", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	mockService.On("Delete", mock.Anything, "test-user", "test-machine", "sha256:abc123", false).Return(nil)

	r := chi.NewRouter()
	r.Delete("/api/machines/{id}/images/{imageId}", handler.Delete)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code, "Should return 204 on success")
	mockService.AssertExpectations(t)
}

// TestImageHandler_Delete_Force tests deletion with force query parameter
func TestImageHandler_Delete_Force(t *testing.T) {
	mockService := new(MockImageService)
	handler := NewImageHandler(mockService, nil)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	req := httptest.NewRequest("DELETE", "/api/machines/test-machine/images/sha256:abc123?force=true", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	mockService.On("Delete", mock.Anything, "test-user", "test-machine", "sha256:abc123", true).Return(nil)

	r := chi.NewRouter()
	r.Delete("/api/machines/{id}/images/{imageId}", handler.Delete)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code, "Should return 204 on success with force=true")
	mockService.AssertExpectations(t)
}

// TestImageHandler_Delete_ConnectionError tests that Delete handles connection errors
func TestImageHandler_Delete_ConnectionError(t *testing.T) {
	mockService := new(MockImageService)
	handler := NewImageHandler(mockService, nil)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	req := httptest.NewRequest("DELETE", "/api/machines/test-machine/images/sha256:abc123", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	connErr := errors.New("connection failed")
	mockService.On("Delete", mock.Anything, "test-user", "test-machine", "sha256:abc123", false).Return(connErr)

	r := chi.NewRouter()
	r.Delete("/api/machines/{id}/images/{imageId}", handler.Delete)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code, "Should return 500 on connection error")
}

// TestImageHandler_Delete_MissingUserID tests that Delete validates user ID
func TestImageHandler_Delete_MissingUserID(t *testing.T) {
	mockService := new(MockImageService)
	handler := NewImageHandler(mockService, nil)

	ctx := context.Background()
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	req := httptest.NewRequest("DELETE", "/api/machines/test-machine/images/sha256:abc123", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	r := chi.NewRouter()
	r.Delete("/api/machines/{id}/images/{imageId}", handler.Delete)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 when user ID is missing")
}

// TestImageHandler_Delete_MissingMachineID tests that Delete validates machine ID
func TestImageHandler_Delete_MissingMachineID(t *testing.T) {
	mockService := new(MockImageService)
	handler := NewImageHandler(mockService, nil)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	req := httptest.NewRequest("DELETE", "/api/machines//images/sha256:abc123", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	r := chi.NewRouter()
	r.Delete("/api/machines/{id}/images/{imageId}", handler.Delete)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 when machine ID is missing")
}

// TestImageHandler_Delete_MissingImageID tests that Delete validates image ID
func TestImageHandler_Delete_MissingImageID(t *testing.T) {
	mockService := new(MockImageService)
	handler := NewImageHandler(mockService, nil)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	req := httptest.NewRequest("DELETE", "/api/machines/test-machine/images/", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	r := chi.NewRouter()
	r.Delete("/api/machines/{id}/images/{imageId}", handler.Delete)

	r.ServeHTTP(w, req)

	// Chi returns 404 for route not found when trailing slash doesn't match
	// This is expected behavior since the route requires the imageId parameter
	assert.Equal(t, http.StatusNotFound, w.Code, "Should return 404 when image ID is missing (route not matched)")
}

// TestImageHandler_Delete_ImageNotFound tests that Delete handles image not found errors
func TestImageHandler_Delete_ImageNotFound(t *testing.T) {
	mockService := new(MockImageService)
	handler := NewImageHandler(mockService, nil)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	req := httptest.NewRequest("DELETE", "/api/machines/test-machine/images/sha256:abc123", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	mockService.On("Delete", mock.Anything, "test-user", "test-machine", "sha256:abc123", false).Return(appErrors.ErrImageNotFound)

	r := chi.NewRouter()
	r.Delete("/api/machines/{id}/images/{imageId}", handler.Delete)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code, "Should return 404 when image not found")
}

// TestImageHandler_Delete_ImageUsedByContainers tests that Delete handles image in use errors
func TestImageHandler_Delete_ImageUsedByContainers(t *testing.T) {
	mockService := new(MockImageService)
	handler := NewImageHandler(mockService, nil)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	req := httptest.NewRequest("DELETE", "/api/machines/test-machine/images/sha256:abc123", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	mockService.On("Delete", mock.Anything, "test-user", "test-machine", "sha256:abc123", false).Return(appErrors.ErrImageUsedByContainers)

	r := chi.NewRouter()
	r.Delete("/api/machines/{id}/images/{imageId}", handler.Delete)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 when image is used by containers")
}

// TestImageHandler_Delete_ServiceError tests that Delete handles general service errors
func TestImageHandler_Delete_ServiceError(t *testing.T) {
	mockService := new(MockImageService)
	handler := NewImageHandler(mockService, nil)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	req := httptest.NewRequest("DELETE", "/api/machines/test-machine/images/sha256:abc123", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	mockService.On("Delete", mock.Anything, "test-user", "test-machine", "sha256:abc123", false).Return(appErrors.ErrInternalError)

	r := chi.NewRouter()
	r.Delete("/api/machines/{id}/images/{imageId}", handler.Delete)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code, "Should return 500 on general service error")
}

// TestImageHandler_Pull_MissingUserID tests that Pull handles missing user ID
func TestImageHandler_Pull_MissingUserID(t *testing.T) {
	mockService := new(MockImageService)
	handler := NewImageHandler(mockService, nil)

	ctx := context.Background()
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	req := httptest.NewRequest("POST", "/api/machines/test-machine/images/pull", nil)
	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()

	r := chi.NewRouter()
	r.Post("/api/machines/{id}/images/pull", handler.Pull)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 when user ID is missing")
}

// TestImageHandler_Pull_MissingMachineID tests that Pull handles missing machine ID
func TestImageHandler_Pull_MissingMachineID(t *testing.T) {
	mockService := new(MockImageService)
	handler := NewImageHandler(mockService, nil)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	req := httptest.NewRequest("POST", "/api/machines//images/pull", nil)
	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()

	r := chi.NewRouter()
	r.Post("/api/machines/{id}/images/pull", handler.Pull)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 when machine ID is missing")
}

// TestImageHandler_Pull_InvalidRequestBody tests that Pull handles invalid request body
func TestImageHandler_Pull_InvalidRequestBody(t *testing.T) {
	mockService := new(MockImageService)
	handler := NewImageHandler(mockService, nil)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	req := httptest.NewRequest("POST", "/api/machines/test-machine/images/pull", nil)
	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()

	r := chi.NewRouter()
	r.Post("/api/machines/{id}/images/pull", handler.Pull)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 when request body is invalid")
}

// TestImageHandler_Pull_ValidationFailure tests that Pull handles validation failures
func TestImageHandler_Pull_ValidationFailure(t *testing.T) {
	mockService := new(MockImageService)
	handler := NewImageHandler(mockService, nil)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	requestBody := map[string]any{
		"platform": "linux/amd64",
	}
	jsonBody, _ := json.Marshal(requestBody)

	req := httptest.NewRequest("POST", "/api/machines/test-machine/images/pull", bytes.NewReader(jsonBody))
	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()

	r := chi.NewRouter()
	r.Post("/api/machines/{id}/images/pull", handler.Pull)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 when validation fails")
}

// splitLines splits a string into lines, filtering out empty lines
func splitLines(s string) []string {
	lines := []string{}
	for line := range strings.SplitSeq(s, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// TestImageHandler_Pull_ServiceError tests that Pull handles service errors
func TestImageHandler_Pull_ServiceError(t *testing.T) {
	mockService := new(MockImageService)
	handler := NewImageHandler(mockService, nil)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	requestBody := map[string]string{
		"image": "nginx:latest",
	}
	jsonBody, _ := json.Marshal(requestBody)

	req := httptest.NewRequest("POST", "/api/machines/test-machine/images/pull", bytes.NewReader(jsonBody))
	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()

	mockService.On("PullWithProgress", mock.Anything, "test-user", "test-machine", "nginx:latest", mock.Anything, mock.Anything).Return((*entities.ImagePullReport)(nil), appErrors.ErrImagePullFailed)

	r := chi.NewRouter()
	r.Post("/api/machines/{id}/images/pull", handler.Pull)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Should return 200 for streaming response")
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"), "Should set Content-Type to application/json")

	// Parse the streaming response
	body := w.Body.String()
	lines := splitLines(body)

	assert.True(t, len(lines) > 0, "Should have at least one event")

	// Find the error event
	var errorEvent PullProgressEvent
	for _, line := range lines {
		var event PullProgressEvent
		if err := json.Unmarshal([]byte(line), &event); err == nil && event.Type == EventTypeError {
			errorEvent = event
			break
		}
	}

	assert.Equal(t, EventTypeError, errorEvent.Type, "Should have error event type")
	assert.Equal(t, appErrors.ErrImagePullFailed.Code, errorEvent.Error.Code, "Should have correct error code")
}

// TestImageHandler_Pull_Success_Streaming tests successful pull with streaming events
func TestImageHandler_Pull_Success_Streaming(t *testing.T) {
	mockService := new(MockImageService)
	handler := NewImageHandler(mockService, nil)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	requestBody := map[string]string{
		"image": "nginx:latest",
	}
	jsonBody, _ := json.Marshal(requestBody)

	req := httptest.NewRequest("POST", "/api/machines/test-machine/images/pull", bytes.NewReader(jsonBody))
	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()

	progressEvents := []PullProgressEvent{}
	report := &entities.ImagePullReport{
		Images: []string{"sha256:abc123"},
	}

	mockService.On("PullWithProgress", mock.Anything, "test-user", "test-machine", "nginx:latest", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			progressWriter := args.Get(5).(func(PullProgressEvent))
			// Simulate some progress events
			progressEvents = append(progressEvents, NewProgressEvent("op-1", "test-machine", "pulling", "Downloading layers", 1, 5))
			progressWriter(progressEvents[len(progressEvents)-1])

			progressEvents = append(progressEvents, NewProgressEvent("op-1", "test-machine", "pulling", "Verifying checksum", 2, 5))
			progressWriter(progressEvents[len(progressEvents)-1])
		}).
		Return(report, nil)

	r := chi.NewRouter()
	r.Post("/api/machines/{id}/images/pull", handler.Pull)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Should return 200 for streaming response")
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"), "Should set Content-Type to application/json")

	// Parse the streaming response
	body := w.Body.String()
	lines := splitLines(body)

	assert.True(t, len(lines) >= 2, "Should have at least progress and complete events")

	// Find the complete event
	var completeEvent PullProgressEvent
	for _, line := range lines {
		var event PullProgressEvent
		if err := json.Unmarshal([]byte(line), &event); err == nil && event.Type == EventTypeComplete {
			completeEvent = event
			break
		}
	}

	assert.Equal(t, EventTypeComplete, completeEvent.Type, "Should have complete event type")
	assert.Equal(t, "test-machine", completeEvent.MachineID, "Should have correct machine ID")
}

// TestImageHandler_Pull_ClientCancellation tests that client cancellation is handled
func TestImageHandler_Pull_ClientCancellation(t *testing.T) {
	mockService := new(MockImageService)
	handler := NewImageHandler(mockService, nil)

	ctx, cancel := context.WithCancel(context.Background())
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	requestBody := map[string]string{
		"image": "nginx:latest",
	}
	jsonBody, _ := json.Marshal(requestBody)

	req := httptest.NewRequest("POST", "/api/machines/test-machine/images/pull", bytes.NewReader(jsonBody))
	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()

	// Simulate immediate cancellation
	go func() {
		cancel()
	}()

	mockService.On("PullWithProgress", mock.Anything, "test-user", "test-machine", "nginx:latest", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			// Simulate context cancellation
			<-args.Get(0).(context.Context).Done()
		}).
		Return((*entities.ImagePullReport)(nil), appErrors.ErrOperationCancelled)

	r := chi.NewRouter()
	r.Post("/api/machines/{id}/images/pull", handler.Pull)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Should return 200 for streaming response")

	// Parse the streaming response
	body := w.Body.String()
	lines := splitLines(body)

	// Should have at least error event
	if len(lines) > 0 {
		var errorEvent PullProgressEvent
		for _, line := range lines {
			var event PullProgressEvent
			if err := json.Unmarshal([]byte(line), &event); err == nil && event.Type == EventTypeError {
				errorEvent = event
				break
			}
		}

		assert.Equal(t, appErrors.ErrOperationCancelled.Code, errorEvent.Error.Code, "Should have OPERATION_CANCELLED error code")
	}
}
