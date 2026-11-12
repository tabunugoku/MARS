package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenMigratesAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "test.db")

	d, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	var v int
	if err := d.QueryRow("PRAGMA user_version").Scan(&v); err != nil || v < 1 {
		t.Fatalf("user_version = %d, err %v", v, err)
	}
	var mode string
	d.QueryRow("PRAGMA journal_mode").Scan(&mode)
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want wal", mode)
	}
	// trigram FTS5 must be available for Japanese substring search.
	if _, err := d.Exec(`INSERT INTO search_rows(kind, ref, library_id, text) VALUES ('series','s1','l1','夜更けの測量士')`); err != nil {
		t.Fatal(err)
	}
	var ref string
	if err := d.QueryRow(`SELECT ref FROM search_index WHERE text MATCH '測量士'`).Scan(&ref); err != nil || ref != "s1" {
		t.Fatalf("fts trigram match: ref %q err %v", ref, err)
	}
	d.Close()

	// Reopening must not reapply migrations.
	d, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	d.Close()
}

// Upgrading to 0007 fills search_rows (and through it the FTS index) from
// the live series and books.
func TestSearchRowsMigration(t *testing.T) {
	ctx := context.Background()
	d, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(t.TempDir(), "old.db")))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := migrateTo(ctx, d, 6); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO libraries (id, name, root, kind, created_at) VALUES ('L', 'L', '/x', 'manga', '')`,
		`INSERT INTO series (id, library_id, dir, title, title_sort, authors, created_at, updated_at)
		 VALUES ('S', 'L', 'd', '夜更けの測量士', '', '[{"name":"架空太郎","role":"writer"}]', '', '')`,
		`INSERT INTO series (id, library_id, dir, title, title_sort, deleted_at, created_at, updated_at)
		 VALUES ('T', 'L', 'e', '捨てた測量士', '', 'x', '', '')`,
		`INSERT INTO books (id, series_id, library_id, path, format, size_bytes, mtime, title, number, number_sort, created_at, updated_at)
		 VALUES ('B', 'S', 'L', 'd/1.cbz', 'cbz', 1, '', '測量士 第1巻', 1, 1, '', '')`,
		`INSERT INTO search_index (kind, ref, library_id, text) VALUES ('series', 'S', 'L', '夜更けの測量士')`,
	} {
		if _, err := d.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if err := Migrate(ctx, d); err != nil {
		t.Fatal(err)
	}
	var refs []string
	rows, err := d.Query(`SELECT ref FROM search_index WHERE text MATCH '測量士' ORDER BY ref`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var r string
		rows.Scan(&r)
		refs = append(refs, r)
	}
	rows.Close()
	if strings.Join(refs, ",") != "B,S" {
		t.Fatalf("search after upgrade: %v", refs)
	}
	var ref string
	if err := d.QueryRow(`SELECT ref FROM search_index WHERE text MATCH '架空太郎'`).Scan(&ref); err != nil || ref != "S" {
		t.Fatalf("author search: %q %v", ref, err)
	}
}
