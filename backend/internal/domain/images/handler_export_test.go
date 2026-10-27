package images

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
)

// exportRequest serves GET
// /api/machines/{id}/images/{imageId}/export through the handler with an
// authenticated test identity.
func exportRequest(export http.HandlerFunc, machineID, imageID string) *httptest.ResponseRecorder {
	return testutils.Perform(export, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/machines/" + machineID + "/images/" + imageID + "/export",
		UserID:    "test-user",
		URLParams: map[string]string{"id": machineID, "imageId": imageID},
	})
}

// exportEngine arms the fake engine for an export of an existing image:
// the exists check answers true, export streams the given bytes.
func exportEngine() *fakeImageEngine {
	return &fakeImageEngine{
		existsFn: func(string) (bool, error) {
			return true, nil
		},
		exportFn: func(_ string, w io.Writer) error {
			_, err := w.Write([]byte("tar-bytes"))
			return err
		},
	}
}

// TestImageHandler_Export_Success pins the export contract: 200 with tar
// headers naming the short image ID (any sha256: prefix stripped,
// truncated to 12 hex characters, .tar suffix), and the streamed bytes
// passed through unbuffered to the response.
func TestImageHandler_Export_Success(t *testing.T) {
	tests := []struct {
		name     string
		imageID  string
		filename string
	}{
		{
			name:     "sha256-prefixed ID is stripped and truncated",
			imageID:  "sha256:abc123def4567890abc123def4567890abc123def4567890abc123def4567890",
			filename: "abc123def456.tar",
		},
		{
			name:     "bare ID is truncated only",
			imageID:  "fedcba0987654321fedcba0987654321",
			filename: "fedcba098765.tar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := exportRequest(newTestHandler(exportEngine()).Export, "test-machine", tt.imageID)

			assert.Equal(t, 200, w.Code)
			assert.Equal(t, "application/x-tar", w.Header().Get("Content-Type"))
			assert.Equal(t, `attachment; filename="`+tt.filename+`"`, w.Header().Get("Content-Disposition"))
			assert.Equal(t, "tar-bytes", w.Body.String(), "streamed bytes must reach the response")
		})
	}
}

// TestImageHandler_Export_NotFound pins that an unknown image ID gets the
// standard 404 envelope — no tar headers may leak into it, and no byte
// of a partial download is written.
func TestImageHandler_Export_NotFound(t *testing.T) {
	engine := &fakeImageEngine{
		existsFn: func(string) (bool, error) {
			return false, nil
		},
		// exportFn stays unarmed: it must never be reached for a
		// missing image.
	}

	w := exportRequest(newTestHandler(engine).Export, "test-machine", "sha256:no-such-image")

	assert.Equal(t, 404, w.Code)
	assert.Contains(t, w.Body.String(), "IMAGE_NOT_FOUND")
	assert.Contains(t, w.Body.String(), "requestId")
	assert.NotEqual(t, "application/x-tar", w.Header().Get("Content-Type"),
		"error response must not keep the tar content type")
	assert.Empty(t, w.Header().Get("Content-Disposition"),
		"error response must not offer the error JSON as a .tar download")
}
