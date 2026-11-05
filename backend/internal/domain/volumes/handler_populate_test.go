package volumes

import (
	"archive/tar"
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// buildMultipartBody assembles a multipart/form-data body with the
// given form-name → filename/content parts and its content type.
func buildMultipartBody(t *testing.T, parts map[string]map[string][]byte) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	for formName, files := range parts {
		for filename, content := range files {
			fw, err := mw.CreateFormFile(formName, filename)
			require.NoError(t, err)
			_, err = fw.Write(content)
			require.NoError(t, err)
		}
	}
	require.NoError(t, mw.Close())
	return body, mw.FormDataContentType()
}

// recordPopulateRequest serves POST
// /api/hosts/{id}/volumes/{volumeName}/populate through the handler
// with an authenticated identity and the given body.
func recordPopulateRequest(handler http.HandlerFunc, hostID, volumeName string, body *bytes.Buffer, contentType string) *httptest.ResponseRecorder {
	return testutils.Perform(handler, testutils.Request{
		Method:    http.MethodPost,
		Path:      "/api/hosts/" + hostID + "/volumes/" + volumeName + "/populate",
		UserID:    "test-user",
		URLParams: map[string]string{"id": hostID, "volumeName": volumeName},
		Body:      body,
		Header:    http.Header{"Content-Type": []string{contentType}},
	})
}

// readTarEntries untars the reader handed to the fake engine.
func readTarEntries(t *testing.T, r io.Reader) map[string][]byte {
	t.Helper()
	tr := tar.NewReader(r)
	out := map[string][]byte{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		if hdr.Typeflag == tar.TypeReg {
			data, err := io.ReadAll(tr)
			require.NoError(t, err)
			out[hdr.Name] = data
		}
	}
	return out
}

// TestVolumeHandler_Populate_FilesShape pins the files contract: the
// spooled parts land in the assembled tar with their relative paths
// preserved, and the host's import receives it as the source.
func TestVolumeHandler_Populate_FilesShape(t *testing.T) {
	body, contentType := buildMultipartBody(t, map[string]map[string][]byte{
		"files": {
			"docs/readme.txt": []byte("hello"),
			"data.bin":        {0x00, 0x01},
		},
	})

	var entries map[string][]byte
	engine := &fakeVolumeEngine{
		inspectFn: func(string) (*entities.VolumeInspectReport, error) {
			return inspectFixture("my-volume"), nil
		},
		importFn: func(_ string, r io.Reader) error {
			// Read inside the callback: temp files are closed once the
			// handler returns.
			entries = readTarEntries(t, r)
			return nil
		},
	}

	w := recordPopulateRequest(newTestHandler(engine).Populate, "test-host", "my-volume", body, contentType)

	assert.Equal(t, 204, w.Code, "populate should return 204 on success")
	require.NotNil(t, entries, "the host must receive the assembled tar")
	require.Len(t, entries, 2, "each uploaded file must be a tar entry")
	assert.Equal(t, []byte("hello"), entries["docs/readme.txt"], "relative paths must be preserved")
	assert.Equal(t, []byte{0x00, 0x01}, entries["data.bin"])
}

// TestVolumeHandler_Populate_ArchiveShape pins the archive contract: a
// single archive part is imported as-is, byte-identical.
func TestVolumeHandler_Populate_ArchiveShape(t *testing.T) {
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	require.NoError(t, tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: "in/archive.txt", Size: 2}))
	_, err := tw.Write([]byte("ok"))
	require.NoError(t, err)
	require.NoError(t, tw.Close())

	body, contentType := buildMultipartBody(t, map[string]map[string][]byte{
		"archive": {"seed.tar": tarBuf.Bytes()},
	})

	var got []byte
	engine := &fakeVolumeEngine{
		inspectFn: func(string) (*entities.VolumeInspectReport, error) {
			return inspectFixture("my-volume"), nil
		},
		importFn: func(_ string, r io.Reader) error {
			// Read inside the callback: temp files are closed once the
			// handler returns.
			data, err := io.ReadAll(r)
			require.NoError(t, err)
			got = data
			return nil
		},
	}

	w := recordPopulateRequest(newTestHandler(engine).Populate, "test-host", "my-volume", body, contentType)

	assert.Equal(t, 204, w.Code)
	assert.Equal(t, tarBuf.Bytes(), got, "archive must be imported as-is")
}

// TestVolumeHandler_Populate_ShapeValidation pins the 400s: both shapes
// at once, an empty upload, and a non-multipart body.
func TestVolumeHandler_Populate_ShapeValidation(t *testing.T) {
	handler := newTestHandler(&fakeVolumeEngine{})

	t.Run("BothShapesGets400", func(t *testing.T) {
		body, contentType := buildMultipartBody(t, map[string]map[string][]byte{
			"files":   {"a.txt": []byte("x")},
			"archive": {"seed.tar": []byte("tar")},
		})
		w := recordPopulateRequest(handler.Populate, "test-host", "my-volume", body, contentType)
		assert.Equal(t, 400, w.Code)
		assert.Contains(t, w.Body.String(), "INVALID_REQUEST")
		assert.NotContains(t, w.Body.String(), "requestId\")", "")
	})

	t.Run("EmptyUploadGets400", func(t *testing.T) {
		body, contentType := buildMultipartBody(t, map[string]map[string][]byte{})
		w := recordPopulateRequest(handler.Populate, "test-host", "my-volume", body, contentType)
		assert.Equal(t, 400, w.Code)
		assert.Contains(t, w.Body.String(), "INVALID_REQUEST")
	})

	t.Run("NonMultipartGets400", func(t *testing.T) {
		w := recordPopulateRequest(handler.Populate, "test-host", "my-volume",
			bytes.NewBufferString("not multipart"), "text/plain")
		assert.Equal(t, 400, w.Code)
	})
}

// TestVolumeHandler_Populate_TraversalRejected pins that ".." traversal
// in part filenames is rejected. Leading slashes are normalized into the
// volume root rather than rejected (they cannot escape), covered below.
func TestVolumeHandler_Populate_TraversalRejected(t *testing.T) {
	handler := newTestHandler(&fakeVolumeEngine{})

	for _, name := range []string{"../escape.txt", "../../etc/passwd", "a/../../up.txt"} {
		body, contentType := buildMultipartBody(t, map[string]map[string][]byte{
			"files": {name: []byte("x")},
		})
		w := recordPopulateRequest(handler.Populate, "test-host", "my-volume", body, contentType)
		assert.Equal(t, 400, w.Code, "path %q must be rejected", name)
	}
}

// TestVolumeHandler_Populate_CapEnforced pins the cap: a small
// handler-local cap turns an oversized upload into 413
// VOLUME_UPLOAD_TOO_LARGE, and the service is never called.
func TestVolumeHandler_Populate_CapEnforced(t *testing.T) {
	handler := newTestHandler(&fakeVolumeEngine{}).WithMaxUploadBytes(16)

	body, contentType := buildMultipartBody(t, map[string]map[string][]byte{
		"files": {"big.bin": bytes.Repeat([]byte{0xAA}, 64)},
	})
	w := recordPopulateRequest(handler.Populate, "test-host", "my-volume", body, contentType)

	assert.Equal(t, 413, w.Code, "oversized uploads must yield 413")
	assert.Contains(t, w.Body.String(), "VOLUME_UPLOAD_TOO_LARGE")
}

// TestVolumeHandler_Populate_NotFound pins that an unknown volume gets
// the standard 404.
func TestVolumeHandler_Populate_NotFound(t *testing.T) {
	body, contentType := buildMultipartBody(t, map[string]map[string][]byte{
		"files": {"a.txt": []byte("x")},
	})
	engine := &fakeVolumeEngine{
		inspectFn: func(string) (*entities.VolumeInspectReport, error) {
			return nil, notFoundErr()
		},
		// importFn stays unarmed: the archive must not be consumed for
		// a missing volume.
	}

	w := recordPopulateRequest(newTestHandler(engine).Populate, "test-host", "no-such-volume", body, contentType)

	assert.Equal(t, 404, w.Code)
	assert.Contains(t, w.Body.String(), "VOLUME_NOT_FOUND")
}
