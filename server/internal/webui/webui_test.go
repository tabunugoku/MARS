package webui

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestHandlerContentTypes(t *testing.T) {
	root := fstest.MapFS{
		"index.html":            {Data: []byte("<!doctype html>")},
		"sw.js":                 {Data: []byte("self.addEventListener('fetch', () => {})")},
		"manifest.webmanifest":  {Data: []byte(`{"name":"ComiNavi"}`)},
		"icon-192.png":          {Data: []byte("\x89PNG\r\n\x1a\n")},
		"assets/index-abc12.js": {Data: []byte("export {}")},
	}
	h := handler(root)
	cases := []struct {
		path, contentType, cache string
		status                   int
	}{
		{"/", "text/html; charset=utf-8", "no-cache", 200},
		{"/series/123", "text/html; charset=utf-8", "no-cache", 200},
		{"/sw.js", "text/javascript; charset=utf-8", "no-cache", 200},
		{"/manifest.webmanifest", "application/manifest+json", "no-cache", 200},
		{"/icon-192.png", "image/png", "no-cache", 200},
		{"/assets/index-abc12.js", "text/javascript; charset=utf-8", "public, max-age=31536000, immutable", 200},
		{"/missing.png", "", "", 404},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, c.path, nil))
		if rec.Code != c.status {
			t.Errorf("%s: status %d, want %d", c.path, rec.Code, c.status)
			continue
		}
		if c.status != 200 {
			continue
		}
		if got := rec.Header().Get("Content-Type"); got != c.contentType {
			t.Errorf("%s: Content-Type %q, want %q", c.path, got, c.contentType)
		}
		if got := rec.Header().Get("Cache-Control"); got != c.cache {
			t.Errorf("%s: Cache-Control %q, want %q", c.path, got, c.cache)
		}
	}
}
