package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

// LoggingMiddleware logs HTTP requests
func LoggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := NewResponseWriter(w)
			next.ServeHTTP(rw, r)
			logger.Info("http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rw.status,
				"bytes", rw.bytes,
				"duration", time.Since(start),
			)
		})
	}
}

// ResponseWriter wraps http.ResponseWriter to capture status code and bytes written
type ResponseWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

// NewResponseWriter creates a new ResponseWriter
func NewResponseWriter(w http.ResponseWriter) *ResponseWriter {
	return &ResponseWriter{
		ResponseWriter: w,
		status:         http.StatusOK,
		bytes:          0,
	}
}

// WriteHeader captures the status code
func (w *ResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// Write captures the number of bytes written
func (w *ResponseWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}
