package httpapi

import "testing"

// With deleteEmptyCollections on (the default), a collection whose last
// series is deleted goes too; one that still holds a series, or that was
// empty already, stays. With it off, nothing goes.
func TestDeleteEmptyCollections(t *testing.T) {
	f := newListFixture(t)
	c := f.e.admin
	alpha, bravo, charlie := f.sid("Alpha"), f.sid("Bravo"), f.sid("Charlie")
	mk := func(name string, ids ...string) string {
		var col Collection
		c.json("POST", "/api/v1/collections", map[string]any{"name": name, "seriesIds": append([]string{}, ids...)}, 201, &col)
		return col.Id
	}
	exists := func(id string) bool {
		var n int
		if err := f.e.db.QueryRow(`SELECT count(*) FROM collections WHERE id = ?`, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n == 1
	}

	onlyBravo := mk("only bravo", bravo)
	mixed := mk("alpha and bravo", alpha, bravo)
	fresh := mk("fresh")

	// Emptying the trash.
	setTrashed(t, f.e, "series", bravo, true)
	c.json("POST", "/api/v1/trash/empty", nil, 200, nil)
	if exists(onlyBravo) {
		t.Error("the collection that lost its only series is still there")
	}
	if !exists(mixed) || !exists(fresh) {
		t.Errorf("kept: mixed %v, fresh %v; want both", exists(mixed), exists(fresh))
	}

	// Deleting a library.
	onlyCharlie := mk("only charlie", charlie)
	c.json("DELETE", "/api/v1/libraries/"+f.libB.Id, nil, 204, nil)
	if exists(onlyCharlie) {
		t.Error("the collection of a deleted library's series is still there")
	}

	// Off: the last series goes, the collection stays.
	c.json("PATCH", "/api/v1/settings", map[string]any{"deleteEmptyCollections": false}, 200, nil)
	onlyAlpha := mk("only alpha", alpha)
	c.json("DELETE", "/api/v1/libraries/"+f.libA.Id, nil, 204, nil)
	if !exists(onlyAlpha) || !exists(fresh) {
		t.Errorf("setting off: only alpha %v, fresh %v; want both kept", exists(onlyAlpha), exists(fresh))
	}
}
