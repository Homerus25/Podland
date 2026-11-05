// Package webui_test verifies the SPA-serving surface the router mounts
// under the catch-all: embedded files with the Caddy try_files contract
// (unknown paths fall back to index.html) and the Vite cache-header
// vocabulary (no-cache index, immutable hashed assets).
package webui_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Homerus25/podland/backend/internal/webui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const indexBody = "<!doctype html><html><body>podland-spa</body></html>"

// testBundle mirrors a Vite build as New sees it: already rooted at the
// bundle (index.html at the root, a content-hashed bundle under assets/,
// plus an unhashed extra file). The dist/ prefix itself is Handler's
// concern and is exercised by the embedded-handler test below.
func testBundle() fstest.MapFS {
	return fstest.MapFS{
		"index.html":           &fstest.MapFile{Data: []byte(indexBody)},
		"assets/app-BQ4xzL.js": &fstest.MapFile{Data: []byte("console.log('podland')\n")},
		"favicon.svg":          &fstest.MapFile{Data: []byte("<svg/>")},
	}
}

func perform(t *testing.T, handler http.Handler, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

func TestWebUI_ServesIndexAtRoot(t *testing.T) {
	rec := perform(t, webui.New(testBundle()), http.MethodGet, "/")

	require.Equal(t, http.StatusOK, rec.Code)
	body, err := io.ReadAll(rec.Body)
	require.NoError(t, err)
	assert.Equal(t, indexBody, string(body), "/ must serve index.html")
	assert.Equal(t, "text/html; charset=utf-8", rec.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache", rec.Header().Get("Cache-Control"),
		"index.html must never be cached stale — revalidate on every load")
}

func TestWebUI_ServesHashedAssetsImmutable(t *testing.T) {
	rec := perform(t, webui.New(testBundle()), http.MethodGet, "/assets/app-BQ4xzL.js")

	require.Equal(t, http.StatusOK, rec.Code)
	body, err := io.ReadAll(rec.Body)
	require.NoError(t, err)
	assert.Equal(t, "console.log('podland')\n", string(body))
	assert.Equal(t, "public, max-age=31536000, immutable", rec.Header().Get("Cache-Control"),
		"content-hashed assets may be cached forever")
	assert.Contains(t, rec.Header().Get("Content-Type"), "javascript")
}

func TestWebUI_FallsBackOnRouteShapedMiss(t *testing.T) {
	rec := perform(t, webui.New(testBundle()), http.MethodGet, "/hosts/xyz")

	require.Equal(t, http.StatusOK, rec.Code,
		"client-side routes must get the SPA, not a 404")
	body, err := io.ReadAll(rec.Body)
	require.NoError(t, err)
	assert.Equal(t, indexBody, string(body))
	assert.Equal(t, "no-cache", rec.Header().Get("Cache-Control"))
}

func TestWebUI_FallsBackOnFileShapedMiss(t *testing.T) {
	rec := perform(t, webui.New(testBundle()), http.MethodGet, "/foo.png")

	require.Equal(t, http.StatusOK, rec.Code,
		"try_files {path} /index.html: a missing file still falls back, not 404")
	body, err := io.ReadAll(rec.Body)
	require.NoError(t, err)
	assert.Equal(t, indexBody, string(body))
}

func TestWebUI_DirectIndexAndDirectoryRequests(t *testing.T) {
	rec := perform(t, webui.New(testBundle()), http.MethodGet, "/index.html")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "no-cache", rec.Header().Get("Cache-Control"))

	dir := perform(t, webui.New(testBundle()), http.MethodGet, "/assets/")
	require.Equal(t, http.StatusOK, dir.Code,
		"directory hits fall through to the SPA fallback like any other miss")
	body, err := io.ReadAll(dir.Body)
	require.NoError(t, err)
	assert.Equal(t, indexBody, string(body))
}

func TestWebUI_HeadRequestsAreServed(t *testing.T) {
	rec := perform(t, webui.New(testBundle()), http.MethodHead, "/")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "no-cache", rec.Header().Get("Cache-Control"))
	assert.Empty(t, rec.Body.String(), "HEAD must not carry a body")
}

func TestWebUI_RejectsNonGetMethods(t *testing.T) {
	rec := perform(t, webui.New(testBundle()), http.MethodPost, "/hosts/xyz")

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code,
		"a POST to a non-API path is a caller bug — refuse it instead of serving HTML")
	assert.Contains(t, rec.Header().Get("Allow"), "GET")
}

// TestWebUI_EmbeddedHandlerServesCommittedPlaceholder pins that the
// committed placeholder is actually embedded: the production Handler()
// must answer 200 with the no-cache header even when `make ui` has never
// run, so the backend compiles, tests, and serves standalone.
func TestWebUI_EmbeddedHandlerServesCommittedPlaceholder(t *testing.T) {
	rec := perform(t, webui.Handler(), http.MethodGet, "/")

	require.Equal(t, http.StatusOK, rec.Code,
		"the embedded bundle must always contain index.html")
	assert.True(t, strings.Contains(strings.ToLower(rec.Body.String()), "<!doctype html>"),
		"placeholder must be a real HTML page")
	assert.Equal(t, "text/html; charset=utf-8", rec.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache", rec.Header().Get("Cache-Control"))
}
