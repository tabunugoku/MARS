package httpapi

import (
	"context"
	"errors"
)

// Stubs; the trash list and per-item delete are implemented in phase 10.

func (s *server) ListTrash(ctx context.Context, _ ListTrashRequestObject) (ListTrashResponseObject, error) {
	return nil, errors.New("not implemented")
}

func (s *server) DeleteTrashedSeries(ctx context.Context, _ DeleteTrashedSeriesRequestObject) (DeleteTrashedSeriesResponseObject, error) {
	return nil, errors.New("not implemented")
}

func (s *server) DeleteTrashedBook(ctx context.Context, _ DeleteTrashedBookRequestObject) (DeleteTrashedBookResponseObject, error) {
	return nil, errors.New("not implemented")
}
