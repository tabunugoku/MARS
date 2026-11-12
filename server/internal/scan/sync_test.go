package scan

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tabunugoku/cominavi/server/internal/db"
)

func write(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func count(t *testing.T, d *sql.DB, q string, args ...any) int {
	t.Helper()
	var n int
	if err := d.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestWalkAndSync(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	write(t, root, "夜更けの測量士/夜更けの測量士 第01巻.cbz", "a")
	write(t, root, "夜更けの測量士/夜更けの測量士 第02巻.cbr", "b")
	write(t, root, "夜更けの測量士/notes.txt", "x")
	write(t, root, "Harbor Lights (2019)/Harbor Lights v10.cb7", "c")
	write(t, root, "Harbor Lights (2019)/Harbor Lights v2.cb7", "d")
	write(t, root, "読み切り.cbz", "e")
	write(t, root, ".hidden/x.cbz", "f")
	write(t, root, "@eaDir/x.cbz", "g")
	write(t, root, "skipme/x.cbz", "h")

	found, err := Walk(ctx, root, []string{"skipme"})
	if err != nil {
		t.Fatal(err)
	}
	if len(found.Series) != 3 {
		t.Fatalf("found %d series: %+v", len(found.Series), found)
	}
	if got := found.Series[0].Books; found.Series[0].Dir != "Harbor Lights (2019)" || got[0].Rel != "Harbor Lights (2019)/Harbor Lights v2.cb7" {
		t.Fatalf("order: %+v", found.Series[0])
	}

	d, err := db.Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.Exec(`INSERT INTO libraries (id, name, root, kind, created_at) VALUES ('L', 'L', ?, 'manga', '')`, root); err != nil {
		t.Fatal(err)
	}

	st, err := Sync(ctx, d, "L", "manga", found, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if st.SeriesAdded != 3 || st.BooksAdded != 5 {
		t.Fatalf("first sync %+v", st)
	}
	var title, dir string
	var num float64
	d.QueryRow(`SELECT s.title, s.reading_direction, b.number FROM books b JOIN series s ON s.id = b.series_id WHERE b.path LIKE '%v10.cb7'`).Scan(&title, &dir, &num)
	if title != "Harbor Lights" || dir != "rtl" || num != 10 {
		t.Fatalf("got %q %q %v", title, dir, num)
	}
	if n := count(t, d, `SELECT count(*) FROM search_index WHERE search_index MATCH '測量士'`); n != 3 {
		t.Fatalf("search hits %d", n)
	}

	// Same tree again: nothing changes, no duplicates.
	st, err = Sync(ctx, d, "L", "manga", found, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if st != (Stats{}) {
		t.Fatalf("second sync %+v", st)
	}
	if n := count(t, d, `SELECT count(*) FROM series`); n != 3 {
		t.Fatalf("series rows %d", n)
	}

	// Change one, remove one: outdated and trashed; restore brings it back.
	if _, err := d.Exec(`UPDATE books SET media_status = 'ready'`); err != nil {
		t.Fatal(err)
	}
	write(t, root, "夜更けの測量士/夜更けの測量士 第01巻.cbz", "changed")
	oneshot := filepath.Join(root, "読み切り.cbz")
	before, err := os.Stat(oneshot)
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(oneshot)
	found, _ = Walk(ctx, root, []string{"skipme"})
	st, err = Sync(ctx, d, "L", "manga", found, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if st.BooksChanged != 1 || st.BooksRemoved != 1 || st.SeriesRemoved != 1 {
		t.Fatalf("third sync %+v", st)
	}
	if n := count(t, d, `SELECT count(*) FROM books WHERE media_status = 'outdated'`); n != 1 {
		t.Fatalf("outdated %d", n)
	}
	if n := count(t, d, `SELECT count(*) FROM search_index WHERE search_index MATCH '読み切り'`); n != 0 {
		t.Fatalf("trashed oneshot still found %d times", n)
	}
	// Put back as it was (same size and time): nothing to analyse, but the
	// series and book are back and must be reported.
	write(t, root, "読み切り.cbz", "e")
	if err := os.Chtimes(oneshot, before.ModTime(), before.ModTime()); err != nil {
		t.Fatal(err)
	}
	found, _ = Walk(ctx, root, []string{"skipme"})
	st, err = Sync(ctx, d, "L", "manga", found, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if st != (Stats{Restored: 2}) {
		t.Fatalf("restore sync %+v", st)
	}
	if n := count(t, d, `SELECT count(*) FROM series WHERE deleted_at IS NULL`); n != 3 {
		t.Fatalf("live series after restore %d", n)
	}
	if n := count(t, d, `SELECT count(*) FROM search_index WHERE search_index MATCH '読み切り'`); n != 2 {
		t.Fatalf("restored oneshot found %d times, want series and book", n)
	}
	// The FTS index matches search_rows exactly.
	if _, err := d.Exec(`INSERT INTO search_index (search_index, rank) VALUES ('integrity-check', 1)`); err != nil {
		t.Fatal(err)
	}
}

func TestSyncUnreadableAndRenumber(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	write(t, root, "S/Bravo.cbz", "b")
	write(t, root, "S/Delta.cbz", "d")
	write(t, root, "T/T 01.cbz", "t")
	d, err := db.Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.Exec(`INSERT INTO libraries (id, name, root, kind, created_at) VALUES ('L', 'L', ?, 'comics', '')`, root); err != nil {
		t.Fatal(err)
	}
	sync := func() Listing {
		l, err := Walk(ctx, root, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Sync(ctx, d, "L", "comics", l, time.Now()); err != nil {
			t.Fatal(err)
		}
		return l
	}
	sync()
	write(t, root, "S/Alpha.cbz", "a")
	write(t, root, "S/Charlie.cbz", "c")
	sync()
	rows, err := d.Query(`SELECT path FROM books WHERE path LIKE 'S/%' ORDER BY number_sort, path`)
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for rows.Next() {
		var p string
		rows.Scan(&p)
		order = append(order, p)
	}
	rows.Close()
	if strings.Join(order, ",") != "S/Alpha.cbz,S/Bravo.cbz,S/Charlie.cbz,S/Delta.cbz" {
		t.Fatalf("order %v", order)
	}

	// A folder the walk could not read keeps its books.
	l, _ := Walk(ctx, root, nil)
	var kept []FoundSeries
	for _, s := range l.Series {
		if s.Dir != "T" {
			kept = append(kept, s)
		}
	}
	st, err := Sync(ctx, d, "L", "comics", Listing{Series: kept, Unreadable: []string{"T"}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if st.BooksRemoved != 0 || st.SeriesRemoved != 0 {
		t.Fatalf("unreadable folder was trashed: %+v", st)
	}
}

// TestSyncOneWritePerTransaction runs the sync tests again with a commit
// after every write, so every batch boundary is crossed.
func TestSyncOneWritePerTransaction(t *testing.T) {
	syncBatch = 1
	t.Cleanup(func() { syncBatch = 0 })
	t.Run("WalkAndSync", TestWalkAndSync)
	t.Run("UnreadableAndRenumber", TestSyncUnreadableAndRenumber)
}
