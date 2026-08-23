package quadlets

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/Homerus25/podland/backend/internal/contextutils"
	appErrors "github.com/Homerus25/podland/backend/internal/errors"
)

// TestQuadletHandler_GetLogs_Success tests successful log retrieval
func TestQuadletHandler_GetLogs_Success(t *testing.T) {
	mockService := new(MockQuadletService)
	handler := NewQuadletHandler(mockService, nil)

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
	handler := NewQuadletHandler(mockService, nil)

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
	handler := NewQuadletHandler(mockService, nil)

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

// TestQuadletHandler_GetLogs_WithFollowParameter tests log retrieval with follow parameter
func TestQuadletHandler_GetLogs_WithFollowParameter(t *testing.T) {
	mockService := new(MockQuadletService)
	handler := NewQuadletHandler(mockService, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	req := httptest.NewRequest("GET", "/api/machines/test-machine/quadlets/my-quadlet/logs?follow=true&tail=0", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	// Mock the streaming service call to emit a log event
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

	assert.Equal(t, http.StatusOK, w.Code, "Should return 200 with follow=true")
	output := w.Body.String()
	assert.Contains(t, output, `"level":"INFO"`, "Response should contain log level")
	assert.Contains(t, output, `"message":"Starting application"`, "Response should contain first log message")
	assert.Contains(t, output, `"message":"Application started successfully"`, "Response should contain second log message")
}

// TestQuadletHandler_GetLogs_InvalidSinceParameter tests log retrieval with invalid since parameter
func TestQuadletHandler_GetLogs_InvalidSinceParameter(t *testing.T) {
	mockService := new(MockQuadletService)
	handler := NewQuadletHandler(mockService, nil)

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
	handler := NewQuadletHandler(mockService, nil)

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
	handler := NewQuadletHandler(mockService, nil)

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
	handler := NewQuadletHandler(mockService, nil)

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
	handler := NewQuadletHandler(mockService, nil)

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
	handler := NewQuadletHandler(mockService, nil)

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
	handler := NewQuadletHandler(mockService, nil)

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
}

// TestQuadletHandler_GetLogs_NonStreamingUnchanged tests that follow=false returns non-streaming response unchanged
func TestQuadletHandler_GetLogs_NonStreamingUnchanged(t *testing.T) {
	mockService := new(MockQuadletService)
	handler := NewQuadletHandler(mockService, nil)

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
	handler := NewQuadletHandler(mockService, nil)

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
	assert.Contains(t, output, `QUADLET_NOT_FOUND`, "Response should contain QUADLET_NOT_FOUND error")
}

// TestQuadletHandler_GetLogs_StreamingClientDisconnect tests client disconnect during streaming
func TestQuadletHandler_GetLogs_StreamingClientDisconnect(t *testing.T) {
	mockService := new(MockQuadletService)
	handler := NewQuadletHandler(mockService, nil)

	ctx, cancel := context.WithCancel(context.Background())
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")

	req := httptest.NewRequest("GET", "/api/machines/test-machine/quadlets/my-quadlet/logs?follow=true&tail=0", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	// Create a channel to signal when the log event is written
	logWritten := make(chan struct{}, 1)

	// Mock the streaming service call to emit log events and simulate slow operation
	mockService.mockGetLogsWithStream = func(ctx context.Context, userID, machineID, quadletName string, since time.Time, tail int64, logCallback func(event QuadletLogEvent)) error {
		logCallback(QuadletLogEvent{
			Timestamp: "2024-01-01T00:00:00Z",
			Level:     "INFO",
			Message:   "Before disconnect",
		})
		logWritten <- struct{}{}
		// Simulate a delay to ensure the disconnect channel is set up
		time.Sleep(50 * time.Millisecond)
		// Cancel the context to simulate client disconnect
		cancel()
		// Wait a bit more to ensure disconnect is processed
		time.Sleep(50 * time.Millisecond)
		return nil
	}

	r := chi.NewRouter()
	r.Get("/api/machines/{id}/quadlets/{quadletId}/logs", handler.GetLogs)

	// Run handler in a goroutine to allow for disconnect timing
	go func() {
		r.ServeHTTP(w, req)
	}()

	// Wait for the log event to be written
	<-logWritten
	// Give some time for the disconnect to be processed
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, http.StatusOK, w.Code, "Should return 200 even on client disconnect")
	output := w.Body.String()
	assert.Contains(t, output, `"message":"Before disconnect"`, "Response should contain log before disconnect")
	assert.Contains(t, output, `"OPERATION_CANCELLED"`, "Response should contain OPERATION_CANCELLED on disconnect")
}
