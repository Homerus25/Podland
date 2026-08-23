package containers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/Homerus25/podland/backend/internal/contextutils"
)

// TestContainerHandler_Delete_Success tests successful container deletion
func TestContainerHandler_Delete_Success(t *testing.T) {
	mockService := new(MockContainerService)
	handler := NewContainerHandler(mockService, nil)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	req := httptest.NewRequest("DELETE", "/api/machines/test-machine/containers/container-123", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	mockService.On("Delete", mock.Anything, "test-user", "test-machine", "container-123", false).Return(nil)

	r := chi.NewRouter()
	r.Delete("/api/machines/{id}/containers/{containerId}", handler.Delete)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code, "Should return 204 on success")
	mockService.AssertExpectations(t)
}

// TestContainerHandler_Delete_Force tests deletion with force query parameter
func TestContainerHandler_Delete_Force(t *testing.T) {
	mockService := new(MockContainerService)
	handler := NewContainerHandler(mockService, nil)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	req := httptest.NewRequest("DELETE", "/api/machines/test-machine/containers/container-123?force=true", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	mockService.On("Delete", mock.Anything, "test-user", "test-machine", "container-123", true).Return(nil)

	r := chi.NewRouter()
	r.Delete("/api/machines/{id}/containers/{containerId}", handler.Delete)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code, "Should return 204 on success with force=true")
	mockService.AssertExpectations(t)
}

// TestContainerHandler_Delete_ConnectionError tests that Delete handles connection errors
func TestContainerHandler_Delete_ConnectionError(t *testing.T) {
	mockService := new(MockContainerService)
	handler := NewContainerHandler(mockService, nil)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	req := httptest.NewRequest("DELETE", "/api/machines/test-machine/containers/container-123", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	connErr := errors.New("connection failed")
	mockService.On("Delete", mock.Anything, "test-user", "test-machine", "container-123", false).Return(connErr)

	r := chi.NewRouter()
	r.Delete("/api/machines/{id}/containers/{containerId}", handler.Delete)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code, "Should return 500 on connection error")
}

// TestContainerHandler_Delete_MissingUserID tests that Delete validates user ID
func TestContainerHandler_Delete_MissingUserID(t *testing.T) {
	mockService := new(MockContainerService)
	handler := NewContainerHandler(mockService, nil)

	ctx := context.Background()

	req := httptest.NewRequest("DELETE", "/api/machines/test-machine/containers/container-123", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	r := chi.NewRouter()
	r.Delete("/api/machines/{id}/containers/{containerId}", handler.Delete)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 when user ID is missing")
}

// TestContainerHandler_Delete_MissingMachineID tests that Delete validates machine ID
func TestContainerHandler_Delete_MissingMachineID(t *testing.T) {
	mockService := new(MockContainerService)
	handler := NewContainerHandler(mockService, nil)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	req := httptest.NewRequest("DELETE", "/api/machines//containers/container-123", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	r := chi.NewRouter()
	r.Delete("/api/machines/{id}/containers/{containerId}", handler.Delete)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 when machine ID is missing")
}
