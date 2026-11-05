package streaming

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/Homerus25/podland/backend/internal/web"
)

// LogsDispatchOp is the handler-side log dispatch kit's input: the
// domain's URL-param name and missing-param guard code plus the domain's
// two service calls. The dispatch is generic over the buffered response
// type, so no domain vocabulary crosses into the kit — the buffered
// closure returns the domain's own logs response DTO.
type LogsDispatchOp[Response any] struct {
	// ParamName is the domain's target URL-param name ("containerId",
	// "podId", "quadletId").
	ParamName string
	// GuardCode is the domain's missing-param guard code
	// ("MISSING_CONTAINER_ID"); the guard itself is web.URLParam's
	// vocabulary, never hand-minted here.
	GuardCode string
	// Follow is the domain's follow-mode log service call. It receives
	// the resolved target and the parsed window; events cross through
	// onEvent.
	Follow func(ctx context.Context, userID, hostID, target string, since time.Time, tail int64, onEvent func(LogEvent)) error
	// Buffered is the domain's buffered log service call. It receives
	// the resolved target and the parsed window and returns the domain's
	// own logs response type.
	Buffered func(ctx context.Context, userID, hostID, target string, since time.Time, tail int64) (Response, error)
}

// HandleLogs is the one handler-side entry point for log endpoints, the
// handler-side twin of what the kit already owns below the handler: it
// performs the whole dispatch — the strict since/tail window parse, the
// query-error fallback (the error rides the endpoint envelope even when
// follow=true was requested), the URL-param guard, and the follow fork:
// a clean follow request crosses the streaming envelope, everything
// else the buffered endpoint envelope. The target param is resolved once
// and handed to both closures, and the follow-dispatch log line is
// kit-owned. A log handler shrinks to one HandleLogs call carrying its
// param name, guard code, and two service calls.
func HandleLogs[Response any](w http.ResponseWriter, r *http.Request, op LogsDispatchOp[Response]) {
	since, tail, queryErr := ParseLogWindow(r)
	if queryErr != nil {
		web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, hostID string) (struct{}, error) {
			return struct{}{}, queryErr
		})
		return
	}

	target, guard := web.URLParam(r, op.ParamName, op.GuardCode)

	if guard == nil && ParseFollow(r) {
		slog.InfoContext(r.Context(), "streamLogs", slog.String("target", target), slog.Bool("follow", true))
		RunLogStream(w, r, LogStreamOp{
			Since: since,
			Tail:  tail,
			Stream: func(ctx context.Context, userID, hostID string, onEvent func(LogEvent)) error {
				return op.Follow(ctx, userID, hostID, target, since, tail, onEvent)
			},
		})
		return
	}

	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, hostID string) (Response, error) {
		if guard != nil {
			var zero Response
			return zero, guard
		}
		return op.Buffered(ctx, userID, hostID, target, since, tail)
	})
}
