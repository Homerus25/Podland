package containers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

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

// TestContainerHandler_Logs_BufferedCancellationAnswers504 pins the
// buffered cancellation contract end to end: a cancelled context maps to
// 504 OPERATION_CANCELLED even when the engine returns a generic error.
func TestContainerHandler_Logs_BufferedCancellationAnswers504(t *testing.T) {
	ctx, cancel := context.WithCancel(authedContext())
	cancel()
	engine := &fakeContainerEngine{
		logsFn: func(string, entities.ContainerLogsOptions) error {
			return errors.New("boom")
		},
	}

	req := httptest.NewRequest("GET", "/api/machines/test-machine/containers/container-123/logs", nil).
		WithContext(ctx)

	w := httptest.NewRecorder()
	r := chi.NewRouter()
	r.Get("/api/machines/{id}/containers/{containerId}/logs", newTestHandler(engine).Logs)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusGatewayTimeout, w.Code,
		"a cancelled buffered fetch must answer the 504 cancellation sentinel, not a wrapped 500")
	assert.NotContains(t, w.Body.String(), "boom", "the raw cause must never ride the wire")
}

// TestContainerHandler_Logs_StreamingHappyPath tests successful streaming
// logs with follow=true through the real service: follow=true crosses the
// streaming envelope and dispatches to the service (the dispatch pin —
// the kit owns the options build and pins it once), podman's log lines
// are decoded into events, cross byte-identical, and a clean end fires
// exactly one INFO STREAM_ENDED terminal event.
func TestContainerHandler_Logs_StreamingHappyPath(t *testing.T) {
	engine := &fakeContainerEngine{
		logsFn: func(name string, opts entities.ContainerLogsOptions) error {
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
