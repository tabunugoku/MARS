package httpapi

import (
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"
)

// setTrashed moves a series or book to the trash, or restores it.
func setTrashed(t *testing.T, e *env, table, id string, trashed bool) {
	t.Helper()
	var at any // nil stores NULL, which restores the row
	if trashed {
		at = time.Now().UTC().Format(time.RFC3339)
	}
	if _, err := e.db.Exec(`UPDATE `+table+` SET deleted_at = ? WHERE id = ?`, at, id); err != nil {
		t.Fatal(err)
	}
}

// postAll sends every POST at once and fails unless each answers 200.
func postAll(t *testing.T, c client, urls []string) {
	t.Helper()
	var wg sync.WaitGroup
	codes := make([]int, len(urls))
	for i, u := range urls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes[i] = c.do("POST", u, nil).Code
		}()
	}
	wg.Wait()
	for i, code := range codes {
		if code != 200 {
			t.Fatalf("POST %s: status %d, want 200", urls[i], code)
		}
	}
}

func sorted(ss []string) []string {
	out := slices.Clone(ss)
	slices.Sort(out)
	return out
}

func TestCollectionMembership(t *testing.T) {
	f := newListFixture(t)
	c := f.e.admin
	alpha, bravo, charlie := f.sid("Alpha"), f.sid("Bravo"), f.sid("Charlie")
	var col Collection
	c.json("POST", "/api/v1/collections", map[string]any{"name": "members"}, 201, &col)
	base := "/api/v1/collections/" + col.Id

	var got Collection
	c.json("POST", base+"/series/"+alpha, nil, 200, &got)
	c.json("POST", base+"/series/"+bravo, nil, 200, &got)
	if !slices.Equal(got.SeriesIds, []string{alpha, bravo}) {
		t.Fatalf("after two adds %v, want Alpha then Bravo", titles(members(t, c, col.Id)...))
	}
	// Adding a member twice keeps one copy and its place.
	c.json("POST", base+"/series/"+alpha, nil, 200, &got)
	if !slices.Equal(got.SeriesIds, []string{alpha, bravo}) {
		t.Fatalf("re-add changed the list: %v", got.SeriesIds)
	}

	// Removing is idempotent, and the other members keep their order.
	c.json("DELETE", base+"/series/"+bravo, nil, 200, &got)
	c.json("DELETE", base+"/series/"+bravo, nil, 200, &got)
	if !slices.Equal(got.SeriesIds, []string{alpha}) {
		t.Fatalf("after remove %v, want Alpha", got.SeriesIds)
	}
	c.json("POST", base+"/series/"+charlie, nil, 200, &got)
	if !slices.Equal(got.SeriesIds, []string{alpha, charlie}) {
		t.Fatalf("add after remove %v, want Alpha then Charlie", got.SeriesIds)
	}
	if !slices.Equal(idsOfSeries(members(t, c, col.Id)...), []string{alpha, charlie}) {
		t.Fatalf("members endpoint disagrees with the collection")
	}

	c.json("POST", "/api/v1/collections/nope/series/"+alpha, nil, 404, nil)
	c.json("POST", base+"/series/nope", nil, 404, nil)
	c.json("DELETE", "/api/v1/collections/nope/series/"+alpha, nil, 404, nil)

	f.e.admin.json("POST", "/api/v1/users", map[string]any{"name": "member", "email": "member@example.com",
		"password": "member pass", "role": "user"}, 201, nil)
	r := f.e.login("member", "member pass")
	r.json("POST", base+"/series/"+bravo, nil, 403, nil)
	r.json("DELETE", base+"/series/"+alpha, nil, 403, nil)
	r.json("PUT", base+"/order", map[string]any{"seriesIds": []string{alpha, charlie}}, 403, nil)
}

// members returns the series of a collection through the members endpoint.
func members(t *testing.T, c client, colID string) []Series {
	t.Helper()
	var out []Series
	c.json("GET", "/api/v1/collections/"+colID+"/series", nil, 200, &out)
	return out
}

// Two clients adding different members at the same moment both land.
func TestCollectionConcurrentAdds(t *testing.T) {
	f := newListFixture(t)
	c := f.e.admin
	alpha, bravo, charlie := f.sid("Alpha"), f.sid("Bravo"), f.sid("Charlie")
	var col Collection
	c.json("POST", "/api/v1/collections", map[string]any{"name": "race"}, 201, &col)
	base := "/api/v1/collections/" + col.Id

	postAll(t, c, []string{base + "/series/" + alpha, base + "/series/" + bravo, base + "/series/" + charlie})
	var got Collection
	c.json("GET", base, nil, 200, &got)
	if !slices.Equal(sorted(got.SeriesIds), sorted([]string{alpha, bravo, charlie})) {
		t.Fatalf("concurrent adds kept %v, want all three", got.SeriesIds)
	}
}

func TestCollectionReorder(t *testing.T) {
	f := newListFixture(t)
	c := f.e.admin
	alpha, bravo, charlie := f.sid("Alpha"), f.sid("Bravo"), f.sid("Charlie")
	var col Collection
	c.json("POST", "/api/v1/collections", map[string]any{"name": "order", "ordered": true,
		"seriesIds": []string{alpha, bravo, charlie}}, 201, &col)
	base := "/api/v1/collections/" + col.Id

	// Each wrong set is refused, with nothing changed.
	for _, bad := range [][]string{
		{alpha, bravo},                 // missing Charlie
		{alpha, bravo, charlie, alpha}, // extra and duplicate
		{alpha, alpha, charlie},        // duplicate, Bravo missing
		{alpha, bravo, "nope"},         // unknown id
		{},                             // empty
	} {
		c.json("PUT", base+"/order", map[string]any{"seriesIds": bad}, 409, nil)
	}
	c.json("PUT", "/api/v1/collections/nope/order", map[string]any{"seriesIds": []string{alpha}}, 404, nil)
	var got Collection
	c.json("GET", base, nil, 200, &got)
	if !slices.Equal(got.SeriesIds, []string{alpha, bravo, charlie}) {
		t.Fatalf("refused reorder changed the list: %v", got.SeriesIds)
	}

	c.json("PUT", base+"/order", map[string]any{"seriesIds": []string{charlie, alpha, bravo}}, 200, &got)
	if !slices.Equal(got.SeriesIds, []string{charlie, alpha, bravo}) {
		t.Fatalf("reordered %v, want Charlie, Alpha, Bravo", got.SeriesIds)
	}
	if !slices.Equal(idsOfSeries(members(t, c, col.Id)...), []string{charlie, alpha, bravo}) {
		t.Fatalf("members endpoint ignores the new order")
	}
}

// A trashed member stays in the list and keeps its slot while the others
// change around it.
func TestCollectionTrashedMemberKeepsPlace(t *testing.T) {
	f := newListFixture(t)
	c := f.e.admin
	alpha, bravo, charlie := f.sid("Alpha"), f.sid("Bravo"), f.sid("Charlie")
	var col Collection
	c.json("POST", "/api/v1/collections", map[string]any{"name": "trash", "seriesIds": []string{alpha, bravo, charlie}}, 201, &col)
	base := "/api/v1/collections/" + col.Id
	setTrashed(t, f.e, "series", bravo, true)

	var got Collection
	c.json("GET", base, nil, 200, &got)
	if !slices.Equal(got.SeriesIds, []string{alpha, charlie}) {
		t.Fatalf("visible %v, want Alpha and Charlie", got.SeriesIds)
	}
	c.json("POST", base+"/series/"+bravo, nil, 404, nil)

	// Stored order is Alpha, Bravo (hidden), Charlie. Removing and re-adding
	// Charlie puts it last.
	c.json("DELETE", base+"/series/"+charlie, nil, 200, &got)
	c.json("POST", base+"/series/"+charlie, nil, 200, &got)
	if !slices.Equal(got.SeriesIds, []string{alpha, charlie}) {
		t.Fatalf("after re-add %v, want Alpha then Charlie", got.SeriesIds)
	}

	// Stored order is now Alpha, Bravo (hidden), Charlie. Swapping the
	// visible two leaves Bravo in the middle.
	c.json("PUT", base+"/order", map[string]any{"seriesIds": []string{charlie}}, 409, nil)
	c.json("PUT", base+"/order", map[string]any{"seriesIds": []string{charlie, alpha, bravo}}, 409, nil)
	c.json("PUT", base+"/order", map[string]any{"seriesIds": []string{charlie, alpha}}, 200, &got)
	if !slices.Equal(got.SeriesIds, []string{charlie, alpha}) {
		t.Fatalf("visible after reorder %v, want Charlie then Alpha", got.SeriesIds)
	}

	// Restored: Bravo returns in the middle, where it was.
	setTrashed(t, f.e, "series", bravo, false)
	c.json("GET", base, nil, 200, &got)
	if !slices.Equal(got.SeriesIds, []string{charlie, bravo, alpha}) {
		t.Fatalf("restored %v, want Charlie, Bravo, Alpha", got.SeriesIds)
	}
}

func TestReadListMembership(t *testing.T) {
	f := newListFixture(t)
	c := f.e.admin
	a0, a1, c0 := f.bid("Alpha", 0), f.bid("Alpha", 1), f.bid("Charlie", 0)
	var rl ReadList
	c.json("POST", "/api/v1/read-lists", map[string]any{"name": "members"}, 201, &rl)
	base := "/api/v1/read-lists/" + rl.Id

	var got ReadList
	c.json("POST", base+"/books/"+a0, nil, 200, &got)
	c.json("POST", base+"/books/"+c0, nil, 200, &got)
	c.json("POST", base+"/books/"+a0, nil, 200, &got)
	if !slices.Equal(got.BookIds, []string{a0, c0}) {
		t.Fatalf("after adds %v, want Alpha v01 then Charlie v01", got.BookIds)
	}
	c.json("DELETE", base+"/books/"+c0, nil, 200, &got)
	c.json("DELETE", base+"/books/"+c0, nil, 200, &got)
	c.json("POST", base+"/books/"+a1, nil, 200, &got)
	if !slices.Equal(got.BookIds, []string{a0, a1}) {
		t.Fatalf("after remove and add %v, want Alpha v01, v02", got.BookIds)
	}
	var books []Book
	c.json("GET", base+"/books", nil, 200, &books)
	if !slices.Equal(idsOfBooks(books...), []string{a0, a1}) {
		t.Fatalf("books endpoint %v, want the same order", idsOfBooks(books...))
	}

	c.json("POST", "/api/v1/read-lists/nope/books/"+a0, nil, 404, nil)
	c.json("POST", base+"/books/nope", nil, 404, nil)
	c.json("DELETE", "/api/v1/read-lists/nope/books/"+a0, nil, 404, nil)

	f.e.admin.json("POST", "/api/v1/users", map[string]any{"name": "member", "email": "member@example.com",
		"password": "member pass", "role": "user"}, 201, nil)
	r := f.e.login("member", "member pass")
	r.json("POST", base+"/books/"+c0, nil, 403, nil)
	r.json("DELETE", base+"/books/"+a0, nil, 403, nil)
	r.json("PUT", base+"/order", map[string]any{"bookIds": []string{a0, a1}}, 403, nil)
}

func TestReadListConcurrentAdds(t *testing.T) {
	f := newListFixture(t)
	c := f.e.admin
	var all []string
	for _, title := range []string{"Alpha", "Bravo", "Charlie"} {
		for i := range 2 {
			all = append(all, f.bid(title, i))
		}
	}
	var rl ReadList
	c.json("POST", "/api/v1/read-lists", map[string]any{"name": "race"}, 201, &rl)
	urls := make([]string, len(all))
	for i, id := range all {
		urls[i] = fmt.Sprintf("/api/v1/read-lists/%s/books/%s", rl.Id, id)
	}
	postAll(t, c, urls)
	var got ReadList
	c.json("GET", "/api/v1/read-lists/"+rl.Id, nil, 200, &got)
	if !slices.Equal(sorted(got.BookIds), sorted(all)) {
		t.Fatalf("concurrent adds kept %d books, want %d", len(got.BookIds), len(all))
	}
}

func TestReadListReorderAndTrash(t *testing.T) {
	f := newListFixture(t)
	c := f.e.admin
	a0, b0, c0 := f.bid("Alpha", 0), f.bid("Bravo", 0), f.bid("Charlie", 0)
	var rl ReadList
	c.json("POST", "/api/v1/read-lists", map[string]any{"name": "order", "bookIds": []string{a0, b0, c0}}, 201, &rl)
	base := "/api/v1/read-lists/" + rl.Id

	setTrashed(t, f.e, "books", b0, true)
	var got ReadList
	c.json("GET", base, nil, 200, &got)
	if !slices.Equal(got.BookIds, []string{a0, c0}) {
		t.Fatalf("visible %v, want Alpha v01 and Charlie v01", got.BookIds)
	}
	c.json("POST", base+"/books/"+b0, nil, 404, nil)

	// Stored order is a0, b0 (hidden), c0. Wrong sets are refused.
	c.json("PUT", base+"/order", map[string]any{"bookIds": []string{c0}}, 409, nil)
	c.json("PUT", base+"/order", map[string]any{"bookIds": []string{c0, a0, b0}}, 409, nil)
	c.json("PUT", "/api/v1/read-lists/nope/order", map[string]any{"bookIds": []string{a0}}, 404, nil)

	c.json("PUT", base+"/order", map[string]any{"bookIds": []string{c0, a0}}, 200, &got)
	if !slices.Equal(got.BookIds, []string{c0, a0}) {
		t.Fatalf("visible after reorder %v, want Charlie v01 then Alpha v01", got.BookIds)
	}
	setTrashed(t, f.e, "books", b0, false)
	c.json("GET", base, nil, 200, &got)
	if !slices.Equal(got.BookIds, []string{c0, b0, a0}) {
		t.Fatalf("restored %v, want Charlie, Bravo, Alpha", got.BookIds)
	}
}
