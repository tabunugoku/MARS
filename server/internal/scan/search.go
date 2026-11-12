package scan

import (
	"context"
	"database/sql"

	"github.com/tabunugoku/cominavi/server/internal/db"
)

// Search text of a series s and of a book b: title plus author names. The
// same expressions fill search_rows in migration 0007.
const (
	seriesText = `s.title || ' ' || s.alternate_titles || ' ' || COALESCE((SELECT group_concat(json_extract(a.value, '$.name'), ' ') FROM json_each(s.authors) a), '')`
	bookText   = `b.title || ' ' || b.isbn || ' ' || COALESCE((SELECT group_concat(json_extract(a.value, '$.name'), ' ') FROM json_each(b.authors) a), '')`
)

// Each statement pair brings one search row in line with its series or
// book, reading the row as it is when the statement runs: the upsert writes
// only when the text changed, the delete drops the row of something gone
// or trashed. search_rows' triggers update the FTS index.
var indexStmts = map[string][2]string{
	"series": {
		`INSERT INTO search_rows (kind, ref, library_id, text)
		SELECT 'series', s.id, s.library_id, ` + seriesText + ` FROM series s WHERE s.id = ? AND s.deleted_at IS NULL
		ON CONFLICT (kind, ref) DO UPDATE SET library_id = excluded.library_id, text = excluded.text
		WHERE search_rows.text IS NOT excluded.text OR search_rows.library_id IS NOT excluded.library_id`,
		`DELETE FROM search_rows WHERE kind = 'series' AND ref = ?1
		AND NOT EXISTS (SELECT 1 FROM series WHERE id = ?1 AND deleted_at IS NULL)`,
	},
	"book": {
		`INSERT INTO search_rows (kind, ref, library_id, text)
		SELECT 'book', b.id, b.library_id, ` + bookText + ` FROM books b WHERE b.id = ? AND b.deleted_at IS NULL
		ON CONFLICT (kind, ref) DO UPDATE SET library_id = excluded.library_id, text = excluded.text
		WHERE search_rows.text IS NOT excluded.text OR search_rows.library_id IS NOT excluded.library_id`,
		`DELETE FROM search_rows WHERE kind = 'book' AND ref = ?1
		AND NOT EXISTS (SELECT 1 FROM books WHERE id = ?1 AND deleted_at IS NULL)`,
	},
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// IndexSeries updates the search row of one series inside tx.
func IndexSeries(ctx context.Context, tx execer, id string) error {
	return index(ctx, tx, "series", id)
}

// IndexBook updates the search row of one book inside tx.
func IndexBook(ctx context.Context, tx execer, id string) error { return index(ctx, tx, "book", id) }

func index(ctx context.Context, tx execer, kind, id string) error {
	for _, q := range indexStmts[kind] {
		if _, err := tx.ExecContext(ctx, q, id); err != nil {
			return err
		}
	}
	return nil
}

// staleSearch lists the series and books of a library whose search row is
// missing, out of date, or left over from something gone. It only reads.
const staleSearch = `
	SELECT 'series', s.id FROM series s LEFT JOIN search_rows r ON r.kind = 'series' AND r.ref = s.id
	WHERE s.library_id = ?1 AND s.deleted_at IS NULL
	  AND (r.id IS NULL OR r.library_id IS NOT s.library_id OR r.text IS NOT (` + seriesText + `))
	UNION ALL
	SELECT 'book', b.id FROM books b LEFT JOIN search_rows r ON r.kind = 'book' AND r.ref = b.id
	WHERE b.library_id = ?1 AND b.deleted_at IS NULL
	  AND (r.id IS NULL OR r.library_id IS NOT b.library_id OR r.text IS NOT (` + bookText + `))
	UNION ALL
	SELECT r.kind, r.ref FROM search_rows r WHERE r.library_id = ?1 AND NOT CASE r.kind
	  WHEN 'series' THEN EXISTS (SELECT 1 FROM series s WHERE s.id = r.ref AND s.library_id = r.library_id AND s.deleted_at IS NULL)
	  ELSE EXISTS (SELECT 1 FROM books b WHERE b.id = r.ref AND b.library_id = r.library_id AND b.deleted_at IS NULL) END`

// SyncSearch brings the library's search rows in line with its live series
// and books. The comparison is a read; only the rows that differ are
// written, through b, in short transactions.
func SyncSearch(ctx context.Context, d *sql.DB, b *db.Batch, libraryID string) error {
	rows, err := d.QueryContext(ctx, staleSearch, libraryID)
	if err != nil {
		return err
	}
	type ref struct{ kind, id string }
	var stale []ref
	for rows.Next() {
		var r ref
		if err := rows.Scan(&r.kind, &r.id); err != nil {
			rows.Close()
			return err
		}
		stale = append(stale, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, r := range stale {
		for _, q := range indexStmts[r.kind] {
			if _, err := b.Exec(ctx, q, r.id); err != nil {
				return err
			}
		}
		if err := b.Step(ctx); err != nil {
			return err
		}
	}
	return nil
}
