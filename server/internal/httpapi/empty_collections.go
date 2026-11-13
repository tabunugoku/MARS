package httpapi

import (
	"context"
	"database/sql"
	"strings"
)

// The deleteEmptyCollections setting removes a collection once the last of
// its series is deleted (emptying the trash, deleting a library). Only
// collections that lost series in that same step go: one an admin just
// created, or emptied by hand, stays.
//
// Use it in two steps around the delete, inside its transaction:
//
//	ids, err := collectionsLosing(ctx, tx, `<series ids subquery>`, args...)
//	... DELETE the series ...
//	err = dropEmptied(ctx, tx, ids)

// collectionsLosing returns the collections holding a series that
// seriesQuery (a SELECT of series ids) names, or nil when the setting is off.
func collectionsLosing(ctx context.Context, tx *sql.Tx, seriesQuery string, args ...any) ([]string, error) {
	v, err := loadSettings(ctx, tx)
	if err != nil {
		return nil, err
	}
	if !v.DeleteEmptyCollections {
		return nil, nil
	}
	rows, err := tx.QueryContext(ctx,
		`SELECT DISTINCT collection_id FROM collection_series WHERE series_id IN (`+seriesQuery+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// dropEmptied deletes those of ids that hold no series any more.
func dropEmptied(ctx context.Context, tx *sql.Tx, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM collections WHERE id IN (?`+strings.Repeat(",?", len(ids)-1)+`)
		AND NOT EXISTS (SELECT 1 FROM collection_series cs WHERE cs.collection_id = collections.id)`, anyStrings(ids)...)
	return err
}
