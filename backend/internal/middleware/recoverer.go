package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	apperrors "github.com/Homerus25/podland/backend/internal/errors"
)

// Recoverer is the global panic recovery in the middleware chain, ported
// off chi's middleware.Recoverer when the router moved to net/http's
// ServeMux (issue #451): a panicking handler must not take the
// connection goroutine down with it. The panic is logged once (slog,
// with the requestID the RequestID middleware already put in the
// context, plus the stack) and the caller gets the JSON error envelope's
// generic 500 — internal detail never leaks to the wire.
//
// http.ErrAbortHandler is deliberately not recovered: it is the standard
// library's sentinel for intentionally aborting a handler (the client is
// gone), not a defect, so it re-panics for the server's own teardown
// path to handle.
//
// When the panic fires after the response has started (mid-stream), the
// envelope write is a no-op superfluous WriteHeader — the client sees
// the truncated response either way, and the log line still lands.
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rv := recover(); rv != nil && rv != http.ErrAbortHandler {
				requestID := contextutils.GetRequestID(r.Context())
				slog.ErrorContext(r.Context(), "panic recovered",
					slog.String("requestID", requestID),
					slog.Any("panic", rv),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.String("stack", string(debug.Stack())))
				apperrors.RespondWithError(w, apperrors.ErrInternalError, requestID)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
