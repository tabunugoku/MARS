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
		return Compass
	}
	var v struct {
		LoginBackdrop LoginBackdrop `json:"loginBackdrop"`
	}
	if json.Unmarshal([]byte(raw), &v) != nil || v.LoginBackdrop != Covers {
		return Compass
	}
	return Covers
}

// coverWall returns the series shown on the login screen: the newest live
// series that have a cover, from every library. It is empty unless the
// administrator chose the cover wall. Nothing else is public.
func (s *server) coverWall(ctx context.Context) ([]LoginCover, error) {
	out := []LoginCover{}
	if s.loginBackdrop(ctx) != Covers {
		return out, nil
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT s.id, s.title FROM series s
		WHERE s.deleted_at IS NULL AND EXISTS (
		  SELECT 1 FROM books b WHERE b.series_id = s.id AND b.deleted_at IS NULL AND b.thumbnail != '')
		ORDER BY s.created_at DESC, s.id LIMIT ?`, coverWallSize)
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

// GetLoginScreen is public: it returns covers only when an admin chose the
// cover wall.
func (s *server) GetLoginScreen(ctx context.Context, _ GetLoginScreenRequestObject) (GetLoginScreenResponseObject, error) {
	covers, err := s.coverWall(ctx)
	if err != nil {
		return nil, err
	}
	backdrop := Compass
	if len(covers) > 0 {
		backdrop = Covers
	}
	return GetLoginScreen200JSONResponse{Backdrop: backdrop, Covers: covers, NeedsSetup: s.Gate.open()}, nil
}

// servePublicCover serves a cover without sign-in, but only one that is on
// the cover wall right now.
func (s *server) servePublicCover(w http.ResponseWriter, r *http.Request) {
	covers, err := s.coverWall(r.Context())
	if err != nil {
		writeError(r.Context(), w, http.StatusInternalServerError, "server.cover_unreadable")
		return
	}
	want := "/api/v1/public/covers/" + r.PathValue("seriesId")
	for _, c := range covers {
		if c.ThumbnailUrl == want {
			s.sendThumbnail(w, r, "series", r.PathValue("seriesId"))
			return
		}
	}
	writeError(r.Context(), w, http.StatusNotFound, "not_found.cover")
}
