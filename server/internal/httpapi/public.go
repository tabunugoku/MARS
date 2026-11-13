package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
)

// coverWallSize is how many covers the login screen shows at most.
const coverWallSize = 24

// loginBackdrop reads the administrator's choice of login backdrop.
func (s *server) loginBackdrop(ctx context.Context) LoginBackdrop {
	var raw string
	if err := s.DB.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'server'`).Scan(&raw); err != nil {
		return Planet
	}
	var v struct {
		LoginBackdrop LoginBackdrop `json:"loginBackdrop"`
	}
	if json.Unmarshal([]byte(raw), &v) != nil {
		return Planet
	}
	switch v.LoginBackdrop {
	case Covers, BookCovers:
		return v.LoginBackdrop
	}
	return Planet
}

// coverWall returns the covers shown on the login screen: for Covers the
// newest live series that have a cover, and for BookCovers the newest live
// books that have one, from live series only, from every library. It is
// empty unless the administrator chose a cover wall. Nothing else is public.
func (s *server) coverWall(ctx context.Context) ([]LoginCover, error) {
	out := []LoginCover{}
	var query string
	switch s.loginBackdrop(ctx) {
	case Covers:
		query = `
		SELECT s.id, s.title FROM series s
		WHERE s.deleted_at IS NULL AND s.library_id IN (SELECT id FROM libraries WHERE deleting = 0) AND EXISTS (
		  SELECT 1 FROM books b WHERE b.series_id = s.id AND b.deleted_at IS NULL AND b.thumbnail != '')
		ORDER BY s.created_at DESC, s.id LIMIT ?`
	case BookCovers:
		query = `
		SELECT b.id, s.title || ' ' || b.title FROM books b
		JOIN series s ON s.id = b.series_id
		WHERE b.deleted_at IS NULL AND b.thumbnail != '' AND s.deleted_at IS NULL AND b.library_id IN (SELECT id FROM libraries WHERE deleting = 0)
		ORDER BY b.created_at DESC, b.id LIMIT ?`
	default:
		return out, nil
	}
	rows, err := s.DB.QueryContext(ctx, query, coverWallSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, title string
		if err := rows.Scan(&id, &title); err != nil {
			return nil, err
		}
		out = append(out, LoginCover{Title: title, ThumbnailUrl: "/api/v1/public/covers/" + id})
	}
	return out, rows.Err()
}

// GetLoginScreen is public: it returns covers only when an admin chose a
// cover wall.
func (s *server) GetLoginScreen(ctx context.Context, _ GetLoginScreenRequestObject) (GetLoginScreenResponseObject, error) {
	covers, err := s.coverWall(ctx)
	if err != nil {
		return nil, err
	}
	backdrop := Planet
	if len(covers) > 0 {
		backdrop = s.loginBackdrop(ctx)
	}
	return GetLoginScreen200JSONResponse{Backdrop: backdrop, Covers: covers, NeedsSetup: s.Gate.open()}, nil
}

// servePublicCover serves a cover without sign-in, but only one that is on
// the cover wall right now. The id is a series id under Covers and a book
// id under BookCovers.
func (s *server) servePublicCover(w http.ResponseWriter, r *http.Request) {
	covers, err := s.coverWall(r.Context())
	if err != nil {
		writeError(r.Context(), w, http.StatusInternalServerError, "server.cover_unreadable")
		return
	}
	want := "/api/v1/public/covers/" + r.PathValue("seriesId")
	for _, c := range covers {
		if c.ThumbnailUrl == want {
			kind := "series"
			if s.loginBackdrop(r.Context()) == BookCovers {
				kind = "book"
			}
			s.sendThumbnail(w, r, kind, r.PathValue("seriesId"), false)
			return
		}
	}
	writeError(r.Context(), w, http.StatusNotFound, "not_found.cover")
}
