package httpapi

import (
	"net/url"
	"sort"
	"strconv"
	"testing"
)

// filterFix sets the metadata the series filters look at on browseFix:
//
//	galaxy (library A, 4 books, total 4): ja, A社, 13, tags 宇宙/王道, 山田 (writer), 鈴木 (penciller), genres SF/冒険
//	dawn   (library A, 3 books, total NULL): ja, no publisher, no age rating, tags 日常, 鈴木 (writer), genres 冒険/日常
//	map    (library B, 2 books, total 2): en, C社, 18, tags 宇宙, 佐藤 (writer), genres 秘密
func filterFix(t *testing.T) *browseFix {
	t.Helper()
	f := newBrowseFix(t)
	f.exec(`UPDATE series SET genres = '["SF","冒険"]', tags = '["宇宙","王道"]', publisher = 'A社', language = 'ja',
		age_rating = 13, total_book_count = 4,
		authors = '[{"name":"山田","role":"writer"},{"name":"鈴木","role":"penciller"}]' WHERE id = ?`, f.galaxySeries.Id)
	f.exec(`UPDATE series SET genres = '["冒険","日常"]', tags = '["日常"]', publisher = '', language = 'ja',
		age_rating = NULL, total_book_count = NULL,
		authors = '[{"name":"鈴木","role":"writer"}]' WHERE id = ?`, f.dawnSeries.Id)
	f.exec(`UPDATE series SET genres = '["秘密"]', tags = '["宇宙"]', publisher = 'C社', language = 'en',
		age_rating = 18, total_book_count = 2,
		authors = '[{"name":"佐藤","role":"writer"}]' WHERE id = ?`, f.mapSeries.Id)
	return f
}

// filterQuery builds "?k=v&..." from pairs; repeated keys are allowed.
func filterQuery(kv ...string) string {
	v := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		v.Add(kv[i], kv[i+1])
	}
	return "?" + v.Encode()
}

func listFiltered(t *testing.T, c client, kv ...string) []Series {
	t.Helper()
	var list []Series
	c.json("GET", "/api/v1/series"+filterQuery(kv...), nil, 200, &list)
	if list == nil {
		t.Fatalf("series %v: null list", kv)
	}
	return list
}

func TestSeriesFilters(t *testing.T) {
	f := filterFix(t)
	g, d, m := f.galaxySeries.Id, f.dawnSeries.Id, f.mapSeries.Id
	admin, reader := f.e.admin, f.reader

	// Each filter alone. Within one parameter the values are alternatives.
	sameSet(t, "tags 宇宙 (admin)", seriesIDs(listFiltered(t, admin, "tags", "宇宙")), g, m)
	sameSet(t, "tags 宇宙 (reader: library B hidden)", seriesIDs(listFiltered(t, reader, "tags", "宇宙")), g)
	sameSet(t, "tags 王道 or 日常", seriesIDs(listFiltered(t, admin, "tags", "王道", "tags", "日常")), g, d)

	sameSet(t, "publishers A社", seriesIDs(listFiltered(t, admin, "publishers", "A社")), g)
	sameSet(t, "publishers A社 or C社", seriesIDs(listFiltered(t, admin, "publishers", "A社", "publishers", "C社")), g, m)
	sameSet(t, "publishers C社 (reader)", seriesIDs(listFiltered(t, reader, "publishers", "C社")))

	sameSet(t, "languages ja", seriesIDs(listFiltered(t, admin, "languages", "ja")), g, d)
	sameSet(t, "languages en (admin)", seriesIDs(listFiltered(t, admin, "languages", "en")), m)
	sameSet(t, "languages en (reader)", seriesIDs(listFiltered(t, reader, "languages", "en")))

	// Authors match by name in any role.
	sameSet(t, "authors 鈴木 (writer and penciller)", seriesIDs(listFiltered(t, admin, "authors", "鈴木")), g, d)
	sameSet(t, "authors 山田 or 佐藤 (admin)", seriesIDs(listFiltered(t, admin, "authors", "山田", "authors", "佐藤")), g, m)
	sameSet(t, "authors 佐藤 (reader)", seriesIDs(listFiltered(t, reader, "authors", "佐藤")))
	sameSet(t, "authors 鈴 (partial name)", seriesIDs(listFiltered(t, admin, "authors", "鈴")))
	sameSet(t, "authors writer (role, not name)", seriesIDs(listFiltered(t, admin, "authors", "writer")))

	// Age rating at most n; a series without one is included.
	sameSet(t, "ageRatingMax 13", seriesIDs(listFiltered(t, admin, "ageRatingMax", "13")), g, d)
	sameSet(t, "ageRatingMax 12", seriesIDs(listFiltered(t, admin, "ageRatingMax", "12")), d)
	sameSet(t, "ageRatingMax 18 (admin)", seriesIDs(listFiltered(t, admin, "ageRatingMax", "18")), g, d, m)
	sameSet(t, "ageRatingMax 0 (reader)", seriesIDs(listFiltered(t, reader, "ageRatingMax", "0")), d)

	// complete: total known and reached, or the others (unknown total included).
	sameSet(t, "complete=true (admin)", seriesIDs(listFiltered(t, admin, "complete", "true")), g, m)
	sameSet(t, "complete=true (reader)", seriesIDs(listFiltered(t, reader, "complete", "true")), g)
	sameSet(t, "complete=false (admin)", seriesIDs(listFiltered(t, admin, "complete", "false")), d)
	sameSet(t, "complete=false (reader)", seriesIDs(listFiltered(t, reader, "complete", "false")), d)

	// Filters combine with AND; within each, values are alternatives.
	sameSet(t, "languages ja AND authors 鈴木", seriesIDs(listFiltered(t, admin, "languages", "ja", "authors", "鈴木")), g, d)
	sameSet(t, "ja AND 鈴木 AND ageRatingMax 13",
		seriesIDs(listFiltered(t, admin, "languages", "ja", "authors", "鈴木", "ageRatingMax", "13")), g, d)
	sameSet(t, "ja AND 鈴木 AND ageRatingMax 13 AND complete",
		seriesIDs(listFiltered(t, admin, "languages", "ja", "authors", "鈴木", "ageRatingMax", "13", "complete", "true")), g)
	sameSet(t, "tags 宇宙 AND genres 冒険", seriesIDs(listFiltered(t, admin, "tags", "宇宙", "genres", "冒険")), g)
	sameSet(t, "genres 冒険 AND complete=false", seriesIDs(listFiltered(t, admin, "genres", "冒険", "complete", "false")), d)
	sameSet(t, "publishers C社 AND authors 鈴木 (no match)",
		seriesIDs(listFiltered(t, admin, "publishers", "C社", "authors", "鈴木")))
	sameSet(t, "reader: tags 宇宙 AND languages en (library B hidden)",
		seriesIDs(listFiltered(t, reader, "tags", "宇宙", "languages", "en")))

	// Filters stay inside the caller's scope and combine with libraryId.
	sameSet(t, "tags 宇宙 with libraryId B", seriesIDs(listFiltered(t, admin, "tags", "宇宙", "libraryId", f.libB.Id)), m)
	sameSet(t, "tags 宇宙 with libraryId B (reader)",
		seriesIDs(listFiltered(t, reader, "tags", "宇宙", "libraryId", f.libB.Id)))
}

// A repeated parameter is a list of values, and a value may contain a comma:
// "Doe, Jane" is one publisher and one author, not two.
func TestSeriesFilterCommaInValue(t *testing.T) {
	f := filterFix(t)
	admin := f.e.admin
	f.exec(`UPDATE series SET publisher = 'Doe, Jane', authors = '[{"name":"Doe, Jane","role":"writer"}]' WHERE id = ?`, f.mapSeries.Id)

	sameSet(t, "publishers Doe, Jane", seriesIDs(listFiltered(t, admin, "publishers", "Doe, Jane")), f.mapSeries.Id)
	sameSet(t, "authors Doe, Jane", seriesIDs(listFiltered(t, admin, "authors", "Doe, Jane")), f.mapSeries.Id)
	// The pieces of the value match nothing, and a comma-joined list is one value.
	sameSet(t, "publishers Doe", seriesIDs(listFiltered(t, admin, "publishers", "Doe")))
	sameSet(t, "publishers Doe,A社 as one value", seriesIDs(listFiltered(t, admin, "publishers", "Doe,A社")))
	sameSet(t, "publishers Doe, Jane or A社",
		seriesIDs(listFiltered(t, admin, "publishers", "Doe, Jane", "publishers", "A社")), f.mapSeries.Id, f.galaxySeries.Id)
}

// facetsOf returns the facets for the caller and query.
func facetsOf(t *testing.T, c client, query string) SeriesFacets {
	t.Helper()
	var out SeriesFacets
	c.json("GET", "/api/v1/series/facets"+query, nil, 200, &out)
	if out.Genres == nil || out.Tags == nil || out.Publishers == nil || out.Languages == nil ||
		out.Authors == nil || out.AgeRatings == nil {
		t.Fatalf("facets %s has a null list: %+v", query, out)
	}
	return out
}

// sortedCopy returns the strings sorted, the order the facets come in.
func sortedCopy(ss ...string) []string {
	out := append([]string{}, ss...)
	sort.Strings(out)
	return out
}

func TestSeriesFacets(t *testing.T) {
	f := filterFix(t)

	// Administrator: every library.
	all := facetsOf(t, f.e.admin, "")
	sameIDs(t, "admin genres", all.Genres, sortedCopy("SF", "冒険", "日常", "秘密")...)
	sameIDs(t, "admin tags", all.Tags, sortedCopy("宇宙", "王道", "日常")...)
	sameIDs(t, "admin publishers", all.Publishers, sortedCopy("A社", "C社")...)
	sameIDs(t, "admin languages", all.Languages, sortedCopy("en", "ja")...)
	sameIDs(t, "admin authors", all.Authors, sortedCopy("山田", "鈴木", "佐藤")...)
	sameIDs(t, "admin ageRatings", intsAsStrings(all.AgeRatings), "13", "18")

	// Reader: library A only. Library B's values never appear, and the
	// empty publisher of dawn is not a facet.
	rd := facetsOf(t, f.reader, "")
	sameIDs(t, "reader genres", rd.Genres, sortedCopy("SF", "冒険", "日常")...)
	sameIDs(t, "reader tags", rd.Tags, sortedCopy("宇宙", "王道", "日常")...)
	sameIDs(t, "reader publishers", rd.Publishers, "A社")
	sameIDs(t, "reader languages", rd.Languages, "ja")
	sameIDs(t, "reader authors", rd.Authors, sortedCopy("山田", "鈴木")...)
	sameIDs(t, "reader ageRatings", intsAsStrings(rd.AgeRatings), "13")

	// Library B asked for by the reader: empty lists, not nulls.
	hidden := facetsOf(t, f.reader, "?libraryId="+f.libB.Id)
	for name, got := range map[string][]string{"genres": hidden.Genres, "tags": hidden.Tags,
		"publishers": hidden.Publishers, "languages": hidden.Languages, "authors": hidden.Authors} {
		if len(got) != 0 {
			t.Fatalf("reader sees %s of library B: %v", name, got)
		}
	}
	if len(hidden.AgeRatings) != 0 {
		t.Fatalf("reader sees ageRatings of library B: %v", hidden.AgeRatings)
	}

	// Administrator, one library.
	b := facetsOf(t, f.e.admin, "?libraryId="+f.libB.Id)
	sameIDs(t, "library B genres", b.Genres, "秘密")
	sameIDs(t, "library B tags", b.Tags, "宇宙")
	sameIDs(t, "library B publishers", b.Publishers, "C社")
	sameIDs(t, "library B languages", b.Languages, "en")
	sameIDs(t, "library B authors", b.Authors, "佐藤")
	sameIDs(t, "library B ageRatings", intsAsStrings(b.AgeRatings), "18")

	// Trashed series contribute nothing.
	f.exec(`UPDATE series SET deleted_at = 'x' WHERE id = ?`, f.galaxySeries.Id)
	after := facetsOf(t, f.reader, "")
	sameIDs(t, "trashed: reader genres", after.Genres, sortedCopy("冒険", "日常")...)
	sameIDs(t, "trashed: reader publishers", after.Publishers)
	sameIDs(t, "trashed: reader ageRatings", intsAsStrings(after.AgeRatings))
	sameIDs(t, "trashed: reader authors", after.Authors, "鈴木")
}

// The user with every library gets all the values; a user with no library
// gets none.
func TestSeriesFacetsScope(t *testing.T) {
	f := filterFix(t)
	all := facetsOf(t, f.other, "")
	sameIDs(t, "other genres", all.Genres, sortedCopy("SF", "冒険", "日常", "秘密")...)
	sameIDs(t, "other publishers", all.Publishers, sortedCopy("A社", "C社")...)
	sameIDs(t, "other authors", all.Authors, sortedCopy("山田", "鈴木", "佐藤")...)

	var u User
	f.e.admin.json("POST", "/api/v1/users", map[string]any{"name": "無所属", "email": "none@example.com", "password": "none pass",
		"role": "user", "allLibraries": false, "libraryIds": []string{}}, 201, &u)
	none := f.e.login("none@example.com", "none pass")
	empty := facetsOf(t, none, "")
	if len(empty.Genres)+len(empty.Tags)+len(empty.Publishers)+len(empty.Languages)+len(empty.Authors)+len(empty.AgeRatings) != 0 {
		t.Fatalf("user with no library sees facet values: %+v", empty)
	}
}

// intsAsStrings renders ints for sameIDs.
func intsAsStrings(vs []int) []string {
	out := []string{}
	for _, v := range vs {
		out = append(out, strconv.Itoa(v))
	}
	return out
}
