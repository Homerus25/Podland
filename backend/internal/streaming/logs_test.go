package streaming

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Kit suites for the follow-mode log stream: RunLogStream pins the
// follow=true ritual once (terminal events, error mapping, connection
// failure default); LogEventWriter pins the line handling every log
// domain's podman writers share.

// serveLogStream drives RunLogStream through a chi route so Begin's
// identity extraction sees a real {id} URL param, the way the router
// wires it in production.
func serveLogStream(t *testing.T, ctx context.Context, op LogStreamOp) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/machines/test-machine/containers/c-123/logs?follow=true", nil).
		WithContext(ctx)
	w := httptest.NewRecorder()
	r := chi.NewRouter()
	r.Get("/api/machines/{id}/containers/{containerId}/logs", func(w http.ResponseWriter, r *http.Request) {
		RunLogStream(w, r, op)
	})
	r.ServeHTTP(w, req)
	return w
}

func authedStreamContext() context.Context {
	return contextutils.SetUserID(context.Background(), "test-user")
}

func logStreamOp(stream func(ctx context.Context, userID, machineID string, onEvent func(LogEvent)) error) LogStreamOp {
	return LogStreamOp{
		Since:  time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC),
		Tail:   100,
		Stream: stream,
	}
}

func TestRunLogStreamEmitsEventsThenExactlyOneStreamEnded(t *testing.T) {
	w := serveLogStream(t, authedStreamContext(), logStreamOp(func(ctx context.Context, userID, machineID string, onEvent func(LogEvent)) error {
		assert.Equal(t, "test-user", userID, "the stream callback must receive the caller identity")
		assert.Equal(t, "test-machine", machineID, "the stream callback must receive the machine identity")
		onEvent(LogEvent{Timestamp: "2026-09-01T10:00:00Z", Level: "INFO", Message: "hello"})
		onEvent(LogEvent{Timestamp: "2026-09-01T10:00:01Z", Level: "ERROR", Message: "broke"})
		return nil
	}))

	require.Equal(t, http.StatusOK, w.Code)
	lines := decodeLines(t, w.Body.String())
	require.Len(t, lines, 3, "two events plus exactly one terminal event")
	assert.Equal(t, `{"timestamp":"2026-09-01T10:00:00Z","level":"INFO","message":"hello"}`, lines[0],
		"event wire shape must stay byte-identical")
	assert.Equal(t, `{"timestamp":"2026-09-01T10:00:01Z","level":"ERROR","message":"broke"}`, lines[1])

	assert.Equal(t, 1, strings.Count(w.Body.String(), `"STREAM_ENDED"`),
		"a clean stream end must emit exactly one terminal event")
	ended := strings.HasSuffix(lines[2], `,"level":"INFO","message":"STREAM_ENDED"}`)
	require.True(t, ended, "terminal event must be INFO STREAM_ENDED, got %s", lines[2])
	var terminal LogEvent
	require.NoError(t, json.Unmarshal([]byte(lines[2]), &terminal))
	_, err := time.Parse(time.RFC3339, terminal.Timestamp)
	assert.NoError(t, err, "terminal timestamp must be RFC3339")
}

func TestRunLogStreamBeginFailureReturnsSilently(t *testing.T) {
	opCalled := false
	w := serveLogStream(t, context.Background(), logStreamOp(func(ctx context.Context, userID, machineID string, onEvent func(LogEvent)) error {
		opCalled = true
		return nil
	}))

	assert.False(t, opCalled, "a Begin failure must not reach the stream callback")
	assert.NotContains(t, w.Body.String(), "STREAM_ENDED", "a Begin failure must not open a stream")
	assert.Contains(t, w.Body.String(), "INTERNAL_ERROR", "Begin answers through the envelope itself")
}

func TestRunLogStreamMapsAppErrorToCodeMessageErrorEvent(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantMessage string
	}{
		{
			name:        "plain AppError keeps its code and message",
			err:         appErrors.ErrContainerNotFound,
			wantMessage: "CONTAINER_NOT_FOUND: Container not found",
		},
		{
			name:        "wrapped AppError keeps its code and message",
			err:         fmt.Errorf("fetching logs: %w", appErrors.ErrContainerNotFound),
			wantMessage: "CONTAINER_NOT_FOUND: Container not found",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := serveLogStream(t, authedStreamContext(), logStreamOp(func(ctx context.Context, userID, machineID string, onEvent func(LogEvent)) error {
				onEvent(LogEvent{Timestamp: "2026-09-01T10:00:00Z", Level: "INFO", Message: "hello"})
				return tc.err
			}))

			lines := decodeLines(t, w.Body.String())
			require.Len(t, lines, 2, "one event plus exactly one error terminal")
			assert.Equal(t, `{"timestamp":"2026-09-01T10:00:00Z","level":"INFO","message":"hello"}`, lines[0])
			assert.True(t, strings.HasSuffix(lines[1], `,"level":"ERROR","message":"`+tc.wantMessage+`"}`),
				"an AppError must surface one CODE: message ERROR event, got %s", lines[1])
		})
	}
}

func TestRunLogStreamMapsCancellationToWarnEvent(t *testing.T) {
	ctx, cancel := context.WithCancel(authedStreamContext())
	cancel()

	w := serveLogStream(t, ctx, logStreamOp(func(ctx context.Context, userID, machineID string, onEvent func(LogEvent)) error {
		onEvent(LogEvent{Timestamp: "2026-09-01T10:00:00Z", Level: "INFO", Message: "hello"})
		<-ctx.Done()
		return ctx.Err()
	}))

	lines := decodeLines(t, w.Body.String())
	require.Len(t, lines, 2, "one event plus exactly one cancellation terminal")
	assert.Equal(t, `{"timestamp":"2026-09-01T10:00:00Z","level":"INFO","message":"hello"}`, lines[0])
	assert.True(t, strings.HasSuffix(lines[1], `,"level":"WARN","message":"`+
		appErrors.ErrOperationCancelled.Code+": "+appErrors.ErrOperationCancelled.Message+`"}`),
		"a cancelled context must surface one WARN event, got %s", lines[1])
}

func TestRunLogStreamDefaultsToConnectionFailedAndLogsCauseOnce(t *testing.T) {
	var logBuf bytes.Buffer
	prevHandler := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelError})))
	defer slog.SetDefault(prevHandler)

	w := serveLogStream(t, authedStreamContext(), logStreamOp(func(ctx context.Context, userID, machineID string, onEvent func(LogEvent)) error {
		onEvent(LogEvent{Timestamp: "2026-09-01T10:00:00Z", Level: "INFO", Message: "hello"})
		return errors.New("boom: secret cause detail")
	}))

	lines := decodeLines(t, w.Body.String())
	require.Len(t, lines, 2, "one event plus exactly one terminal event")
	assert.Equal(t, `{"timestamp":"2026-09-01T10:00:00Z","level":"INFO","message":"hello"}`, lines[0])
	assert.True(t, strings.HasSuffix(lines[1], `,"level":"ERROR","message":"`+
		appErrors.ErrConnectionFailed.Code+": "+appErrors.ErrConnectionFailed.Message+`"}`),
		"an unmapped cause must surface the baked-in connection-failed default, got %s", lines[1])
	assert.NotContains(t, w.Body.String(), "boom", "the raw cause must never ride the wire")
	assert.NotContains(t, w.Body.String(), "secret", "the raw cause must never ride the wire")

	assert.Equal(t, 1, strings.Count(logBuf.String(), "unmapped operation failure"),
		"the kit logs the unmapped cause exactly once, server-side only")
}

func captureWriterEvents(t *testing.T) (*LogEventWriter, *[]LogEvent) {
	t.Helper()
	events := &[]LogEvent{}
	return NewLogEventWriter("INFO", func(e LogEvent) { *events = append(*events, e) }), events
}

func TestLogEventWriterTrimsAndParsesNanoZTimestamps(t *testing.T) {
	w, events := captureWriterEvents(t)

	n, err := w.Write([]byte("2026-09-01T10:00:00.123456789Z hello\n"))

	require.NoError(t, err)
	assert.Equal(t, len("2026-09-01T10:00:00.123456789Z hello\n"), n, "the writer must consume everything it is handed")
	require.Len(t, *events, 1)
	assert.Equal(t, "2026-09-01T10:00:00Z", (*events)[0].Timestamp,
		"a fractional-Z podman timestamp must re-emit as RFC3339 UTC")
	assert.Equal(t, "INFO", (*events)[0].Level, "the configured level rides every event")
	assert.Equal(t, "hello", (*events)[0].Message, "the podman timestamp must be parsed out of the message")
}

func TestLogEventWriterParsesOffsetTimestamps(t *testing.T) {
	w, events := captureWriterEvents(t)

	_, err := w.Write([]byte("2026-09-01T12:00:00.5+02:00 hello\n"))

	require.NoError(t, err)
	require.Len(t, *events, 1)
	assert.Equal(t, "2026-09-01T10:00:00Z", (*events)[0].Timestamp,
		"an offset-form timestamp must re-emit converted to RFC3339 UTC")
}

func TestLogEventWriterUnparseablePrefixKeepsLineAndNowTimestamp(t *testing.T) {
	w, events := captureWriterEvents(t)

	_, err := w.Write([]byte("not-a-timestamp hello\n"))

	require.NoError(t, err)
	require.Len(t, *events, 1)
	assert.Equal(t, "not-a-timestamp hello", (*events)[0].Message,
		"a non-timestamp prefix stays in the message")
	parsed, err := time.Parse(time.RFC3339, (*events)[0].Timestamp)
	require.NoError(t, err, "the fallback timestamp must still be RFC3339")
	assert.WithinDuration(t, time.Now().UTC(), parsed, time.Minute,
		"the fallback timestamp is the current time")
}

func TestLogEventWriterSkipsEmptyLines(t *testing.T) {
	w, events := captureWriterEvents(t)

	_, err := w.Write([]byte("\n"))

	require.NoError(t, err)
	assert.Empty(t, *events, "an empty line emits nothing")
}

func TestLogEventWriterSkipsSpacelessLinesWithoutPanicking(t *testing.T) {
	w, events := captureWriterEvents(t)

	assert.NotPanics(t, func() {
		_, err := w.Write([]byte("spaceless\n"))
		require.NoError(t, err)
	})
	assert.Empty(t, *events, "a spaceless line emits nothing instead of panicking mid-stream")
}

func TestLogEventWriterErrorLevelConfiguration(t *testing.T) {
	var got LogEvent
	w := NewLogEventWriter("ERROR", func(e LogEvent) { got = e })

	_, err := w.Write([]byte("2026-09-01T10:00:00Z broke\n"))

	require.NoError(t, err)
	assert.Equal(t, "ERROR", got.Level, "the stderr writer is configured with the ERROR level")
}
