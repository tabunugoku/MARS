// Package webui serves the built React app embedded in the binary.
// `npm run build:embed` in web/ copies web/dist here before `go build`.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var dist embed.FS

// Handler serves static files and falls back to index.html for client-side
// routes, so a reload on /series/123 still loads the app. Hashed assets are
// cached for a year; index.html is always revalidated.
func Handler() http.Handler {
	root, _ := fs.Sub(dist, "dist")
	if _, err := fs.Stat(root, "index.html"); err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "ComiNavi: this build has no web UI. Run `npm run build:embed` in web/ and rebuild.", http.StatusNotFound)
		})
	}
	files := http.FileServerFS(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}
		if _, err := fs.Stat(root, name); err != nil {
			// Unknown path without an extension: a client route.
			if path.Ext(name) != "" {
				http.NotFound(w, r)
				return
			}
			name = "index.html"
		}
		if strings.HasPrefix(name, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		if name == "index.html" {
			http.ServeFileFS(w, r, root, "index.html")
			return
		}
		files.ServeHTTP(w, r)
	})
}
