//go:build perf

package httpapi

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tabunugoku/mars/server/internal/auth"
	"github.com/tabunugoku/mars/server/internal/db"
)

// TestPerfExplain times the main read queries on the database a
// TestPerfLargeLibrary run left in PERF_DATA and prints their plans. The
// database is opened without migrating it.
func TestPerfExplain(t *testing.T) {
	data := os.Getenv("PERF_DATA")
	if data == "" {
		t.Skip("PERF_DATA not set")
	}
	if os.Getenv("PERF_MIGRATE") != "" { // bring a copied database to the current schema first
		t0 := time.Now()
		m, err := db.Open(context.Background(), filepath.Join(data, "t.db"))
		if err != nil {
			t.Fatal(err)
		}
		m.Close()
		t.Logf("migrated in %s", time.Since(t0).Round(time.Millisecond))
	}
	d, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(data, "t.db"))+"?mode=ro&"+os.Getenv("PERF_PRAGMAS"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var uid, libID string
	if err := d.QueryRow(`SELECT id FROM users WHERE name = 'perf-reader'`).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	d.QueryRow(`SELECT library_id FROM books GROUP BY library_id ORDER BY count(*) DESC LIMIT 1`).Scan(&libID)
	ctx := context.WithValue(context.Background(), userKey, auth.User{ID: uid, Role: "user", AllLibraries: true})
	sq, sa := seriesQuery(ctx)
	bq, ba := bookQuery(ctx)
	with := func(base []any, more ...any) []any { return append(append([]any{}, base...), more...) }
	type query struct {
		name, sql string
		args      []any
	}
	qs := []query{
		{"series all", sq + ` ORDER BY s.title_sort, s.title_sort, s.id`, sa},
		{"series count", `SELECT count(*) FROM series s WHERE s.deleted_at IS NULL AND 1`, nil},
		{"series unread", sq + ` AND NOT ` + seriesTouched + ` ORDER BY s.title_sort LIMIT 50`, with(sa, uid)},
		{"series read", sq + ` AND (` + seriesTotalBooks + ` > 0 AND ` + seriesReadBooks + ` = ` + seriesTotalBooks + `) ORDER BY s.title_sort LIMIT 50`, with(sa, uid)},
		{"keepReading", bq + ` AND rp.completed = 0 ORDER BY rp.read_at DESC, b.id LIMIT 20`, ba},
		{"onDeck", bq + onDeckWhere + ` ORDER BY (SELECT max(sr.read_at) FROM books sb
	  CROSS JOIN read_progress sr ON sr.book_id = sb.id AND sr.user_id = ?
	  WHERE sb.series_id = b.series_id AND sb.deleted_at IS NULL) DESC, b.id LIMIT 20`, with(ba, uid, uid, uid, uid)},
		{"recentBooks", bq + ` ORDER BY b.created_at DESC, b.id DESC LIMIT 20`, ba},
		{"recentSeries", sq + ` ORDER BY s.created_at DESC, s.id DESC LIMIT 20`, sa},
		{"recentlyRead", bq + ` AND rp.completed = 1 ORDER BY rp.read_at DESC, b.id LIMIT 20`, ba},
		{"updatedSeries", sq + ` AND s.updated_at > s.created_at ORDER BY s.updated_at DESC, s.id DESC LIMIT 20`, sa},
		{"search books 2ch", bq + ` AND b.id IN (SELECT ref FROM search_index WHERE kind = 'book' AND text LIKE ? ESCAPE '\') ORDER BY b.title, b.number_sort LIMIT 20`, with(ba, "%騎士%")},
		{"search books 3ch", bq + ` AND b.id IN (SELECT ref FROM search_index WHERE kind = 'book' AND text MATCH ?) ORDER BY b.title, b.number_sort LIMIT 20`, with(ba, `"魔法使"`)},
		{"series books", bq + ` AND b.series_id = (SELECT series_id FROM books GROUP BY series_id ORDER BY count(*) DESC LIMIT 1) ORDER BY b.number_sort, b.path`, ba},
		{"pages", `SELECT number, width, height, media_type FROM pages WHERE book_id = (SELECT id FROM books LIMIT 1 OFFSET 20000) ORDER BY number`, nil},
		{"libraries", `SELECT l.id, (SELECT count(*) FROM books b WHERE b.library_id = l.id AND b.deleted_at IS NULL) FROM libraries l`, nil},
	}
	for _, q := range qs {
		if f := os.Getenv("PERF_ONLY"); f != "" && !strings.Contains(q.name, f) {
			continue
		}
		var best time.Duration
		n := 0
		for i := range 3 {
			t0 := time.Now()
			rows, err := d.Query(q.sql, q.args...)
			if err != nil {
				t.Fatalf("%s: %v", q.name, err)
			}
			n = 0
			for rows.Next() {
				n++
			}
			rows.Close()
			if el := time.Since(t0); i == 0 || el < best {
				best = el
			}
		}
		var plan strings.Builder
		rows, err := d.Query(`EXPLAIN QUERY PLAN `+q.sql, q.args...)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var id, parent, unused int
			var detail string
			rows.Scan(&id, &parent, &unused, &detail)
			fmt.Fprintf(&plan, "    %d<%d %s\n", id, parent, detail)
		}
		rows.Close()
		t.Logf("%s: %d rows, best %s\n%s", q.name, n, ms(best), plan.String())
	}
}
