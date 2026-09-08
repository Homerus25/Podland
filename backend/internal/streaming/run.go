package streaming

import (
	"context"
	"errors"
	"log/slog"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
)

// Log-stream end-of-stream vocabulary: a clean operation end fires this
// terminal event, so exactly-one-terminal holds in every exit direction.
const (
	StreamEndedLevel   = "INFO"
	StreamEndedMessage = "STREAM_ENDED"
)

// StreamConfig describes a streaming operation to be orchestrated by Run.
type StreamConfig[Event, Result any] struct {
	// Operation runs the long-running work. It must honour ctx cancellation
	// and report progress through the provided callback.
	Operation func(ctx context.Context, onProgress func(Event)) (Result, error)

	// OnEvent forwards one progress event to the response (typically via
	// NDJSONWriter.Write). A returned error aborts the stream without a
	// terminal event, since the response is no longer writable.
	OnEvent func(Event) error

	// CompleteEvent builds the terminal event for a successful result.
	CompleteEvent func(Result) any

	// ErrorDefault is the domain's fallback sentinel: failures that are
	// not (wrapped) AppErrors surface its code and generic message. A nil
	// default falls back to the internal-error sentinel.
	ErrorDefault *appErrors.AppError

	// ErrorEvent builds the terminal event for a failure from the
	// module-mapped vocabulary: code and message resolve cancelled /
	// (wrapped) AppError / unknown per the shared mapping, and cancelled
	// is true when the operation ended because the request context was
	// cancelled (including client disconnects detected by Run).
	ErrorEvent func(code, message string, cancelled bool) any
}

// terminalError maps a stream failure to the (code, message, cancelled)
// event vocabulary. Cancelled wins: a client disconnect reports
// OPERATION_CANCELLED whatever the operation returned. A (possibly
// wrapped) *AppError carries its own code and message. Anything else is
// unmapped: the cause is logged once, centrally, and the domain's default
// sentinel answers — causes never ride wire payloads.
func (cfg StreamConfig[Event, Result]) terminalError(ctx context.Context, err error, cancelled bool) (string, string, bool) {
	if cancelled {
		return appErrors.ErrOperationCancelled.Code, appErrors.ErrOperationCancelled.Message, true
	}
	if appErr, ok := errors.AsType[*appErrors.AppError](err); ok {
		return appErr.Code, appErr.Message, false
	}

	slog.ErrorContext(ctx, "streaming: unmapped operation failure",
		slog.Any("error", err))
	def := cfg.ErrorDefault
	if def == nil {
		def = appErrors.ErrInternalError
	}
	return def.Code, def.Message, false
}

// flushProgress writes whatever progress events are still queued when the
// terminal outcome has been reached: a terminal event must never overtake
// progress the operation already delivered. Returns false when a write
// fails — the response is gone and no terminal event follows.
func flushProgress[Event any](ctx context.Context, progress <-chan Event, onEvent func(Event) error) bool {
	for {
		select {
		case event := <-progress:
			if err := onEvent(event); err != nil {
				slog.ErrorContext(ctx, "Failed to write event", slog.Any("error", err))
				return false
			}
		default:
			return true
		}
	}
}

// Run executes cfg.Operation with progress streaming over the given NDJSON
// writer. It runs the operation in a goroutine, forwards progress events via
// cfg.OnEvent, cancels the operation when the client disconnects, and writes
// exactly one terminal event (complete or error) before returning.
//
// If a progress-event write fails, the operation is cancelled and Run returns
// without writing a terminal event, because the response is gone.
func Run[Event, Result any](ctx context.Context, w *NDJSONWriter, cfg StreamConfig[Event, Result]) {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	disconnect := DisconnectChan(runCtx)

	progress := make(chan Event, 10)
	resultChan := make(chan Result, 1)
	errorChan := make(chan error, 1)

	go func() {
		result, err := cfg.Operation(runCtx, func(event Event) {
			select {
			case progress <- event:
			case <-runCtx.Done():
			}
		})
		if err != nil {
			errorChan <- err
		} else {
			resultChan <- result
		}
	}()

	for {
		select {
		case event := <-progress:
			if err := cfg.OnEvent(event); err != nil {
				slog.ErrorContext(ctx, "Failed to write event", slog.Any("error", err))
				cancel()
				return
			}

		case result := <-resultChan:
			if flushProgress(ctx, progress, cfg.OnEvent) {
				w.Write(cfg.CompleteEvent(result))
			}
			return

		case err := <-errorChan:
			if flushProgress(ctx, progress, cfg.OnEvent) {
				code, message, cancelled := cfg.terminalError(ctx, err, runCtx.Err() == context.Canceled)
				w.Write(cfg.ErrorEvent(code, message, cancelled))
			}
			return

		case <-disconnect:
			slog.InfoContext(ctx, "Client disconnected, cancelling operation")
			cancel()
			if flushProgress(ctx, progress, cfg.OnEvent) {
				code, message, cancelled := cfg.terminalError(ctx, context.Canceled, true)
				w.Write(cfg.ErrorEvent(code, message, cancelled))
			}
			return
		}
	}
}
