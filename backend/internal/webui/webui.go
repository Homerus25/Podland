// Package webui serves the compiled frontend SPA from files embedded in
// the backend binary, replacing the Caddy frontend container's delivery
// role. The embed directory (dist/) holds a committed placeholder
// index.html so the backend compiles, tests, and serves standalone even
// when the frontend has never been built; `make ui` in the backend drops
// the real Vite bundle into the same directory.
//
// The handler reproduces the Caddy container's `try_files {path}
// /index.html` contract: any path that is not a real file in the bundle
// — route-shaped (/machines/xyz) and file-shaped (/foo.png) misses alike
// — answers index.html with HTTP 200, so client-side routing keeps
// working on deep links and refreshes. Cache headers follow the bundle's
// layout: index.html is revalidated on every load (no-cache), while
// Vite's content-hashed files under assets/ are immutable and cacheable
// for a year.
package webui

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

//go:embed all:dist
var distFS embed.FS

// Cache-Control vocabulary, keyed off the bundle layout: index.html is
// revalidated on every load, hashed assets never change and may live in
// caches forever.
const (
	indexCacheControl   = "no-cache"
	assetCacheControl   = "public, max-age=31536000, immutable"
	indexFile           = "index.html"
	assetsDir           = "assets/"
	methodNotAllowedMsg = "method not allowed"
)

// Handler returns the SPA handler over the embedded frontend build. This
// is the entry point the router mounts under its catch-all, after every
// /api route.
func Handler() http.Handler {
	files, err := fs.Sub(distFS, "dist")
	if err != nil {
		// Unreachable: "dist" is a fixed valid path and the go:embed
		// directive fails the build when the directory is missing.
		panic("webui: open embedded dist: " + err.Error())
	}
	return New(files)
}

// New builds the SPA handler over a filesystem already rooted at the
// bundle (index.html at the root, assets/ beneath). Handler is the
// production entry; New exists so tests can serve a synthetic bundle.
func New(files fs.FS) http.Handler {
	return spaHandler{files: files}
}

type spaHandler struct {
	files fs.FS
}

func (h spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, methodNotAllowedMsg, http.StatusMethodNotAllowed)
		return
	}

	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if file, ok := h.lookup(name); ok {
		h.serveFile(w, r, file, cacheControlFor(name))
		return
	}
	h.serveIndex(w, r)
}

// lookup opens the named bundle file, treating misses and directory hits
// (both never serveable) the same way: not found.
func (h spaHandler) lookup(name string) (fs.File, bool) {
	if name == "" {
		return nil, false
	}
	file, err := h.files.Open(name)
	if err != nil {
		return nil, false
	}
	stat, err := file.Stat()
	if err != nil || stat.IsDir() {
		_ = file.Close()
		return nil, false
	}
	return file, true
}

// serveIndex answers every non-file path with the SPA shell — the
// try_files fallback — always under the no-cache header.
func (h spaHandler) serveIndex(w http.ResponseWriter, r *http.Request) {
	file, ok := h.lookup(indexFile)
	if !ok {
		http.Error(w, "internal server error: "+indexFile+" missing from the embedded UI",
			http.StatusInternalServerError)
		return
	}
	h.serveFile(w, r, file, indexCacheControl)
}

func (h spaHandler) serveFile(w http.ResponseWriter, r *http.Request, file fs.File, cacheControl string) {
	defer func() { _ = file.Close() }()

	stat, err := file.Stat()
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if cacheControl != "" {
		w.Header().Set("Cache-Control", cacheControl)
	}
	// ServeContent owns Content-Type (from the file extension), ETag-less
	// conditional handling, ranges, and HEAD. The zero modtime suppresses
	// Last-Modified: the cache headers above are the whole freshness
	// story. Bundle files (embed, fstest) implement io.Seeker, which
	// ServeContent requires.
	seeker, ok := file.(io.ReadSeeker)
	if !ok {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	http.ServeContent(w, r, stat.Name(), time.Time{}, seeker)
}

// cacheControlFor maps a bundle path onto its Cache-Control; bundle files
// outside index.html and assets/ carry no header, matching the Caddy
// container's file_server behaviour for unhashed extras (favicon et al).
func cacheControlFor(name string) string {
	switch {
	case name == indexFile:
		return indexCacheControl
	case strings.HasPrefix(name, assetsDir):
		return assetCacheControl
	default:
		return ""
	}
}
