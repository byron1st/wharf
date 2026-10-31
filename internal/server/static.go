package server

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// spa serves the files of static, and index.html for any extension-less path
// that is not a file, so a reload on a client-side route such as
// /settings/general loads the web app. A missing file with an extension, such
// as a hashed asset from an older build, is a 404. Paths under /api are not
// the web app's and answer a JSON 404.
//
// Every response forbids framing, since a framed page sends same-origin
// requests that the local origin guard lets through, and forbids MIME
// sniffing. The embedded files have no modification time, so hashed assets
// are cached for good and index.html is revalidated every time.
func spa(static fs.FS) http.HandlerFunc {
	files := http.FileServerFS(static)

	return func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "frame-ancestors 'none'")
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")

		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "server: no such api route"})
			return
		}

		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if info, err := fs.Stat(static, name); err == nil && !info.IsDir() {
			if strings.HasPrefix(name, "assets/") {
				h.Set("Cache-Control", "public, max-age=31536000, immutable")
			}

			files.ServeHTTP(w, r)

			return
		}

		if path.Ext(name) != "" {
			http.NotFound(w, r)
			return
		}

		if _, err := fs.Stat(static, "index.html"); err != nil {
			http.Error(w, "The web app is not built into this binary. Run make build.", http.StatusNotFound)
			return
		}

		h.Set("Cache-Control", "no-cache")
		http.ServeFileFS(w, r, static, "index.html")
	}
}
