package quadlets

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
	req := httptest.NewRequest("GET", target, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "machine123")
	rctx.URLParams.Add("quadletId", "web.container")
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
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

// TestQuadletHandler_GetLogs_WithSinceParameter tests that the since query
// parameter is parsed and reaches the machine's log call.
func TestQuadletHandler_GetLogs_WithSinceParameter(t *testing.T) {
	var gotOpts entities.ContainerLogsOptions
	engine := &fakeQuadletEngine{}
	handler := logsHandler(engine, func(containerName string, opts entities.ContainerLogsOptions) error {
		gotOpts = opts
		return nil
	})

	sinceTime := "2024-01-01T00:00:00Z"
	w := httptest.NewRecorder()
	handler.GetLogs(w, logsRequest("/api/machines/machine123/quadlets/web.container/logs?since="+sinceTime))

	require.Equal(t, http.StatusOK, w.Code)
	parsedSince, _ := time.Parse(time.RFC3339, sinceTime)
	assert.Equal(t, parsedSince, gotOpts.Since, "the parsed since value must reach the machine")
}

// TestQuadletHandler_GetLogs_WithTailParameter tests that the tail query
// parameter is parsed and reaches the machine's log call.
func TestQuadletHandler_GetLogs_WithTailParameter(t *testing.T) {
	var gotOpts entities.ContainerLogsOptions
	engine := &fakeQuadletEngine{}
	handler := logsHandler(engine, func(containerName string, opts entities.ContainerLogsOptions) error {
		gotOpts = opts
		return nil
	})

	w := httptest.NewRecorder()
	handler.GetLogs(w, logsRequest("/api/machines/machine123/quadlets/web.container/logs?tail=100"))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, int64(100), gotOpts.Tail, "the parsed tail value must reach the machine")
}

// TestQuadletHandler_GetLogs_InvalidSinceParameter tests log retrieval with invalid since parameter
func TestQuadletHandler_GetLogs_InvalidSinceParameter(t *testing.T) {
	engine := &fakeQuadletEngine{}

	w := httptest.NewRecorder()
	newTestHandler(engine).GetLogs(w, logsRequest("/api/machines/machine123/quadlets/web.container/logs?since=invalid-date"))

	assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 for invalid since parameter")
	assert.Contains(t, w.Body.String(), "INVALID_SINCE_PARAM", "Response should contain INVALID_SINCE_PARAM error code")
}

// TestQuadletHandler_GetLogs_InvalidTailParameter pins that a malformed
// tail value is IGNORED (the request proceeds with the all-lines default),
// matching this endpoint's documented behaviour.
func TestQuadletHandler_GetLogs_InvalidTailParameter(t *testing.T) {
	var gotOpts entities.ContainerLogsOptions
	engine := &fakeQuadletEngine{}
	handler := logsHandler(engine, func(containerName string, opts entities.ContainerLogsOptions) error {
		gotOpts = opts
		return nil
	})

	w := httptest.NewRecorder()
	handler.GetLogs(w, logsRequest("/api/machines/machine123/quadlets/web.container/logs?tail=not-a-number"))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, int64(-1), gotOpts.Tail, "a malformed tail must fall back to the all-lines default")
}

// TestQuadletHandler_GetLogs_MissingMachineID tests that Logs validates machine ID
func TestQuadletHandler_GetLogs_MissingMachineID(t *testing.T) {
	engine := &fakeQuadletEngine{}

	req := httptest.NewRequest("GET", "/api/machines//quadlets/web.container/logs", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("quadletId", "web.container")
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, contextutils.UserIDContextKey, "user123")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	newTestHandler(engine).GetLogs(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 when machine ID is missing")
	assert.Contains(t, w.Body.String(), "MISSING_MACHINE_ID", "Response should contain MISSING_MACHINE_ID error code")
}

// TestQuadletHandler_GetLogs_MissingQuadletID tests that Logs validates quadlet ID
func TestQuadletHandler_GetLogs_MissingQuadletID(t *testing.T) {
	engine := &fakeQuadletEngine{}

	req := httptest.NewRequest("GET", "/api/machines/machine123/quadlets//logs", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "machine123")
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, contextutils.UserIDContextKey, "user123")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	newTestHandler(engine).GetLogs(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 when quadlet ID is missing")
	assert.Contains(t, w.Body.String(), "MISSING_QUADLET_ID", "Response should contain MISSING_QUADLET_ID error code")
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
