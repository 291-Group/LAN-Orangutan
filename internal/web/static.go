package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

// staticHandler serves the embedded assets with a content based ETag.
//
// The previous handler served these files with no ETag and no Cache-Control,
// and embedded files all carry a zero modification time, so browsers cached
// them heuristically and kept showing stale CSS and JS after an update. Here
// each file gets an ETag derived from its own contents: the browser revalidates
// on each load and receives a small 304 when nothing changed, or the fresh file
// the moment it does. On a local network that revalidation costs nothing.
type staticHandler struct {
	fsys  fs.FS
	etags map[string]string
}

// staticSubFS returns the embedded static assets rooted at their directory.
func staticSubFS() (fs.FS, error) {
	return fs.Sub(staticFS, "static")
}

// newStaticHandler indexes the content hash of every embedded asset once, at
// startup, so requests do not have to hash anything.
func newStaticHandler(fsys fs.FS) *staticHandler {
	h := &staticHandler{fsys: fsys, etags: make(map[string]string)}

	fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return nil
		}
		sum := sha256.Sum256(data)
		// Eight bytes of the hash is ample to detect a change, and keeps the
		// header short.
		h.etags[p] = `"` + hex.EncodeToString(sum[:8]) + `"`
		return nil
	})

	return h
}

// ServeHTTP serves a single asset. The request path is expected to begin with
// "/static/".
func (h *staticHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := path.Clean(strings.TrimPrefix(r.URL.Path, "/static/"))
	if name == "." || strings.HasPrefix(name, "..") {
		http.NotFound(w, r)
		return
	}

	file, err := h.fsys.Open(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}

	if etag, ok := h.etags[name]; ok {
		w.Header().Set("Etag", etag)
		// Cache, but revalidate against the ETag every load so an update is
		// never missed.
		w.Header().Set("Cache-Control", "no-cache")
	}

	// http.ServeContent handles Range and, because an ETag is set above,
	// If-None-Match: it replies 304 when the browser's copy is current. Files
	// from an embed.FS are seekable; the buffer is only a fallback.
	if rs, ok := file.(io.ReadSeeker); ok {
		http.ServeContent(w, r, info.Name(), time.Time{}, rs)
		return
	}
	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "could not read asset", http.StatusInternalServerError)
		return
	}
	http.ServeContent(w, r, info.Name(), time.Time{}, bytes.NewReader(data))
}
