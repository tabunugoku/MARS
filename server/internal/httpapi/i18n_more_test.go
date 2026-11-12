package httpapi

import "testing"

// TestBadQueryParameterIsJSON: a query parameter that does not parse gets the
// same JSON error as any other bad request, in the request's language.
func TestBadQueryParameterIsJSON(t *testing.T) {
	e := newEnv(t)
	ja := e.admin.do("GET", "/api/v1/series?limit=oops", nil)
	if ja.Code != 400 {
		t.Fatalf("status %d, want 400", ja.Code)
	}
	jaErr := errorBody(t, ja.Body.Bytes())
	if jaErr.Code != "bad_request.malformed" || jaErr.Message != "リクエストの形式が正しくありません" {
		t.Fatalf("ja error: %+v", jaErr)
	}

	en := e.admin.do("GET", "/api/v1/series?limit=oops", nil, "Accept-Language", "en")
	if en.Code != 400 {
		t.Fatalf("en status %d, want 400", en.Code)
	}
	enErr := errorBody(t, en.Body.Bytes())
	if enErr.Code != jaErr.Code || enErr.Message != "The request is malformed" {
		t.Fatalf("en error: %+v", enErr)
	}
}

// TestLangQueryOverridesAcceptLanguage: lang=en|ja in the URL wins over
// Accept-Language (EventSource cannot set headers); any other value is ignored.
func TestLangQueryOverridesAcceptLanguage(t *testing.T) {
	e := newEnv(t)
	cases := []struct {
		url, accept, want string
	}{
		{"/api/v1/libraries/nope?lang=en", "ja", "Library not found"},
		{"/api/v1/libraries/nope?lang=ja", "en", "ライブラリが見つかりません"},
		{"/api/v1/libraries/nope?lang=fr", "en", "Library not found"},
		{"/api/v1/libraries/nope?lang=fr", "ja", "ライブラリが見つかりません"},
		{"/api/v1/libraries/nope", "en", "Library not found"},
	}
	for _, c := range cases {
		rec := e.admin.do("GET", c.url, nil, "Accept-Language", c.accept)
		if rec.Code != 404 {
			t.Fatalf("%s: status %d", c.url, rec.Code)
		}
		if got := errorBody(t, rec.Body.Bytes()).Message; got != c.want {
			t.Fatalf("%s with Accept-Language %q: message %q, want %q", c.url, c.accept, got, c.want)
		}
	}
}
