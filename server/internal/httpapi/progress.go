package httpapi

import "context"

func (s *server) SetBookProgress(ctx context.Context, req SetBookProgressRequestObject) (SetBookProgressResponseObject, error) {
	return nil, errTODO
}

func (s *server) ClearBookProgress(ctx context.Context, req ClearBookProgressRequestObject) (ClearBookProgressResponseObject, error) {
	return nil, errTODO
}

func (s *server) MarkSeriesRead(ctx context.Context, req MarkSeriesReadRequestObject) (MarkSeriesReadResponseObject, error) {
	return nil, errTODO
}

func (s *server) GetReaderPrefs(ctx context.Context, req GetReaderPrefsRequestObject) (GetReaderPrefsResponseObject, error) {
	return nil, errTODO
}

func (s *server) SetGlobalReaderPrefs(ctx context.Context, req SetGlobalReaderPrefsRequestObject) (SetGlobalReaderPrefsResponseObject, error) {
	return nil, errTODO
}

func (s *server) SetSeriesReaderPrefs(ctx context.Context, req SetSeriesReaderPrefsRequestObject) (SetSeriesReaderPrefsResponseObject, error) {
	return nil, errTODO
}

func (s *server) DeleteSeriesReaderPrefs(ctx context.Context, req DeleteSeriesReaderPrefsRequestObject) (DeleteSeriesReaderPrefsResponseObject, error) {
	return nil, errTODO
}

func (s *server) SetBookReaderPrefs(ctx context.Context, req SetBookReaderPrefsRequestObject) (SetBookReaderPrefsResponseObject, error) {
	return nil, errTODO
}

func (s *server) DeleteBookReaderPrefs(ctx context.Context, req DeleteBookReaderPrefsRequestObject) (DeleteBookReaderPrefsResponseObject, error) {
	return nil, errTODO
}
