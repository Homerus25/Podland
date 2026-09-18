package quadlets

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// quadletLogsFixture builds the container-type quadlet the logs endpoints
// resolve before fetching the managed container's logs.
func quadletLogsFixture() []*entities.ListQuadlet {
	return []*entities.ListQuadlet{{
		Name:     "web.container",
		UnitName: "web.service",
		Path:     "/etc/containers/systemd/web.container",
		Status:   "running",
	}}
}

func logsRequest(target string) *http.Request {
	return logsRequestWithBase(context.Background(), target)
}

func logsRequestWithBase(base context.Context, target string) *http.Request {
	req := httptest.NewRequest("GET", target, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "machine123")
	rctx.URLParams.Add("quadletId", "web.container")
	ctx := context.WithValue(base, chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, contextutils.UserIDContextKey, "user123")
	return req.WithContext(ctx)
}

func logsHandler(engine *fakeQuadletEngine, logsFn func(containerName string, opts entities.ContainerLogsOptions) error) *QuadletHandler {
	engine.logsFn = logsFn
	engine.listFn = func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
		return quadletLogsFixture(), nil
	}
	// No explicit ContainerName in the unit file: the managed container
	// name falls back to the quadlet-name derivation.
	engine.printFn = func(string) (string, error) { return "", nil }
	return newTestHandler(engine)
}

// TestQuadletHandler_GetLogs_Success tests successful buffered log retrieval
// through the real service: the quadlet is resolved on the machine and its
// managed container's logs land in the response.
func TestQuadletHandler_GetLogs_Success(t *testing.T) {
	var gotContainer string
	engine := &fakeQuadletEngine{}
	handler := logsHandler(engine, func(containerName string, opts entities.ContainerLogsOptions) error {
		gotContainer = containerName
		_, _ = opts.StdoutWriter.Write([]byte("quadlet log line\n"))
		return nil
	})

	w := httptest.NewRecorder()
	handler.GetLogs(w, logsRequest("/api/machines/machine123/quadlets/web.container/logs"))

	require.Equal(t, http.StatusOK, w.Code, "Should return 200 on success")
	assert.Equal(t, "systemd-web", gotContainer,
		"a container quadlet logs under the systemd-spawned container name derived from the quadlet name")
	assert.Contains(t, w.Body.String(), `"id":"web.container"`, "Response should contain quadlet ID")
	assert.Contains(t, w.Body.String(), `"logs":"quadlet log line"`, "Response should contain logs")
}

// TestQuadletHandler_GetLogs_BufferedCancellationAnswers504 pins the
// buffered cancellation contract end to end: a cancelled context maps to
// 504 OPERATION_CANCELLED even when the engine returns a generic error.
func TestQuadletHandler_GetLogs_BufferedCancellationAnswers504(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	engine := &fakeQuadletEngine{}
	handler := logsHandler(engine, func(string, entities.ContainerLogsOptions) error {
		return errors.New("boom")
	})

	w := httptest.NewRecorder()
	handler.GetLogs(w, logsRequestWithBase(ctx, "/api/machines/machine123/quadlets/web.container/logs"))

	assert.Equal(t, http.StatusGatewayTimeout, w.Code,
		"a cancelled buffered fetch must answer the 504 cancellation sentinel, not a wrapped 500")
	assert.NotContains(t, w.Body.String(), "boom", "the raw cause must never ride the wire")
}

// TestQuadletHandler_GetLogs_QueryErrorWithFollowRidesJSONEnvelope tests
// that a query error answers through the JSON envelope even when
// follow=true was requested: 400 before any stream begins.
func TestQuadletHandler_GetLogs_QueryErrorWithFollowRidesJSONEnvelope(t *testing.T) {
	engine := &fakeQuadletEngine{}

	w := httptest.NewRecorder()
	newTestHandler(engine).GetLogs(w, logsRequest("/api/machines/machine123/quadlets/web.container/logs?follow=true&since=invalid-date"))

	assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 for invalid since even with follow=true")
	assert.Contains(t, w.Body.String(), "INVALID_SINCE_PARAM", "Response should contain INVALID_SINCE_PARAM error code")
	assert.NotContains(t, w.Body.String(), "STREAM_ENDED", "No stream may begin for a query error")
}

// TestQuadletHandler_GetLogs_StreamingHappyPath tests successful streaming
// logs with follow=true through the real service: podman's timestamped log
// lines are decoded into events, cross with their levels, and a clean end
// fires exactly one INFO STREAM_ENDED terminal event.
func TestQuadletHandler_GetLogs_StreamingHappyPath(t *testing.T) {
	var gotOpts entities.ContainerLogsOptions
	engine := &fakeQuadletEngine{}
	handler := logsHandler(engine, func(containerName string, opts entities.ContainerLogsOptions) error {
		gotOpts = opts
		_, _ = opts.StdoutWriter.Write([]byte("2024-01-01T00:00:00Z Starting application\n"))
		_, _ = opts.StdoutWriter.Write([]byte("2024-01-01T00:00:01Z Application started successfully\n"))
		return nil
	})

	w := httptest.NewRecorder()
	handler.GetLogs(w, logsRequest("/api/machines/machine123/quadlets/web.container/logs?follow=true&tail=0"))

	require.Equal(t, http.StatusOK, w.Code, "Should return 200 on streaming success")
	assert.True(t, gotOpts.Follow, "follow=true must reach the machine as a follow request")
	output := w.Body.String()
	assert.Contains(t, output, `"message":"Starting application"`, "Response should contain first log")
	assert.Contains(t, output, `"message":"Application started successfully"`, "Response should contain second log")
	assert.Contains(t, output, `"level":"INFO","message":"STREAM_ENDED"`, "A clean stream end must emit the STREAM_ENDED terminal event")
}

// TestQuadletHandler_GetLogs_NonStreamingUnchanged tests that follow=false returns non-streaming response unchanged
func TestQuadletHandler_GetLogs_NonStreamingUnchanged(t *testing.T) {
	engine := &fakeQuadletEngine{}
	handler := logsHandler(engine, func(containerName string, opts entities.ContainerLogsOptions) error {
		_, _ = opts.StdoutWriter.Write([]byte("quadlet log line\n"))
		return nil
	})

	w := httptest.NewRecorder()
	handler.GetLogs(w, logsRequest("/api/machines/machine123/quadlets/web.container/logs?follow=false"))

	require.Equal(t, http.StatusOK, w.Code)
	var got QuadletLogsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "web.container", got.ID)
	assert.Equal(t, "quadlet log line", got.Logs)
}
