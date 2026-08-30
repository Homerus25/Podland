package sshexec

import (
	"bytes"
	"context"
	"fmt"
	"sync"
)

// OutputStream identifies which remote stream a line of output came from.
type OutputStream string

const (
	// StdoutStream is the remote command's standard output.
	StdoutStream OutputStream = "stdout"
	// StderrStream is the remote command's standard error.
	StderrStream OutputStream = "stderr"
)

// OutputLine is a single line of remote output, tagged with the stream it
// came from.
type OutputLine struct {
	Stream OutputStream
	Text   string
}

// ExecStream runs a command on a remote machine like Exec, but invokes
// onLine for each output line as it arrives (not batched after completion).
// It still returns the fully captured result, so callers can parse a final
// machine-readable report from the captured stdout and interpret the exit
// code exactly as with Exec.
//
// Seam semantics are identical to Exec: a non-zero exit code is a result,
// not an error; SSH dial/session failures are errors; context cancellation
// aborts the session and returns an error.
func (s *Service) ExecStream(ctx context.Context, machineID, userID, command string, opts ExecOptions, onLine func(OutputLine)) (ExecResult, error) {
	_ = userID // accepted for interface symmetry; see Service.Exec.

	cmd := shellQuote(command, opts.Args)

	runner, err := s.dialer.Dial(ctx, machineID)
	if err != nil {
		return ExecResult{}, fmt.Errorf("sshexec: failed to dial machine %q: %w", machineID, err)
	}
	defer runner.Close()

	result, runErr := runner.RunStream(ctx, cmd, onLine)
	if runErr != nil {
		// A real failure (network drop, session closed, ctx cancellation).
		// The cached client is likely stale — drop it so the next call
		// re-dials.
		s.dialer.Invalidate(machineID)
		return ExecResult{}, runErr
	}
	return result, nil
}

// lineTeeGroup lets two lineTeeWriters (session stdout and stderr) share one
// callback safely: x/crypto/ssh pumps the two streams from separate
// goroutines, so the callback must be serialized.
type lineTeeGroup struct {
	mu     sync.Mutex
	onLine func(OutputLine)
}

func newLineTeeGroup(onLine func(OutputLine)) *lineTeeGroup {
	return &lineTeeGroup{onLine: onLine}
}

func (g *lineTeeGroup) emit(line OutputLine) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.onLine(line)
}

// lineTeeWriter tees writes for one remote stream into a capture buffer and,
// on every completed line, invokes the shared callback with that line — so
// lines are emitted as they arrive from the SSH session. Flush emits a
// trailing partial line (output not ending in a newline) as a final line.
type lineTeeWriter struct {
	stream  OutputStream
	group   *lineTeeGroup
	capture *bytes.Buffer
	pending []byte
}

func newLineTeeWriter(stream OutputStream, group *lineTeeGroup, capture *bytes.Buffer) *lineTeeWriter {
	return &lineTeeWriter{stream: stream, group: group, capture: capture}
}

func (w *lineTeeWriter) Write(p []byte) (int, error) {
	w.capture.Write(p)
	w.pending = append(w.pending, p...)
	for {
		idx := bytes.IndexByte(w.pending, '\n')
		if idx < 0 {
			break
		}
		line := string(w.pending[:idx])
		w.pending = w.pending[idx+1:]
		w.group.emit(OutputLine{Stream: w.stream, Text: line})
	}
	return len(p), nil
}

// Flush emits any held partial line (a final line without a trailing
// newline) so the callback sees the complete output.
func (w *lineTeeWriter) Flush() {
	if len(w.pending) > 0 {
		w.group.emit(OutputLine{Stream: w.stream, Text: string(w.pending)})
		w.pending = nil
	}
}
