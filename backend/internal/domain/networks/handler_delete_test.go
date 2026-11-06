package networks

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// deleteRequest serves DELETE
// /api/hosts/{id}/networks/{networkName}{query} through the handler with
// an authenticated test identity.
func deleteRequest(handler http.HandlerFunc, networkName, query string) *httptest.ResponseRecorder {
	return testutils.Perform(handler, testutils.Request{
		Method:    http.MethodDelete,
		Path:      "/api/hosts/test-host/networks/" + networkName + query,
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-host", "networkName": networkName},
	})
}

// TestNetworkHandler_Delete_Success pins the delete contract: 204 with an
// empty body, and the force query flag forwarded to the host parsed
// (default false).
func TestNetworkHandler_Delete_Success(t *testing.T) {
	for _, tc := range []struct {
		query      string
		wantForce  bool
		queryLabel string
	}{
		{query: "", wantForce: false, queryLabel: "default"},
		{query: "?force=true", wantForce: true, queryLabel: "force=true"},
		{query: "?force=false", wantForce: false, queryLabel: "force=false"},
	} {
		t.Run(tc.queryLabel, func(t *testing.T) {
			var gotName string
			var gotForce bool
			engine := &fakeNetworkEngine{
				removeFn: func(name string, force bool) error {
					gotName = name
					gotForce = force
					return nil
				},
			}

			w := deleteRequest(newTestHandler(engine).Delete, "frontend", tc.query)

			assert.Equal(t, 204, w.Code, "delete of an unused network should return 204")
			assert.Empty(t, w.Body.String(), "204 response must have an empty body")
			assert.Equal(t, "frontend", gotName)
			assert.Equal(t, tc.wantForce, gotForce, "force flag must reach the host parsed")
		})
	}
}

// TestNetworkHandler_Delete_NonBooleanForceIsIgnored pins force-flag
// parsing: a non-boolean force value is ignored (deletes as unforced), the
// same lenient contract every domain's delete shares via web.QueryBool.
func TestNetworkHandler_Delete_NonBooleanForceIsIgnored(t *testing.T) {
	var gotForce bool
	engine := &fakeNetworkEngine{
		removeFn: func(name string, force bool) error {
			gotForce = force
			return nil
		},
	}

	w := deleteRequest(newTestHandler(engine).Delete, "frontend", "?force=yes")

	assert.Equal(t, 204, w.Code, "a non-boolean force deletes as unforced, not as a client error")
	assert.False(t, gotForce, "the non-boolean force must reach the host as false")
}

// TestNetworkHandler_Delete_NotFound pins the vanished-network branch
// through the HTTP surface: 404 NETWORK_NOT_FOUND.
func TestNetworkHandler_Delete_NotFound(t *testing.T) {
	engine := &fakeNetworkEngine{
		removeFn: func(string, bool) error {
			return &errorhandling.ErrorModel{
				ResponseCode: http.StatusNotFound,
				Message:      "no such network",
			}
		},
	}

	w := deleteRequest(newTestHandler(engine).Delete, "ghost-net", "")
	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	envelope := testutils.DecodeErrorEnvelope(t, resp)
	assert.Equal(t, "NETWORK_NOT_FOUND", envelope.Error.Code)
}

// TestNetworkHandler_Delete_InUseRefusal pins the in-use branch through
// the HTTP surface: 409 NETWORK_IN_USE carrying podman's refusal text.
// The error shape is podman's real one — the compat handler answers a
// 500 whose message ends in the ErrNetworkInUse text, not a 409 — so
// this exercises the conflict phrase-match end to end, not just the
// response-code shortcut.
func TestNetworkHandler_Delete_InUseRefusal(t *testing.T) {
	const refusal = `"frontend" has associated containers with it. Use -f to forcibly delete containers and pods: network is being used`
	engine := &fakeNetworkEngine{
		removeFn: func(string, bool) error {
			return &errorhandling.ErrorModel{
				ResponseCode: http.StatusInternalServerError,
				Message:      refusal,
			}
		},
	}

	w := deleteRequest(newTestHandler(engine).Delete, "frontend", "")
	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusConflict, resp.StatusCode)
	envelope := testutils.DecodeErrorEnvelope(t, resp)
	assert.Equal(t, "NETWORK_IN_USE", envelope.Error.Code)
	assert.Contains(t, envelope.Error.Message, refusal,
		"podman's refusal text must reach the client")
}

// TestNetworkHandler_Delete_MissingName pins the route-param guard: an
// empty networkName answers 400 MISSING_NETWORK_NAME before any engine
// call (removeFn stays unarmed, so a call would panic).
func TestNetworkHandler_Delete_MissingName(t *testing.T) {
	engine := &fakeNetworkEngine{}

	w := deleteRequest(newTestHandler(engine).Delete, "", "")
	resp := w.Result()
	defer resp.Body.Close()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "MISSING_NETWORK_NAME", testutils.DecodeErrorEnvelope(t, resp).Error.Code)
}
