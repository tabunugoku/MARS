package httpapi

import (
	"testing"
	"time"
)

// waitLibraryGone waits until the library row is gone: deleting a library
// finishes in the background (see library.Service.QueueDelete).
func (e *env) waitLibraryGone(id string) {
	e.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var n int
		if err := e.db.QueryRow(`SELECT count(*) FROM libraries WHERE id = ?`, id).Scan(&n); err != nil {
			e.t.Fatal(err)
		}
		if n == 0 {
			return
		}
		if time.Now().After(deadline) {
			e.t.Fatalf("library %s still in the database", id)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Deleting a library answers 204 and hides it at once, for reads, scans and
// edits alike. Its books and series go in the background, and the other
// library keeps its books.
func TestDeleteLibraryHidesItAtOnce(t *testing.T) {
	f := newListFixture(t)
	c := f.e.admin
	gone := f.libB.Id

	count := func(query string, args ...any) int {
		t.Helper()
		var n int
		if err := f.e.db.QueryRow(query, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(`SELECT count(*) FROM books WHERE library_id = ?`, gone); n != 2 {
		t.Fatalf("setup: library B has %d books, want 2", n)
	}

	c.json("DELETE", "/api/v1/libraries/"+gone, nil, 204, nil)

	var libs []Library
	c.json("GET", "/api/v1/libraries", nil, 200, &libs)
	if len(libs) != 1 || libs[0].Id != f.libA.Id {
		t.Errorf("library list after the delete: %+v, want only A", libs)
	}
	c.json("GET", "/api/v1/libraries/"+gone, nil, 404, nil)
	c.json("PATCH", "/api/v1/libraries/"+gone, map[string]any{"name": "renamed"}, 404, nil)
	c.json("POST", "/api/v1/libraries/"+gone+"/scan", nil, 404, nil)
	c.json("DELETE", "/api/v1/libraries/"+gone, nil, 404, nil)

	var series []Series
	c.json("GET", "/api/v1/series", nil, 200, &series)
	for _, s := range series {
		if s.LibraryId == gone {
			t.Errorf("series %q of the deleting library is still listed", s.Title)
		}
	}

	f.e.waitLibraryGone(gone)
	if n := count(`SELECT count(*) FROM books WHERE library_id = ?`, gone); n != 0 {
		t.Errorf("books of the deleted library left: %d", n)
	}
	if n := count(`SELECT count(*) FROM series WHERE library_id = ?`, gone); n != 0 {
		t.Errorf("series of the deleted library left: %d", n)
	}
	if n := count(`SELECT count(*) FROM search_rows WHERE library_id = ?`, gone); n != 0 {
		t.Errorf("search rows of the deleted library left: %d", n)
	}
	if n := count(`SELECT count(*) FROM books WHERE library_id = ?`, f.libA.Id); n != 4 {
		t.Errorf("books of library A = %d, want 4", n)
	}
	c.json("GET", "/api/v1/series", nil, 200, &series)
	if len(series) != 2 {
		t.Errorf("series after the delete: %v, want Alpha and Bravo", titles(series...))
	}
}
