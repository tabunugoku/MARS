// Package httpapi implements the REST API described in api/openapi.yaml.
// The routing and request/response types are generated (api.gen.go); this
// package only holds the handlers.
package httpapi

//go:generate go tool oapi-codegen -config oapi-codegen.yaml ../../../api/openapi.yaml

import (
	"context"
	"database/sql"
	"net/http"
)

type Deps struct {
	DB      *sql.DB
	Version string
}

type server struct{ Deps }

var _ StrictServerInterface = (*server)(nil)

// New returns the API handler, rooted at "/" (mount it under /api/v1).
func New(d Deps) http.Handler {
	return HandlerFromMux(NewStrictHandler(&server{d}, nil), http.NewServeMux())
}

func (s *server) GetHealth(ctx context.Context, _ GetHealthRequestObject) (GetHealthResponseObject, error) {
	if err := s.DB.PingContext(ctx); err != nil {
		return nil, err
	}
	return GetHealth200JSONResponse{Status: Ok, Version: s.Version}, nil
}

// GetLoginScreen is public: it returns covers only when an admin chose the
// cover wall. Until libraries exist (M1) there is nothing to show.
func (s *server) GetLoginScreen(ctx context.Context, _ GetLoginScreenRequestObject) (GetLoginScreenResponseObject, error) {
	return GetLoginScreen200JSONResponse{Backdrop: Compass, Covers: []LoginCover{}}, nil
}
