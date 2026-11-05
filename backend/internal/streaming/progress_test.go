package streaming

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/slogtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Kit suite for the progress-stream seam: HandleProgress pins the
// handler-side ritual once — the write-deadline extension, the operation
// context handed to every domain callback, and the kit-assembled operation
// slog line — so no domain re-pins it.

// serveProgress runs a HandleProgress endpoint the way the router does:
// request-scoped context values plus chi URL params (mirrors the begin
// test setup).
func serveProgress(t *testing.T, w http.ResponseWriter, userID, hostID string, endpoint http.HandlerFunc) {
	t.Helper()

	ctx := contextutils.SetRequestID(context.Background(), "test-request-id")
	if userID != "" {
		ctx = contextutils.SetUserID(ctx, userID)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/hosts/"+hostID+"/op", nil).WithContext(ctx)

	if hostID != "" {
		req.SetPathValue("id", hostID)
	}

	endpoint(w, req)
}

// progressEndpoint answers one HandleProgress call with the given config
// and operation name/attrs.
func progressEndpoint(operation string, attrs []slog.Attr, cfg ProgressConfig[int, string]) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		HandleProgress(w, r, operation, attrs, cfg)
	}
}

// deadlineRecorder wraps httptest.ResponseRecorder so the kit's
// SetWriteDeadline call (via http.NewResponseController) can be observed.
type deadlineRecorder struct {
	*httptest.ResponseRecorder
	deadline    time.Time
	gotDeadline bool
}

func (d *deadlineRecorder) SetWriteDeadline(t time.Time) error {
	d.deadline = t
	d.gotDeadline = true
	return nil
}

// TestHandleProgressExtendsWriteDeadline pins the kit-level deadline: one
// constant applied to every progress stream, so no handler can forget it.
func TestHandleProgressExtendsWriteDeadline(t *testing.T) {
	rec := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}

	serveProgress(t, rec, "test-user", "test-host", progressEndpoint("test-op", nil, ProgressConfig[int, string]{
		Operation: func(ctx context.Context, op OperationContext, onProgress func(int)) (string, error) {
			return "done", nil
		},
		CompleteEvent: func(op OperationContext, result string) any { return result },
		ErrorDefault:  appErrors.ErrInternalError,
		ErrorEvent:    func(op OperationContext, code, message string, cancelled bool) any { return code },
	}))

	require.True(t, rec.gotDeadline, "HandleProgress must extend the per-response write deadline")
	remaining := time.Until(rec.deadline)
	assert.Greater(t, remaining, 9*time.Minute, "deadline must be roughly now+10min, got %v", rec.deadline)
	assert.LessOrEqual(t, remaining, ProgressWriteDeadline, "deadline must be the kit constant, got %v", rec.deadline)
}

// TestHandleProgressPassesOperationContext pins the operation context: the
// kit mints the operationID and hands OperationID/HostID/UserID to every
// domain callback, and the stream itself carries progress plus exactly one
// terminal event.
func TestHandleProgressPassesOperationContext(t *testing.T) {
	rec := httptest.NewRecorder()

	var opInOperation, opInComplete OperationContext
	serveProgress(t, rec, "test-user", "test-host", progressEndpoint("test-op", nil, ProgressConfig[int, string]{
		Operation: func(ctx context.Context, op OperationContext, onProgress func(int)) (string, error) {
			opInOperation = op
			onProgress(1)
			return "done", nil
		},
		CompleteEvent: func(op OperationContext, result string) any {
			opInComplete = op
			return map[string]any{"complete": result}
		},
		ErrorDefault: appErrors.ErrInternalError,
		ErrorEvent:   func(op OperationContext, code, message string, cancelled bool) any { return code },
	}))

	assert.NotEmpty(t, opInOperation.OperationID, "the kit mints the operation ID")
	assert.Equal(t, "test-host", opInOperation.HostID)
	assert.Equal(t, "test-user", opInOperation.UserID)
	assert.Equal(t, opInOperation, opInComplete, "every domain callback receives the same operation context")

	lines := decodeLines(t, rec.Body.String())
	require.Len(t, lines, 2, "one progress event plus exactly one terminal event")
	assert.Equal(t, `1`, lines[0])
	assert.JSONEq(t, `{"complete":"done"}`, lines[1])
}

// TestHandleProgressErrorEventReceivesOperationContext pins the failure
// half: the error-event builder receives the same minted operation context
// alongside the mapped code/message.
func TestHandleProgressErrorEventReceivesOperationContext(t *testing.T) {
	rec := httptest.NewRecorder()

	var opInError OperationContext
	serveProgress(t, rec, "test-user", "test-host", progressEndpoint("test-op", nil, ProgressConfig[int, string]{
		Operation: func(ctx context.Context, op OperationContext, onProgress func(int)) (string, error) {
			return "", appErrors.ErrConnectionFailed
		},
		CompleteEvent: func(op OperationContext, result string) any { return result },
		ErrorDefault:  appErrors.ErrInternalError,
		ErrorEvent: func(op OperationContext, code, message string, cancelled bool) any {
			opInError = op
			return map[string]any{"code": code}
		},
	}))

	assert.NotEmpty(t, opInError.OperationID, "the kit mints the operation ID")
	assert.Equal(t, "test-host", opInError.HostID)
	assert.Equal(t, "test-user", opInError.UserID)

	lines := decodeLines(t, rec.Body.String())
	require.Len(t, lines, 1, "exactly one terminal event")
	assert.JSONEq(t, `{"code":"CONNECTION_FAILED"}`, lines[0])
}

// TestHandleProgressLogsOperationLine pins the kit-assembled operation slog
// line: the handler's operation name and attrs over the kit's
// requestID/hostID/operationID base, with the operation ID matching the
// one handed to the domain callbacks.
func TestHandleProgressLogsOperationLine(t *testing.T) {
	capture := slogtest.CaptureDefault(t)

	// The deadline recorder answers SetWriteDeadline, so the ritual's only
	// slog line is the operation line under test.
	rec := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}

	var opInOperation OperationContext
	serveProgress(t, rec, "test-user", "test-host", progressEndpoint("test-op",
		[]slog.Attr{slog.String("quadletID", "web.container")},
		ProgressConfig[int, string]{
			Operation: func(ctx context.Context, op OperationContext, onProgress func(int)) (string, error) {
				opInOperation = op
				return "done", nil
			},
			CompleteEvent: func(op OperationContext, result string) any { return result },
			ErrorDefault:  appErrors.ErrInternalError,
			ErrorEvent:    func(op OperationContext, code, message string, cancelled bool) any { return code },
		}))

	records := capture.Records()
	require.Len(t, records, 1, "exactly one operation slog line")
	record := records[0]
	assert.Equal(t, "test-op", record.Message)
	assert.Equal(t, slog.LevelInfo, record.Level)

	attrs := map[string]string{}
	record.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.String()
		return true
	})
	assert.Equal(t, "test-request-id", attrs["requestID"])
	assert.Equal(t, "test-host", attrs["hostID"])
	assert.Equal(t, opInOperation.OperationID, attrs["operationID"],
		"the slog line's operation ID must be the one handed to the domain callbacks")
	assert.Equal(t, "web.container", attrs["quadletID"], "the handler's attrs ride along")
}
