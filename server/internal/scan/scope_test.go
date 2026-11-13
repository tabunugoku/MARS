package scan

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tabunugoku/mars/server/internal/db"
)

// openLibrary opens a database holding one library "L" over root.
func openLibrary(t *testing.T, root string) *sql.DB {
	t.Helper()
	d, err := db.Open(context.Background(), filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if _, err := d.Exec(`INSERT INTO libraries (id, name, root, kind, created_at) VALUES ('L', 'L', ?, 'comics', '')`, root); err != nil {
		t.Fatal(err)
	}
	return d
}

// syncAt walks root (the whole library when scope is nil, else the scope) and
// syncs the result at the given time.
func syncAt(t *testing.T, d *sql.DB, root string, scope []string, now time.Time) Stats {
	t.Helper()
	ctx := context.Background()
	var l Listing
	var err error
	if scope == nil {
		l, err = Walk(ctx, root, nil)
	} else {
		l, err = WalkScope(ctx, root, nil, scope)
	}
	if err != nil {
		t.Fatal(err)
	}
	st, err := Sync(ctx, d, "L", "comics", l, now)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// live counts the rows of path (a book path or a series dir) not in the trash.
func live(t *testing.T, d *sql.DB, table, col, path string) int {
	t.Helper()
	return count(t, d, `SELECT COUNT(*) FROM `+table+` WHERE `+col+` = ? AND deleted_at IS NULL`, path)
}

var (
	scopeT0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	scopeT1 = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
)

// A scoped sync changes only what is inside its scope: new and deleted books
// and new series there are applied, and rows outside are left alone even when
// the walk did not see them.
func TestScopedSyncLeavesOutsideAlone(t *testing.T) {
	root := t.TempDir()
	write(t, root, "A/a1.cbz", "a")
	write(t, root, "B/b1.cbz", "b")
	write(t, root, "C/c1.cbz", "c")
	write(t, root, "loose.cbz", "l")
	d := openLibrary(t, root)
	syncAt(t, d, root, nil, scopeT0)

	// Changes on disk. The scope is A and a new folder N; B, C and the root
	// file change without any event.
	write(t, root, "A/a2.cbz", "a2")
	write(t, root, "N/n1.cbz", "n")
	if err := os.Remove(filepath.Join(root, "A", "a1.cbz")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, "B")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "C", "c1.cbz")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "loose.cbz")); err != nil {
		t.Fatal(err)
	}

	st := syncAt(t, d, root, []string{"A", "N"}, scopeT1)
	if st.SeriesAdded != 1 || st.BooksAdded != 2 || st.BooksRemoved != 1 || st.SeriesRemoved != 0 {
		t.Fatalf("scoped sync %+v", st)
	}
	if n := live(t, d, "books", "path", "N/n1.cbz"); n != 1 {
		t.Fatalf("new book in scope: %d live rows", n)
	}
	if n := live(t, d, "books", "path", "A/a2.cbz"); n != 1 {
		t.Fatalf("new book in scope: %d live rows", n)
	}
	if n := live(t, d, "books", "path", "A/a1.cbz"); n != 0 {
		t.Fatalf("deleted book in scope still live")
	}
	for _, p := range []string{"B/b1.cbz", "C/c1.cbz", "loose.cbz"} {
		if n := live(t, d, "books", "path", p); n != 1 {
			t.Fatalf("%s outside the scope was trashed", p)
		}
	}
	if n := live(t, d, "series", "dir", "B"); n != 1 {
		t.Fatalf("series B outside the scope was trashed")
	}
}

// A scope folder that is gone from disk trashes its series and books.
func TestScopedSyncMissingFolderTrashesSeries(t *testing.T) {
	root := t.TempDir()
	write(t, root, "A/a1.cbz", "a")
	write(t, root, "B/b1.cbz", "b")
	d := openLibrary(t, root)
	syncAt(t, d, root, nil, scopeT0)

	if err := os.RemoveAll(filepath.Join(root, "B")); err != nil {
		t.Fatal(err)
	}
	st := syncAt(t, d, root, []string{"B"}, scopeT1)
	if st.SeriesRemoved != 1 || st.BooksRemoved != 1 {
		t.Fatalf("missing scope folder: %+v", st)
	}
	if n := live(t, d, "series", "dir", "B"); n != 0 {
		t.Fatalf("series B still live")
	}
	if n := live(t, d, "books", "path", "A/a1.cbz"); n != 1 {
		t.Fatalf("book outside the scope was trashed")
	}
}

// WalkScope drops entries that would escape the root or name more than one
// segment, and reports ErrNoScope when none is left.
func TestWalkScopeRejectsBadEntries(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	write(t, root, "A/a1.cbz", "a")

	for _, bad := range []string{"../x", "a/b", "", ".", "..", "/abs", ".hidden"} {
		_, err := WalkScope(ctx, root, nil, []string{bad})
		if !errors.Is(err, ErrNoScope) {
			t.Fatalf("entry %q: err %v, want ErrNoScope", bad, err)
		}
	}

	l, err := WalkScope(ctx, root, nil, []string{"../x", "A", "A"})
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Scope) != 1 || l.Scope[0] != "A" {
		t.Fatalf("scope %v, want [A]", l.Scope)
	}
	if len(l.Series) != 1 || l.Series[0].Dir != "A" {
		t.Fatalf("series %+v", l.Series)
	}
}

// A scoped sync does not move last_scan; a full sync does.
func TestScopedSyncKeepsLastScan(t *testing.T) {
	root := t.TempDir()
	write(t, root, "A/a1.cbz", "a")
	d := openLibrary(t, root)
	lastScan := func() string {
		var s sql.NullString
		if err := d.QueryRow(`SELECT last_scan FROM libraries WHERE id = 'L'`).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s.String
	}

	syncAt(t, d, root, nil, scopeT0)
	full := lastScan()
	if full == "" {
		t.Fatal("full sync did not set last_scan")
	}

	write(t, root, "A/a2.cbz", "a2")
	syncAt(t, d, root, []string{"A"}, scopeT1)
	if got := lastScan(); got != full {
		t.Fatalf("scoped sync moved last_scan from %q to %q", full, got)
	}

	syncAt(t, d, root, nil, scopeT1)
	if got := lastScan(); got == full {
		t.Fatalf("full sync left last_scan at %q", got)
	}
}
