package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

const upsertProgress = `INSERT INTO read_progress (user_id, book_id, page, completed, read_at) VALUES (?, ?, ?, ?, ?)
	ON CONFLICT(user_id, book_id) DO UPDATE SET page = excluded.page, completed = excluded.completed, read_at = excluded.read_at`

func (s *server) SetBookProgress(ctx context.Context, req SetBookProgressRequestObject) (SetBookProgressResponseObject, error) {
	book, ok, err := s.oneBook(ctx, req.BookId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return SetBookProgress404JSONResponse{notFound("本が見つかりません")}, nil
	}
	b := req.Body
	if b == nil {
		return SetBookProgress400JSONResponse{badRequest("本文がありません")}, nil
	}
	if b.Page < 1 || b.Page > book.PageCount {
		return SetBookProgress400JSONResponse{badRequest("ページ番号が正しくありません")}, nil
	}
	p := ReadProgress{Page: b.Page, Completed: b.Page == book.PageCount, ReadAt: time.Now().UTC().Format(time.RFC3339)}
	if b.Completed != nil {
		p.Completed = *b.Completed
	}
	if _, err := s.DB.ExecContext(ctx, upsertProgress, currentUser(ctx).ID, book.Id, p.Page, p.Completed, p.ReadAt); err != nil {
		return nil, err
	}
	return SetBookProgress200JSONResponse(p), nil
}

func (s *server) ClearBookProgress(ctx context.Context, req ClearBookProgressRequestObject) (ClearBookProgressResponseObject, error) {
	ok, err := s.visible(ctx, "books", req.BookId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return ClearBookProgress404JSONResponse{notFound("本が見つかりません")}, nil
	}
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM read_progress WHERE user_id = ? AND book_id = ?`,
		currentUser(ctx).ID, req.BookId); err != nil {
		return nil, err
	}
	return ClearBookProgress204Response{}, nil
}

func (s *server) MarkSeriesRead(ctx context.Context, req MarkSeriesReadRequestObject) (MarkSeriesReadResponseObject, error) {
	ok, err := s.visible(ctx, "series", req.SeriesId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return MarkSeriesRead404JSONResponse{notFound("シリーズが見つかりません")}, nil
	}
	if err := s.markRead(ctx, "series_id", req.SeriesId, req.Body != nil && req.Body.Completed); err != nil {
		return nil, err
	}
	return MarkSeriesRead204Response{}, nil
}

func (s *server) MarkLibraryRead(ctx context.Context, req MarkLibraryReadRequestObject) (MarkLibraryReadResponseObject, error) {
	cond, args := libScope(ctx, "l.id")
	var one int
	err := s.DB.QueryRowContext(ctx, `SELECT 1 FROM libraries l WHERE l.id = ? AND `+cond, append([]any{req.LibraryId}, args...)...).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return MarkLibraryRead404JSONResponse{notFound("ライブラリが見つかりません")}, nil
	}
	if err != nil {
		return nil, err
	}
	if err := s.markRead(ctx, "library_id", req.LibraryId, req.Body != nil && req.Body.Completed); err != nil {
		return nil, err
	}
	return MarkLibraryRead204Response{}, nil
}

// markRead marks every live book whose col (series_id or library_id) is id
// read, or clears the caller's progress on them. The caller checked access.
func (s *server) markRead(ctx context.Context, col, id string, completed bool) error {
	uid := currentUser(ctx).ID
	var err error
	if completed {
		// Read on the book's last page, or page 1 for a book with no pages.
		_, err = s.DB.ExecContext(ctx, `INSERT INTO read_progress (user_id, book_id, page, completed, read_at)
			SELECT ?, b.id, max(b.page_count, 1), 1, ? FROM books b WHERE b.`+col+` = ? AND b.deleted_at IS NULL
			ON CONFLICT(user_id, book_id) DO UPDATE SET page = excluded.page, completed = 1, read_at = excluded.read_at`,
			uid, time.Now().UTC().Format(time.RFC3339), id)
	} else {
		_, err = s.DB.ExecContext(ctx, `DELETE FROM read_progress WHERE user_id = ?
			AND book_id IN (SELECT id FROM books WHERE `+col+` = ?)`, uid, id)
	}
	return err
}

var defaultGlobalPrefs = GlobalReaderPrefs{
	Direction: GlobalReaderPrefsDirectionSeries,
	Spread:    GlobalReaderPrefsSpreadOff,
	Fit:       GlobalReaderPrefsFitScreen,
	Bg:        GlobalReaderPrefsBgBlack,
}

// visiblePrefs loads the caller's reader prefs of one scope, keeping only the
// targets (series or books, in table) that the caller can see.
func (s *server) visiblePrefs(ctx context.Context, scope, table string, dst map[string]ReaderPrefs) error {
	cond, args := libScope(ctx, "t.library_id")
	rows, err := s.DB.QueryContext(ctx, `SELECT rp.target, rp.prefs FROM reader_prefs rp
		WHERE rp.user_id = ? AND rp.scope = ? AND rp.target IN (SELECT t.id FROM `+table+` t WHERE t.deleted_at IS NULL AND `+cond+`)`,
		append([]any{currentUser(ctx).ID, scope}, args...)...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var target, raw string
		if err := rows.Scan(&target, &raw); err != nil {
			return err
		}
		var p ReaderPrefs
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			return err
		}
		dst[target] = p
	}
	return rows.Err()
}

func (s *server) GetReaderPrefs(ctx context.Context, req GetReaderPrefsRequestObject) (GetReaderPrefsResponseObject, error) {
	out := ReaderPrefsAll{Global: defaultGlobalPrefs, Series: map[string]ReaderPrefs{}, Books: map[string]ReaderPrefs{}}
	var raw string
	err := s.DB.QueryRowContext(ctx, `SELECT prefs FROM reader_prefs WHERE user_id = ? AND scope = 'global' AND target = ''`,
		currentUser(ctx).ID).Scan(&raw)
	switch {
	case err == nil:
		if err := json.Unmarshal([]byte(raw), &out.Global); err != nil {
			return nil, err
		}
	case !errors.Is(err, sql.ErrNoRows):
		return nil, err
	}
	if err := s.visiblePrefs(ctx, "series", "series", out.Series); err != nil {
		return nil, err
	}
	if err := s.visiblePrefs(ctx, "book", "books", out.Books); err != nil {
		return nil, err
	}
	return GetReaderPrefs200JSONResponse(out), nil
}

func (s *server) SetGlobalReaderPrefs(ctx context.Context, req SetGlobalReaderPrefsRequestObject) (SetGlobalReaderPrefsResponseObject, error) {
	b := req.Body
	if b == nil {
		return SetGlobalReaderPrefs400JSONResponse{badRequest("本文がありません")}, nil
	}
	if !b.Direction.Valid() || !b.Spread.Valid() || !b.Fit.Valid() || !b.Bg.Valid() {
		return SetGlobalReaderPrefs400JSONResponse{badRequest("読書設定の値が正しくありません")}, nil
	}
	if err := s.putPrefs(ctx, "global", "", jsonText(b)); err != nil {
		return nil, err
	}
	return SetGlobalReaderPrefs204Response{}, nil
}

// putPrefs upserts one reader_prefs row for the caller.
func (s *server) putPrefs(ctx context.Context, scope, target, prefs string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO reader_prefs (user_id, scope, target, prefs) VALUES (?, ?, ?, ?)
		ON CONFLICT(user_id, scope, target) DO UPDATE SET prefs = excluded.prefs`,
		currentUser(ctx).ID, scope, target, prefs)
	return err
}

// checkPrefs reports whether a series or book override holds only known values.
func checkPrefs(b *ReaderPrefs) bool {
	return b != nil && b.Direction.Valid() && b.Spread.Valid() && b.Fit.Valid() && b.Bg.Valid()
}

func (s *server) SetSeriesReaderPrefs(ctx context.Context, req SetSeriesReaderPrefsRequestObject) (SetSeriesReaderPrefsResponseObject, error) {
	ok, err := s.visible(ctx, "series", req.SeriesId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return SetSeriesReaderPrefs404JSONResponse{notFound("シリーズが見つかりません")}, nil
	}
	if !checkPrefs(req.Body) {
		return SetSeriesReaderPrefs400JSONResponse{badRequest("読書設定の値が正しくありません")}, nil
	}
	if err := s.putPrefs(ctx, "series", req.SeriesId, jsonText(req.Body)); err != nil {
		return nil, err
	}
	return SetSeriesReaderPrefs204Response{}, nil
}

func (s *server) DeleteSeriesReaderPrefs(ctx context.Context, req DeleteSeriesReaderPrefsRequestObject) (DeleteSeriesReaderPrefsResponseObject, error) {
	ok, err := s.visible(ctx, "series", req.SeriesId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return DeleteSeriesReaderPrefs404JSONResponse{notFound("シリーズが見つかりません")}, nil
	}
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM reader_prefs WHERE user_id = ? AND scope = 'series' AND target = ?`,
		currentUser(ctx).ID, req.SeriesId); err != nil {
		return nil, err
	}
	return DeleteSeriesReaderPrefs204Response{}, nil
}

func (s *server) SetBookReaderPrefs(ctx context.Context, req SetBookReaderPrefsRequestObject) (SetBookReaderPrefsResponseObject, error) {
	ok, err := s.visible(ctx, "books", req.BookId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return SetBookReaderPrefs404JSONResponse{notFound("本が見つかりません")}, nil
	}
	if !checkPrefs(req.Body) {
		return SetBookReaderPrefs400JSONResponse{badRequest("読書設定の値が正しくありません")}, nil
	}
	if err := s.putPrefs(ctx, "book", req.BookId, jsonText(req.Body)); err != nil {
		return nil, err
	}
	return SetBookReaderPrefs204Response{}, nil
}

func (s *server) DeleteBookReaderPrefs(ctx context.Context, req DeleteBookReaderPrefsRequestObject) (DeleteBookReaderPrefsResponseObject, error) {
	ok, err := s.visible(ctx, "books", req.BookId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return DeleteBookReaderPrefs404JSONResponse{notFound("本が見つかりません")}, nil
	}
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM reader_prefs WHERE user_id = ? AND scope = 'book' AND target = ?`,
		currentUser(ctx).ID, req.BookId); err != nil {
		return nil, err
	}
	return DeleteBookReaderPrefs204Response{}, nil
}
