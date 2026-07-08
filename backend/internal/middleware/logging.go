package middleware

import (
	"net/http"
)

// LoggingMiddleware logs HTTP requests
// TODO: Implement proper logging when logrus is added
func LoggingMiddleware(logger any) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip logging for now until logrus is properly configured
			next.ServeHTTP(w, r)
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
