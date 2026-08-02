package containers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/yourorg/podland/backend/internal/contextutils"
	appErrors "github.com/yourorg/podland/backend/internal/errors"
)

// TestContainerHandler_Logs_Success tests successful log retrieval
func TestContainerHandler_Logs_Success(t *testing.T) {
	mockService := new(MockContainerService)
	handler := NewContainerHandler(mockService, nil)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	req := httptest.NewRequest("GET", "/api/machines/test-machine/containers/container-123/logs", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	expectedResponse := &ContainerLogsResponse{
		ID:   "container-123",
		Logs: "test log line 1\ntest log line 2",
	}
	mockService.On("GetLogs", mock.Anything, "test-user", "test-machine", "container-123", time.Time{}, int64(0)).Return(expectedResponse, nil)

	r := chi.NewRouter()
	r.Get("/api/machines/{id}/containers/{containerId}/logs", handler.Logs)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Should return 200 on success")
	assert.Contains(t, w.Body.String(), `"id":"container-123"`, "Response should contain container ID")
	assert.Contains(t, w.Body.String(), `"logs":"test log line 1\ntest log line 2"`, "Response should contain logs")
	mockService.AssertExpectations(t)
}

// TestContainerHandler_Logs_WithSinceParameter tests log retrieval with since parameter
func TestContainerHandler_Logs_WithSinceParameter(t *testing.T) {
	mockService := new(MockContainerService)
	handler := NewContainerHandler(mockService, nil)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	sinceTime := "2024-01-01T00:00:00Z"
	req := httptest.NewRequest("GET", "/api/machines/test-machine/containers/container-123/logs?since="+sinceTime, nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	expectedResponse := &ContainerLogsResponse{
		ID:   "container-123",
		Logs: "recent logs",
	}
	
	parsedSince, _ := time.Parse(time.RFC3339, sinceTime)
	mockService.On("GetLogs", mock.Anything, "test-user", "test-machine", "container-123", parsedSince, int64(0)).Return(expectedResponse, nil)

	r := chi.NewRouter()
	r.Get("/api/machines/{id}/containers/{containerId}/logs", handler.Logs)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Should return 200 on success")
	mockService.AssertExpectations(t)
}

// TestContainerHandler_Logs_WithTailParameter tests log retrieval with tail parameter
func TestContainerHandler_Logs_WithTailParameter(t *testing.T) {
	mockService := new(MockContainerService)
	handler := NewContainerHandler(mockService, nil)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	req := httptest.NewRequest("GET", "/api/machines/test-machine/containers/container-123/logs?tail=100", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	expectedResponse := &ContainerLogsResponse{
		ID:   "container-123",
		Logs: "last 100 lines",
	}
	mockService.On("GetLogs", mock.Anything, "test-user", "test-machine", "container-123", time.Time{}, int64(100)).Return(expectedResponse, nil)

	r := chi.NewRouter()
	r.Get("/api/machines/{id}/containers/{containerId}/logs", handler.Logs)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Should return 200 on success")
	mockService.AssertExpectations(t)
}

// TestContainerHandler_Logs_WithFollowParameter tests log retrieval with follow parameter
func TestContainerHandler_Logs_WithFollowParameter(t *testing.T) {
	mockService := new(MockContainerService)
	handler := NewContainerHandler(mockService, nil)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	// follow=true is accepted but not implemented in this slice
	req := httptest.NewRequest("GET", "/api/machines/test-machine/containers/container-123/logs?follow=true", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	expectedResponse := &ContainerLogsResponse{
		ID:   "container-123",
		Logs: "logs without streaming",
	}
	mockService.On("GetLogs", mock.Anything, "test-user", "test-machine", "container-123", time.Time{}, int64(0)).Return(expectedResponse, nil)

	r := chi.NewRouter()
	r.Get("/api/machines/{id}/containers/{containerId}/logs", handler.Logs)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Should return 200 even with follow=true")
	mockService.AssertExpectations(t)
}

// TestContainerHandler_Logs_InvalidSinceParameter tests log retrieval with invalid since parameter
func TestContainerHandler_Logs_InvalidSinceParameter(t *testing.T) {
	mockService := new(MockContainerService)
	handler := NewContainerHandler(mockService, nil)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	req := httptest.NewRequest("GET", "/api/machines/test-machine/containers/container-123/logs?since=invalid-date", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	r := chi.NewRouter()
	r.Get("/api/machines/{id}/containers/{containerId}/logs", handler.Logs)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 for invalid since parameter")
	assert.Contains(t, w.Body.String(), "INVALID_SINCE_PARAM", "Response should contain INVALID_SINCE_PARAM error code")
}

// TestContainerHandler_Logs_InvalidTailParameter tests log retrieval with invalid tail parameter
func TestContainerHandler_Logs_InvalidTailParameter(t *testing.T) {
	mockService := new(MockContainerService)
	handler := NewContainerHandler(mockService, nil)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	req := httptest.NewRequest("GET", "/api/machines/test-machine/containers/container-123/logs?tail=not-a-number", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	r := chi.NewRouter()
	r.Get("/api/machines/{id}/containers/{containerId}/logs", handler.Logs)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 for invalid tail parameter")
	assert.Contains(t, w.Body.String(), "INVALID_TAIL_PARAM", "Response should contain INVALID_TAIL_PARAM error code")
}

// TestContainerHandler_Logs_ContainerNotFound tests that Logs handles container not found error
func TestContainerHandler_Logs_ContainerNotFound(t *testing.T) {
	mockService := new(MockContainerService)
	handler := NewContainerHandler(mockService, nil)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	req := httptest.NewRequest("GET", "/api/machines/test-machine/containers/container-123/logs", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	mockService.On("GetLogs", mock.Anything, "test-user", "test-machine", "container-123", time.Time{}, int64(0)).Return(nil, appErrors.ErrContainerNotFound)

	r := chi.NewRouter()
	r.Get("/api/machines/{id}/containers/{containerId}/logs", handler.Logs)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code, "Should return 404 when container not found")
	assert.Contains(t, w.Body.String(), "CONTAINER_NOT_FOUND", "Response should contain CONTAINER_NOT_FOUND error code")
}

// TestContainerHandler_Logs_ConnectionError tests that Logs handles connection errors
func TestContainerHandler_Logs_ConnectionError(t *testing.T) {
	mockService := new(MockContainerService)
	handler := NewContainerHandler(mockService, nil)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	req := httptest.NewRequest("GET", "/api/machines/test-machine/containers/container-123/logs", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	connErr := fmt.Errorf("connection failed")
	mockService.On("GetLogs", mock.Anything, "test-user", "test-machine", "container-123", time.Time{}, int64(0)).Return(nil, connErr)

	r := chi.NewRouter()
	r.Get("/api/machines/{id}/containers/{containerId}/logs", handler.Logs)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code, "Should return 500 on connection error")
}

// TestContainerHandler_Logs_MissingUserID tests that Logs validates user ID
func TestContainerHandler_Logs_MissingUserID(t *testing.T) {
	mockService := new(MockContainerService)
	handler := NewContainerHandler(mockService, nil)

	ctx := context.Background()

	req := httptest.NewRequest("GET", "/api/machines/test-machine/containers/container-123/logs", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	r := chi.NewRouter()
	r.Get("/api/machines/{id}/containers/{containerId}/logs", handler.Logs)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 when user ID is missing")
}

// TestContainerHandler_Logs_MissingMachineID tests that Logs validates machine ID
func TestContainerHandler_Logs_MissingMachineID(t *testing.T) {
	mockService := new(MockContainerService)
	handler := NewContainerHandler(mockService, nil)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	req := httptest.NewRequest("GET", "/api/machines//containers/container-123/logs", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	r := chi.NewRouter()
	r.Get("/api/machines/{id}/containers/{containerId}/logs", handler.Logs)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 when machine ID is missing")
	assert.Contains(t, w.Body.String(), "MISSING_MACHINE_ID", "Response should contain MISSING_MACHINE_ID error code")
}

// TestContainerHandler_Logs_MissingContainerID tests that Logs validates container ID
func TestContainerHandler_Logs_MissingContainerID(t *testing.T) {
	mockService := new(MockContainerService)
	handler := NewContainerHandler(mockService, nil)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	req := httptest.NewRequest("GET", "/api/machines/test-machine/containers//logs", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	r := chi.NewRouter()
	r.Get("/api/machines/{id}/containers/{containerId}/logs", handler.Logs)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 when container ID is missing")
	assert.Contains(t, w.Body.String(), "MISSING_CONTAINER_ID", "Response should contain MISSING_CONTAINER_ID error code")
}