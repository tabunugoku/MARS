package httpapi

import (
	"context"
	"errors"
)

// Stub; implemented in the folder picker change.
func (s *server) ListDirectories(ctx context.Context, _ ListDirectoriesRequestObject) (ListDirectoriesResponseObject, error) {
	return nil, errors.New("not implemented")
}
