package httpapi

import (
	"image/color"
	"net/url"
	"path/filepath"
	"sort"
	"testing"
)

// browseFix is two libraries with fixed content. Library A holds 銀河の灯台
// (v01-v04) and 夜明けの計算 (v01-v03); library B holds 星の地図 (v01-v02). The
// reader may only see A. The other user sees everything but has no progress.
type browseFix struct {
	t             *testing.T
	e             *env
	libA, libB    Library
	reader, other client
	readerID      string
	otherID       string
	galaxySeries  Series
	dawnSeries    Series
	mapSeries     Series
	galaxy        []Book
	dawn          []Book
	mapBooks      []Book
}

func newBrowseFix(t *testing.T) *browseFix {
	t.Helper()
	e := newEnv(t)
	red := pngBytes(t, 40, 60, color.RGBA{200, 0, 0, 255})
	rootA, rootB := t.TempDir(), t.TempDir()
	for _, v := range []string{"v01", "v02", "v03", "v04"} {
		writeCBZ(t, filepath.Join(rootA, "銀河の灯台", "銀河の灯台 "+v+".cbz"), map[string][]byte{"1.png": red})
	}
	for _, v := range []string{"v01", "v02", "v03"} {
		writeCBZ(t, filepath.Join(rootA, "夜明けの計算", "夜明けの計算 "+v+".cbz"), map[string][]byte{"1.png": red})
	}
	for _, v := range []string{"v01", "v02"} {
		writeCBZ(t, filepath.Join(rootB, "星の地図", "星の地図 "+v+".cbz"), map[string][]byte{"1.png": red})
	}

	f := &browseFix{t: t, e: e}
	e.admin.json("POST", "/api/v1/libraries", map[string]any{"name": "A", "root": rootA, "kind": "manga"}, 201, &f.libA)
	e.admin.json("POST", "/api/v1/libraries", map[string]any{"name": "B", "root": rootB, "kind": "manga"}, 201, &f.libB)
	e.waitTasks()

	var u User
	e.admin.json("POST", "/api/v1/users", map[string]any{"name": "読者", "email": "reader@example.com", "password": "reader pass",
		"role": "user", "allLibraries": false, "libraryIds": []string{f.libA.Id}}, 201, &u)
	f.readerID = u.Id
	f.reader = e.login("reader@example.com", "reader pass")
	e.admin.json("POST", "/api/v1/users", map[string]any{"name": "他人", "email": "other@example.com", "password": "other pass",
		"role": "user", "allLibraries": true}, 201, &u)
	f.otherID = u.Id
	f.other = e.login("other@example.com", "other pass")

	f.galaxySeries = f.seriesOf(f.libA.Id, "銀河の灯台")
	f.dawnSeries = f.seriesOf(f.libA.Id, "夜明けの計算")
	f.mapSeries = f.seriesOf(f.libB.Id, "星の地図")
	f.galaxy = f.booksOf(f.galaxySeries.Id)
	f.dawn = f.booksOf(f.dawnSeries.Id)
	f.mapBooks = f.booksOf(f.mapSeries.Id)
	return f
}

func (f *browseFix) seriesOf(libID, title string) Series {
	f.t.Helper()
	var list []Series
	f.e.admin.json("GET", "/api/v1/series?libraryId="+libID, nil, 200, &list)
	for _, s := range list {
		if s.Title == title {
			return s
		}
	}
	f.t.Fatalf("series %q not in library %s: %+v", title, libID, list)
	return Series{}
}

func (f *browseFix) booksOf(seriesID string) []Book {
	f.t.Helper()
	var list []Book
	f.e.admin.json("GET", "/api/v1/series/"+seriesID+"/books", nil, 200, &list)
	return list
}

func (f *browseFix) exec(q string, args ...any) {
	f.t.Helper()
	if _, err := f.e.db.Exec(q, args...); err != nil {
		f.t.Fatal(err)
	}
}

func (f *browseFix) progress(userID string, b Book, page int, completed bool, readAt string) {
	c := 0
	if completed {
		c = 1
	}
	f.exec(`INSERT INTO read_progress (user_id, book_id, page, completed, read_at) VALUES (?, ?, ?, ?, ?)`,
		userID, b.Id, page, c, readAt)
}

func (f *browseFix) dashboard(c client, query string) Dashboard {
	f.t.Helper()
	var d Dashboard
	c.json("GET", "/api/v1/dashboard"+query, nil, 200, &d)
	if d.KeepReading == nil || d.OnDeck == nil || d.RecentlyAddedBooks == nil || d.RecentlyAddedSeries == nil ||
		d.RecentlyRead == nil || d.RecentlyUpdatedSeries == nil {
		f.t.Fatalf("dashboard %s has a null shelf: %+v", query, d)
	}
	return d
}

func (f *browseFix) search(c client, q string) SearchResult {
	f.t.Helper()
	var r SearchResult
	c.json("GET", "/api/v1/search?q="+url.QueryEscape(q), nil, 200, &r)
	if r.Series == nil || r.Books == nil || r.Collections == nil || r.ReadLists == nil {
		f.t.Fatalf("search %q has a null list: %+v", q, r)
	}
	for _, col := range r.Collections {
		if col.SeriesIds == nil {
			f.t.Fatalf("search %q: collection %s has null seriesIds", q, col.Id)
		}
	}
	for _, rl := range r.ReadLists {
		if rl.BookIds == nil {
			f.t.Fatalf("search %q: read list %s has null bookIds", q, rl.Id)
		}
	}
	return r
}

func idsOf[T any](items []T, id func(T) string) []string {
	out := []string{}
	for _, x := range items {
		out = append(out, id(x))
	}
	return out
}

func bookIDs(bs []Book) []string {
	return idsOf(bs, func(b Book) string { return b.Id })
}

func seriesIDs(ss []Series) []string {
	return idsOf(ss, func(s Series) string { return s.Id })
}

// sameIDs checks the ids in order.
func sameIDs(t *testing.T, what string, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: got %v, want %v", what, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s: got %v, want %v", what, got, want)
		}
	}
}

// sameSet checks the ids regardless of order.
func sameSet(t *testing.T, what string, got []string, want ...string) {
	t.Helper()
	g := append([]string{}, got...)
	w := append([]string{}, want...)
	sort.Strings(g)
	sort.Strings(w)
	sameIDs(t, what, g, w...)
}

func TestAdjacentBooks(t *testing.T) {
	f := newBrowseFix(t)
	g := f.galaxy
	get := func(c client, id string) AdjacentBooks {
		t.Helper()
		var a AdjacentBooks
		c.json("GET", "/api/v1/books/"+id+"/adjacent", nil, 200, &a)
		return a
	}
	check := func(what string, a AdjacentBooks, prev, next *Book) {
		t.Helper()
		if (prev == nil) != (a.Prev == nil) || (prev != nil && a.Prev.Id != prev.Id) {
			t.Fatalf("%s: prev %+v", what, a.Prev)
		}
		if (next == nil) != (a.Next == nil) || (next != nil && a.Next.Id != next.Id) {
			t.Fatalf("%s: next %+v", what, a.Next)
		}
	}

	check("middle", get(f.e.admin, g[1].Id), &g[0], &g[2])
	check("middle as reader", get(f.reader, g[1].Id), &g[0], &g[2])
	check("first", get(f.e.admin, g[0].Id), nil, &g[1])
	check("last", get(f.e.admin, g[3].Id), &g[2], nil)

	// Library B is not the reader's: same answer as a missing book.
	f.reader.json("GET", "/api/v1/books/"+f.mapBooks[0].Id+"/adjacent", nil, 404, nil)
	f.e.admin.json("GET", "/api/v1/books/nope/adjacent", nil, 404, nil)
	check("hidden book, admin", get(f.e.admin, f.mapBooks[0].Id), nil, &f.mapBooks[1])
}

func TestDashboard(t *testing.T) {
	f := newBrowseFix(t)
	g, d := f.galaxy, f.dawn
	f.progress(f.readerID, g[0], 3, true, "2026-11-10T10:00:00Z")
	f.progress(f.readerID, g[2], 3, true, "2026-11-10T10:30:00Z") // v02 skipped: v04 must not be on deck
	f.progress(f.readerID, d[0], 3, true, "2026-11-10T09:00:00Z")
	f.progress(f.readerID, d[1], 5, false, "2026-11-10T11:00:00Z") // in progress: the series is not on deck
	f.progress(f.readerID, f.mapBooks[0], 3, true, "2026-11-10T12:00:00Z")
	f.progress(f.readerID, f.mapBooks[1], 2, false, "2026-11-10T12:30:00Z")
	// Scanned series have updated_at == created_at; move one on.
	f.exec(`UPDATE series SET updated_at = '2099-01-01T00:00:00Z' WHERE id = ?`, f.galaxySeries.Id)

	rd := f.dashboard(f.reader, "")
	sameIDs(t, "keepReading", bookIDs(rd.KeepReading), d[1].Id)
	sameIDs(t, "recentlyRead", bookIDs(rd.RecentlyRead), g[2].Id, g[0].Id, d[0].Id)
	sameIDs(t, "onDeck", bookIDs(rd.OnDeck), g[1].Id)
	sameSet(t, "recentlyAddedBooks", bookIDs(rd.RecentlyAddedBooks), append(bookIDs(g), bookIDs(d)...)...)
	sameSet(t, "recentlyAddedSeries", seriesIDs(rd.RecentlyAddedSeries), f.galaxySeries.Id, f.dawnSeries.Id)
	sameIDs(t, "recentlyUpdatedSeries", seriesIDs(rd.RecentlyUpdatedSeries), f.galaxySeries.Id)
	for _, b := range append(append([]Book{}, rd.KeepReading...), rd.RecentlyAddedBooks...) {
		if b.LibraryId != f.libA.Id {
			t.Fatalf("reader sees book %s of another library", b.Id)
		}
	}

	// Asking for library B gives nothing, progress included.
	rb := f.dashboard(f.reader, "?libraryId="+f.libB.Id)
	if len(rb.KeepReading)+len(rb.OnDeck)+len(rb.RecentlyRead)+len(rb.RecentlyAddedBooks)+
		len(rb.RecentlyAddedSeries)+len(rb.RecentlyUpdatedSeries) != 0 {
		t.Fatalf("reader dashboard for library B: %+v", rb)
	}

	// The other user has none of the reader's progress shelves.
	od := f.dashboard(f.other, "")
	if len(od.KeepReading) != 0 || len(od.OnDeck) != 0 || len(od.RecentlyRead) != 0 {
		t.Fatalf("other user sees the reader's progress: %+v", od)
	}
	if len(od.RecentlyAddedBooks) != 9 || len(od.RecentlyAddedSeries) != 3 {
		t.Fatalf("other user recently added: %d books, %d series", len(od.RecentlyAddedBooks), len(od.RecentlyAddedSeries))
	}

	// The administrator's own shelves, scoped to library A.
	ad := f.dashboard(f.e.admin, "?libraryId="+f.libA.Id)
	if len(ad.RecentlyAddedBooks) != 7 || len(ad.KeepReading) != 0 {
		t.Fatalf("admin dashboard for library A: %+v", ad)
	}
}

func TestSearch(t *testing.T) {
	f := newBrowseFix(t)
	g := f.galaxy
	now := "2026-11-10T00:00:00Z"

	// Collections and read lists, inserted directly.
	f.exec(`INSERT INTO collections (id, name, ordered, created_at) VALUES ('c-night', '夜の本棚', 1, ?)`, now)
	f.exec(`INSERT INTO collections (id, name, ordered, created_at) VALUES ('c-star', '星の本棚', 0, ?)`, now)
	f.exec(`INSERT INTO collections (id, name, ordered, created_at) VALUES ('c-empty', '空の本棚', 0, ?)`, now)
	f.exec(`INSERT INTO collection_series (collection_id, series_id, position) VALUES ('c-night', ?, 1)`, f.galaxySeries.Id)
	f.exec(`INSERT INTO collection_series (collection_id, series_id, position) VALUES ('c-night', ?, 2)`, f.mapSeries.Id)
	f.exec(`INSERT INTO collection_series (collection_id, series_id, position) VALUES ('c-night', ?, 3)`, f.dawnSeries.Id)
	f.exec(`INSERT INTO collection_series (collection_id, series_id, position) VALUES ('c-star', ?, 1)`, f.mapSeries.Id)
	f.exec(`INSERT INTO read_lists (id, name, summary, created_at) VALUES ('rl-night', '夜の読み順', '', ?)`, now)
	f.exec(`INSERT INTO read_lists (id, name, summary, created_at) VALUES ('rl-star', '星の読み順', '', ?)`, now)
	f.exec(`INSERT INTO read_list_books (read_list_id, book_id, position) VALUES ('rl-night', ?, 1)`, g[0].Id)
	f.exec(`INSERT INTO read_list_books (read_list_id, book_id, position) VALUES ('rl-night', ?, 2)`, f.mapBooks[0].Id)
	f.exec(`INSERT INTO read_list_books (read_list_id, book_id, position) VALUES ('rl-star', ?, 1)`, f.mapBooks[0].Id)

	// 3+ runes: trigram MATCH. Series and books.
	r := f.search(f.reader, "銀河の灯台")
	sameIDs(t, "series 5 runes", seriesIDs(r.Series), f.galaxySeries.Id)
	sameSet(t, "books 5 runes", bookIDs(r.Books), bookIDs(g)...)
	r = f.search(f.e.admin, "銀河の灯台")
	sameIDs(t, "admin series 5 runes", seriesIDs(r.Series), f.galaxySeries.Id)

	// 2 runes and 1 rune: LIKE on the index.
	r = f.search(f.reader, "灯台")
	sameIDs(t, "series 2 runes", seriesIDs(r.Series), f.galaxySeries.Id)
	sameSet(t, "books 2 runes", bookIDs(r.Books), bookIDs(g)...)
	r = f.search(f.reader, "灯")
	sameIDs(t, "series 1 rune", seriesIDs(r.Series), f.galaxySeries.Id)

	// Library B is hidden from the reader and visible to the administrator.
	r = f.search(f.reader, "星の地図")
	if len(r.Series) != 0 || len(r.Books) != 0 {
		t.Fatalf("reader finds library B: %+v", r)
	}
	r = f.search(f.reader, "地図")
	if len(r.Series) != 0 || len(r.Books) != 0 {
		t.Fatalf("reader finds library B by 2 runes: %+v", r)
	}
	r = f.search(f.e.admin, "星の地図")
	sameIDs(t, "admin series", seriesIDs(r.Series), f.mapSeries.Id)
	sameSet(t, "admin books", bookIDs(r.Books), bookIDs(f.mapBooks)...)
	r = f.search(f.e.admin, "地図")
	sameIDs(t, "admin series 2 runes", seriesIDs(r.Series), f.mapSeries.Id)

	// Quotes and wildcards are plain text.
	for _, q := range []string{`"`, `%`, `_`, `\`, `"%`, `灯台"%`, `銀河"の"灯台`, `"""`} {
		for _, c := range []client{f.reader, f.e.admin} {
			r := f.search(c, q)
			if len(r.Series) != 0 || len(r.Books) != 0 {
				t.Fatalf("query %q matched %+v", q, r)
			}
		}
	}

	// A blank query is the empty result.
	f.e.admin.json("GET", "/api/v1/search?q=", nil, 200, nil)
	empty := f.search(f.e.admin, "  ")
	if len(empty.Series)+len(empty.Books)+len(empty.Collections)+len(empty.ReadLists) != 0 {
		t.Fatalf("blank query: %+v", empty)
	}

	// Collections match by name. A normal user sees only collections with a
	// visible member, and only those members, in position order.
	r = f.search(f.reader, "本棚")
	sameIDs(t, "reader collections", idsOf(r.Collections, func(c Collection) string { return c.Id }), "c-night")
	sameIDs(t, "reader collection members", r.Collections[0].SeriesIds, f.galaxySeries.Id, f.dawnSeries.Id)
	if !r.Collections[0].Ordered {
		t.Fatalf("collection ordered flag lost: %+v", r.Collections[0])
	}
	r = f.search(f.other, "本棚")
	sameIDs(t, "other collections", idsOf(r.Collections, func(c Collection) string { return c.Id }), "c-night", "c-star")
	sameIDs(t, "other collection members", r.Collections[0].SeriesIds, f.galaxySeries.Id, f.mapSeries.Id, f.dawnSeries.Id)
	r = f.search(f.e.admin, "本棚")
	sameSet(t, "admin collections", idsOf(r.Collections, func(c Collection) string { return c.Id }), "c-night", "c-star", "c-empty")
	sameIDs(t, "admin collection members", r.Collections[0].SeriesIds, f.galaxySeries.Id, f.mapSeries.Id, f.dawnSeries.Id)

	// Read lists follow the same rule.
	r = f.search(f.reader, "読み順")
	sameIDs(t, "reader read lists", idsOf(r.ReadLists, func(l ReadList) string { return l.Id }), "rl-night")
	sameIDs(t, "reader read list books", r.ReadLists[0].BookIds, g[0].Id)
	r = f.search(f.e.admin, "読み順")
	sameSet(t, "admin read lists", idsOf(r.ReadLists, func(l ReadList) string { return l.Id }), "rl-night", "rl-star")
	sameIDs(t, "admin read list books", r.ReadLists[0].BookIds, g[0].Id, f.mapBooks[0].Id)
}
