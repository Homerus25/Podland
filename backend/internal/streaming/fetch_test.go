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

// Kit suite for the buffered log fetch: FetchLogs pins the follow=false
// ritual once (the podman logs options, the stdout/stderr writer pair,
// the combine, and the terminal mapping: Cancellable's cancelled-wins,
// then the caller's not-found sentinel, then one kit-owned wrap).

func fetchOp(logs func(name string, opts entities.ContainerLogsOptions) error) FetchLogsOp {
	return FetchLogsOp{
		Target:   "c-123",
		Since:    time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC),
		Tail:     100,
		NotFound: appErrors.ErrContainerNotFound,
		Logs:     logs,
	}
}

func TestFetchLogsCombinesStdoutAndStderr(t *testing.T) {
	op := fetchOp(func(name string, opts entities.ContainerLogsOptions) error {
		_, _ = opts.StdoutWriter.Write([]byte("out line\n"))
		_, _ = opts.StderrWriter.Write([]byte("err line\n"))
		return nil
	})

	got, err := FetchLogs(context.Background(), op)

	require.NoError(t, err)
	assert.Equal(t, "out line\nerr line", got,
		"stdout comes first, stderr appended, trailing newline trimmed")
}

func TestFetchLogsStderrOnly(t *testing.T) {
	op := fetchOp(func(name string, opts entities.ContainerLogsOptions) error {
		_, _ = opts.StderrWriter.Write([]byte("err line\n"))
		return nil
	})

	got, err := FetchLogs(context.Background(), op)

	require.NoError(t, err)
	assert.Equal(t, "err line", got, "stderr-only output must not grow a leading newline")
}

func TestFetchLogsTrimsTrailingNewlineOnly(t *testing.T) {
	op := fetchOp(func(name string, opts entities.ContainerLogsOptions) error {
		_, _ = opts.StdoutWriter.Write([]byte("first\nsecond\n"))
		return nil
	})

	got, err := FetchLogs(context.Background(), op)

	require.NoError(t, err)
	assert.Equal(t, "first\nsecond", got, "exactly one trailing newline is trimmed, interior ones survive")
}

func TestFetchLogsWiresBufferedOptions(t *testing.T) {
	var gotName string
	var gotOpts entities.ContainerLogsOptions
	op := fetchOp(func(name string, opts entities.ContainerLogsOptions) error {
		gotName = name
		gotOpts = opts
		return nil
	})

	_, err := FetchLogs(context.Background(), op)

	require.NoError(t, err)
	assert.Equal(t, "c-123", gotName, "the op's target is the name the Logs call receives")
	assert.False(t, gotOpts.Follow, "a buffered fetch must reach podman with Follow=false")
	assert.False(t, gotOpts.Timestamps, "the kit fetches bare lines: Timestamps=false")
	assert.Equal(t, op.Since, gotOpts.Since, "the Since window must flow through")
	assert.Equal(t, int64(100), gotOpts.Tail, "the Tail window must flow through")
	require.NotNil(t, gotOpts.StdoutWriter, "the kit owns the stdout writer")
	require.NotNil(t, gotOpts.StderrWriter, "the kit owns the stderr writer")
}

func TestFetchLogsMapsNotFoundToCallerSentinel(t *testing.T) {
	op := fetchOp(func(name string, opts entities.ContainerLogsOptions) error {
		return fmt.Errorf("tunnel: %w", &errorhandling.ErrorModel{
			ResponseCode: http.StatusNotFound,
			Message:      "no such container",
		})
	})

	got, err := FetchLogs(context.Background(), op)

	require.ErrorIs(t, err, op.NotFound, "podman's 404 answer maps to the caller's sentinel, even wrapped")
	assert.Empty(t, got)
}

func TestFetchLogsCancelledContextAnswersCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	op := fetchOp(func(name string, opts entities.ContainerLogsOptions) error {
		return errors.New("boom")
	})

	got, err := FetchLogs(ctx, op)

	require.ErrorIs(t, err, appErrors.ErrOperationCancelled,
		"a cancelled buffered fetch answers OPERATION_CANCELLED deterministically, through Cancellable")
	assert.Same(t, appErrors.ErrOperationCancelled, err,
		"cancelled-wins: the sentinel rides bare, ahead of the not-found mapping and the wrap")
	assert.Empty(t, got)
}

func TestFetchLogsWrapsGenericFailureOnce(t *testing.T) {
	op := fetchOp(func(name string, opts entities.ContainerLogsOptions) error {
		return errors.New("boom: secret cause detail")
	})

	got, err := FetchLogs(context.Background(), op)

	require.Error(t, err)
	assert.EqualError(t, err, "failed to get logs: boom: secret cause detail",
		"unmapped causes carry exactly one kit-owned wrap")
	assert.Empty(t, got)
}
