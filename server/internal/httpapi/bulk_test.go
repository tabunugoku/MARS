package httpapi

import (
	"slices"
	"testing"
)

// bulkReader makes a reader who sees library a only.
func bulkReader(e *env, a Library) client {
	e.admin.json("POST", "/api/v1/users", map[string]any{"name": "一括", "email": "bulk@example.com", "password": "bulk pass 1",
		"role": "user", "allLibraries": false, "libraryIds": []string{a.Id}}, 201, nil)
	return e.login("一括", "bulk pass 1")
}

func seriesNow(e *env, id string) Series {
	var s Series
	e.admin.json("GET", "/api/v1/series/"+id, nil, 200, &s)
	return s
}

func bookNow(e *env, id string) Book {
	var b Book
	e.admin.json("GET", "/api/v1/books/"+id, nil, 200, &b)
	return b
}

func TestBulkUpdateSeries(t *testing.T) {
	e, a, b := progressEnv(t)
	sa, sb := libSeries(t, e.admin, a.Id), libSeries(t, e.admin, b.Id)
	var res BulkResult

	// Readers may not bulk-edit at all.
	bulkReader(e, a).json("POST", "/api/v1/series/bulk", map[string]any{"seriesIds": []string{sa.Id}, "publisher": "x"}, 403, nil)

	// An id that cannot be resolved refuses the whole request.
	e.admin.json("POST", "/api/v1/series/bulk", map[string]any{"seriesIds": []string{sa.Id, "missing"}, "publisher": "出版社"}, 404, nil)
	if got := seriesNow(e, sa.Id); got.Publisher != "" || len(got.LockedFields) != 0 {
		t.Fatalf("404 still changed the series: %+v", got)
	}

	// A request that changes nothing, or sends a bad value, is refused.
	e.admin.json("POST", "/api/v1/series/bulk", map[string]any{"seriesIds": []string{sa.Id}}, 400, nil)
	e.admin.json("POST", "/api/v1/series/bulk", map[string]any{"seriesIds": []string{sa.Id}, "ageRating": 120}, 400, nil)
	e.admin.json("POST", "/api/v1/series/bulk", map[string]any{"seriesIds": []string{}, "publisher": "x"}, 400, nil)

	// Scalars on both libraries' series; a repeated id counts once.
	e.admin.json("POST", "/api/v1/series/bulk", map[string]any{"seriesIds": []string{sa.Id, sb.Id, sa.Id},
		"publisher": "出版社", "status": "ended", "language": "ja"}, 200, &res)
	if res.Updated != 2 {
		t.Fatalf("updated %d, want 2", res.Updated)
	}
	for _, id := range []string{sa.Id, sb.Id} {
		got := seriesNow(e, id)
		if got.Publisher != "出版社" || string(got.Status) != "ended" || got.Language != "ja" {
			t.Fatalf("series %s after scalars: %+v", id, got)
		}
		for _, f := range []string{"publisher", "status", "language"} {
			if !slices.Contains(got.LockedFields, f) {
				t.Fatalf("series %s: %s not locked: %v", id, f, got.LockedFields)
			}
		}
	}
	// The same values again change nothing.
	e.admin.json("POST", "/api/v1/series/bulk", map[string]any{"seriesIds": []string{sa.Id, sb.Id},
		"publisher": "出版社"}, 200, &res)
	if res.Updated != 0 {
		t.Fatalf("repeat updated %d, want 0", res.Updated)
	}

	// Age rating: set, then null clears it, and the lock follows.
	e.admin.json("POST", "/api/v1/series/bulk", map[string]any{"seriesIds": []string{sa.Id}, "ageRating": 12}, 200, &res)
	if got := seriesNow(e, sa.Id); got.AgeRating == nil || *got.AgeRating != 12 {
		t.Fatalf("age rating not set: %+v", got.AgeRating)
	}
	e.admin.json("POST", "/api/v1/series/bulk", map[string]any{"seriesIds": []string{sa.Id}, "ageRating": nil}, 200, &res)
	if got := seriesNow(e, sa.Id); got.AgeRating != nil || !slices.Contains(got.LockedFields, "ageRating") {
		t.Fatalf("null did not clear age rating: %+v", got)
	}

	// The invisible-to-library id of the other series was not touched by a failed call.
	e.admin.json("POST", "/api/v1/series/bulk", map[string]any{"seriesIds": []string{sb.Id, "missing"}, "publisher": "別"}, 404, nil)
	if got := seriesNow(e, sb.Id); got.Publisher != "出版社" {
		t.Fatalf("404 changed sb: %+v", got)
	}
}

func TestBulkSeriesGenresAndTags(t *testing.T) {
	e, a, _ := progressEnv(t)
	sa := libSeries(t, e.admin, a.Id)
	var res BulkResult

	e.admin.json("POST", "/api/v1/series/bulk", map[string]any{"seriesIds": []string{sa.Id},
		"addGenres": []string{" SF ", "", "SF", "ミステリー"}}, 200, &res)
	got := seriesNow(e, sa.Id)
	if !slices.Equal(got.Genres, []string{"SF", "ミステリー"}) || !slices.Contains(got.LockedFields, "genres") {
		t.Fatalf("add genres: %+v locks %v", got.Genres, got.LockedFields)
	}

	// Remove one, add another: order kept, the rest untouched.
	e.admin.json("POST", "/api/v1/series/bulk", map[string]any{"seriesIds": []string{sa.Id},
		"removeGenres": []string{"SF"}, "addGenres": []string{"ホラー", "ミステリー"}}, 200, &res)
	if got := seriesNow(e, sa.Id); !slices.Equal(got.Genres, []string{"ミステリー", "ホラー"}) {
		t.Fatalf("remove/add genres: %+v", got.Genres)
	}

	e.admin.json("POST", "/api/v1/series/bulk", map[string]any{"seriesIds": []string{sa.Id},
		"addTags": []string{"名作"}, "removeTags": []string{"無い"}}, 200, &res)
	if res.Updated != 1 {
		t.Fatalf("tags updated %d, want 1", res.Updated)
	}
	if got := seriesNow(e, sa.Id); !slices.Equal(got.Tags, []string{"名作"}) || !slices.Contains(got.LockedFields, "tags") {
		t.Fatalf("add tags: %+v locks %v", got.Tags, got.LockedFields)
	}
}

func TestBulkUpdateBooks(t *testing.T) {
	e, a, _ := progressEnv(t)
	books := seriesBooks(t, e.admin, libSeries(t, e.admin, a.Id).Id)
	ids := []string{books[0].Id, books[1].Id}
	var res BulkResult

	e.admin.json("POST", "/api/v1/books/bulk", map[string]any{"bookIds": ids, "addTags": []string{"名作", " 名作 "}}, 200, &res)
	if res.Updated != 2 {
		t.Fatalf("updated %d, want 2", res.Updated)
	}
	for _, id := range ids {
		got := bookNow(e, id)
		if !slices.Equal(got.Tags, []string{"名作"}) || !slices.Contains(got.LockedFields, "tags") {
			t.Fatalf("book %s tags %v locks %v", id, got.Tags, got.LockedFields)
		}
	}

	e.admin.json("POST", "/api/v1/books/bulk", map[string]any{"bookIds": ids}, 400, nil)
	e.admin.json("POST", "/api/v1/books/bulk", map[string]any{"bookIds": []string{ids[0], "missing"}, "addTags": []string{"x"}}, 404, nil)
	if got := bookNow(e, ids[0]); !slices.Equal(got.Tags, []string{"名作"}) {
		t.Fatalf("404 changed the book: %v", got.Tags)
	}

	e.admin.json("POST", "/api/v1/books/bulk", map[string]any{"bookIds": ids, "removeTags": []string{"名作"}}, 200, &res)
	if res.Updated != 2 || len(bookNow(e, ids[1]).Tags) != 0 {
		t.Fatalf("remove tags: updated %d", res.Updated)
	}
}

func TestBulkReadStateRoundTrip(t *testing.T) {
	e, a, b := progressEnv(t)
	sa, sb := libSeries(t, e.admin, a.Id), libSeries(t, e.admin, b.Id)
	r := bulkReader(e, a)

	// A series outside the reader's libraries refuses the whole request.
	r.json("POST", "/api/v1/series/read-state", map[string]any{"seriesIds": []string{sa.Id, sb.Id}, "read": true}, 404, nil)
	for _, bk := range seriesBooks(t, r, sa.Id) {
		if bk.Progress != nil {
			t.Fatalf("404 wrote progress: %+v", bk.Progress)
		}
	}

	r.json("POST", "/api/v1/series/read-state", map[string]any{"seriesIds": []string{sa.Id}, "read": true}, 204, nil)
	for _, bk := range seriesBooks(t, r, sa.Id) {
		if bk.Progress == nil || !bk.Progress.Completed {
			t.Fatalf("book %s not read: %+v", bk.Id, bk.Progress)
		}
	}
	// Progress is per reader: the administrator's is untouched.
	for _, bk := range seriesBooks(t, e.admin, sa.Id) {
		if bk.Progress != nil {
			t.Fatalf("admin progress changed: %+v", bk.Progress)
		}
	}
	r.json("POST", "/api/v1/series/read-state", map[string]any{"seriesIds": []string{sa.Id}, "read": false}, 204, nil)
	for _, bk := range seriesBooks(t, r, sa.Id) {
		if bk.Progress != nil {
			t.Fatalf("unread left progress: %+v", bk.Progress)
		}
	}

	books := seriesBooks(t, r, sa.Id)
	r.json("POST", "/api/v1/books/read-state", map[string]any{"bookIds": []string{books[0].Id}, "read": true}, 204, nil)
	if got := bookNow(e, books[0].Id); got.Progress != nil {
		t.Fatalf("admin read a book the reader marked: %+v", got.Progress)
	}
	r.json("POST", "/api/v1/books/read-state", map[string]any{"bookIds": []string{books[1].Id, "missing"}, "read": true}, 404, nil)
	r.json("POST", "/api/v1/books/read-state", map[string]any{"bookIds": []string{}, "read": true}, 400, nil)
	var first Book
	r.json("GET", "/api/v1/books/"+books[0].Id, nil, 200, &first)
	if first.Progress == nil || !first.Progress.Completed {
		t.Fatalf("book 0 after read: %+v", first.Progress)
	}
	if second := seriesBooks(t, r, sa.Id)[1]; second.Progress != nil {
		t.Fatalf("404 wrote book 1: %+v", second.Progress)
	}
	r.json("POST", "/api/v1/books/read-state", map[string]any{"bookIds": []string{books[0].Id}, "read": false}, 204, nil)
	if first := seriesBooks(t, r, sa.Id)[0]; first.Progress != nil {
		t.Fatalf("book 0 still read: %+v", first.Progress)
	}
}

func TestAddCollectionSeriesMany(t *testing.T) {
	e, a, b := progressEnv(t)
	sa, sb := libSeries(t, e.admin, a.Id), libSeries(t, e.admin, b.Id)
	var c Collection
	e.admin.json("POST", "/api/v1/collections", map[string]any{"name": "連作", "seriesIds": []string{sb.Id}}, 201, &c)

	// Order: sb already there keeps its place; sa is appended once.
	e.admin.json("POST", "/api/v1/collections/"+c.Id+"/series", map[string]any{"seriesIds": []string{sa.Id, sb.Id, sa.Id}}, 200, &c)
	if !slices.Equal(c.SeriesIds, []string{sb.Id, sa.Id}) {
		t.Fatalf("members %v, want [sb sa]", c.SeriesIds)
	}

	// A missing member refuses everything; nothing is added.
	e.admin.json("POST", "/api/v1/collections/"+c.Id+"/series", map[string]any{"seriesIds": []string{sa.Id, "missing"}}, 404, nil)
	e.admin.json("POST", "/api/v1/collections/nope/series", map[string]any{"seriesIds": []string{sa.Id}}, 404, nil)
	e.admin.json("POST", "/api/v1/collections/"+c.Id+"/series", map[string]any{"seriesIds": []string{}}, 400, nil)
	var got Collection
	e.admin.json("GET", "/api/v1/collections/"+c.Id, nil, 200, &got)
	if !slices.Equal(got.SeriesIds, []string{sb.Id, sa.Id}) {
		t.Fatalf("members after refusals %v", got.SeriesIds)
	}
}

func TestAddReadListBooks(t *testing.T) {
	e, a, _ := progressEnv(t)
	books := seriesBooks(t, e.admin, libSeries(t, e.admin, a.Id).Id)
	var l ReadList
	e.admin.json("POST", "/api/v1/read-lists", map[string]any{"name": "順番", "bookIds": []string{books[1].Id}}, 201, &l)

	e.admin.json("POST", "/api/v1/read-lists/"+l.Id+"/books", map[string]any{"bookIds": []string{books[0].Id, books[1].Id, books[0].Id}}, 200, &l)
	if !slices.Equal(l.BookIds, []string{books[1].Id, books[0].Id}) {
		t.Fatalf("books %v, want [b1 b0]", l.BookIds)
	}
	e.admin.json("POST", "/api/v1/read-lists/"+l.Id+"/books", map[string]any{"bookIds": []string{books[0].Id, "missing"}}, 404, nil)
	e.admin.json("POST", "/api/v1/read-lists/"+l.Id+"/books", map[string]any{"bookIds": []string{}}, 400, nil)
	var got ReadList
	e.admin.json("GET", "/api/v1/read-lists/"+l.Id, nil, 200, &got)
	if !slices.Equal(got.BookIds, []string{books[1].Id, books[0].Id}) {
		t.Fatalf("books after refusals %v", got.BookIds)
	}
}
