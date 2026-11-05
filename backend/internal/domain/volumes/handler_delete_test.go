package volumes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
)

// deleteRequest serves DELETE
// /api/hosts/{id}/volumes/{volumeName}{query} through the handler
// with an authenticated test identity.
func deleteRequest(handler http.HandlerFunc, volumeName, query string) *httptest.ResponseRecorder {
	return testutils.Perform(handler, testutils.Request{
		Method:    http.MethodDelete,
		Path:      "/api/hosts/test-host/volumes/" + volumeName + query,
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-host", "volumeName": volumeName},
	})
}

// TestVolumeHandler_Delete_Success pins the delete contract: 204 with an
// empty body, and the force query flag forwarded to the host parsed
// (default false).
func TestVolumeHandler_Delete_Success(t *testing.T) {
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
			engine := &fakeVolumeEngine{
				removeFn: func(name string, force bool) error {
					gotName = name
					gotForce = force
					return nil
				},
			}

			w := deleteRequest(newTestHandler(engine).Delete, "my-volume", tc.query)

			assert.Equal(t, 204, w.Code, "delete of an unused volume should return 204")
			assert.Empty(t, w.Body.String(), "204 response must have an empty body")
			assert.Equal(t, "my-volume", gotName)
			assert.Equal(t, tc.wantForce, gotForce, "force flag must reach the host parsed")
		})
	}
}

// TestVolumeHandler_Delete_NonBooleanForceIsIgnored pins force-flag
// parsing: a non-boolean force value is ignored (deletes as unforced),
// the same lenient contract every domain's delete shares via
// web.QueryBool.
func TestVolumeHandler_Delete_NonBooleanForceIsIgnored(t *testing.T) {
	var gotForce bool
	engine := &fakeVolumeEngine{
		removeFn: func(name string, force bool) error {
			gotForce = force
			return nil
		},
	}

	w := deleteRequest(newTestHandler(engine).Delete, "my-volume", "?force=yes")

	assert.Equal(t, 204, w.Code, "a non-boolean force deletes as unforced, not as a client error")
	assert.False(t, gotForce, "the non-boolean force must reach the host as false")
}
