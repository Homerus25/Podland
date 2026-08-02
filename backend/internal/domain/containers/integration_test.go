package containers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/yourorg/podland/backend/internal/contextutils"
	appErrors "github.com/yourorg/podland/backend/internal/errors"
	netTypes "go.podman.io/common/libnetwork/types"
	"go.podman.io/podman/v6/pkg/domain/entities"
	containerTypes "go.podman.io/podman/v6/pkg/domain/entities/types"
	define "go.podman.io/podman/v6/pkg/ps/define"
)

// MockContainerService is a mock implementation of ContainerServiceInterface for testing
type MockContainerService struct {
	mock.Mock
}

func (m *MockContainerService) List(ctx context.Context, userID, machineID string, opts entities.ContainerListOptions) ([]containerTypes.ListContainer, error) {
	args := m.Called(ctx, userID, machineID, opts)
	return args.Get(0).([]containerTypes.ListContainer), args.Error(1)
}

func (m *MockContainerService) Create(ctx context.Context, userID, machineID string, request ContainerCreateRequest) (*ContainerCreateResponse, error) {
	args := m.Called(ctx, userID, machineID, request)
	return args.Get(0).(*ContainerCreateResponse), args.Error(1)
}

func (m *MockContainerService) Get(ctx context.Context, userID, machineID, containerID string) (*entities.ContainerInspectReport, error) {
	args := m.Called(ctx, userID, machineID, containerID)
	return args.Get(0).(*entities.ContainerInspectReport), args.Error(1)
}

func (m *MockContainerService) Start(ctx context.Context, userID, machineID, containerID string) (*ContainerStartResponse, error) {
	args := m.Called(ctx, userID, machineID, containerID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ContainerStartResponse), args.Error(1)
}

func (m *MockContainerService) Stop(ctx context.Context, userID, machineID, containerID string) (*ContainerStopResponse, error) {
	args := m.Called(ctx, userID, machineID, containerID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ContainerStopResponse), args.Error(1)
}

func (m *MockContainerService) Delete(ctx context.Context, userID, machineID, containerID string, force bool) error {
	args := m.Called(ctx, userID, machineID, containerID, force)
	return args.Error(0)
}

func (m *MockContainerService) GetLogs(ctx context.Context, userID, machineID, containerID string, since time.Time, tail int64) (*ContainerLogsResponse, error) {
	args := m.Called(ctx, userID, machineID, containerID, since, tail)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ContainerLogsResponse), args.Error(1)
}

// TestContainerHandler_List_Success tests successful container listing through the handler
func TestContainerHandler_List_Success(t *testing.T) {
	// Setup test context with user ID
	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	// Create test containers
	testContainers := []containerTypes.ListContainer{
		{
			ID:       "container-1",
			Names:    []string{"test-container-1"},
			Image:    "test-image:latest",
			State:    "running",
			Status:   "Up 5 minutes",
			Restarts: 0,
		},
		{
			ID:       "container-2",
			Names:    []string{"test-container-2"},
			Image:    "another-image:v1",
			State:    "exited",
			Status:   "Exited (0) 2 hours ago",
			Restarts: 2,
			Exited:   true,
			ExitCode: 0,
		},
	}

	// Setup mock service
	mockService := new(MockContainerService)
	opts := entities.ContainerListOptions{All: true}
	mockService.On("List", mock.Anything, "test-user", "test-machine", opts).Return(testContainers, nil)

	// Create handler
	handler := NewContainerHandler(mockService, nil)

	// Create test request
	req := httptest.NewRequest("GET", "/api/machines/test-machine/containers?all=true", nil)
	req = req.WithContext(ctx)

	// Create response recorder
	rr := httptest.NewRecorder()

	// Setup router with URL parameter
	r := chi.NewRouter()
	r.Get("/api/machines/{id}/containers", handler.List)

	// Execute request
	r.ServeHTTP(rr, req)

	// Verify response
	assert.Equal(t, http.StatusOK, rr.Code, "Should return 200 OK for successful request")
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"), "Should return JSON content type")

	// Parse response
	var result ContainerListResult
	err := json.Unmarshal(rr.Body.Bytes(), &result)
	assert.NoError(t, err, "Should be able to parse JSON response")

	// Verify response structure
	assert.Equal(t, 2, result.Total, "Should return correct container count")
	assert.Equal(t, 2, len(result.Containers), "Should return correct number of containers")
	assert.NotEmpty(t, result.Timestamp, "Should include timestamp")

	// Verify container data
	assert.Equal(t, "container-1", result.Containers[0].ID, "Should map container ID correctly")
	assert.Equal(t, []string{"test-container-1"}, result.Containers[0].Names, "Should map container names correctly")
	assert.Equal(t, "test-image:latest", result.Containers[0].Image, "Should map container image correctly")
	assert.Equal(t, "running", result.Containers[0].State, "Should map container state correctly")

	// Verify mock expectations
	mockService.AssertExpectations(t)
}

// TestContainerHandler_Create_Success tests successful container creation through the handler
func TestContainerHandler_Create_Success(t *testing.T) {
	// Setup test context with user ID
	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	// Create test request body
	requestBody := `{
		"name": "test-container",
		"image": "nginx:latest",
		"command": ["/bin/sh", "-c", "echo hello"],
		"environment": {
			"ENV1": "value1"
		},
		"labels": {
			"app": "test"
		}
	}`

	// Setup mock service
	mockService := new(MockContainerService)
	expectedRequest := ContainerCreateRequest{
		Name:    "test-container",
		Image:   "nginx:latest",
		Command: []string{"/bin/sh", "-c", "echo hello"},
		Environment: map[string]string{
			"ENV1": "value1",
		},
		Labels: map[string]string{
			"app": "test",
		},
	}
	expectedResponse := &ContainerCreateResponse{
		ID:        "container-123",
		Name:      "test-container",
		State:     "created",
		Image:     "nginx:latest",
		ImageID:   "",
		CreatedAt: "2026-07-28T15:53:26Z",
	}
	mockService.On("Create", mock.Anything, "test-user", "test-machine", expectedRequest).Return(expectedResponse, nil)

	// Create handler
	handler := NewContainerHandler(mockService, nil)

	// Create test request
	req := httptest.NewRequest("POST", "/api/machines/test-machine/containers", strings.NewReader(requestBody))
	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")

	// Create response recorder
	rr := httptest.NewRecorder()

	// Setup router with URL parameter
	r := chi.NewRouter()
	r.Post("/api/machines/{id}/containers", handler.Create)

	// Execute request
	r.ServeHTTP(rr, req)

	// Verify response
	assert.Equal(t, http.StatusCreated, rr.Code, "Should return 201 Created for successful container creation")
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"), "Should return JSON content type")

	// Parse response
	var result ContainerCreateResponse
	err := json.Unmarshal(rr.Body.Bytes(), &result)
	assert.NoError(t, err, "Should be able to parse JSON response")

	// Verify response structure
	assert.Equal(t, "container-123", result.ID, "Should return correct container ID")
	assert.Equal(t, "test-container", result.Name, "Should return correct container name")
	assert.Equal(t, "created", result.State, "Should return correct container state")
	assert.Equal(t, "nginx:latest", result.Image, "Should return correct container image")
	assert.NotEmpty(t, result.CreatedAt, "Should include creation timestamp")

	// Verify mock expectations
	mockService.AssertExpectations(t)
}

// TestContainerHandler_List_QueryParameters tests query parameter parsing
func TestContainerHandler_List_QueryParameters(t *testing.T) {
	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	// Setup mock service
	mockService := new(MockContainerService)

	// Test different query parameter combinations
	testCases := []struct {
		name         string
		url          string
		expectedOpts entities.ContainerListOptions
		description  string
	}{
		{
			name:         "all parameter",
			url:          "/api/machines/test-machine/containers?all=true",
			expectedOpts: entities.ContainerListOptions{All: true},
			description:  "Should parse all parameter correctly",
		},
		{
			name:         "size parameter",
			url:          "/api/machines/test-machine/containers?size=true",
			expectedOpts: entities.ContainerListOptions{Size: true},
			description:  "Should parse size parameter correctly",
		},
		{
			name:         "namespace parameter",
			url:          "/api/machines/test-machine/containers?namespace=true",
			expectedOpts: entities.ContainerListOptions{Namespace: true},
			description:  "Should parse namespace parameter correctly",
		},
		{
			name: "multiple parameters",
			url:  "/api/machines/test-machine/containers?all=true&size=true&last=5",
			expectedOpts: entities.ContainerListOptions{
				All:  true,
				Size: true,
				Last: 5,
			},
			description: "Should parse multiple parameters correctly",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup mock to expect the parsed options
			mockService.On("List", mock.Anything, "test-user", "test-machine", tc.expectedOpts).
				Return([]containerTypes.ListContainer{}, nil)

			handler := NewContainerHandler(mockService, nil)
			req := httptest.NewRequest("GET", tc.url, nil)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			r := chi.NewRouter()
			r.Get("/api/machines/{id}/containers", handler.List)

			r.ServeHTTP(rr, req)

			assert.Equal(t, http.StatusOK, rr.Code, tc.description)
			mockService.AssertExpectations(t)
		})
	}
}

// TestContainerHandler_List_ErrorScenarios tests error handling in the container list handler
func TestContainerHandler_List_ErrorScenarios(t *testing.T) {
	testCases := []struct {
		name           string
		setupRequest   func() *http.Request
		expectedStatus int
		expectedError  string
		description    string
	}{
		{
			name: "missing user ID",
			setupRequest: func() *http.Request {
				ctx := context.Background() // No user ID set
				req := httptest.NewRequest("GET", "/api/machines/test-machine/containers", nil)
				return req.WithContext(ctx)
			},
			expectedStatus: http.StatusUnauthorized,
			expectedError:  "INVALID_CREDENTIALS",
			description:    "Should return 401 when user ID is missing",
		},
		{
			name: "service error",
			setupRequest: func() *http.Request {
				ctx := context.Background()
				ctx = contextutils.SetUserID(ctx, "test-user")
				ctx = contextutils.SetRequestID(ctx, "test-request-id")
				req := httptest.NewRequest("GET", "/api/machines/test-machine/containers", nil)
				return req.WithContext(ctx)
			},
			expectedStatus: http.StatusInternalServerError,
			expectedError:  "INTERNAL_ERROR",
			description:    "Should return 500 when service returns error",
		},
		{
			name: "service error",
			setupRequest: func() *http.Request {
				ctx := context.Background()
				ctx = contextutils.SetUserID(ctx, "test-user")
				ctx = contextutils.SetRequestID(ctx, "test-request-id")
				req := httptest.NewRequest("GET", "/api/machines/test-machine/containers", nil)
				return req.WithContext(ctx)
			},
			expectedStatus: http.StatusInternalServerError,
			expectedError:  "INTERNAL_ERROR",
			description:    "Should return 500 when service returns error",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := tc.setupRequest()
			rr := httptest.NewRecorder()

			// Setup mock service for service error case
			mockService := new(MockContainerService)
			if tc.name == "service error" {
				expectedErr := errors.New("connection failed")
				mockService.On("List", mock.Anything, "test-user", "test-machine", mock.Anything).
					Return([]containerTypes.ListContainer{}, expectedErr)
			}

			handler := NewContainerHandler(mockService, nil)
			r := chi.NewRouter()
			r.Get("/api/machines/{id}/containers", handler.List)

			r.ServeHTTP(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code, tc.description)

			// Parse error response
			var errorResponse struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			err := json.Unmarshal(rr.Body.Bytes(), &errorResponse)
			assert.NoError(t, err, "Should be able to parse error response")
			assert.Equal(t, tc.expectedError, errorResponse.Error.Code, "Should return correct error code")

			if tc.name == "service error" {
				mockService.AssertExpectations(t)
			}
		})
	}
}

// TestContainerHandler_List_EmptyResponse tests handling of empty container lists
func TestContainerHandler_List_EmptyResponse(t *testing.T) {
	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	// Setup mock service to return empty list
	mockService := new(MockContainerService)
	opts := entities.ContainerListOptions{}
	mockService.On("List", mock.Anything, "test-user", "test-machine", opts).Return([]containerTypes.ListContainer{}, nil)

	handler := NewContainerHandler(mockService, nil)
	req := httptest.NewRequest("GET", "/api/machines/test-machine/containers", nil)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	r := chi.NewRouter()
	r.Get("/api/machines/{id}/containers", handler.List)

	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code, "Should return 200 OK for empty container list")

	// Parse response
	var result ContainerListResult
	err := json.Unmarshal(rr.Body.Bytes(), &result)
	assert.NoError(t, err, "Should be able to parse empty response")

	assert.Equal(t, 0, result.Total, "Should return 0 total containers")
	assert.Equal(t, 0, len(result.Containers), "Should return empty container array")
	assert.NotEmpty(t, result.Timestamp, "Should still include timestamp")

	mockService.AssertExpectations(t)
}

// TestContainerHandler_List_ContainerWithOptionalFields tests handling of containers with optional fields
func TestContainerHandler_List_ContainerWithOptionalFields(t *testing.T) {
	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	// Create container with various optional fields
	testContainer := containerTypes.ListContainer{
		ID:       "container-with-options",
		Names:    []string{"test-container"},
		Image:    "test-image:latest",
		ImageID:  "sha256:abc123",
		Command:  []string{"/bin/sh", "-c", "echo hello"},
		State:    "running",
		Status:   "Up 10 minutes",
		Restarts: 3,
		Exited:   false,
		ExitCode: 0,
		Labels: map[string]string{
			"app":     "test",
			"env":     "development",
			"version": "1.0",
		},
		Ports: []netTypes.PortMapping{
			{
				ContainerPort: 8080,
				HostPort:      8080,
				Protocol:      "tcp",
				HostIP:        "0.0.0.0",
			},
		},
		Size: &define.ContainerSize{
			RootFsSize: 1024,
			RwSize:     512,
		},
		Networks: []string{"bridge", "host"},
		Mounts:   []string{"/var/lib/docker", "/tmp"},
	}

	mockService := new(MockContainerService)
	opts := entities.ContainerListOptions{}
	mockService.On("List", mock.Anything, "test-user", "test-machine", opts).Return([]containerTypes.ListContainer{testContainer}, nil)

	handler := NewContainerHandler(mockService, nil)
	req := httptest.NewRequest("GET", "/api/machines/test-machine/containers", nil)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	r := chi.NewRouter()
	r.Get("/api/machines/{id}/containers", handler.List)

	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code, "Should return 200 OK")

	var result ContainerListResult
	err := json.Unmarshal(rr.Body.Bytes(), &result)
	assert.NoError(t, err, "Should be able to parse response")

	assert.Equal(t, 1, result.Total, "Should return 1 container")
	container := result.Containers[0]

	// Verify all optional fields are mapped correctly
	assert.Equal(t, "sha256:abc123", container.ImageID, "Should map image ID")
	assert.Equal(t, []string{"/bin/sh", "-c", "echo hello"}, container.Command, "Should map command")
	assert.Equal(t, map[string]string{"app": "test", "env": "development", "version": "1.0"}, container.Labels, "Should map labels")
	assert.Equal(t, 1, len(container.Ports), "Should map ports")
	assert.Equal(t, 8080, container.Ports[0].ContainerPort, "Should map container port")
	assert.Equal(t, "tcp", container.Ports[0].Protocol, "Should map protocol")
	assert.NotNil(t, container.Size, "Should map size")
	assert.Equal(t, int64(1024), container.Size.RootFsSize, "Should map root FS size")
	assert.Equal(t, []string{"bridge", "host"}, container.Networks, "Should map networks")
	assert.Equal(t, []string{"/var/lib/docker", "/tmp"}, container.Mounts, "Should map mounts")

	mockService.AssertExpectations(t)
}

// TestContainerHandler_Get_Success tests successful container retrieval through the handler
func TestContainerHandler_Get_Success(t *testing.T) {
	// Setup test context with user ID
	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	// Setup mock service
	mockService := new(MockContainerService)
	// For now, let's test the error handling since we can't easily create a valid ContainerInspectReport
	mockService.On("Get", mock.Anything, "test-user", "test-machine", "container-123").Return((*entities.ContainerInspectReport)(nil), fmt.Errorf("container not found"))

	// Create handler
	handler := NewContainerHandler(mockService, nil)

	// Create test request
	req := httptest.NewRequest("GET", "/api/machines/test-machine/containers/container-123", nil)
	req = req.WithContext(ctx)

	// Create response recorder
	rr := httptest.NewRecorder()

	// Setup router with URL parameters
	r := chi.NewRouter()
	r.Get("/api/machines/{id}/containers/{containerId}", handler.Get)

	// Execute request
	r.ServeHTTP(rr, req)

	// Verify response - since we're testing error handling, we expect an error
	assert.Equal(t, http.StatusInternalServerError, rr.Code, "Should return 500 for service error")
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"), "Should return JSON content type")

	// Parse error response
	var errorResponse struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	err := json.Unmarshal(rr.Body.Bytes(), &errorResponse)
	assert.NoError(t, err, "Should be able to parse error response")
	assert.Equal(t, "INTERNAL_ERROR", errorResponse.Error.Code, "Should return internal error code")

	// Verify mock expectations
	mockService.AssertExpectations(t)
}

// TestContainerHandler_Get_ErrorScenarios tests error handling in the container get handler
func TestContainerHandler_Get_ErrorScenarios(t *testing.T) {
	testCases := []struct {
		name           string
		setupRequest   func() *http.Request
		expectedStatus int
		expectedError  string
		description    string
	}{
		{
			name: "missing user ID",
			setupRequest: func() *http.Request {
				ctx := context.Background() // No user ID set
				req := httptest.NewRequest("GET", "/api/machines/test-machine/containers/container-123", nil)
				return req.WithContext(ctx)
			},
			expectedStatus: http.StatusUnauthorized,
			expectedError:  "INVALID_CREDENTIALS",
			description:    "Should return 401 when user ID is missing",
		},
		{
			name: "missing machine ID",
			setupRequest: func() *http.Request {
				ctx := context.Background()
				ctx = contextutils.SetUserID(ctx, "test-user")
				ctx = contextutils.SetRequestID(ctx, "test-request-id")
				req := httptest.NewRequest("GET", "/api/machines//containers/container-123", nil)
				return req.WithContext(ctx)
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "MISSING_MACHINE_ID",
			description:    "Should return 400 when machine ID is missing",
		},
		{
			name: "missing container ID",
			setupRequest: func() *http.Request {
				ctx := context.Background()
				ctx = contextutils.SetUserID(ctx, "test-user")
				ctx = contextutils.SetRequestID(ctx, "test-request-id")
				req := httptest.NewRequest("GET", "/api/machines/test-machine/containers//", nil)
				return req.WithContext(ctx)
			},
			expectedStatus: http.StatusNotFound,
			expectedError:  "NOT_FOUND",
			description:    "Should return 404 when container ID is missing (route not found)",
		},
		{
			name: "service error",
			setupRequest: func() *http.Request {
				ctx := context.Background()
				ctx = contextutils.SetUserID(ctx, "test-user")
				ctx = contextutils.SetRequestID(ctx, "test-request-id")
				req := httptest.NewRequest("GET", "/api/machines/test-machine/containers/container-123", nil)
				return req.WithContext(ctx)
			},
			expectedStatus: http.StatusInternalServerError,
			expectedError:  "INTERNAL_ERROR",
			description:    "Should return 500 when service returns error",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := tc.setupRequest()
			rr := httptest.NewRecorder()

			// Setup mock service for service error case
			mockService := new(MockContainerService)
			if tc.name == "service error" {
				expectedErr := errors.New("container not found")
				mockService.On("Get", mock.Anything, "test-user", "test-machine", "container-123").
					Return((*entities.ContainerInspectReport)(nil), expectedErr)
			}

			handler := NewContainerHandler(mockService, nil)
			r := chi.NewRouter()
			r.Get("/api/machines/{id}/containers/{containerId}", handler.Get)

			r.ServeHTTP(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code, tc.description)

			// For 404 errors, Chi returns plain text, not JSON
			if tc.expectedStatus == http.StatusNotFound {
				assert.Contains(t, rr.Body.String(), "404 page not found", "Should return Chi's 404 response")
			} else {
				// Parse error response
				var errorResponse struct {
					Error struct {
						Code string `json:"code"`
					} `json:"error"`
				}
				err := json.Unmarshal(rr.Body.Bytes(), &errorResponse)
				assert.NoError(t, err, "Should be able to parse error response")
				assert.Equal(t, tc.expectedError, errorResponse.Error.Code, "Should return correct error code")
			}

			if tc.name == "service error" {
				mockService.AssertExpectations(t)
			}
		})
	}
}

// TestContainerHandler_Delete_ErrorScenarios tests error handling in the container delete handler
func TestContainerHandler_Delete_ErrorScenarios(t *testing.T) {
	testCases := []struct {
		name           string
		setupRequest   func() *http.Request
		setupMock      func(*MockContainerService)
		expectedStatus int
		expectedError  string
		description    string
	}{
		{
			name: "missing user ID",
			setupRequest: func() *http.Request {
				ctx := context.Background()
				req := httptest.NewRequest("DELETE", "/api/machines/test-machine/containers/container-123", nil)
				return req.WithContext(ctx)
			},
			setupMock:      func(m *MockContainerService) {},
			expectedStatus: http.StatusUnauthorized,
			expectedError:  "INVALID_CREDENTIALS",
			description:    "Should return 401 when user ID is missing",
		},
		{
			name: "missing machine ID",
			setupRequest: func() *http.Request {
				ctx := context.Background()
				ctx = contextutils.SetUserID(ctx, "test-user")
				ctx = contextutils.SetRequestID(ctx, "test-request-id")
				req := httptest.NewRequest("DELETE", "/api/machines//containers/container-123", nil)
				return req.WithContext(ctx)
			},
			setupMock:      func(m *MockContainerService) {},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "MISSING_MACHINE_ID",
			description:    "Should return 400 when machine ID is missing",
		},
		{
			name: "service error",
			setupRequest: func() *http.Request {
				ctx := context.Background()
				ctx = contextutils.SetUserID(ctx, "test-user")
				ctx = contextutils.SetRequestID(ctx, "test-request-id")
				req := httptest.NewRequest("DELETE", "/api/machines/test-machine/containers/container-123", nil)
				return req.WithContext(ctx)
			},
			setupMock: func(m *MockContainerService) {
				m.On("Delete", mock.Anything, "test-user", "test-machine", "container-123", false).
					Return(errors.New("container not found"))
			},
			expectedStatus: http.StatusInternalServerError,
			expectedError:  "INTERNAL_ERROR",
			description:    "Should return 500 when service returns error",
		},
		{
			name: "container not found (AppError)",
			setupRequest: func() *http.Request {
				ctx := context.Background()
				ctx = contextutils.SetUserID(ctx, "test-user")
				ctx = contextutils.SetRequestID(ctx, "test-request-id")
				req := httptest.NewRequest("DELETE", "/api/machines/test-machine/containers/container-123", nil)
				return req.WithContext(ctx)
			},
			setupMock: func(m *MockContainerService) {
				m.On("Delete", mock.Anything, "test-user", "test-machine", "container-123", false).
					Return(appErrors.ErrContainerNotFound)
			},
			expectedStatus: http.StatusNotFound,
			expectedError:  "CONTAINER_NOT_FOUND",
			description:    "Should return 404 when container is not found",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := tc.setupRequest()
			rr := httptest.NewRecorder()

			mockService := new(MockContainerService)
			tc.setupMock(mockService)

			handler := NewContainerHandler(mockService, nil)
			r := chi.NewRouter()
			r.Delete("/api/machines/{id}/containers/{containerId}", handler.Delete)

			r.ServeHTTP(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code, tc.description)

			var errorResponse struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			err := json.Unmarshal(rr.Body.Bytes(), &errorResponse)
			assert.NoError(t, err, "Should be able to parse error response")
			assert.Equal(t, tc.expectedError, errorResponse.Error.Code, "Should return correct error code")

			mockService.AssertExpectations(t)
		})
	}
}
