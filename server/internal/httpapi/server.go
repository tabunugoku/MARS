// Package httpapi implements the REST API described in api/openapi.yaml.
// The routing and request/response types are generated (api.gen.go); this
// package only holds the handlers. Image routes (pages, thumbnails) are
// registered by hand so they can use http.ServeContent (ETag, 304, Range);
// they are still described in the spec.
package httpapi

//go:generate go tool oapi-codegen -config oapi-codegen.yaml ../../../api/openapi.yaml

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/tabunugoku/cominavi/server/internal/auth"
	"log/slog"
	"net/http"
	"net/netip"

	"github.com/tabunugoku/cominavi/server/internal/library"
	"github.com/tabunugoku/cominavi/server/internal/task"
)

type Deps struct {
	DB      *sql.DB
	Version string
	Library *library.Service
	Tasks   *task.Queue
	Auth    auth.Store
	Limiter *auth.Limiter
	Gate    *SetupGate
	// TrustedProxies are reverse proxies whose X-Forwarded-For is believed.
	TrustedProxies []netip.Prefix
}

type server struct{ Deps }

var _ StrictServerInterface = (*server)(nil)

// New returns the API handler, rooted at "/" (mount it under /api/v1).
func New(d Deps) http.Handler {
	s := &server{d}
	strict := NewStrictHandlerWithOptions(s, []StrictMiddlewareFunc{requireAdmin}, StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeError(w, http.StatusBadRequest, err.Error())
		},
		ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			slog.Error("api", "path", r.URL.Path, "err", err)
			writeError(w, http.StatusInternalServerError, "サーバーでエラーが起きました")
		},
	})
	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth/login", s.handleLogin)
	mux.HandleFunc("POST /auth/logout", s.handleLogout)
	mux.HandleFunc("POST /setup", s.handleSetup)
	mux.HandleFunc("GET /books/{bookId}/pages/{page}", s.servePage)
	mux.HandleFunc("GET /books/{bookId}/thumbnail", s.serveThumbnail("book"))
	mux.HandleFunc("GET /series/{seriesId}/thumbnail", s.serveThumbnail("series"))
	mux.HandleFunc("GET /public/covers/{seriesId}", s.servePublicCover)
	mux.Handle("/", HandlerFromMux(strict, http.NewServeMux()))
	return s.authenticate(mux)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(Error{Message: msg})
}

func (s *server) GetHealth(ctx context.Context, _ GetHealthRequestObject) (GetHealthResponseObject, error) {
	if err := s.DB.PingContext(ctx); err != nil {
		return nil, err
	}
	return GetHealth200JSONResponse{Status: Ok, Version: s.Version}, nil
}

// GetLoginScreen is in public.go.
