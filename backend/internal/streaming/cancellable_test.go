package streaming

import (
	"context"
	"errors"
	"testing"
	"time"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Kit suite for the abandonment contract: Cancellable pins the helper
// once — pre-cancelled contexts never start the work, fn's outcome
// passes through untouched, and a cancelled context returns promptly
// with the operation-cancelled sentinel while fn's goroutine is
// abandoned.

func TestCancellablePassesFunctionErrorThrough(t *testing.T) {
	want := errors.New("boom: mapped by the caller")

	err := Cancellable(context.Background(), func() error { return want })

	require.Error(t, err)
	assert.Same(t, want, err, "fn's error passes through untouched, for the caller to map")
}

func TestCancellablePassesSuccessThrough(t *testing.T) {
	err := Cancellable(context.Background(), func() error { return nil })

	require.NoError(t, err)
}

func TestCancellablePreCancelledContextSkipsFunction(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ran := false
	err := Cancellable(ctx, func() error {
		ran = true
		return errors.New("never started")
	})

	require.ErrorIs(t, err, appErrors.ErrOperationCancelled,
		"a pre-cancelled context answers the operation-cancelled sentinel")
	assert.False(t, ran, "no remote work starts for a dead request")
}

func TestCancellableReturnsPromptlyOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	block := make(chan struct{})
	defer close(block) // release the abandoned goroutine when the test ends

	errCh := make(chan error, 1)
	go func() {
		errCh <- Cancellable(ctx, func() error {
			close(started)
			<-block
			return errors.New("abandoned before this could be observed")
		})
	}()
	<-started
	cancel()

	select {
	case err := <-errCh:
		require.ErrorIs(t, err, appErrors.ErrOperationCancelled,
			"cancellation abandons fn's goroutine and returns the sentinel")
	case <-time.After(2 * time.Second):
		t.Fatal("Cancellable did not return promptly after cancellation")
	}
}
