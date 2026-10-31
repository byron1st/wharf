package server

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// spa serves the files of static, and index.html for any path that is not a
// file, so a reload on a client-side route such as /settings/general loads
// the web app. Paths under /api are not the web app's and answer a JSON 404.
func spa(static fs.FS) http.HandlerFunc {
	files := http.FileServerFS(static)

	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "server: no such api route"})
			return
		}

		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if info, err := fs.Stat(static, name); err != nil || info.IsDir() {
			if _, err := fs.Stat(static, "index.html"); err != nil {
				http.Error(w, "The web app is not built into this binary. Run make build.", http.StatusNotFound)
				return
			}

			http.ServeFileFS(w, r, static, "index.html")

			return
		}

		files.ServeHTTP(w, r)
	}
}
