package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestLogging_CapturesStatus(t *testing.T) {
	var inner http.ResponseWriter

	handler := RequestLogging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inner = w
		w.WriteHeader(http.StatusTeapot)
		w.Write([]byte("body"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/machines/m1/volumes", nil).
		WithContext(contextutils.SetRequestID(httptest.NewRequest(http.MethodGet, "/", nil).Context(), "req-1"))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusTeapot, w.Code, "status must pass through to the client")
	assert.Equal(t, "body", w.Body.String(), "body must pass through")
	require.NotNil(t, inner)
	_, canFlush := inner.(http.Flusher)
	assert.True(t, canFlush, "streaming handlers must be able to flush through the recorder")
}

func TestRequestLogging_DefaultsTo200WhenHandlerNeverWritesHeader(t *testing.T) {
	handler := RequestLogging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
