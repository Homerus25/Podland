package streaming

import (
	"context"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
)

// Cancellable is the kit's abandonment contract for engine-backed
// blocking calls: the engine runs on the connection cache's detached
// context (§4), so per-call cancellation is impossible below the
// streaming loop — the caller's only cancellation is abandoning the
// call's goroutine and returning.
//
// A pre-cancelled context answers ErrOperationCancelled without running
// fn (deterministic cancelled-wins: no remote work starts for a dead
// request). Otherwise fn runs in a goroutine: its error passes through
// untouched for the caller to map, context cancellation answers
// ErrOperationCancelled immediately and abandons fn's goroutine, which
// keeps running on the engine's detached context until it ends naturally
// or the connection is reaped (idle cleanup, LRU eviction, host
// removal).
//
// Placement rule: call this inside the WithEngine closure. Abandonment
// only helps if it makes that closure return — that is what releases the
// connection lease. Abandoning at the streaming Run loop's level would
// leave the closure blocked on the engine call with the lease pinned.
func Cancellable(ctx context.Context, fn func() error) error {
	if ctx.Err() != nil {
		return appErrors.ErrOperationCancelled
	}

	done := make(chan error, 1)
	go func() {
		done <- fn()
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return appErrors.ErrOperationCancelled
	}
}
