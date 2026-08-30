package streaming

import (
	"context"
	"log/slog"
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

	// ErrorEvent builds the terminal event for a failure. cancelled is true
	// when the operation ended because the request context was cancelled
	// (including client disconnects detected by Run).
	ErrorEvent func(err error, cancelled bool) any
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
			w.Write(cfg.CompleteEvent(result))
			return

		case err := <-errorChan:
			w.Write(cfg.ErrorEvent(err, runCtx.Err() == context.Canceled))
			return

		case <-disconnect:
			slog.InfoContext(ctx, "Client disconnected, cancelling operation")
			cancel()
			w.Write(cfg.ErrorEvent(context.Canceled, true))
			return
		}
	}
}
