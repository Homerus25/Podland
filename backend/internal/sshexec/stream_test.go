package sshexec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// streamingFakeRunner is a scriptable sessionRunner whose RunStream emits
// preconfigured lines before returning the preconfigured result/error. It
// records the emitted lines so tests can assert arrival order.
type streamingFakeRunner struct {
	mu       sync.Mutex
	lines    []OutputLine
	result   ExecResult
	err      error
	lastCmd  string
	emitted  []OutputLine
	emitFunc func(callback func(OutputLine))
}

func (r *streamingFakeRunner) Run(ctx context.Context, cmd string) (ExecResult, error) {
	return ExecResult{}, errors.New("not implemented")
}

func (r *streamingFakeRunner) RunStream(ctx context.Context, cmd string, onLine func(OutputLine)) (ExecResult, error) {
	r.mu.Lock()
	r.lastCmd = cmd
	for _, line := range r.lines {
		onLine(line)
		r.emitted = append(r.emitted, line)
	}
	if r.emitFunc != nil {
		r.emitFunc(onLine)
	}
	res, err := r.result, r.err
	r.mu.Unlock()
	if ctx.Err() != nil {
		return ExecResult{}, ctx.Err()
	}
	return res, err
}

func (r *streamingFakeRunner) Close() error { return nil }

// collectedCallback returns a callback that records lines and can be
// cancelled mid-stream via the returned control channel.
func collectedCallback(t *testing.T, ctx context.Context) (func(OutputLine), *[]OutputLine) {
	t.Helper()
	var mu sync.Mutex
	var lines []OutputLine
	return func(line OutputLine) {
		select {
		case <-ctx.Done():
			return
		default:
		}
		mu.Lock()
		defer mu.Unlock()
		lines = append(lines, line)
	}, &lines
}

func TestExecStream_EmitsLinesInArrivalOrderAndReturnsResult(t *testing.T) {
	runner := &streamingFakeRunner{
		lines: []OutputLine{
			{Stream: StdoutStream, Text: "line 1"},
			{Stream: StderrStream, Text: "oops"},
			{Stream: StdoutStream, Text: "line 2"},
		},
		result: ExecResult{Stdout: "line 1\nline 2\n", Stderr: "oops\n", ExitCode: 0},
	}
	dialer := &fakeDialer{dialRunner: runner}
	svc := newServiceWithDialer(dialer)

	onLine, lines := collectedCallback(t, context.Background())
	result, err := svc.ExecStream(context.Background(), "machine-1", "user-1", "podman", ExecOptions{Args: []string{"info"}}, onLine)

	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "line 1\nline 2\n", result.Stdout)
	assert.Equal(t, "oops\n", result.Stderr)

	// Lines arrive interleaved, in arrival order, tagged with their stream.
	require.Len(t, *lines, 3)
	assert.Equal(t, OutputLine{Stream: StdoutStream, Text: "line 1"}, (*lines)[0])
	assert.Equal(t, OutputLine{Stream: StderrStream, Text: "oops"}, (*lines)[1])
	assert.Equal(t, OutputLine{Stream: StdoutStream, Text: "line 2"}, (*lines)[2])

	assert.Equal(t, `podman 'info'`, runner.lastCmd)
}

func TestExecStream_NonZeroExitCodeIsAResult(t *testing.T) {
	runner := &streamingFakeRunner{
		lines:  []OutputLine{{Stream: StderrStream, Text: "not found"}},
		result: ExecResult{Stdout: "", Stderr: "not found\n", ExitCode: 3},
	}
	svc := newServiceWithDialer(&fakeDialer{dialRunner: runner})

	onLine, lines := collectedCallback(t, context.Background())
	result, err := svc.ExecStream(context.Background(), "machine-1", "user-1", "systemctl", ExecOptions{}, onLine)

	require.NoError(t, err, "a non-zero exit code is a result, not an error")
	assert.Equal(t, 3, result.ExitCode)
	require.Len(t, *lines, 1)
}

func TestExecStream_DialFailureIsAnError(t *testing.T) {
	dialer := &fakeDialer{dialErr: errors.New("connection refused")}
	svc := newServiceWithDialer(dialer)

	onLine, lines := collectedCallback(t, context.Background())
	result, err := svc.ExecStream(context.Background(), "machine-1", "user-1", "systemctl", ExecOptions{}, onLine)

	require.Error(t, err)
	assert.Equal(t, ExecResult{}, result)
	assert.Empty(t, *lines)
	assert.Empty(t, dialer.Invalidated(), "dial failure must not invalidate a non-existent cached client")
}

func TestExecStream_SessionFailureInvalidatesAndErrors(t *testing.T) {
	runner := &streamingFakeRunner{err: errors.New("session closed")}
	dialer := &fakeDialer{dialRunner: runner}
	svc := newServiceWithDialer(dialer)

	onLine, _ := collectedCallback(t, context.Background())
	_, err := svc.ExecStream(context.Background(), "machine-1", "user-1", "systemctl", ExecOptions{}, onLine)

	require.Error(t, err)
	assert.Equal(t, []string{"machine-1"}, dialer.Invalidated(), "session failure must invalidate the cached client")
}

func TestExecStream_ContextCancellationMidStream(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	runner := &streamingFakeRunner{
		emitFunc: func(callback func(OutputLine)) {
			callback(OutputLine{Stream: StdoutStream, Text: "before cancel"})
			cancel()
		},
	}
	svc := newServiceWithDialer(&fakeDialer{dialRunner: runner})

	onLine, lines := collectedCallback(t, ctx)
	result, err := svc.ExecStream(ctx, "machine-1", "user-1", "podman", ExecOptions{}, onLine)

	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, ExecResult{}, result)
	require.Len(t, *lines, 1, "the line emitted before cancellation is delivered; nothing after")
	assert.Equal(t, OutputLine{Stream: StdoutStream, Text: "before cancel"}, (*lines)[0])
}

// --- line splitting (tee writer used by the production runner) ---

func TestLineTeeWriter_EmitsLinesAsTheyArrive(t *testing.T) {
	var mu sync.Mutex
	var got []OutputLine
	record := func(line OutputLine) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, line)
	}
	stdoutBuf, stderrBuf := &bytes.Buffer{}, &bytes.Buffer{}
	lineGroup := newLineTeeGroup(record)
	stdoutW := newLineTeeWriter(StdoutStream, lineGroup, stdoutBuf)
	stderrW := newLineTeeWriter(StderrStream, lineGroup, stderrBuf)

	n, err := fmt.Fprintf(stdoutW, "out 1\npartial out")
	require.NoError(t, err)
	assert.Equal(t, len("out 1\npartial out"), n)
	// stdout line emitted immediately; the partial line is held until the
	// newline arrives (or the stream ends).
	require.Len(t, got, 1)
	assert.Equal(t, OutputLine{Stream: StdoutStream, Text: "out 1"}, got[0])

	mu.Lock()
	got = nil
	mu.Unlock()

	_, err = fmt.Fprint(stderrW, " err\n")
	require.NoError(t, err)
	mu.Lock()
	require.Len(t, got, 1)
	assert.Equal(t, OutputLine{Stream: StderrStream, Text: " err"}, got[0])
	mu.Unlock()

	// Flush emits the trailing partial stdout line.
	stdoutW.Flush()
	mu.Lock()
	require.Len(t, got, 2)
	assert.Equal(t, OutputLine{Stream: StderrStream, Text: " err"}, got[0])
	assert.Equal(t, OutputLine{Stream: StdoutStream, Text: "partial out"}, got[1])
	mu.Unlock()
	assert.Equal(t, "out 1\npartial out", stdoutBuf.String())
	assert.Equal(t, " err\n", stderrBuf.String())
}
