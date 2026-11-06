package testutils

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Homerus25/podland/backend/internal/bulkremove"
	"github.com/Homerus25/podland/backend/internal/contextutils"
	"github.com/stretchr/testify/require"
)

// The bulk-remove handler-test kit: the request builder, the report
// decoder, and the as-user request the seven domains' bulk-remove
// handler tests share. The per-domain removeRequest/bulkRemoveRequest,
// decodeRemoveReport, and removeRequestAsUser copies this replaces had
// already diverged in naming, which is exactly the drift the
// cross-domain duplication ruling says triggers extraction (the
// Perform precedent). The gated-router mounting stays per-domain, the
// pruneAccessRouter precedent: it pins each domain's real app.go wiring.

// BulkRemoveRequest serves POST /api/hosts/{id}/<domain>/remove through
// the handler with an authenticated test identity and the given JSON
// body.
func BulkRemoveRequest(handler http.HandlerFunc, domain, body string) *httptest.ResponseRecorder {
	return Perform(handler, Request{
		Method:    http.MethodPost,
		Path:      "/api/hosts/test-host/" + domain + "/remove",
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-host"},
		Body:      strings.NewReader(body),
	})
}

// DecodeBulkRemoveReport decodes the bulk-remove report array.
func DecodeBulkRemoveReport(t *testing.T, w *httptest.ResponseRecorder) []bulkremove.Entry {
	t.Helper()
	var got []bulkremove.Entry
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got), "response must decode as a report array")
	require.NotNil(t, got, "the report must be an array, never null")
	return got
}

// BulkRemoveRequestAsUser serves POST /api/hosts/host-1/<domain>/remove
// through the mounted router as the given user.
func BulkRemoveRequestAsUser(router http.Handler, domain, userID, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/hosts/host-1/"+domain+"/remove", strings.NewReader(body))
	req = req.WithContext(contextutils.SetUserID(req.Context(), userID))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}
