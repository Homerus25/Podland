package streaming

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
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
// that owns the follow=true ritual end to end. Begin runs the streaming
// envelope (headers, writer, identity — a failure answers through the
// envelope itself and RunLogStream returns silently); Run wires the
// select loop, disconnect cancellation, the clean-end terminal INFO
// STREAM_ENDED event, and the error mapping — a (possibly wrapped)
// AppError surfaces as one "CODE: message" ERROR event, a cancelled
// context as one WARN event, and any other cause as the baked-in
// connection-failed default, logged once by the kit and never surfaced
// raw.
func RunLogStream(w http.ResponseWriter, r *http.Request, op LogStreamOp) {
	slog.InfoContext(r.Context(), "log stream starting", slog.Bool("follow", true))

	stream := Begin(w, r)
	if stream == nil {
		return
	}

	Run(r.Context(), stream.Writer, StreamConfig[LogEvent, struct{}]{
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

// Compile-time proof the kit writer satisfies the io.Writer seam the
// podman Logs options expect.
var _ io.Writer = (*LogEventWriter)(nil)
