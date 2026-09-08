package quadlets

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
)

// TestQuadletHandler_GetLogs_Success tests successful log retrieval
func TestQuadletHandler_GetLogs_Success(t *testing.T) {
	mockService := new(MockQuadletService)
	handler := NewQuadletHandler(mockService)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	req := httptest.NewRequest("GET", "/api/machines/test-machine/quadlets/my-quadlet/logs", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	expectedResponse := &QuadletLogsResponse{
		ID:   "my-quadlet",
		Logs: "test log line 1\ntest log line 2",
	}
	mockService.mockGetLogs = func(ctx context.Context, userID, machineID, quadletName string, since time.Time, tail int64) (*QuadletLogsResponse, error) {
		return expectedResponse, nil
	}

	r := chi.NewRouter()
	r.Get("/api/machines/{id}/quadlets/{quadletId}/logs", handler.GetLogs)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Should return 200 on success")
	assert.Contains(t, w.Body.String(), `"id":"my-quadlet"`, "Response should contain quadlet ID")
	assert.Contains(t, w.Body.String(), `"logs":"test log line 1\ntest log line 2"`, "Response should contain logs")
}

// TestQuadletHandler_GetLogs_WithSinceParameter tests log retrieval with since parameter
func TestQuadletHandler_GetLogs_WithSinceParameter(t *testing.T) {
	mockService := new(MockQuadletService)
	handler := NewQuadletHandler(mockService)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	sinceTime := "2024-01-01T00:00:00Z"
	req := httptest.NewRequest("GET", "/api/machines/test-machine/quadlets/my-quadlet/logs?since="+sinceTime, nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	expectedResponse := &QuadletLogsResponse{
		ID:   "my-quadlet",
		Logs: "recent logs",
	}

	mockService.mockGetLogs = func(ctx context.Context, userID, machineID, quadletName string, since time.Time, tail int64) (*QuadletLogsResponse, error) {
		return expectedResponse, nil
	}

	r := chi.NewRouter()
	r.Get("/api/machines/{id}/quadlets/{quadletId}/logs", handler.GetLogs)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Should return 200 on success")
}

// TestQuadletHandler_GetLogs_WithTailParameter tests log retrieval with tail parameter
func TestQuadletHandler_GetLogs_WithTailParameter(t *testing.T) {
	mockService := new(MockQuadletService)
	handler := NewQuadletHandler(mockService)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	req := httptest.NewRequest("GET", "/api/machines/test-machine/quadlets/my-quadlet/logs?tail=100", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	expectedResponse := &QuadletLogsResponse{
		ID:   "my-quadlet",
		Logs: "last 100 lines",
	}
	mockService.mockGetLogs = func(ctx context.Context, userID, machineID, quadletName string, since time.Time, tail int64) (*QuadletLogsResponse, error) {
		return expectedResponse, nil
	}

	r := chi.NewRouter()
	r.Get("/api/machines/{id}/quadlets/{quadletId}/logs", handler.GetLogs)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Should return 200 on success")
}

// TestQuadletHandler_GetLogs_InvalidSinceParameter tests log retrieval with invalid since parameter
func TestQuadletHandler_GetLogs_InvalidSinceParameter(t *testing.T) {
	mockService := new(MockQuadletService)
	handler := NewQuadletHandler(mockService)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	req := httptest.NewRequest("GET", "/api/machines/test-machine/quadlets/my-quadlet/logs?since=invalid-date", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	r := chi.NewRouter()
	r.Get("/api/machines/{id}/quadlets/{quadletId}/logs", handler.GetLogs)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 for invalid since parameter")
	assert.Contains(t, w.Body.String(), "INVALID_SINCE_PARAM", "Response should contain INVALID_SINCE_PARAM error code")
}

// TestQuadletHandler_GetLogs_QuadletNotFound tests that GetLogs handles quadlet not found error
func TestQuadletHandler_GetLogs_QuadletNotFound(t *testing.T) {
	mockService := new(MockQuadletService)
	handler := NewQuadletHandler(mockService)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	req := httptest.NewRequest("GET", "/api/machines/test-machine/quadlets/my-quadlet/logs", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	mockService.mockGetLogs = func(ctx context.Context, userID, machineID, quadletName string, since time.Time, tail int64) (*QuadletLogsResponse, error) {
		return nil, appErrors.ErrQuadletNotFound
	}

	r := chi.NewRouter()
	r.Get("/api/machines/{id}/quadlets/{quadletId}/logs", handler.GetLogs)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code, "Should return 404 when quadlet not found")
	assert.Contains(t, w.Body.String(), "QUADLET_NOT_FOUND", "Response should contain QUADLET_NOT_FOUND error code")
}

// TestQuadletHandler_GetLogs_ConnectionError tests that GetLogs handles connection errors
func TestQuadletHandler_GetLogs_ConnectionError(t *testing.T) {
	mockService := new(MockQuadletService)
	handler := NewQuadletHandler(mockService)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	req := httptest.NewRequest("GET", "/api/machines/test-machine/quadlets/my-quadlet/logs", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	connErr := fmt.Errorf("connection failed")
	mockService.mockGetLogs = func(ctx context.Context, userID, machineID, quadletName string, since time.Time, tail int64) (*QuadletLogsResponse, error) {
		return nil, connErr
	}

	r := chi.NewRouter()
	r.Get("/api/machines/{id}/quadlets/{quadletId}/logs", handler.GetLogs)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code, "Should return 500 on connection error")
}

// TestQuadletHandler_GetLogs_MissingUserID tests that GetLogs validates user ID
func TestQuadletHandler_GetLogs_MissingUserID(t *testing.T) {
	mockService := new(MockQuadletService)
	handler := NewQuadletHandler(mockService)

	ctx := context.Background()

	req := httptest.NewRequest("GET", "/api/machines/test-machine/quadlets/my-quadlet/logs", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	r := chi.NewRouter()
	r.Get("/api/machines/{id}/quadlets/{quadletId}/logs", handler.GetLogs)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 when user ID is missing")
}

// TestQuadletHandler_GetLogs_MissingMachineID tests that GetLogs validates machine ID
func TestQuadletHandler_GetLogs_MissingMachineID(t *testing.T) {
	mockService := new(MockQuadletService)
	handler := NewQuadletHandler(mockService)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	req := httptest.NewRequest("GET", "/api/machines//quadlets/my-quadlet/logs", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	r := chi.NewRouter()
	r.Get("/api/machines/{id}/quadlets/{quadletId}/logs", handler.GetLogs)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 when machine ID is missing")
	assert.Contains(t, w.Body.String(), "MISSING_MACHINE_ID", "Response should contain MISSING_MACHINE_ID error code")
}

// TestQuadletHandler_GetLogs_MissingQuadletID tests that GetLogs validates quadlet ID
func TestQuadletHandler_GetLogs_MissingQuadletID(t *testing.T) {
	mockService := new(MockQuadletService)
	handler := NewQuadletHandler(mockService)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	req := httptest.NewRequest("GET", "/api/machines/test-machine/quadlets//logs", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	r := chi.NewRouter()
	r.Get("/api/machines/{id}/quadlets/{quadletId}/logs", handler.GetLogs)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 when quadlet ID is missing")
	assert.Contains(t, w.Body.String(), "MISSING_QUADLET_ID", "Response should contain MISSING_QUADLET_ID error code")
}

// TestQuadletHandler_GetLogs_StreamingHappyPath tests successful streaming logs with follow=true
func TestQuadletHandler_GetLogs_StreamingHappyPath(t *testing.T) {
	mockService := new(MockQuadletService)
	handler := NewQuadletHandler(mockService)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	req := httptest.NewRequest("GET", "/api/machines/test-machine/quadlets/my-quadlet/logs?follow=true&tail=0", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	// Mock the streaming service call to emit some log events
	mockService.mockGetLogsWithStream = func(ctx context.Context, userID, machineID, quadletName string, since time.Time, tail int64, logCallback func(event QuadletLogEvent)) error {
		logCallback(QuadletLogEvent{
			Timestamp: "2024-01-01T00:00:00Z",
			Level:     "INFO",
			Message:   "Starting application",
		})
		logCallback(QuadletLogEvent{
			Timestamp: "2024-01-01T00:00:01Z",
			Level:     "INFO",
			Message:   "Application started successfully",
		})
		return nil
	}

	r := chi.NewRouter()
	r.Get("/api/machines/{id}/quadlets/{quadletId}/logs", handler.GetLogs)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Should return 200 on streaming success")
	output := w.Body.String()
	assert.Contains(t, output, `"message":"Starting application"`, "Response should contain first log")
	assert.Contains(t, output, `"message":"Application started successfully"`, "Response should contain second log")
	assert.Contains(t, output, `"level":"INFO","message":"STREAM_ENDED"`, "A clean stream end must emit the STREAM_ENDED terminal event")
}

// TestQuadletHandler_GetLogs_NonStreamingUnchanged tests that follow=false returns non-streaming response unchanged
func TestQuadletHandler_GetLogs_NonStreamingUnchanged(t *testing.T) {
	mockService := new(MockQuadletService)
	handler := NewQuadletHandler(mockService)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	req := httptest.NewRequest("GET", "/api/machines/test-machine/quadlets/my-quadlet/logs?follow=false", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	expectedResponse := &QuadletLogsResponse{
		ID:   "my-quadlet",
		Logs: "test log line 1\ntest log line 2",
	}
	mockService.mockGetLogs = func(ctx context.Context, userID, machineID, quadletName string, since time.Time, tail int64) (*QuadletLogsResponse, error) {
		return expectedResponse, nil
	}

	r := chi.NewRouter()
	r.Get("/api/machines/{id}/quadlets/{quadletId}/logs", handler.GetLogs)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Should return 200 on success")
	assert.Contains(t, w.Body.String(), `"id":"my-quadlet"`, "Response should contain quadlet ID")
	assert.Contains(t, w.Body.String(), `"logs":"test log line 1\ntest log line 2"`, "Response should contain logs as single field")
}

// TestQuadletHandler_GetLogs_StreamingQuadletNotFound tests streaming with quadlet not found
func TestQuadletHandler_GetLogs_StreamingQuadletNotFound(t *testing.T) {
	mockService := new(MockQuadletService)
	handler := NewQuadletHandler(mockService)

	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	req := httptest.NewRequest("GET", "/api/machines/test-machine/quadlets/my-quadlet/logs?follow=true&tail=0", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	// Mock the streaming service call to return quadlet not found error
	mockService.mockGetLogsWithStream = func(ctx context.Context, userID, machineID, quadletName string, since time.Time, tail int64, logCallback func(event QuadletLogEvent)) error {
		return appErrors.ErrQuadletNotFound
	}

	r := chi.NewRouter()
	r.Get("/api/machines/{id}/quadlets/{quadletId}/logs", handler.GetLogs)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Should return 200 even on error in streaming mode")
	output := w.Body.String()
	assert.Contains(t, output, `"level":"ERROR"`, "Response should contain error level")
	assert.Contains(t, output, `"message":"QUADLET_NOT_FOUND: Quadlet not found"`,
		"Terminal errors are log-shaped CODE: message events the frontend parser understands")
	assert.NotContains(t, output, `"code":`, "The ad-hoc code-field map shape is gone")
}

// TestQuadletHandler_GetLogs_StreamingMissingUserID pins the streaming
// branch's identity ruling: a missing identity is a wiring bug answered
// with 500 INTERNAL_ERROR, not a client error.
func TestQuadletHandler_GetLogs_StreamingMissingUserID(t *testing.T) {
	mockService := new(MockQuadletService)
	handler := NewQuadletHandler(mockService)

	req := httptest.NewRequest("GET", "/api/machines/test-machine/quadlets/my-quadlet/logs?follow=true", nil)
	w := httptest.NewRecorder()

	r := chi.NewRouter()
	r.Get("/api/machines/{id}/quadlets/{quadletId}/logs", handler.GetLogs)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code, "Missing identity on a streaming route is a wiring bug: 500")
	assert.Contains(t, w.Body.String(), "INTERNAL_ERROR", "Response should contain INTERNAL_ERROR error code")
}
