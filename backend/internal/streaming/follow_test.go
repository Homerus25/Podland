package streaming

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// Kit suite for the follow log fetch: FollowLogs pins the follow=true
// ritual once (the podman logs options with the one intentional
// Timestamps divergence, the stdout-INFO / stderr-ERROR writer pair, and
// the terminal mapping: Cancellable's cancelled-wins, then the caller's
// not-found sentinel, then one kit-owned wrap).

func followOp(logs func(name string, opts entities.ContainerLogsOptions) error) FollowLogsOp {
	return FollowLogsOp{
		Target:   "c-123",
		Since:    time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC),
		Tail:     100,
		NotFound: appErrors.ErrContainerNotFound,
		Logs:     logs,
		OnEvent:  func(LogEvent) {},
	}
}

func TestFollowLogsWiresFollowOptions(t *testing.T) {
	var gotName string
	var gotOpts entities.ContainerLogsOptions
	op := followOp(func(name string, opts entities.ContainerLogsOptions) error {
		gotName = name
		gotOpts = opts
		return nil
	})

	err := FollowLogs(context.Background(), op)

	require.NoError(t, err)
	assert.Equal(t, "c-123", gotName, "the op's target is the name the Logs call receives")
	assert.True(t, gotOpts.Follow, "a follow fetch must reach podman with Follow=true")
	assert.True(t, gotOpts.Timestamps,
		"the one intentional divergence from the buffered fetch: the event writer parses the timestamps back out")
	assert.Equal(t, op.Since, gotOpts.Since, "the Since window must flow through")
	assert.Equal(t, int64(100), gotOpts.Tail, "the Tail window must flow through")
	require.NotNil(t, gotOpts.StdoutWriter, "the kit owns the stdout writer")
	require.NotNil(t, gotOpts.StderrWriter, "the kit owns the stderr writer")
}

func TestFollowLogsInstallsInfoErrorWriterPair(t *testing.T) {
	var events []LogEvent
	op := followOp(func(name string, opts entities.ContainerLogsOptions) error {
		_, _ = opts.StdoutWriter.Write([]byte("2024-01-01T00:00:00Z out line\n"))
		_, _ = opts.StderrWriter.Write([]byte("2024-01-01T00:00:01Z err line\n"))
		return nil
	})
	op.OnEvent = func(event LogEvent) { events = append(events, event) }

	err := FollowLogs(context.Background(), op)

	require.NoError(t, err)
	require.Len(t, events, 2, "each podman line becomes exactly one event")
	assert.Equal(t, "INFO", events[0].Level, "stdout streams configure the INFO writer")
	assert.Equal(t, "out line", events[0].Message, "the writer parses the timestamp out of the message")
	assert.Equal(t, "ERROR", events[1].Level, "stderr streams configure the ERROR writer")
	assert.Equal(t, "err line", events[1].Message)
}

func TestFollowLogsMapsNotFoundToCallerSentinel(t *testing.T) {
	op := followOp(func(name string, opts entities.ContainerLogsOptions) error {
		return fmt.Errorf("tunnel: %w", &errorhandling.ErrorModel{
			ResponseCode: http.StatusNotFound,
			Message:      "no such container",
		})
	})

	err := FollowLogs(context.Background(), op)

	require.ErrorIs(t, err, op.NotFound, "podman's 404 answer maps to the caller's sentinel, even wrapped")
}

func TestFollowLogsWrapsGenericFailureOnce(t *testing.T) {
	op := followOp(func(name string, opts entities.ContainerLogsOptions) error {
		return errors.New("boom: secret cause detail")
	})

	err := FollowLogs(context.Background(), op)

	require.Error(t, err)
	assert.EqualError(t, err, "failed to get logs: boom: secret cause detail",
		"unmapped causes carry exactly one kit-owned, log-only wrap")
}

func TestFollowLogsReturnsPromptlyOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	block := make(chan struct{})
	defer close(block) // release the abandoned goroutine when the test ends

	var events []LogEvent
	op := followOp(func(name string, opts entities.ContainerLogsOptions) error {
		// One event lands before the cancellation: it must reach the
		// callback even though the fetch never ends on its own.
		_, _ = opts.StdoutWriter.Write([]byte("2024-01-01T00:00:00Z before the cancel\n"))
		close(started)
		<-block
		return errors.New("abandoned before this could be observed")
	})
	op.OnEvent = func(event LogEvent) { events = append(events, event) }

	errCh := make(chan error, 1)
	go func() {
		errCh <- FollowLogs(ctx, op)
	}()
	<-started
	cancel()

	select {
	case err := <-errCh:
		require.ErrorIs(t, err, appErrors.ErrOperationCancelled,
			"cancellation abandons the blocking Logs call and returns the sentinel")
		assert.Same(t, appErrors.ErrOperationCancelled, err,
			"cancelled-wins: the sentinel rides bare, ahead of the not-found mapping and the wrap")
	case <-time.After(2 * time.Second):
		t.Fatal("FollowLogs did not return promptly after cancellation")
	}
	require.Len(t, events, 1, "events emitted before cancellation reach the callback")
	assert.Equal(t, "before the cancel", events[0].Message)
}
