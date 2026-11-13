package db

import (
	"context"
	"database/sql"
	"strings"
)

// CollectionsLosing returns the ids of the collections that hold one of the
// series named by seriesQuery (a SELECT of series ids, with its arguments).
// Call it before those series are deleted, then DropEmptiedCollections after.
func CollectionsLosing(ctx context.Context, tx *sql.Tx, seriesQuery string, args ...any) ([]string, error) {
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

// DropEmptiedCollections deletes those of ids that hold no series any more.
func DropEmptiedCollections(ctx context.Context, tx *sql.Tx, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM collections WHERE id IN (?`+strings.Repeat(",?", len(ids)-1)+`)
		AND NOT EXISTS (SELECT 1 FROM collection_series cs WHERE cs.collection_id = collections.id)`, args...)
	return err
}
