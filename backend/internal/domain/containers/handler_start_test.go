package containers

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/yourorg/podland/backend/internal/contextutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// TestContainerHandler_Start_ConnectionError tests that Start handles connection errors
func TestContainerHandler_Start_ConnectionError(t *testing.T) {
	mockService := new(MockContainerService)
	handler := NewContainerHandler(mockService, nil)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	req := httptest.NewRequest("POST", "/api/machines/test-machine/containers/container-123/start", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	connErr := errors.New("connection failed")
	mockService.On("Start", mock.Anything, "test-user", "test-machine", "container-123").Return(nil, connErr)

	r := chi.NewRouter()
	r.Post("/api/machines/{id}/containers/{containerId}/start", handler.Start)

	r.ServeHTTP(w, req)

	assert.Equal(t, 500, w.Code, "Should return 500 on connection error")
}

// TestContainerHandler_Start_Success tests successful container start
func TestContainerHandler_Start_Success(t *testing.T) {
	mockService := new(MockContainerService)
	handler := NewContainerHandler(mockService, nil)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	req := httptest.NewRequest("POST", "/api/machines/test-machine/containers/container-123/start", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	expectedResponse := &ContainerStartResponse{
		ID:        "container-123",
		Name:      "test-container",
		State:     "running",
		Timestamp: "2026-07-30T13:00:00Z",
	}
	mockService.On("Start", mock.Anything, "test-user", "test-machine", "container-123").Return(expectedResponse, nil)

	r := chi.NewRouter()
	r.Post("/api/machines/{id}/containers/{containerId}/start", handler.Start)

	r.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code, "Should return 200 on success")
}
