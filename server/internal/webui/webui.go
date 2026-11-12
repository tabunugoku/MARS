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

// contentTypes are set explicitly rather than left to the mime package: Go's
// built-in table has no .webmanifest, the distroless image has no
// /etc/mime.types, and on Windows the registry can map .js to text/plain,
// which makes the browser refuse the service worker.
var contentTypes = map[string]string{
	".html":        "text/html; charset=utf-8",
	".js":          "text/javascript; charset=utf-8",
	".mjs":         "text/javascript; charset=utf-8",
	".css":         "text/css; charset=utf-8",
	".webmanifest": "application/manifest+json",
	".json":        "application/json",
	".svg":         "image/svg+xml",
	".png":         "image/png",
	".woff2":       "font/woff2",
}

// Handler serves static files and falls back to index.html for client-side
// routes, so a reload on /series/123 still loads the app. Hashed assets are
// cached for a year; everything else (index.html, the manifest, the service
// worker) is always revalidated.
func Handler() http.Handler {
	root, _ := fs.Sub(dist, "dist")
	return handler(root)
}

func handler(root fs.FS) http.Handler {
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
		if ct, ok := contentTypes[path.Ext(name)]; ok {
			w.Header().Set("Content-Type", ct)
		}
		if name == "index.html" {
			http.ServeFileFS(w, r, root, "index.html")
			return
		}
		files.ServeHTTP(w, r)
	})
}
