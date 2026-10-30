package pods

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// logsRequest serves GET
// /api/machines/{id}/pods/{podId}/logs{query} through the handler with
// an authenticated test identity.
func logsRequest(handler http.HandlerFunc, podID, query string) *httptest.ResponseRecorder {
	return logsRequestWithBase(handler, context.Background(), podID, query)
}

// logsRequestWithBase serves the same request over a caller-supplied
// base context (a cancelled one, for the cancellation pin).
func logsRequestWithBase(handler http.HandlerFunc, base context.Context, podID, query string) *httptest.ResponseRecorder {
	return testutils.Perform(handler, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/machines/test-machine/pods/" + podID + "/logs" + query,
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-machine", "podId": podID},
		Context:   base,
	})
}

// TestPodHandler_Logs_Success tests successful buffered log retrieval
// through the real service: the buffered service call receives the right
// target and window through the HTTP surface, and the pod's aggregated
// log lines land in the response.
func TestPodHandler_Logs_Success(t *testing.T) {
	got := testutils.CallCapture[entities.ContainerLogsOptions]{}
	engine := &fakePodEngine{
		logsFn: func(name string, opts entities.ContainerLogsOptions) error {
			got.Record(name, opts)
			writeLogsInto(opts, "test log line 1\ntest log line 2\n", "")
			return nil
		},
	}

	w := logsRequest(newTestHandler(engine).Logs, "pod-123", "?since=2024-01-01T00:00:00Z&tail=3")

	assert.Equal(t, http.StatusOK, w.Code, "Should return 200 on success")
	assert.Equal(t, "pod-123", got.Name, "the buffered service call must receive the resolved target")
	assert.False(t, got.Opts.Follow, "the buffered service call must receive the buffered follow flag")
	assert.Equal(t, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), got.Opts.Since,
		"the buffered service call must receive the parsed since")
	assert.Equal(t, int64(3), got.Opts.Tail, "the buffered service call must receive the parsed tail")
	assert.Contains(t, w.Body.String(), `"id":"pod-123"`, "Response should contain pod ID")
	assert.Contains(t, w.Body.String(), `"logs":"test log line 1\ntest log line 2"`, "Response should contain logs")
}

// TestPodHandler_Logs_BufferedCancellationAnswers504 pins the
// buffered cancellation contract end to end: a cancelled context maps to
// 504 OPERATION_CANCELLED even when the engine returns a generic error.
func TestPodHandler_Logs_BufferedCancellationAnswers504(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	engine := &fakePodEngine{
		logsFn: func(string, entities.ContainerLogsOptions) error {
			return errors.New("boom")
		},
	}

	w := logsRequestWithBase(newTestHandler(engine).Logs, ctx, "pod-123", "")

	assert.Equal(t, http.StatusGatewayTimeout, w.Code,
		"a cancelled buffered fetch must answer the 504 cancellation sentinel, not a wrapped 500")
	assert.NotContains(t, w.Body.String(), "boom", "the raw cause must never ride the wire")
}

// TestPodHandler_Logs_StreamingHappyPath tests successful streaming
// logs with follow=true through the real service: the follow service call
// receives the right target and window through the HTTP surface (the
// buffered/follow dispatch fork itself is pinned once in the streaming
// kit's suite), podman's log lines are decoded into events, cross
// byte-identical, and a clean end fires exactly one INFO STREAM_ENDED
// terminal event.
func TestPodHandler_Logs_StreamingHappyPath(t *testing.T) {
	got := testutils.CallCapture[entities.ContainerLogsOptions]{}
	engine := &fakePodEngine{
		logsFn: func(name string, opts entities.ContainerLogsOptions) error {
			got.Record(name, opts)
			// Simulate podman's follow-mode delivery: each line arrives
			// with a podman timestamp prefix.
			writeLogsInto(opts, "2024-01-01T00:00:00.000000000Z Starting application\n", "")
			writeLogsInto(opts, "2024-01-01T00:00:01.000000000Z Application started successfully\n", "")
			return nil
		},
	}

	w := logsRequest(newTestHandler(engine).Logs, "pod-123", "?follow=true&since=2024-01-01T00:00:00Z&tail=0")

	assert.Equal(t, http.StatusOK, w.Code, "Should return 200 on streaming success")
	assert.Equal(t, "pod-123", got.Name, "the follow service call must receive the resolved target")
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

// TestPodHandler_Logs_NonStreamingUnchanged tests that follow=false returns non-streaming response unchanged
func TestPodHandler_Logs_NonStreamingUnchanged(t *testing.T) {
	engine := &fakePodEngine{
		logsFn: func(name string, opts entities.ContainerLogsOptions) error {
			writeLogsInto(opts, "test log line 1\ntest log line 2\n", "")
			return nil
		},
	}

	w := logsRequest(newTestHandler(engine).Logs, "pod-123", "?follow=false")

	assert.Equal(t, http.StatusOK, w.Code, "Should return 200 on success")
	assert.Contains(t, w.Body.String(), `"id":"pod-123"`, "Response should contain pod ID")
	assert.Contains(t, w.Body.String(), `"logs":"test log line 1\ntest log line 2"`, "Response should contain logs as single field")
}
