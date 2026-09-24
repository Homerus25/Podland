package testutils

import (
	"context"
	"testing"
	"time"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/stretchr/testify/require"
)

// The cancellation-promptness harness: the shared shape of the per-domain
// "returns promptly on cancel" regression tests (containers and quadlets
// logs follow, images pull). The engine fake is armed with the stub
// channels — its blocking call closes started once it is running and
// waits on block — and the helper drives the cancel-and-abandon ritual:
// call runs under a context cancelled as soon as started closes, and the
// operation-cancelled sentinel must come back before the promptness
// window closes. The WithEngine closure returning is the connection
// lease release — this is the closest seam to the refCount a fake can
// see. The contract itself is pinned once in internal/streaming's
// Cancellable suite.

// CancelStubs returns the started/block channel pair a test arms its
// engine fake with. block is closed automatically when the test ends,
// releasing any abandoned goroutine.
func CancelStubs(t testing.TB) (started chan struct{}, block chan struct{}) {
	started = make(chan struct{})
	block = make(chan struct{})
	t.Cleanup(func() { close(block) })
	return started, block
}

// RequirePromptReturnOnCancel drives the cancelled-context call and pins
// the release: call runs in a goroutine under a context cancelled as
// soon as started closes, and must answer ErrOperationCancelled within
// the promptness window. what names the operation in the fatal message.
func RequirePromptReturnOnCancel(t testing.TB, what string, started <-chan struct{}, call func(ctx context.Context) error) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- call(ctx)
	}()
	<-started
	cancel()

	select {
	case err := <-errCh:
		require.ErrorIs(t, err, appErrors.ErrOperationCancelled,
			"cancellation abandons the blocking engine call and returns the sentinel")
	case <-time.After(2 * time.Second):
		t.Fatalf("%s did not return promptly after cancellation — the WithEngine closure (the lease release) is still blocked", what)
	}
}
