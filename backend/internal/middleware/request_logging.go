package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/Homerus25/podland/backend/internal/contextutils"
)

// statusRecorder captures the response status for logging while
// passing Flush through, so streamed responses (SSE events, volume
// exports, NDJSON progress) keep flushing chunk-by-chunk.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (w *statusRecorder) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusRecorder) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// RequestLogging logs one line per request (method, path, status,
// duration, requestID) once the response has finished. It replaces the
// per-handler slog.InfoContext call that used to open every endpoint:
// request logging is a property of the router, not of each handler, so
// long-lived streams log at stream end and every endpoint — including
// streaming ones — is covered without handler-side ceremony.
//
// It must run inside RequestIDMiddleware so the requestID is already in
// the context. The user ID is not logged here: JWTMiddleware runs
// further in and enriches only the inner request's context, which this
// outer wrapper cannot see.
func RequestLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(recorder, r)

		slog.InfoContext(r.Context(), "http_request",
			slog.Any("method", r.Method),
			slog.Any("path", r.URL.Path),
			slog.Any("status", recorder.status),
			slog.Any("duration_ms", time.Since(start).Milliseconds()),
			slog.Any("requestID", contextutils.GetRequestID(r.Context())))
	})
}
