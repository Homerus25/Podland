package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRecoverer_PanickingHandlerAnswers500Envelope pins the ported
// Recoverer's contract (issue #451): a panicking handler does not take
// the goroutine down, the 500 is the JSON error envelope (generic
// INTERNAL_ERROR, internal detail never on the wire), and the
// requestID from the context rides along.
func TestRecoverer_PanickingHandlerAnswers500Envelope(t *testing.T) {
	panicking := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom: handler defect")
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/hosts", nil).
		WithContext(contextutils.SetRequestID(t.Context(), "test-request-id"))

	require.NotPanics(t, func() {
		Recoverer(panicking).ServeHTTP(rec, req)
	})

	require.Equal(t, http.StatusInternalServerError, rec.Code, "a recovered panic must answer 500")

	var body struct {
		RequestID string `json:"requestId"`
		Error     struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body), "the 500 must be the JSON error envelope")
	assert.Equal(t, "test-request-id", body.RequestID)
	assert.Equal(t, "INTERNAL_ERROR", body.Error.Code)
	assert.Equal(t, "An unexpected error occurred", body.Error.Message)
}

// TestRecoverer_CleanHandlerPassesThrough pins that the middleware is
// invisible on the happy path: status and body arrive unmodified.
func TestRecoverer_CleanHandlerPassesThrough(t *testing.T) {
	clean := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("fine"))
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)

	Recoverer(clean).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusTeapot, rec.Code)
	assert.Equal(t, "fine", rec.Body.String())
}
