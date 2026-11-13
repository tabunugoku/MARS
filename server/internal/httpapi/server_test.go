package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/tabunugoku/mars/server/internal/db"
)

func TestHealth(t *testing.T) {
	d, err := db.Open(context.Background(), filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	h := New(Deps{DB: d, Version: "test"})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var body Health
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != Ok || body.Version != "test" {
		t.Fatalf("body %+v", body)
	}
}
