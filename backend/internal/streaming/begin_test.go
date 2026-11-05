package streaming

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// serveBegin runs a begin endpoint the way the router does: request-scoped
// context values plus chi URL params (mirrors the web envelope test setup).
func serveBegin(t *testing.T, w http.ResponseWriter, userID, hostID string, endpoint http.HandlerFunc) {
	t.Helper()

	ctx := contextutils.SetRequestID(context.Background(), "test-request-id")
	if userID != "" {
		ctx = contextutils.SetUserID(ctx, userID)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/hosts/"+hostID+"/containers/x/logs", nil).WithContext(ctx)

	if hostID != "" {
		req.SetPathValue("id", hostID)
	}

	endpoint(w, req)
}

// captureBegin is an endpoint that records the stream handle begin returned.
func captureBegin(out **streamHandle) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		*out = begin(w, r)
	}
}

func TestBegin_SetsHeadersAndReturnsIdentities(t *testing.T) {
	rec := httptest.NewRecorder()

	var gotStream *streamHandle
	serveBegin(t, rec, "test-user", "test-host", captureBegin(&gotStream))

	require.NotNil(t, gotStream, "begin must return a stream handle")
	assert.Equal(t, "test-user", gotStream.UserID)
	assert.Equal(t, "test-host", gotStream.HostID)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache", rec.Header().Get("Cache-Control"))
}

func TestBegin_WriterEmitsNDJSON(t *testing.T) {
	rec := httptest.NewRecorder()

	var gotStream *streamHandle
	serveBegin(t, rec, "test-user", "test-host", captureBegin(&gotStream))
	require.NotNil(t, gotStream)

	require.NoError(t, gotStream.Writer.Write(map[string]string{"hello": "world"}))
	assert.JSONEq(t, `{"hello":"world"}`, rec.Body.String())
}

func TestBegin_MissingUserIDIsAnInvariantViolation(t *testing.T) {
	rec := httptest.NewRecorder()

	var gotStream *streamHandle
	serveBegin(t, rec, "", "test-host", captureBegin(&gotStream))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Nil(t, gotStream, "begin must return nil after writing the error response")
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "INTERNAL_ERROR", body.Error.Code)
}

func TestBegin_MissingHostIDIsAnInvariantViolation(t *testing.T) {
	rec := httptest.NewRecorder()

	var gotStream *streamHandle
	serveBegin(t, rec, "test-user", "", captureBegin(&gotStream))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Nil(t, gotStream, "begin must return nil after writing the error response")
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "INTERNAL_ERROR", body.Error.Code)
}

// noFlushWriter is a ResponseWriter that cannot flush, so begin must answer
// with the early 500 STREAMING_NOT_SUPPORTED response.
type noFlushWriter struct {
	header http.Header
	body   bytes.Buffer
	code   int
}

func (w *noFlushWriter) Header() http.Header         { return w.header }
func (w *noFlushWriter) Write(p []byte) (int, error) { return w.body.Write(p) }
func (w *noFlushWriter) WriteHeader(code int)        { w.code = code }

func TestBegin_UnsupportedFlushingWrites500(t *testing.T) {
	w := &noFlushWriter{header: http.Header{}}
	var gotStream *streamHandle

	serveBegin(t, w, "test-user", "test-host", captureBegin(&gotStream))

	assert.Equal(t, http.StatusInternalServerError, w.code)
	assert.Contains(t, w.body.String(), "STREAMING_NOT_SUPPORTED")
	assert.Nil(t, gotStream, "begin must return nil after writing the error response")
}
