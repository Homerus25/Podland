package testutils

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// ErrorEnvelope mirrors the structured error body the endpoint envelope
// writes: request metadata at the top level, the parseable cause
// under "error".
type ErrorEnvelope struct {
	RequestID string `json:"requestId"`
	Error     struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// DecodeErrorEnvelope reads a structured error response into its
// ErrorEnvelope, so failure tests can assert the exact parseable
// cause — and, where relevant, the message or request ID — instead of
// substring-matching the raw body. One decode ritual serves every
// package's error-code assertions.
func DecodeErrorEnvelope(t *testing.T, resp *http.Response) ErrorEnvelope {
	t.Helper()

	var envelope ErrorEnvelope
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&envelope), "Should decode the error envelope")
	require.NotEmpty(t, envelope.Error.Code, "Error envelope must carry a code")
	return envelope
}
