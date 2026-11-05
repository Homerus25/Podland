package streaming

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"sync"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/stretchr/testify/assert"
)

func newTestWriter(t *testing.T) (*ndjsonWriter, *httptest.ResponseRecorder) {
	t.Helper()
	rec := httptest.NewRecorder()
	w, err := newNDJSONWriter(rec)
	if err != nil {
		t.Fatalf("newNDJSONWriter: %v", err)
	}
	return w, rec
}

func decodeLines(t *testing.T, body string) []string {
	t.Helper()
	var lines []string
	start := 0
	for i := 0; i < len(body); i++ {
		if body[i] == '\n' {
			lines = append(lines, body[start:i])
			start = i + 1
		}
	}
	if start < len(body) {
		lines = append(lines, body[start:])
	}
	if len(lines) == 0 {
		t.Fatalf("expected at least one NDJSON line, body: %q", body)
	}
	return lines
}

func TestRunStreamsProgressAndCompletes(t *testing.T) {
	w, rec := newTestWriter(t)

	delivered := make(chan struct{}, 2)
	run(context.Background(), w, streamConfig[int, string]{
		Operation: func(ctx context.Context, onProgress func(int)) (string, error) {
			onProgress(1)
			onProgress(2)
			<-delivered
			<-delivered
			return "done", nil
		},
		OnEvent: func(event int) error {
			err := w.Write(map[string]any{"progress": event})
			delivered <- struct{}{}
			return err
		},
		CompleteEvent: func(result string) any { return map[string]any{"complete": result} },
		ErrorDefault:  appErrors.ErrInternalError,
		ErrorEvent:    func(code, message string, cancelled bool) any { return map[string]any{"error": code} },
	})

	lines := decodeLines(t, rec.Body.String())
	if len(lines) != 3 {
		t.Fatalf("expected 3 NDJSON lines, got %d: %q", len(lines), rec.Body.String())
	}
	want := []string{`{"progress":1}`, `{"progress":2}`, `{"complete":"done"}`}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("line %d: got %s, want %s", i, lines[i], want[i])
		}
	}
}

func TestRunWritesExactlyOneTerminalErrorEvent(t *testing.T) {
	w, rec := newTestWriter(t)
	terminal := errors.New("boom")
	fallback := appErrors.NewAppError("TEST_FAILED", "test operation failed", 500)

	var gotCancelled bool
	run(context.Background(), w, streamConfig[int, string]{
		Operation: func(ctx context.Context, onProgress func(int)) (string, error) {
			onProgress(1)
			return "", terminal
		},
		OnEvent:       func(event int) error { return w.Write(event) },
		CompleteEvent: func(result string) any { return map[string]any{"complete": result} },
		ErrorDefault:  fallback,
		ErrorEvent: func(code, message string, cancelled bool) any {
			gotCancelled = cancelled
			return map[string]any{"code": code, "message": message}
		},
	})

	assert.False(t, gotCancelled, "a plain operation failure must not be reported as cancelled")
	lines := decodeLines(t, rec.Body.String())
	if len(lines) != 2 {
		t.Fatalf("expected 2 NDJSON lines (progress + one error), got %d: %q", len(lines), rec.Body.String())
	}
	assert.Equal(t, `{"code":"TEST_FAILED","message":"test operation failed"}`, lines[1])
}

func TestRunCancelsOperationOnClientDisconnect(t *testing.T) {
	w, rec := newTestWriter(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	operationCancelled := false
	run(ctx, w, streamConfig[int, string]{
		Operation: func(ctx context.Context, onProgress func(int)) (string, error) {
			defer wg.Done()
			<-ctx.Done()
			operationCancelled = true
			return "", ctx.Err()
		},
		OnEvent:       func(event int) error { return nil },
		CompleteEvent: func(result string) any { return result },
		ErrorDefault:  appErrors.ErrInternalError,
		ErrorEvent: func(code, message string, cancelled bool) any {
			if !cancelled {
				t.Fatal("disconnect must report cancelled=true")
			}
			if code != appErrors.ErrOperationCancelled.Code {
				t.Fatalf("disconnect must map to OPERATION_CANCELLED, got %s", code)
			}
			return map[string]any{"error": "cancelled"}
		},
	})
	wg.Wait()
	if !operationCancelled {
		t.Fatal("operation was not cancelled on disconnect")
	}

	lines := decodeLines(t, rec.Body.String())
	if len(lines) != 1 {
		t.Fatalf("expected exactly one terminal event, got %d: %q", len(lines), rec.Body.String())
	}
	if lines[0] != `{"error":"cancelled"}` {
		t.Fatalf("terminal line: got %s", lines[0])
	}
}

func TestRunStopsWithoutTerminalEventWhenWriteFails(t *testing.T) {
	w, rec := newTestWriter(t)

	completed := false
	finished := make(chan struct{})
	run(context.Background(), w, streamConfig[int, string]{
		Operation: func(ctx context.Context, onProgress func(int)) (string, error) {
			defer close(finished)
			onProgress(1)
			<-ctx.Done()
			completed = ctx.Err() != nil
			return "done", nil
		},
		OnEvent: func(event int) error {
			return errors.New("write failed")
		},
		CompleteEvent: func(result string) any { return result },
		ErrorDefault:  appErrors.ErrInternalError,
		ErrorEvent:    func(code, message string, cancelled bool) any { return code },
	})

	<-finished
	if rec.Body.String() != "" {
		t.Fatalf("no terminal event should be written after write failure, body: %q", rec.Body.String())
	}
	if !completed {
		t.Fatal("operation should have been cancelled after write failure")
	}
}

// TestRunErrorMapping pins the error→event vocabulary table once, for
// every domain: a (possibly wrapped) AppError surfaces its own code and
// message; anything else surfaces the config's default sentinel (with the
// raw cause logged server-side only); the cancelled flag rides along
// unchanged and takes precedence on disconnects.
func TestRunErrorMapping(t *testing.T) {
	fallback := appErrors.NewAppError("TEST_DEFAULT", "test default message", 502)
	notFound := appErrors.NewAppError("THING_NOT_FOUND", "Thing not found", 404)

	tests := []struct {
		name         string
		operation    func(ctx context.Context, onProgress func(int)) (string, error)
		errorDefault *appErrors.AppError
		wantCode     string
		wantMessage  string
	}{
		{
			name: "plain AppError keeps its code and message",
			operation: func(ctx context.Context, onProgress func(int)) (string, error) {
				return "", notFound
			},
			wantCode:    "THING_NOT_FOUND",
			wantMessage: "Thing not found",
		},
		{
			name: "wrapped AppError is unwrapped",
			operation: func(ctx context.Context, onProgress func(int)) (string, error) {
				return "", fmt.Errorf("dial %s: %w", "host-1", notFound)
			},
			wantCode:    "THING_NOT_FOUND",
			wantMessage: "Thing not found",
		},
		{
			name: "unknown error falls back to the default sentinel",
			operation: func(ctx context.Context, onProgress func(int)) (string, error) {
				return "", errors.New("boom")
			},
			errorDefault: fallback,
			wantCode:     "TEST_DEFAULT",
			wantMessage:  "test default message",
		},
		{
			name: "nil default falls back to the internal-error sentinel",
			operation: func(ctx context.Context, onProgress func(int)) (string, error) {
				return "", errors.New("boom")
			},
			wantCode:    appErrors.ErrInternalError.Code,
			wantMessage: appErrors.ErrInternalError.Message,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, rec := newTestWriter(t)

			var gotCancelled bool
			run(context.Background(), w, streamConfig[int, string]{
				Operation:     tt.operation,
				OnEvent:       func(event int) error { return nil },
				CompleteEvent: func(result string) any { return result },
				ErrorDefault:  tt.errorDefault,
				ErrorEvent: func(code, message string, cancelled bool) any {
					gotCancelled = cancelled
					return map[string]any{"code": code, "message": message}
				},
			})

			assert.False(t, gotCancelled)
			lines := decodeLines(t, rec.Body.String())
			assert.Len(t, lines, 1, "exactly one terminal event")
			assert.JSONEq(t, `{"code":"`+tt.wantCode+`","message":"`+tt.wantMessage+`"}`, lines[0])
		})
	}
}

// TestRunForwardQueuedProgressBeforeTerminal pins the ordering invariant:
// a terminal event never overtakes progress events the operation already
// delivered — they cross the wire before the terminal line.
func TestRunForwardQueuedProgressBeforeTerminal(t *testing.T) {
	w, rec := newTestWriter(t)

	run(context.Background(), w, streamConfig[int, string]{
		Operation: func(ctx context.Context, onProgress func(int)) (string, error) {
			onProgress(1)
			onProgress(2)
			onProgress(3)
			return "done", nil
		},
		OnEvent:       func(event int) error { return w.Write(map[string]any{"progress": event}) },
		CompleteEvent: func(result string) any { return map[string]any{"complete": result} },
		ErrorDefault:  appErrors.ErrInternalError,
		ErrorEvent:    func(code, message string, cancelled bool) any { return map[string]any{"error": code} },
	})

	lines := decodeLines(t, rec.Body.String())
	assert.Len(t, lines, 4, "every progress event must cross the wire")
	want := []string{`{"progress":1}`, `{"progress":2}`, `{"progress":3}`, `{"complete":"done"}`}
	assert.Equal(t, want, lines)
}

// TestRunStreamEndedVocabulary pins the shared end-of-stream vocabulary.
func TestRunStreamEndedVocabulary(t *testing.T) {
	assert.Equal(t, "INFO", StreamEndedLevel)
	assert.Equal(t, "STREAM_ENDED", StreamEndedMessage)
}
