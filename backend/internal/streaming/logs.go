package streaming

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/podmanerr"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// LogEvent is the kit-owned wire shape of one follow-mode log-stream
// event: {timestamp, level, message}, byte-identical across every log
// domain.
type LogEvent struct {
	// ISO8601 timestamp
	Timestamp string `json:"timestamp"`
	// Log level (INFO, WARN, ERROR, etc.)
	Level string `json:"level"`
	// Log message content
	Message string `json:"message"`
}

// LogEventWriter is the io.Writer a domain service hands to the podman
// Logs call: podman streams one log line per Write, and the writer
// converts each line into a LogEvent for the onEvent callback.
//
// Line handling: the trailing newline is trimmed, empty lines and
// spaceless lines are skipped (a spaceless line has no timestamp to
// parse, and emitting it raw would format a spaceless blob as an event),
// a leading RFC3339Nano timestamp (literal-Z and offset forms) is parsed
// out of the message and re-emitted as RFC3339 UTC, and anything else
// keeps the whole line as the message with the current time as
// timestamp.
type LogEventWriter struct {
	onEvent func(LogEvent)
	level   string
}

// NewLogEventWriter creates a LogEventWriter emitting at the given level
// (stdout streams configure "INFO", stderr "ERROR") through the onEvent
// callback.
func NewLogEventWriter(level string, onEvent func(LogEvent)) *LogEventWriter {
	return &LogEventWriter{onEvent: onEvent, level: level}
}

// Write implements io.Writer: one podman log line in, one LogEvent out
// (or none, for skipped lines). It always consumes the full write.
func (w *LogEventWriter) Write(data []byte) (int, error) {
	line := strings.TrimSuffix(string(data), "\n")
	if line == "" {
		return len(data), nil
	}

	timestamp := time.Now().UTC().Format(time.RFC3339)
	message := line

	parts := strings.SplitN(line, " ", 2)
	if len(parts) == 1 {
		return len(data), nil
	}
	if parsed, err := time.Parse(time.RFC3339Nano, parts[0]); err == nil {
		timestamp = parsed.UTC().Format(time.RFC3339)
		message = parts[1]
	}

	w.onEvent(LogEvent{
		Timestamp: timestamp,
		Level:     w.level,
		Message:   message,
	})

	return len(data), nil
}

// LogStreamOp is the log-stream kit's input: the parsed query window plus
// the domain's stream function. The window fields record the request the
// op was built from; the Stream closure captures them itself when it
// resolves its target.
type LogStreamOp struct {
	Since time.Time
	Tail  int64
	// Stream runs the domain's follow-mode log fetch, reporting each log
	// event through onEvent. It must honour ctx cancellation.
	Stream func(ctx context.Context, userID, machineID string, onEvent func(LogEvent)) error
}

// RunLogStream is the entry point for follow-mode log streams: one call
// that owns the follow=true ritual end to end. begin runs the streaming
// envelope (headers, writer, identity — a failure answers through the
// envelope itself and RunLogStream returns silently); run wires the
// select loop, disconnect cancellation, the clean-end terminal INFO
// STREAM_ENDED event, and the error mapping — a (possibly wrapped)
// AppError surfaces as one "CODE: message" ERROR event, a cancelled
// context as one WARN event, and any other cause as the baked-in
// connection-failed default, logged once by the kit and never surfaced
// raw.
func RunLogStream(w http.ResponseWriter, r *http.Request, op LogStreamOp) {
	slog.InfoContext(r.Context(), "log stream starting", slog.Bool("follow", true))

	stream := begin(w, r)
	if stream == nil {
		return
	}

	run(r.Context(), stream.Writer, streamConfig[LogEvent, struct{}]{
		Operation: func(ctx context.Context, onEvent func(LogEvent)) (struct{}, error) {
			return struct{}{}, op.Stream(ctx, stream.UserID, stream.MachineID, onEvent)
		},
		OnEvent: func(event LogEvent) error {
			return stream.Writer.Write(event)
		},
		CompleteEvent: func(struct{}) any {
			return LogEvent{
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Level:     StreamEndedLevel,
				Message:   StreamEndedMessage,
			}
		},
		ErrorDefault: appErrors.ErrConnectionFailed,
		ErrorEvent: func(code, message string, cancelled bool) any {
			level := "ERROR"
			if cancelled {
				level = "WARN"
			}
			return LogEvent{
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Level:     level,
				Message:   code + ": " + message,
			}
		},
	})
}

// FetchLogsOp is the buffered-fetch kit's input: the already-resolved
// target (the container name to log from; quadlet name resolution stays
// domain work), the parsed query window, the domain's not-found sentinel,
// and the domain's Logs function.
type FetchLogsOp struct {
	Target string
	Since  time.Time
	Tail   int64
	// NotFound is the caller's not-found sentinel: podman's not-found
	// answer (containers → ErrContainerNotFound, quadlets →
	// ErrQuadletNotFound) maps to it.
	NotFound *appErrors.AppError
	// Logs runs the domain's log fetch through the options FetchLogs
	// builds. Cancellation is the kit's job: Cancellable abandons the
	// call when the context is cancelled, so the function needs no
	// context of its own.
	Logs func(name string, opts entities.ContainerLogsOptions) error
}

// FetchLogs is the entry point for buffered (follow=false) log fetches:
// one call that owns that ritual end to end. It builds the podman logs
// options itself (Follow=false, Timestamps=false, the Since/Tail window
// passed through), installs the stdout/stderr writer pair, combines the
// captured output (stdout first, stderr appended, exactly one trailing
// newline trimmed), and runs the fetch through Cancellable: a cancelled
// context answers ErrOperationCancelled without waiting for the fetch
// (logs can be very long, so a buffered fetch being fast was never a
// guarantee), podman's not-found answer maps to the op's NotFound
// sentinel, and any other cause carries one kit-owned "failed to get
// logs" wrap whose text is log-only, never a wire payload.
func FetchLogs(ctx context.Context, op FetchLogsOp) (string, error) {
	var stdout, stderr bytes.Buffer
	opts := entities.ContainerLogsOptions{
		Follow:       false,
		Since:        op.Since,
		Tail:         op.Tail,
		Timestamps:   false,
		StdoutWriter: &stdout,
		StderrWriter: &stderr,
	}

	err := Cancellable(ctx, func() error { return op.Logs(op.Target, opts) })
	if err != nil {
		if errors.Is(err, appErrors.ErrOperationCancelled) {
			return "", err
		}
		if podmanerr.IsNotFound(err) {
			return "", op.NotFound
		}
		return "", fmt.Errorf("failed to get logs: %w", err)
	}

	combined := strings.TrimSuffix(stdout.String(), "\n")
	if stderr.Len() > 0 {
		stderrContent := strings.TrimSuffix(stderr.String(), "\n")
		if combined != "" {
			combined += "\n" + stderrContent
		} else {
			combined = stderrContent
		}
	}
	return combined, nil
}

// FollowLogsOp is the follow-fetch kit's input: the already-resolved
// target (the container id to log from; quadlet name resolution stays
// domain work), the parsed query window, the domain's not-found sentinel,
// the domain's Logs function, and the event callback.
type FollowLogsOp struct {
	Target string
	Since  time.Time
	Tail   int64
	// NotFound is the caller's not-found sentinel: podman's not-found
	// answer (containers → ErrContainerNotFound, quadlets →
	// ErrQuadletNotFound) maps to it.
	NotFound *appErrors.AppError
	// Logs runs the domain's follow-mode fetch through the options
	// FollowLogs builds. Cancellation is the kit's job: Cancellable
	// abandons the call when the context is cancelled, so the function
	// needs no context of its own.
	Logs func(name string, opts entities.ContainerLogsOptions) error
	// OnEvent receives one LogEvent per podman log line, stdout lines at
	// INFO, stderr lines at ERROR.
	OnEvent func(LogEvent)
}

// FollowLogs is the entry point for follow-mode (follow=true) log
// fetches: one call that owns that ritual end to end. It builds the
// podman logs options itself (Follow=true, Timestamps=true — the one
// intentional divergence from FetchLogs, because the event writer parses
// the timestamps back out — and the Since/Tail window passed through),
// installs the stdout-INFO / stderr-ERROR event-writer pair, and runs the
// fetch through Cancellable: a cancelled context answers
// ErrOperationCancelled without waiting for the follow to end (it never
// does on its own), podman's not-found answer maps to the op's NotFound
// sentinel, and any other cause carries one kit-owned "failed to get
// logs" wrap whose text is log-only, never a wire payload.
func FollowLogs(ctx context.Context, op FollowLogsOp) error {
	opts := entities.ContainerLogsOptions{
		Follow:       true,
		Since:        op.Since,
		Tail:         op.Tail,
		Timestamps:   true,
		StdoutWriter: NewLogEventWriter("INFO", op.OnEvent),
		StderrWriter: NewLogEventWriter("ERROR", op.OnEvent),
	}

	err := Cancellable(ctx, func() error { return op.Logs(op.Target, opts) })
	if err != nil {
		if errors.Is(err, appErrors.ErrOperationCancelled) {
			return err
		}
		if podmanerr.IsNotFound(err) {
			return op.NotFound
		}
		return fmt.Errorf("failed to get logs: %w", err)
	}
	return nil
}

// Compile-time proof the kit writer satisfies the io.Writer seam the
// podman Logs options expect.
var _ io.Writer = (*LogEventWriter)(nil)
