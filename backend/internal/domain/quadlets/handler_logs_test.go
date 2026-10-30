package quadlets

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	"github.com/Homerus25/podland/backend/internal/testutils"
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
// through the real service: the quadlet is resolved on the machine, the
// buffered service call receives the right target and window through the
// HTTP surface, and its managed container's logs land in the response.
func TestQuadletHandler_GetLogs_Success(t *testing.T) {
	got := testutils.CallCapture[entities.ContainerLogsOptions]{}
	engine := &fakeQuadletEngine{}
	handler := logsHandler(engine, func(containerName string, opts entities.ContainerLogsOptions) error {
		got.Record(containerName, opts)
		_, _ = opts.StdoutWriter.Write([]byte("quadlet log line\n"))
		return nil
	})

	w := httptest.NewRecorder()
	handler.GetLogs(w, logsRequest("/api/machines/machine123/quadlets/web.container/logs?since=2024-01-01T00:00:00Z&tail=3"))

	require.Equal(t, http.StatusOK, w.Code, "Should return 200 on success")
	assert.Equal(t, "systemd-web", got.Name,
		"a container quadlet logs under the systemd-spawned container name derived from the quadlet name")
	assert.False(t, got.Opts.Follow, "the buffered service call must receive the buffered follow flag")
	assert.Equal(t, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), got.Opts.Since,
		"the buffered service call must receive the parsed since")
	assert.Equal(t, int64(3), got.Opts.Tail, "the buffered service call must receive the parsed tail")
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

// TestQuadletHandler_GetLogs_StreamingHappyPath tests successful streaming
// logs with follow=true through the real service: the follow service call
// receives the right target and window through the HTTP surface (the
// buffered/follow dispatch fork itself is pinned once in the streaming
// kit's suite), podman's log lines are decoded into events, cross with
// their levels, and a clean end fires exactly one INFO STREAM_ENDED
// terminal event.
func TestQuadletHandler_GetLogs_StreamingHappyPath(t *testing.T) {
	got := testutils.CallCapture[entities.ContainerLogsOptions]{}
	engine := &fakeQuadletEngine{}
	handler := logsHandler(engine, func(containerName string, opts entities.ContainerLogsOptions) error {
		got.Record(containerName, opts)
		// Simulate podman's follow-mode delivery: each line arrives
		// with a podman timestamp prefix.
		_, _ = opts.StdoutWriter.Write([]byte("2024-01-01T00:00:00Z Starting application\n"))
		_, _ = opts.StdoutWriter.Write([]byte("2024-01-01T00:00:01Z Application started successfully\n"))
		return nil
	})

	w := httptest.NewRecorder()
	handler.GetLogs(w, logsRequest("/api/machines/machine123/quadlets/web.container/logs?follow=true&since=2024-01-01T00:00:00Z&tail=0"))

	require.Equal(t, http.StatusOK, w.Code, "Should return 200 on streaming success")
	assert.Equal(t, "systemd-web", got.Name,
		"the follow service call must receive the resolved container target")
	assert.True(t, got.Opts.Follow, "the follow service call must receive the follow flag")
	assert.True(t, got.Opts.Timestamps, "the follow service call must receive the timestamps flag")
	assert.Equal(t, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), got.Opts.Since,
		"the follow service call must receive the parsed since")
	assert.Equal(t, int64(0), got.Opts.Tail, "the follow service call must receive the parsed tail")
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
