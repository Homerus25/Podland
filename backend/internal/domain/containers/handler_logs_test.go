package containers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// TestContainerHandler_Logs_Success tests successful buffered log retrieval
// through the real service: the machine's log lines land in the response.
func TestContainerHandler_Logs_Success(t *testing.T) {
	engine := &fakeContainerEngine{
		logsFn: func(name string, opts entities.ContainerLogsOptions) error {
			writeLogsInto(opts, "test log line 1\ntest log line 2\n", "")
			return nil
		},
	}

	req := httptest.NewRequest("GET", "/api/machines/test-machine/containers/container-123/logs", nil).
		WithContext(authedContext())

	w := httptest.NewRecorder()
	r := chi.NewRouter()
	r.Get("/api/machines/{id}/containers/{containerId}/logs", newTestHandler(engine).Logs)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Should return 200 on success")
	assert.Contains(t, w.Body.String(), `"id":"container-123"`, "Response should contain container ID")
	assert.Contains(t, w.Body.String(), `"logs":"test log line 1\ntest log line 2"`, "Response should contain logs")
}

// TestContainerHandler_Logs_WithSinceParameter tests that the since query
// parameter is parsed and reaches the machine's log call.
func TestContainerHandler_Logs_WithSinceParameter(t *testing.T) {
	var gotOpts entities.ContainerLogsOptions
	engine := &fakeContainerEngine{
		logsFn: func(name string, opts entities.ContainerLogsOptions) error {
			gotOpts = opts
			return nil
		},
	}

	sinceTime := "2024-01-01T00:00:00Z"
	req := httptest.NewRequest("GET", "/api/machines/test-machine/containers/container-123/logs?since="+sinceTime, nil).
		WithContext(authedContext())

	w := httptest.NewRecorder()
	r := chi.NewRouter()
	r.Get("/api/machines/{id}/containers/{containerId}/logs", newTestHandler(engine).Logs)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Should return 200 on success")
	parsedSince, _ := time.Parse(time.RFC3339, sinceTime)
	assert.Equal(t, parsedSince, gotOpts.Since, "the parsed since value must reach the machine")
}

// TestContainerHandler_Logs_WithTailParameter tests that the tail query
// parameter is parsed and reaches the machine's log call.
func TestContainerHandler_Logs_WithTailParameter(t *testing.T) {
	var gotOpts entities.ContainerLogsOptions
	engine := &fakeContainerEngine{
		logsFn: func(name string, opts entities.ContainerLogsOptions) error {
			gotOpts = opts
			return nil
		},
	}

	req := httptest.NewRequest("GET", "/api/machines/test-machine/containers/container-123/logs?tail=100", nil).
		WithContext(authedContext())

	w := httptest.NewRecorder()
	r := chi.NewRouter()
	r.Get("/api/machines/{id}/containers/{containerId}/logs", newTestHandler(engine).Logs)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Should return 200 on success")
	assert.Equal(t, int64(100), gotOpts.Tail, "the parsed tail value must reach the machine")
}

// TestContainerHandler_Logs_InvalidSinceParameter tests log retrieval with invalid since parameter
func TestContainerHandler_Logs_InvalidSinceParameter(t *testing.T) {
	engine := &fakeContainerEngine{}

	req := httptest.NewRequest("GET", "/api/machines/test-machine/containers/container-123/logs?since=invalid-date", nil).
		WithContext(authedContext())

	w := httptest.NewRecorder()
	r := chi.NewRouter()
	r.Get("/api/machines/{id}/containers/{containerId}/logs", newTestHandler(engine).Logs)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 for invalid since parameter")
	assert.Contains(t, w.Body.String(), "INVALID_SINCE_PARAM", "Response should contain INVALID_SINCE_PARAM error code")
}

// TestContainerHandler_Logs_InvalidTailParameter tests log retrieval with invalid tail parameter
func TestContainerHandler_Logs_InvalidTailParameter(t *testing.T) {
	engine := &fakeContainerEngine{}

	req := httptest.NewRequest("GET", "/api/machines/test-machine/containers/container-123/logs?tail=not-a-number", nil).
		WithContext(authedContext())

	w := httptest.NewRecorder()
	r := chi.NewRouter()
	r.Get("/api/machines/{id}/containers/{containerId}/logs", newTestHandler(engine).Logs)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 for invalid tail parameter")
	assert.Contains(t, w.Body.String(), "INVALID_TAIL_PARAM", "Response should contain INVALID_TAIL_PARAM error code")
}

// TestContainerHandler_Logs_QueryErrorWithFollowRidesJSONEnvelope tests
// that a query error answers through the JSON envelope even when
// follow=true was requested: 400 before any stream begins.
func TestContainerHandler_Logs_QueryErrorWithFollowRidesJSONEnvelope(t *testing.T) {
	engine := &fakeContainerEngine{}

	req := httptest.NewRequest("GET", "/api/machines/test-machine/containers/container-123/logs?follow=true&since=invalid-date", nil).
		WithContext(authedContext())

	w := httptest.NewRecorder()
	r := chi.NewRouter()
	r.Get("/api/machines/{id}/containers/{containerId}/logs", newTestHandler(engine).Logs)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 for invalid since even with follow=true")
	assert.Contains(t, w.Body.String(), "INVALID_SINCE_PARAM", "Response should contain INVALID_SINCE_PARAM error code")
	assert.NotContains(t, w.Body.String(), "STREAM_ENDED", "No stream may begin for a query error")
}

// TestContainerHandler_Logs_MissingContainerID tests that Logs validates container ID
func TestContainerHandler_Logs_MissingContainerID(t *testing.T) {
	engine := &fakeContainerEngine{}

	req := httptest.NewRequest("GET", "/api/machines/test-machine/containers//logs", nil).
		WithContext(authedContext())

	w := httptest.NewRecorder()
	r := chi.NewRouter()
	r.Get("/api/machines/{id}/containers/{containerId}/logs", newTestHandler(engine).Logs)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 when container ID is missing")
	assert.Contains(t, w.Body.String(), "MISSING_CONTAINER_ID", "Response should contain MISSING_CONTAINER_ID error code")
}

// TestContainerHandler_Logs_StreamingHappyPath tests successful streaming
// logs with follow=true through the real service: podman's log lines are
// decoded into events, cross byte-identical, and a clean end fires exactly
// one INFO STREAM_ENDED terminal event.
func TestContainerHandler_Logs_StreamingHappyPath(t *testing.T) {
	var gotOpts entities.ContainerLogsOptions
	engine := &fakeContainerEngine{
		logsFn: func(name string, opts entities.ContainerLogsOptions) error {
			gotOpts = opts
			// Simulate podman's follow-mode delivery: each line arrives
			// with a podman timestamp prefix.
			writeLogsInto(opts, "2024-01-01T00:00:00.000000000Z Starting application\n", "")
			writeLogsInto(opts, "2024-01-01T00:00:01.000000000Z Application started successfully\n", "")
			return nil
		},
	}

	req := httptest.NewRequest("GET", "/api/machines/test-machine/containers/container-123/logs?follow=true&tail=0", nil).
		WithContext(authedContext())

	w := httptest.NewRecorder()
	r := chi.NewRouter()
	r.Get("/api/machines/{id}/containers/{containerId}/logs", newTestHandler(engine).Logs)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Should return 200 on streaming success")
	assert.True(t, gotOpts.Follow, "follow=true must reach the machine as a follow request")
	output := w.Body.String()
	assert.Contains(t, output, `"message":"Starting application"`, "Response should contain first log")
	assert.Contains(t, output, `"message":"Application started successfully"`, "Response should contain second log")
	assert.Contains(t, output, `"level":"INFO","message":"STREAM_ENDED"`, "A clean stream end must emit the STREAM_ENDED terminal event")
}

// TestContainerHandler_Logs_NonStreamingUnchanged tests that follow=false returns non-streaming response unchanged
func TestContainerHandler_Logs_NonStreamingUnchanged(t *testing.T) {
	engine := &fakeContainerEngine{
		logsFn: func(name string, opts entities.ContainerLogsOptions) error {
			writeLogsInto(opts, "test log line 1\ntest log line 2\n", "")
			return nil
		},
	}

	req := httptest.NewRequest("GET", "/api/machines/test-machine/containers/container-123/logs?follow=false", nil).
		WithContext(authedContext())

	w := httptest.NewRecorder()
	r := chi.NewRouter()
	r.Get("/api/machines/{id}/containers/{containerId}/logs", newTestHandler(engine).Logs)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Should return 200 on success")
	assert.Contains(t, w.Body.String(), `"id":"container-123"`, "Response should contain container ID")
	assert.Contains(t, w.Body.String(), `"logs":"test log line 1\ntest log line 2"`, "Response should contain logs as single field")
}
