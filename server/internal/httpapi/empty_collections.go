package httpapi

import (
	"context"
	"database/sql"

	"github.com/tabunugoku/mars/server/internal/db"
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
	return db.CollectionsLosing(ctx, tx, seriesQuery, args...)
}

// dropEmptied deletes those of ids that hold no series any more.
func dropEmptied(ctx context.Context, tx *sql.Tx, ids []string) error {
	return db.DropEmptiedCollections(ctx, tx, ids)
}
