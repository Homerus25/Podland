package quadlets

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// convertComposeRequest builds the request the mounted user-scoped route
// hands the handler: a user identity in the context and NO machine ID —
// the route sits outside the machine subtree (spec #336), so the request
// carries only a user identity.
func convertComposeRequest(t *testing.T, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/quadlets/convert-compose", bytes.NewReader([]byte(body)))
	ctx := contextutils.SetUserID(req.Context(), "user-1")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")
	return req.WithContext(ctx)
}

// newConvertTestHandler builds the handler around the real lean service
// with the given converter wired, over the fake engine (the conversion
// touches no machine, so the engine stays unarmed).
func newConvertTestHandler(converter ComposeConverter) *QuadletHandler {
	provider := fakeEngineProvider{engine: &fakeQuadletEngine{}}
	service := NewQuadletService(provider, nil, nil).WithComposeConverter(converter)
	return NewQuadletHandler(service, NewQuadletTransferService(provider, nil, nil, 0, service))
}

// errorEnvelope is the failure half of the endpoint envelope
// ({requestId, error{code, message}}) the handler tests decode.
type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// TestHandlerConvertCompose_Success pins the user-scoped contract through
// the real service over the fake converter: a request carrying only a user
// identity (no machine ID) answers 200 with the generated entries in the
// import endpoint's shape, sorted by name.
func TestHandlerConvertCompose_Success(t *testing.T) {
	t.Parallel()

	converter := &fakeComposeConverter{
		convertFn: func(_ context.Context, composeContent string) ([]GeneratedQuadletFile, error) {
			assert.Equal(t, "services:\n  web:\n    image: nginx\n", composeContent)
			return []GeneratedQuadletFile{
				{FileName: "web.container", Content: "[Container]\nImage=nginx\n"},
				{FileName: "dbdata.volume", Content: "[Volume]\nDriver=local\n"},
			}, nil
		},
	}

	w := httptest.NewRecorder()
	newConvertTestHandler(converter).ConvertCompose(w, convertComposeRequest(t,
		`{"content":"services:\n  web:\n    image: nginx\n"}`))

	require.Equal(t, http.StatusOK, w.Code)

	var response QuadletConvertComposeResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
	require.Len(t, response.Quadlets, 2)
	assert.Equal(t, QuadletImportEntry{Name: "dbdata", Resource: "volume", Content: "[Volume]\nDriver=local\n"}, response.Quadlets[0])
	assert.Equal(t, QuadletImportEntry{Name: "web", Resource: "container", Content: "[Container]\nImage=nginx\n"}, response.Quadlets[1])
}

// TestHandlerConvertCompose_ConverterFailure pins the podlet-failure
// contract over HTTP: a converter rejection answers 400 VALIDATION_FAILED
// carrying the converter's stderr verbatim (ANSI color escapes stripped).
func TestHandlerConvertCompose_ConverterFailure(t *testing.T) {
	t.Parallel()

	converter := &fakeComposeConverter{
		convertFn: func(context.Context, string) ([]GeneratedQuadletFile, error) {
			return nil, &ConversionError{Stderr: "\x1b[91merror converting service `app`: `image` and `build` cannot both be set\x1b[0m\n"}
		},
	}

	w := httptest.NewRecorder()
	newConvertTestHandler(converter).ConvertCompose(w, convertComposeRequest(t, `{"content":"services: {}"}`))

	require.Equal(t, http.StatusBadRequest, w.Code)

	var envelope errorEnvelope
	require.NoError(t, json.NewDecoder(w.Body).Decode(&envelope))
	assert.Equal(t, "VALIDATION_FAILED", envelope.Error.Code)
	assert.Equal(t, "error converting service `app`: `image` and `build` cannot both be set\n", envelope.Error.Message)
}

// TestHandlerConvertCompose_Validation pins the decode/validate
// vocabulary: an empty or missing content answers 400 VALIDATION_FAILED,
// and a malformed JSON body answers 400 INVALID_REQUEST.
func TestHandlerConvertCompose_Validation(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		body     string
		wantCode string
	}{
		{name: "empty content", body: `{"content":""}`, wantCode: "VALIDATION_FAILED"},
		{name: "missing content", body: `{}`, wantCode: "VALIDATION_FAILED"},
		{name: "malformed JSON", body: `{not valid json`, wantCode: "INVALID_REQUEST"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// The converter stays unarmed: a rejected body must never
			// reach the conversion.
			converter := &fakeComposeConverter{}

			w := httptest.NewRecorder()
			newConvertTestHandler(converter).ConvertCompose(w, convertComposeRequest(t, tc.body))

			require.Equal(t, http.StatusBadRequest, w.Code)

			var envelope errorEnvelope
			require.NoError(t, json.NewDecoder(w.Body).Decode(&envelope))
			assert.Equal(t, tc.wantCode, envelope.Error.Code)
		})
	}
}

// TestHandlerConvertCompose_ContentSizeCap pins the 1 MiB cap and its
// boundary. The cap is on the RAW REQUEST BODY, enforced at the HTTP layer
// (http.MaxBytesReader in the handler, before web.Decode): a body of
// exactly MaxComposeContentBytes bytes is accepted, one byte more trips the
// reader inside web.Decode's json.Decode and answers 400 INVALID_REQUEST
// "Invalid request body". The boundary content length is derived from the
// marshaled body's own overhead ({"content":"…"} is 14 bytes), so the
// accepted body is exactly 1 MiB and the rejected one exactly 1 MiB + 1.
func TestHandlerConvertCompose_ContentSizeCap(t *testing.T) {
	t.Parallel()

	converter := &fakeComposeConverter{
		convertFn: func(context.Context, string) ([]GeneratedQuadletFile, error) {
			return []GeneratedQuadletFile{{FileName: "web.container", Content: "[Container]\nImage=nginx\n"}}, nil
		},
	}
	handler := newConvertTestHandler(converter)

	marshal := func(content string) []byte {
		body, err := json.Marshal(QuadletConvertComposeRequest{Content: content})
		require.NoError(t, err)
		return body
	}
	overhead := len(marshal(""))
	atCap := strings.Repeat("a", MaxComposeContentBytes-overhead)

	t.Run("body of exactly 1 MiB is accepted", func(t *testing.T) {
		t.Parallel()

		body := marshal(atCap)
		require.Len(t, body, MaxComposeContentBytes, "the boundary body must be exactly the cap")

		w := httptest.NewRecorder()
		handler.ConvertCompose(w, convertComposeRequest(t, string(body)))

		require.Equal(t, http.StatusOK, w.Code)

		var response QuadletConvertComposeResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
		require.Len(t, response.Quadlets, 1, "the accepted body must reach the converter")
	})

	t.Run("body over 1 MiB is rejected", func(t *testing.T) {
		t.Parallel()

		body := marshal(atCap + "a")
		require.Len(t, body, MaxComposeContentBytes+1, "the rejected body must be one byte over the cap")

		w := httptest.NewRecorder()
		handler.ConvertCompose(w, convertComposeRequest(t, string(body)))

		require.Equal(t, http.StatusBadRequest, w.Code)

		var envelope errorEnvelope
		require.NoError(t, json.NewDecoder(w.Body).Decode(&envelope))
		assert.Equal(t, "INVALID_REQUEST", envelope.Error.Code)
		assert.Equal(t, "Invalid request body", envelope.Error.Message)
	})
}

// TestHandlerConvertCompose_ConverterStartFailure pins the wire contract
// for a converter that cannot start (pentest 2026-11-01, finding 7): a
// non-ConversionError from the converter crosses the envelope's 500
// fallback — INTERNAL_ERROR with the generic message, the raw exec text
// only in the server log, never on the wire.
func TestHandlerConvertCompose_ConverterStartFailure(t *testing.T) {
	t.Parallel()

	converter := &fakeComposeConverter{
		convertFn: func(context.Context, string) ([]GeneratedQuadletFile, error) {
			return nil, fmt.Errorf("failed to start podlet: %w", exec.ErrNotFound)
		},
	}

	w := httptest.NewRecorder()
	newConvertTestHandler(converter).ConvertCompose(w, convertComposeRequest(t, `{"content":"services: {}"}`))

	require.Equal(t, http.StatusInternalServerError, w.Code)

	var envelope errorEnvelope
	require.NoError(t, json.NewDecoder(w.Body).Decode(&envelope))
	assert.Equal(t, "INTERNAL_ERROR", envelope.Error.Code)
	assert.Equal(t, "An unexpected error occurred", envelope.Error.Message)
	assert.NotContains(t, w.Body.String(), "executable file not found",
		"the raw exec error must not reach the wire")
	assert.NotContains(t, w.Body.String(), "podlet",
		"the server-side binary name must not reach the wire")
}

// TestHandlerConvertCompose_StartFailureIsLogged pins the server-side
// half of the start-failure contract (pentest 2026-11-01, finding 7): the
// envelope logs the raw wrapped error once — requestID included — before
// answering the generic 500, so the exec detail stays diagnosable from
// the logs while never reaching the wire.
func TestHandlerConvertCompose_StartFailureIsLogged(t *testing.T) {
	converter := &fakeComposeConverter{
		convertFn: func(context.Context, string) ([]GeneratedQuadletFile, error) {
			return nil, fmt.Errorf("failed to start podlet: %w", exec.ErrNotFound)
		},
	}

	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	defer slog.SetDefault(previous)

	w := httptest.NewRecorder()
	newConvertTestHandler(converter).ConvertCompose(w, convertComposeRequest(t, `{"content":"services: {}"}`))

	require.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, logs.String(), "endpoint envelope: unexpected error")
	assert.Contains(t, logs.String(), "test-request-id", "the log line must carry the request ID")
	assert.Contains(t, logs.String(), "failed to start podlet", "the raw start failure must be in the log")
	assert.Contains(t, logs.String(), "executable file not found", "the underlying exec error must be in the log")
}
