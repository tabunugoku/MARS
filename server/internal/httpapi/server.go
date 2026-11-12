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
	"errors"
	"github.com/tabunugoku/cominavi/server/internal/auth"
	"log/slog"
	"net/http"
	"net/netip"
	"time"

	"github.com/tabunugoku/cominavi/server/internal/events"
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
	// Events feeds GET /events; nil turns the stream off.
	Events *events.Hub
	// Heartbeat is the stream's keep-alive interval (25 s when zero).
	Heartbeat time.Duration
}

type server struct{ Deps }

var _ StrictServerInterface = (*server)(nil)

// New returns the API handler, rooted at "/" (mount it under /api/v1).
func New(d Deps) http.Handler {
	s := &server{d}
	strict := NewStrictHandlerWithOptions(s, []StrictMiddlewareFunc{requireAdmin}, StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			if errors.Is(err, errReadRequired) {
				writeError(r.Context(), w, http.StatusBadRequest, "bad_request.read_required")
				return
			}
			writeError(r.Context(), w, http.StatusBadRequest, "bad_request.malformed")
		},
		ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			slog.Error("api", "path", r.URL.Path, "err", err)
			writeError(r.Context(), w, http.StatusInternalServerError, "server.error")
		},
	})
	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth/login", s.handleLogin)
	mux.HandleFunc("POST /auth/logout", s.handleLogout)
	mux.HandleFunc("POST /setup", s.handleSetup)
	mux.HandleFunc("GET /books/{bookId}/pages/{page}", s.servePage)
	mux.HandleFunc("GET /books/{bookId}/pages/{page}/thumbnail", s.servePageThumbnail)
	mux.HandleFunc("GET /books/{bookId}/thumbnail", s.serveThumbnail("book"))
	mux.HandleFunc("GET /series/{seriesId}/thumbnail", s.serveThumbnail("series"))
	mux.HandleFunc("GET /books/{bookId}/file", s.serveBookFile)
	mux.HandleFunc("GET /series/{seriesId}/file", s.serveSeriesZip)
	mux.HandleFunc("GET /public/covers/{seriesId}", s.servePublicCover)
	mux.HandleFunc("GET /events", s.serveEvents)
	// Any path no route matches answers as JSON, not as the mux's plain text.
	api := http.NewServeMux()
	api.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(r.Context(), w, http.StatusNotFound, "not_found.route")
	})
	mux.Handle("/", HandlerWithOptions(strict, StdHTTPServerOptions{
		BaseRouter: api,
		// A query parameter that does not parse (limit=oops) gets the same JSON
		// error as every other bad request, not the generated plain text.
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeError(r.Context(), w, http.StatusBadRequest, "bad_request.malformed")
		},
	}))
	return withLocale(s.authenticate(mux))
}

// writeError answers with the error JSON: a code and its text in the
// request's language (see msg).
func writeError(ctx context.Context, w http.ResponseWriter, status int, code string, args ...any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Error{Code: code, Message: msg(ctx, code, args...)})
}

func (s *server) GetHealth(ctx context.Context, _ GetHealthRequestObject) (GetHealthResponseObject, error) {
	if err := s.DB.PingContext(ctx); err != nil {
		return nil, err
	}
	return GetHealth200JSONResponse{Status: Ok, Version: s.Version}, nil
}

// GetLoginScreen is in public.go.
