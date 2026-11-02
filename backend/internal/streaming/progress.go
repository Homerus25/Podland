package streaming

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/google/uuid"
)

// ProgressWriteDeadline is the one kit-level write deadline applied to
// every progress stream. It is an operation-duration fact, handler-side
// (§18): registry pulls and volume transfers routinely exceed the
// server's 30-second WriteTimeout (cmd/server/main.go), so the kit
// extends the deadline for every progress stream — no per-handler input,
// so no handler can forget it.
const ProgressWriteDeadline = 10 * time.Minute

// OperationContext is the operation identity the kit mints per request
// and hands to every domain callback: the operation ID for event stamping
// plus the caller and machine identities the subtree middleware already
// validated.
type OperationContext struct {
	OperationID string
	MachineID   string
	UserID      string
}

// ProgressConfig describes a progress-streaming operation to be
// orchestrated by HandleProgress. The kit owns the writer and the event
// forwarding; the domain owns the operation and the event vocabulary.
type ProgressConfig[Event, Result any] struct {
	// Operation runs the long-running work. It must honour ctx
	// cancellation and report progress through the provided callback.
	Operation func(ctx context.Context, op OperationContext, onProgress func(Event)) (Result, error)

	// CompleteEvent builds the terminal event for a successful result.
	CompleteEvent func(op OperationContext, result Result) any

	// ErrorDefault is the domain's fallback sentinel: failures that are
	// not (wrapped) AppErrors surface its code and generic message. A nil
	// default falls back to the internal-error sentinel.
	ErrorDefault *appErrors.AppError

	// ErrorEvent builds the terminal event for a failure from the
	// module-mapped vocabulary: code and message resolve cancelled /
	// (wrapped) AppError / unknown per the shared mapping, and cancelled
	// is true when the operation ended because the request context was
	// cancelled (including client disconnects detected by the run loop).
	ErrorEvent func(op OperationContext, code, message string, cancelled bool) any
}

// HandleProgress is the streaming twin of web.Handle: the one handler-side
// entry point for progress streams (§20). It owns the whole ritual —
// operationID minting, the operation slog line (assembled from the
// handler's operation name and attrs over the kit's
// requestID/machineID/operationID base), the 10-minute write-deadline
// extension, the streaming envelope (headers, writer, identity), and the
// run loop — and hands the minted operation context to every domain
// callback. A handler shrinks to its pre-stream ritual (URL-param guards,
// web.Decode 400s, domain forks like the quadlet move's plain/stream
// split) plus one HandleProgress call carrying the domain's config and
// event vocabulary.
//
// The pre-stream ritual stays handler-side: HandleProgress is called only
// once the endpoint is committed to streaming, so errors answered before
// the call keep the plain envelope. A begin failure (missing identity,
// unsupported flushing) answers through the envelope itself and
// HandleProgress returns silently.
func HandleProgress[Event, Result any](w http.ResponseWriter, r *http.Request, operation string, attrs []slog.Attr, cfg ProgressConfig[Event, Result]) {
	operationID := uuid.New().String()

	logArgs := make([]any, 0, len(attrs)+3)
	logArgs = append(logArgs,
		slog.String("requestID", contextutils.GetRequestID(r.Context())),
		slog.String("machineID", r.PathValue("id")),
		slog.String("operationID", operationID))
	for _, attr := range attrs {
		logArgs = append(logArgs, attr)
	}
	slog.InfoContext(r.Context(), operation, logArgs...)

	if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(ProgressWriteDeadline)); err != nil {
		slog.WarnContext(r.Context(), "Failed to extend write deadline", slog.Any("error", err))
	}

	stream := begin(w, r)
	if stream == nil {
		return
	}

	op := OperationContext{OperationID: operationID, MachineID: stream.MachineID, UserID: stream.UserID}

	run(r.Context(), stream.Writer, streamConfig[Event, Result]{
		Operation: func(ctx context.Context, onProgress func(Event)) (Result, error) {
			return cfg.Operation(ctx, op, onProgress)
		},
		OnEvent: func(event Event) error {
			return stream.Writer.Write(event)
		},
		CompleteEvent: func(result Result) any {
			return cfg.CompleteEvent(op, result)
		},
		ErrorDefault: cfg.ErrorDefault,
		ErrorEvent: func(code, message string, cancelled bool) any {
			return cfg.ErrorEvent(op, code, message, cancelled)
		},
	})
}
