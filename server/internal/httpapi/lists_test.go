package httpapi

import (
	"image/color"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// listFixture has two libraries. A holds the series Alpha and Bravo, B holds
// Charlie. Every series has two books, named by volume (Alpha v01, v02).
type listFixture struct {
	e      *env
	libA   Library
	libB   Library
	series map[string]Series
	books  map[string][]Book // by series title, in number order
}

func newListFixture(t *testing.T) *listFixture {
	t.Helper()
	f := &listFixture{e: newEnv(t)}
	red := pngBytes(t, 40, 60, color.RGBA{200, 0, 0, 255})
	rootA, rootB := t.TempDir(), t.TempDir()
	for _, name := range []string{"Alpha", "Bravo"} {
		for _, v := range []string{"v01", "v02"} {
			writeCBZ(t, filepath.Join(rootA, name, name+" "+v+".cbz"), map[string][]byte{"1.png": red})
		}
	}
	for _, v := range []string{"v01", "v02"} {
		writeCBZ(t, filepath.Join(rootB, "Charlie", "Charlie "+v+".cbz"), map[string][]byte{"1.png": red})
	}
	f.e.admin.json("POST", "/api/v1/libraries", map[string]any{"name": "A", "root": rootA, "kind": "manga"}, 201, &f.libA)
	f.e.admin.json("POST", "/api/v1/libraries", map[string]any{"name": "B", "root": rootB, "kind": "manga"}, 201, &f.libB)
	f.e.waitTasks()

	var all []Series
	f.e.admin.json("GET", "/api/v1/series", nil, 200, &all)
	f.series = map[string]Series{}
	for _, s := range all {
		f.series[s.Title] = s
	}
	if len(f.series) != 3 {
		t.Fatalf("series %v, want Alpha, Bravo and Charlie", titles(all...))
	}
	f.books = map[string][]Book{}
	for title, s := range f.series {
		var bs []Book
		f.e.admin.json("GET", "/api/v1/series/"+s.Id+"/books", nil, 200, &bs)
		if len(bs) != 2 {
			t.Fatalf("%s has %d books, want 2", title, len(bs))
		}
		f.books[title] = bs
	}
	return f
}

func (f *listFixture) sid(title string) string { return f.series[title].Id }

func (f *listFixture) bid(title string, i int) string { return f.books[title][i].Id }

func seriesIDs(ss ...Series) []string {
	out := []string{}
	for _, s := range ss {
		out = append(out, s.Id)
	}
	return out
}

func bookIDs(bs ...Book) []string {
	out := []string{}
	for _, b := range bs {
		out = append(out, b.Id)
	}
	return out
}

func titles(ss ...Series) []string {
	out := []string{}
	for _, s := range ss {
		out = append(out, s.Title)
	}
	return out
}

func TestCollectionCRUD(t *testing.T) {
	f := newListFixture(t)
	c := f.e.admin
	alpha, bravo, charlie := f.sid("Alpha"), f.sid("Bravo"), f.sid("Charlie")

	var made Collection
	c.json("POST", "/api/v1/collections", map[string]any{"name": " Favorites ", "ordered": true,
		"seriesIds": []string{bravo, alpha, bravo}}, 201, &made)
	if made.Name != "Favorites" || !made.Ordered || !slices.Equal(made.SeriesIds, []string{bravo, alpha}) {
		t.Fatalf("created %+v", made)
	}
	var got Collection
	c.json("GET", "/api/v1/collections/"+made.Id, nil, 200, &got)
	if got.Name != made.Name || got.Ordered != made.Ordered || !slices.Equal(got.SeriesIds, made.SeriesIds) {
		t.Fatalf("get %+v, created %+v", got, made)
	}
	var all []Collection
	c.json("GET", "/api/v1/collections", nil, 200, &all)
	if len(all) != 1 || all[0].Id != made.Id {
		t.Fatalf("list %+v", all)
	}
	var members []Series
	c.json("GET", "/api/v1/collections/"+made.Id+"/series", nil, 200, &members)
	if !slices.Equal(seriesIDs(members...), []string{bravo, alpha}) {
		t.Fatalf("members %v, want Bravo then Alpha", titles(members...))
	}

	var upd Collection
	c.json("PATCH", "/api/v1/collections/"+made.Id, map[string]any{"name": "Picks",
		"seriesIds": []string{alpha, charlie, bravo}}, 200, &upd)
	if upd.Name != "Picks" || !slices.Equal(upd.SeriesIds, []string{alpha, charlie, bravo}) {
		t.Fatalf("updated %+v", upd)
	}
	c.json("GET", "/api/v1/collections/"+made.Id+"/series", nil, 200, &members)
	if !slices.Equal(seriesIDs(members...), []string{alpha, charlie, bravo}) {
		t.Fatalf("members after update %v", titles(members...))
	}

	c.json("DELETE", "/api/v1/collections/"+made.Id, nil, 204, nil)
	c.json("GET", "/api/v1/collections/"+made.Id, nil, 404, nil)
	c.json("GET", "/api/v1/collections/"+made.Id+"/series", nil, 404, nil)
	c.json("PATCH", "/api/v1/collections/"+made.Id, map[string]any{"name": "x"}, 404, nil)
	c.json("DELETE", "/api/v1/collections/"+made.Id, nil, 404, nil)
	if rec := c.do("GET", "/api/v1/collections", nil); strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("empty list body %q, want []", rec.Body)
	}

	// Read lists: the same round trip, with books.
	a0, a1, c0 := f.bid("Alpha", 0), f.bid("Alpha", 1), f.bid("Charlie", 0)
	var rl ReadList
	c.json("POST", "/api/v1/read-lists", map[string]any{"name": " Start here ", "summary": "first",
		"bookIds": []string{a1, c0, a1, a0}}, 201, &rl)
	if rl.Name != "Start here" || rl.Summary != "first" || !slices.Equal(rl.BookIds, []string{a1, c0, a0}) {
		t.Fatalf("created %+v", rl)
	}
	var rlGot ReadList
	c.json("GET", "/api/v1/read-lists/"+rl.Id, nil, 200, &rlGot)
	if !slices.Equal(rlGot.BookIds, rl.BookIds) || rlGot.Summary != "first" {
		t.Fatalf("get %+v", rlGot)
	}
	var rlAll []ReadList
	c.json("GET", "/api/v1/read-lists", nil, 200, &rlAll)
	if len(rlAll) != 1 || rlAll[0].Id != rl.Id {
		t.Fatalf("list %+v", rlAll)
	}
	var books []Book
	c.json("GET", "/api/v1/read-lists/"+rl.Id+"/books", nil, 200, &books)
	if !slices.Equal(bookIDs(books...), []string{a1, c0, a0}) {
		t.Fatalf("books %v, want position order", bookIDs(books...))
	}

	// A PATCH without summary keeps it; the new order replaces the old one.
	var rlUpd ReadList
	c.json("PATCH", "/api/v1/read-lists/"+rl.Id, map[string]any{"name": "Later", "bookIds": []string{a0, a1}}, 200, &rlUpd)
	if rlUpd.Name != "Later" || rlUpd.Summary != "first" || !slices.Equal(rlUpd.BookIds, []string{a0, a1}) {
		t.Fatalf("updated %+v", rlUpd)
	}
	c.json("GET", "/api/v1/read-lists/"+rl.Id+"/books", nil, 200, &books)
	if !slices.Equal(bookIDs(books...), []string{a0, a1}) {
		t.Fatalf("books after update %v", bookIDs(books...))
	}

	c.json("DELETE", "/api/v1/read-lists/"+rl.Id, nil, 204, nil)
	c.json("GET", "/api/v1/read-lists/"+rl.Id, nil, 404, nil)
	c.json("GET", "/api/v1/read-lists/"+rl.Id+"/books", nil, 404, nil)
	c.json("PATCH", "/api/v1/read-lists/"+rl.Id, map[string]any{"name": "x"}, 404, nil)
	c.json("DELETE", "/api/v1/read-lists/"+rl.Id, nil, 404, nil)
	if rec := c.do("GET", "/api/v1/read-lists", nil); strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("empty list body %q, want []", rec.Body)
	}
}

func TestListOrder(t *testing.T) {
	f := newListFixture(t)
	c := f.e.admin
	alpha, bravo, charlie := f.sid("Alpha"), f.sid("Bravo"), f.sid("Charlie")

	var ordered, plain Collection
	c.json("POST", "/api/v1/collections", map[string]any{"name": "ordered", "ordered": true,
		"seriesIds": []string{charlie, bravo, alpha}}, 201, &ordered)
	c.json("POST", "/api/v1/collections", map[string]any{"name": "plain",
		"seriesIds": []string{charlie, bravo, alpha}}, 201, &plain)

	var got []Series
	c.json("GET", "/api/v1/collections/"+ordered.Id+"/series", nil, 200, &got)
	if !slices.Equal(seriesIDs(got...), []string{charlie, bravo, alpha}) {
		t.Fatalf("ordered %v, want position order", titles(got...))
	}
	c.json("GET", "/api/v1/collections/"+plain.Id+"/series", nil, 200, &got)
	if !slices.Equal(seriesIDs(got...), []string{alpha, bravo, charlie}) {
		t.Fatalf("unordered %v, want title order", titles(got...))
	}
	c.json("PATCH", "/api/v1/collections/"+ordered.Id, map[string]any{"ordered": false}, 200, nil)
	c.json("GET", "/api/v1/collections/"+ordered.Id+"/series", nil, 200, &got)
	if !slices.Equal(seriesIDs(got...), []string{alpha, bravo, charlie}) {
		t.Fatalf("switched to unordered %v, want title order", titles(got...))
	}

	// Read list books always follow position.
	a1, b0, c0 := f.bid("Alpha", 1), f.bid("Bravo", 0), f.bid("Charlie", 0)
	var rl ReadList
	c.json("POST", "/api/v1/read-lists", map[string]any{"name": "order", "bookIds": []string{c0, a1, b0}}, 201, &rl)
	var books []Book
	c.json("GET", "/api/v1/read-lists/"+rl.Id+"/books", nil, 200, &books)
	if !slices.Equal(bookIDs(books...), []string{c0, a1, b0}) {
		t.Fatalf("read list %v, want position order", bookIDs(books...))
	}
}

func TestListValidation(t *testing.T) {
	f := newListFixture(t)
	c := f.e.admin
	alpha, bravo := f.sid("Alpha"), f.sid("Bravo")

	c.json("POST", "/api/v1/collections", map[string]any{"name": "   "}, 400, nil)
	c.json("POST", "/api/v1/collections", map[string]any{"name": "x", "seriesIds": []string{"nope"}}, 400, nil)
	var made Collection
	c.json("POST", "/api/v1/collections", map[string]any{"name": "x", "seriesIds": []string{alpha, alpha}}, 201, &made)
	if !slices.Equal(made.SeriesIds, []string{alpha}) {
		t.Fatalf("duplicates not collapsed: %v", made.SeriesIds)
	}
	c.json("PATCH", "/api/v1/collections/"+made.Id, map[string]any{"name": ""}, 400, nil)
	c.json("PATCH", "/api/v1/collections/"+made.Id, map[string]any{"seriesIds": []string{"nope"}}, 400, nil)
	var got Collection
	c.json("GET", "/api/v1/collections/"+made.Id, nil, 200, &got)
	if got.Name != "x" || !slices.Equal(got.SeriesIds, []string{alpha}) {
		t.Fatalf("rejected update changed the collection: %+v", got)
	}

	a0 := f.bid("Alpha", 0)
	c.json("POST", "/api/v1/read-lists", map[string]any{"name": " "}, 400, nil)
	c.json("POST", "/api/v1/read-lists", map[string]any{"name": "x", "bookIds": []string{"nope"}}, 400, nil)
	var rl ReadList
	c.json("POST", "/api/v1/read-lists", map[string]any{"name": "x", "bookIds": []string{a0, a0}}, 201, &rl)
	if !slices.Equal(rl.BookIds, []string{a0}) {
		t.Fatalf("duplicates not collapsed: %v", rl.BookIds)
	}
	c.json("PATCH", "/api/v1/read-lists/"+rl.Id, map[string]any{"name": " "}, 400, nil)
	c.json("PATCH", "/api/v1/read-lists/"+rl.Id, map[string]any{"bookIds": []string{"nope"}}, 400, nil)
	c.json("PATCH", "/api/v1/read-lists/nope", map[string]any{"name": "x"}, 404, nil)
	c.json("PATCH", "/api/v1/collections/nope", map[string]any{"name": "x"}, 404, nil)

	// A trashed series cannot become a member.
	trashedAt := time.Now().UTC().Format(time.RFC3339)
	if _, err := f.e.db.Exec(`UPDATE series SET deleted_at = ? WHERE id = ?`, trashedAt, bravo); err != nil {
		t.Fatal(err)
	}
	c.json("POST", "/api/v1/collections", map[string]any{"name": "x", "seriesIds": []string{bravo}}, 400, nil)
}

// Trashed members are left out of every response, and a trashed book
// cannot be added to a read list.
func TestListsSkipTrash(t *testing.T) {
	f := newListFixture(t)
	c := f.e.admin
	alpha, bravo := f.sid("Alpha"), f.sid("Bravo")
	a0, a1, c0 := f.bid("Alpha", 0), f.bid("Alpha", 1), f.bid("Charlie", 0)

	var col Collection
	c.json("POST", "/api/v1/collections", map[string]any{"name": "both", "seriesIds": []string{alpha, bravo}}, 201, &col)
	var rl ReadList
	c.json("POST", "/api/v1/read-lists", map[string]any{"name": "three", "bookIds": []string{a0, a1, c0}}, 201, &rl)

	trashedAt := time.Now().UTC().Format(time.RFC3339)
	if _, err := f.e.db.Exec(`UPDATE series SET deleted_at = ? WHERE id = ?`, trashedAt, bravo); err != nil {
		t.Fatal(err)
	}
	if _, err := f.e.db.Exec(`UPDATE books SET deleted_at = ? WHERE id = ?`, trashedAt, a0); err != nil {
		t.Fatal(err)
	}

	var got Collection
	c.json("GET", "/api/v1/collections/"+col.Id, nil, 200, &got)
	if !slices.Equal(got.SeriesIds, []string{alpha}) {
		t.Fatalf("seriesIds %v, want Alpha only", got.SeriesIds)
	}
	var members []Series
	c.json("GET", "/api/v1/collections/"+col.Id+"/series", nil, 200, &members)
	if !slices.Equal(seriesIDs(members...), []string{alpha}) {
		t.Fatalf("members %v, want Alpha only", titles(members...))
	}
	var gotRL ReadList
	c.json("GET", "/api/v1/read-lists/"+rl.Id, nil, 200, &gotRL)
	if !slices.Equal(gotRL.BookIds, []string{a1, c0}) {
		t.Fatalf("bookIds %v, want the two live books", gotRL.BookIds)
	}
	var books []Book
	c.json("GET", "/api/v1/read-lists/"+rl.Id+"/books", nil, 200, &books)
	if !slices.Equal(bookIDs(books...), []string{a1, c0}) {
		t.Fatalf("books %v, want the two live books", bookIDs(books...))
	}
}

func TestListWritesNeedAdmin(t *testing.T) {
	f := newListFixture(t)
	f.e.admin.json("POST", "/api/v1/users", map[string]any{"name": "member", "email": "member@example.com",
		"password": "member pass", "role": "user"}, 201, nil)
	r := f.e.login("member", "member pass")

	var col Collection
	f.e.admin.json("POST", "/api/v1/collections", map[string]any{"name": "c"}, 201, &col)
	r.json("POST", "/api/v1/collections", map[string]any{"name": "x"}, 403, nil)
	r.json("PATCH", "/api/v1/collections/"+col.Id, map[string]any{"name": "y"}, 403, nil)
	r.json("DELETE", "/api/v1/collections/"+col.Id, nil, 403, nil)

	var rl ReadList
	f.e.admin.json("POST", "/api/v1/read-lists", map[string]any{"name": "r"}, 201, &rl)
	r.json("POST", "/api/v1/read-lists", map[string]any{"name": "x"}, 403, nil)
	r.json("PATCH", "/api/v1/read-lists/"+rl.Id, map[string]any{"name": "y"}, 403, nil)
	r.json("DELETE", "/api/v1/read-lists/"+rl.Id, nil, 403, nil)
}

// A restricted user sees only library A. Collections and read lists whose
// members are all in B vanish for that user, mixed ones show only A's
// members, and the administrator sees everything.
func TestListsHideOtherLibraries(t *testing.T) {
	f := newListFixture(t)
	admin := f.e.admin
	admin.json("POST", "/api/v1/users", map[string]any{"name": "restricted", "email": "restricted@example.com",
		"password": "reader pass", "role": "user", "allLibraries": false, "libraryIds": []string{f.libA.Id}}, 201, nil)
	r := f.e.login("restricted", "reader pass")
	alpha, charlie := f.sid("Alpha"), f.sid("Charlie")
	a1, c0 := f.bid("Alpha", 1), f.bid("Charlie", 0)

	var onlyB, mixed, empty Collection
	admin.json("POST", "/api/v1/collections", map[string]any{"name": "only B", "seriesIds": []string{charlie}}, 201, &onlyB)
	admin.json("POST", "/api/v1/collections", map[string]any{"name": "mixed", "ordered": true,
		"seriesIds": []string{charlie, alpha}}, 201, &mixed)
	admin.json("POST", "/api/v1/collections", map[string]any{"name": "empty"}, 201, &empty)

	r.json("GET", "/api/v1/collections/"+onlyB.Id, nil, 404, nil)
	r.json("GET", "/api/v1/collections/"+onlyB.Id+"/series", nil, 404, nil)
	r.json("GET", "/api/v1/collections/"+empty.Id, nil, 404, nil)
	r.json("GET", "/api/v1/collections/"+empty.Id+"/series", nil, 404, nil)
	var visible []Collection
	r.json("GET", "/api/v1/collections", nil, 200, &visible)
	if len(visible) != 1 || visible[0].Id != mixed.Id || !slices.Equal(visible[0].SeriesIds, []string{alpha}) {
		t.Fatalf("restricted list %+v, want only the mixed collection with Alpha", visible)
	}
	var got Collection
	r.json("GET", "/api/v1/collections/"+mixed.Id, nil, 200, &got)
	if !slices.Equal(got.SeriesIds, []string{alpha}) {
		t.Fatalf("restricted seriesIds %v, want Alpha only", got.SeriesIds)
	}
	var members []Series
	r.json("GET", "/api/v1/collections/"+mixed.Id+"/series", nil, 200, &members)
	if !slices.Equal(seriesIDs(members...), []string{alpha}) {
		t.Fatalf("restricted members %v, want Alpha only", titles(members...))
	}

	var all []Collection
	admin.json("GET", "/api/v1/collections", nil, 200, &all)
	if len(all) != 3 {
		t.Fatalf("admin sees %d collections, want 3 including the empty one", len(all))
	}
	admin.json("GET", "/api/v1/collections/"+onlyB.Id+"/series", nil, 200, &members)
	if !slices.Equal(seriesIDs(members...), []string{charlie}) {
		t.Fatalf("admin members of only B: %v", titles(members...))
	}

	// The same two cases for read lists and books.
	var rOnlyB, rMixed, rEmpty ReadList
	admin.json("POST", "/api/v1/read-lists", map[string]any{"name": "only B", "bookIds": []string{c0}}, 201, &rOnlyB)
	admin.json("POST", "/api/v1/read-lists", map[string]any{"name": "mixed", "bookIds": []string{c0, a1}}, 201, &rMixed)
	admin.json("POST", "/api/v1/read-lists", map[string]any{"name": "empty"}, 201, &rEmpty)

	r.json("GET", "/api/v1/read-lists/"+rOnlyB.Id, nil, 404, nil)
	r.json("GET", "/api/v1/read-lists/"+rOnlyB.Id+"/books", nil, 404, nil)
	r.json("GET", "/api/v1/read-lists/"+rEmpty.Id, nil, 404, nil)
	r.json("GET", "/api/v1/read-lists/"+rEmpty.Id+"/books", nil, 404, nil)
	var visibleRL []ReadList
	r.json("GET", "/api/v1/read-lists", nil, 200, &visibleRL)
	if len(visibleRL) != 1 || visibleRL[0].Id != rMixed.Id || !slices.Equal(visibleRL[0].BookIds, []string{a1}) {
		t.Fatalf("restricted read lists %+v, want only the mixed list with Alpha v02", visibleRL)
	}
	var rGot ReadList
	r.json("GET", "/api/v1/read-lists/"+rMixed.Id, nil, 200, &rGot)
	if !slices.Equal(rGot.BookIds, []string{a1}) {
		t.Fatalf("restricted bookIds %v, want Alpha v02 only", rGot.BookIds)
	}
	var rBooks []Book
	r.json("GET", "/api/v1/read-lists/"+rMixed.Id+"/books", nil, 200, &rBooks)
	if !slices.Equal(bookIDs(rBooks...), []string{a1}) {
		t.Fatalf("restricted books %v, want Alpha v02 only", bookIDs(rBooks...))
	}

	var allRL []ReadList
	admin.json("GET", "/api/v1/read-lists", nil, 200, &allRL)
	if len(allRL) != 3 {
		t.Fatalf("admin sees %d read lists, want 3 including the empty one", len(allRL))
	}
	admin.json("GET", "/api/v1/read-lists/"+rOnlyB.Id+"/books", nil, 200, &rBooks)
	if !slices.Equal(bookIDs(rBooks...), []string{c0}) {
		t.Fatalf("admin books of only B: %v", bookIDs(rBooks...))
	}
}
