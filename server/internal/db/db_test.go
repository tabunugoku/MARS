package db

import (
	"context"
	"path/filepath"
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
	if _, err := d.Exec(`INSERT INTO search_index(kind, ref, library_id, text) VALUES ('series','s1','l1','夜更けの測量士')`); err != nil {
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
