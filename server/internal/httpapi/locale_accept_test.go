package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestAcceptLocaleWeights: the best-weighted English or Japanese tag decides,
// tags in other languages are skipped, and no usable tag gives Japanese.
func TestAcceptLocaleWeights(t *testing.T) {
	cases := map[string]string{
		"fr-FR,fr;q=0.9,en;q=0.8": "en",
		"ja;q=0.1,en;q=0.9":       "en",
		"de":                      "ja",
		"":                        "ja",
		"en-US,ja;q=0.5":          "en",
		"ja-JP,en;q=0.5":          "ja",
		"de,en;q=0.5":             "en",
		"*":                       "ja",
	}
	for header, want := range cases {
		if got := acceptLocale(header); got != want {
			t.Errorf("acceptLocale(%q) = %q, want %q", header, got, want)
		}
	}
}

// TestWithLocaleQueryWins: the lang query parameter overrides Accept-Language,
// and a lang value other than en or ja is ignored.
func TestWithLocaleQueryWins(t *testing.T) {
	seen := ""
	h := withLocale(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = localeOf(r.Context())
	}))
	cases := []struct{ url, accept, want string }{
		{"/x?lang=ja", "en", "ja"},
		{"/x?lang=en", "de", "en"},
		{"/x?lang=fr", "en", "en"},
		{"/x", "fr-FR,fr;q=0.9,en;q=0.8", "en"},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, c.url, nil)
		if c.accept != "" {
			req.Header.Set("Accept-Language", c.accept)
		}
		h.ServeHTTP(httptest.NewRecorder(), req)
		if seen != c.want {
			t.Errorf("%s with %q: locale %q, want %q", c.url, c.accept, seen, c.want)
		}
	}
}
