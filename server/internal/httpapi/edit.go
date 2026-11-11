package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/tabunugoku/cominavi/server/internal/scan"
)

// Lock names are the fields the scanner checks in library.go. A locked field
// is left alone by scans and by ComicInfo.xml.
var (
	seriesLockNames = []string{"title", "titleSort", "alternateTitles", "status", "summary", "publisher",
		"readingDirection", "ageRating", "language", "genres", "tags", "authors", "totalBookCount"}
	bookLockNames = []string{"title", "number", "summary", "releaseDate", "isbn", "authors", "tags"}
)

// lockJSON is the lock list after an edit: lockedFields when given (already
// checked), otherwise the current locks plus the fields just edited.
func lockJSON(cur string, edited []string, explicit *[]string) string {
	if explicit != nil {
		return jsonText(orEmpty(*explicit))
	}
	out := strs(cur)
	for _, f := range edited {
		if !slices.Contains(out, f) {
			out = append(out, f)
		}
	}
	return jsonText(out)
}

// badLock names the first lock that is not a known field, or "".
func badLock(list *[]string, known []string) string {
	if list == nil {
		return ""
	}
	for _, f := range *list {
		if !slices.Contains(known, f) {
			return "ロックできない項目があります: " + f
		}
	}
	return ""
}

func badAuthors(list []Author) string {
	for _, a := range list {
		if strings.TrimSpace(a.Name) == "" || !a.Role.Valid() {
			return "作者の名前と役割を入力してください"
		}
	}
	return ""
}

// validReleaseDate accepts the forms ComicInfo.xml gives: YYYY-MM-DD, YYYY-MM or YYYY.
func validReleaseDate(v string) bool {
	for _, layout := range []string{"2006-01-02", "2006-01", "2006"} {
		if _, err := time.Parse(layout, v); err == nil {
			return true
		}
	}
	return false
}

func orEmpty[T any](v []T) []T {
	if v == nil {
		return []T{}
	}
	return v
}

// queryRower is what loadSettings needs: a *sql.DB or a *sql.Tx.
type queryRower interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func defaultSettings() ServerSettings {
	return ServerSettings{
		Port:                   25600,
		TaskThreads:            2,
		ThumbnailSize:          ServerSettingsThumbnailSizeMedium,
		ThumbnailFormat:        ServerSettingsThumbnailFormatJpeg,
		RememberMeDays:         30,
		DeleteEmptyCollections: true,
		LoginBackdrop:          Compass,
	}
}

// settingsProblem names the first invalid value in a complete settings
// object, or "" when all are valid.
func settingsProblem(v ServerSettings) string {
	switch {
	case v.Port < 1 || v.Port > 65535:
		return "ポート番号は1〜65535にしてください"
	case v.TaskThreads < 1 || v.TaskThreads > 16:
		return "同時に処理する数は1〜16にしてください"
	case v.RememberMeDays < 1 || v.RememberMeDays > 3650:
		return "ログインを保つ日数は1〜3650日にしてください"
	case !v.LoginBackdrop.Valid():
		return "ログイン画面の背景が正しくありません"
	case !v.ThumbnailSize.Valid():
		return "表紙のサイズが正しくありません"
	case v.ThumbnailFormat != ServerSettingsThumbnailFormatJpeg:
		return "この形式にはまだ対応していません"
	}
	return ""
}

// loadSettings returns the stored settings merged over the defaults. A
// stored value that does not parse or validate counts as corrupt: the
// defaults are returned instead.
func loadSettings(ctx context.Context, q queryRower) (ServerSettings, error) {
	var raw string
	err := q.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'server'`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return defaultSettings(), nil
	}
	if err != nil {
		return defaultSettings(), err
	}
	v := defaultSettings()
	if json.Unmarshal([]byte(raw), &v) != nil || settingsProblem(v) != "" {
		return defaultSettings(), nil
	}
	return v, nil
}

func checkSeriesUpdate(b *SeriesUpdate) string {
	switch {
	case b.Title != nil && strings.TrimSpace(*b.Title) == "":
		return "タイトルを入力してください"
	case b.TitleSort != nil && strings.TrimSpace(*b.TitleSort) == "":
		return "並び順のタイトルを入力してください"
	case b.Status != nil && !b.Status.Valid():
		return "状態が正しくありません"
	case b.ReadingDirection != nil && !b.ReadingDirection.Valid():
		return "読む方向が正しくありません"
	case b.AgeRating.Value != nil && (*b.AgeRating.Value < 0 || *b.AgeRating.Value > 99):
		return "年齢制限は0〜99にしてください"
	case b.TotalBookCount.Value != nil && *b.TotalBookCount.Value < 0:
		return "巻数は0以上にしてください"
	case b.Authors != nil && badAuthors(*b.Authors) != "":
		return badAuthors(*b.Authors)
	}
	return badLock(b.LockedFields, seriesLockNames)
}

func checkBookUpdate(b *BookUpdate) string {
	switch {
	case b.Title != nil && strings.TrimSpace(*b.Title) == "":
		return "タイトルを入力してください"
	case b.ReleaseDate.Value != nil && !validReleaseDate(strings.TrimSpace(*b.ReleaseDate.Value)):
		return "発売日は YYYY-MM-DD、YYYY-MM、YYYY のいずれかで入力してください"
	case b.Authors != nil && badAuthors(*b.Authors) != "":
		return badAuthors(*b.Authors)
	}
	return badLock(b.LockedFields, bookLockNames)
}

// UpdateSeries sets the given fields and locks them against scans. Every
// write and the search rebuild share one transaction.
func (s *server) UpdateSeries(ctx context.Context, req UpdateSeriesRequestObject) (UpdateSeriesResponseObject, error) {
	if ok, err := s.visible(ctx, "series", req.SeriesId); err != nil {
		return nil, err
	} else if !ok {
		return UpdateSeries404JSONResponse{notFound("シリーズが見つかりません")}, nil
	}
	b := req.Body
	if b == nil {
		return UpdateSeries400JSONResponse{badRequest("本文がありません")}, nil
	}
	if msg := checkSeriesUpdate(b); msg != "" {
		return UpdateSeries400JSONResponse{badRequest(msg)}, nil
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var libraryID, locks string
	err = tx.QueryRowContext(ctx, `SELECT library_id, locked_fields FROM series WHERE id = ? AND deleted_at IS NULL`, req.SeriesId).
		Scan(&libraryID, &locks)
	if errors.Is(err, sql.ErrNoRows) {
		return UpdateSeries404JSONResponse{notFound("シリーズが見つかりません")}, nil
	}
	if err != nil {
		return nil, err
	}

	var set []string
	var args []any
	var edited []string
	field := func(name, col string, v any) {
		set, args = append(set, col+" = ?"), append(args, v)
		edited = append(edited, name)
	}
	if b.Title != nil {
		title := strings.TrimSpace(*b.Title)
		field("title", "title", title)
		if b.TitleSort == nil {
			set, args = append(set, "title_sort = ?"), append(args, scan.SortTitle(title))
		}
	}
	if b.TitleSort != nil {
		field("titleSort", "title_sort", strings.TrimSpace(*b.TitleSort))
	}
	if b.AlternateTitles != nil {
		field("alternateTitles", "alternate_titles", jsonText(orEmpty(*b.AlternateTitles)))
	}
	if b.Status != nil {
		field("status", "status", string(*b.Status))
	}
	if b.Summary != nil {
		field("summary", "summary", *b.Summary)
	}
	if b.Publisher != nil {
		field("publisher", "publisher", *b.Publisher)
	}
	if b.ReadingDirection != nil {
		field("readingDirection", "reading_direction", string(*b.ReadingDirection))
	}
	if b.AgeRating.Set {
		field("ageRating", "age_rating", b.AgeRating.Value)
	}
	if b.Language != nil {
		field("language", "language", *b.Language)
	}
	if b.Genres != nil {
		field("genres", "genres", jsonText(orEmpty(*b.Genres)))
	}
	if b.Tags != nil {
		field("tags", "tags", jsonText(orEmpty(*b.Tags)))
	}
	if b.Authors != nil {
		field("authors", "authors", jsonText(orEmpty(*b.Authors)))
	}
	if b.TotalBookCount.Set {
		field("totalBookCount", "total_book_count", b.TotalBookCount.Value)
	}
	set = append(set, "locked_fields = ?", "updated_at = ?")
	args = append(args, lockJSON(locks, edited, b.LockedFields), time.Now().UTC().Format(time.RFC3339Nano), req.SeriesId)
	if _, err := tx.ExecContext(ctx, `UPDATE series SET `+strings.Join(set, ", ")+` WHERE id = ?`, args...); err != nil {
		return nil, err
	}
	if err := scan.RebuildSearch(ctx, tx, libraryID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	x, ok, err := s.oneSeries(ctx, req.SeriesId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return UpdateSeries404JSONResponse{notFound("シリーズが見つかりません")}, nil
	}
	return UpdateSeries200JSONResponse(x), nil
}

// UpdateBook is UpdateSeries for a book. number and numberSort lock together
// under "number", as the scanner treats them.
func (s *server) UpdateBook(ctx context.Context, req UpdateBookRequestObject) (UpdateBookResponseObject, error) {
	if ok, err := s.visible(ctx, "books", req.BookId); err != nil {
		return nil, err
	} else if !ok {
		return UpdateBook404JSONResponse{notFound("本が見つかりません")}, nil
	}
	b := req.Body
	if b == nil {
		return UpdateBook400JSONResponse{badRequest("本文がありません")}, nil
	}
	if msg := checkBookUpdate(b); msg != "" {
		return UpdateBook400JSONResponse{badRequest(msg)}, nil
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var libraryID, locks string
	err = tx.QueryRowContext(ctx, `SELECT library_id, locked_fields FROM books WHERE id = ? AND deleted_at IS NULL`, req.BookId).
		Scan(&libraryID, &locks)
	if errors.Is(err, sql.ErrNoRows) {
		return UpdateBook404JSONResponse{notFound("本が見つかりません")}, nil
	}
	if err != nil {
		return nil, err
	}

	var set []string
	var args []any
	var edited []string
	field := func(name, col string, v any) {
		set, args = append(set, col+" = ?"), append(args, v)
		edited = append(edited, name)
	}
	if b.Title != nil {
		field("title", "title", strings.TrimSpace(*b.Title))
	}
	if b.Number != nil {
		field("number", "number", *b.Number)
		if b.NumberSort == nil {
			set, args = append(set, "number_sort = ?"), append(args, *b.Number)
		}
	}
	if b.NumberSort != nil {
		field("number", "number_sort", *b.NumberSort)
	}
	if b.Summary != nil {
		field("summary", "summary", *b.Summary)
	}
	if b.ReleaseDate.Set {
		var rd any // null clears the date
		if b.ReleaseDate.Value != nil {
			rd = strings.TrimSpace(*b.ReleaseDate.Value)
		}
		field("releaseDate", "release_date", rd)
	}
	if b.Isbn != nil {
		field("isbn", "isbn", strings.TrimSpace(*b.Isbn))
	}
	if b.Authors != nil {
		field("authors", "authors", jsonText(orEmpty(*b.Authors)))
	}
	if b.Tags != nil {
		field("tags", "tags", jsonText(orEmpty(*b.Tags)))
	}
	set = append(set, "locked_fields = ?", "updated_at = ?")
	args = append(args, lockJSON(locks, edited, b.LockedFields), time.Now().UTC().Format(time.RFC3339Nano), req.BookId)
	if _, err := tx.ExecContext(ctx, `UPDATE books SET `+strings.Join(set, ", ")+` WHERE id = ?`, args...); err != nil {
		return nil, err
	}
	if err := scan.RebuildSearch(ctx, tx, libraryID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	x, ok, err := s.oneBook(ctx, req.BookId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return UpdateBook404JSONResponse{notFound("本が見つかりません")}, nil
	}
	return UpdateBook200JSONResponse(x), nil
}

func (s *server) GetSettings(ctx context.Context, _ GetSettingsRequestObject) (GetSettingsResponseObject, error) {
	cur, err := loadSettings(ctx, s.DB)
	if err != nil {
		return nil, err
	}
	return GetSettings200JSONResponse(cur), nil
}

// UpdateSettings merges the given fields over the stored settings and saves
// the whole object. Read and write share one transaction, so two updates
// cannot undo each other.
func (s *server) UpdateSettings(ctx context.Context, req UpdateSettingsRequestObject) (UpdateSettingsResponseObject, error) {
	b := req.Body
	if b == nil {
		return UpdateSettings400JSONResponse{badRequest("本文がありません")}, nil
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	cur, err := loadSettings(ctx, tx)
	if err != nil {
		return nil, err
	}
	if b.DeleteEmptyCollections != nil {
		cur.DeleteEmptyCollections = *b.DeleteEmptyCollections
	}
	if b.KoboSyncEnabled != nil {
		cur.KoboSyncEnabled = *b.KoboSyncEnabled
	}
	if b.LoginBackdrop != nil {
		cur.LoginBackdrop = *b.LoginBackdrop
	}
	if b.OpdsEnabled != nil {
		cur.OpdsEnabled = *b.OpdsEnabled
	}
	if b.Port != nil {
		cur.Port = *b.Port
	}
	if b.RememberMeDays != nil {
		cur.RememberMeDays = *b.RememberMeDays
	}
	if b.TaskThreads != nil {
		cur.TaskThreads = *b.TaskThreads
	}
	if b.ThumbnailFormat != nil {
		cur.ThumbnailFormat = ServerSettingsThumbnailFormat(*b.ThumbnailFormat)
	}
	if b.ThumbnailSize != nil {
		cur.ThumbnailSize = ServerSettingsThumbnailSize(*b.ThumbnailSize)
	}
	if msg := settingsProblem(cur); msg != "" {
		return UpdateSettings400JSONResponse{badRequest(msg)}, nil
	}
	raw, err := json.Marshal(cur)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES ('server', ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, string(raw)); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return UpdateSettings200JSONResponse(cur), nil
}

// EmptyTrash hard-deletes trashed books, then trashed series. Deleting a
// series cascades to its live books and their pages, progress and list
// entries; those cascaded rows are not counted, only the rows named directly.
func (s *server) EmptyTrash(ctx context.Context, _ EmptyTrashRequestObject) (EmptyTrashResponseObject, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	// Cover files of every book that goes, including those cascaded away.
	rows, err := tx.QueryContext(ctx, `SELECT thumbnail FROM books WHERE thumbnail != '' AND
		(deleted_at IS NOT NULL OR series_id IN (SELECT id FROM series WHERE deleted_at IS NOT NULL))`)
	if err != nil {
		return nil, err
	}
	var thumbs []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return nil, err
		}
		thumbs = append(thumbs, name)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var res TrashResult
	if res.Books, err = execCount(ctx, tx, `DELETE FROM books WHERE deleted_at IS NOT NULL`); err != nil {
		return nil, err
	}
	if res.Series, err = execCount(ctx, tx, `DELETE FROM series WHERE deleted_at IS NOT NULL`); err != nil {
		return nil, err
	}
	// Search rows for the removed rows (the sync keeps the rest current).
	if _, err := tx.ExecContext(ctx, `DELETE FROM search_index
		WHERE (kind = 'book' AND ref NOT IN (SELECT id FROM books))
		   OR (kind = 'series' AND ref NOT IN (SELECT id FROM series))`); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	// Files go only after the rows are gone. A file left behind is harmless
	// (its name is unique to a removed book), so errors are ignored.
	for _, name := range thumbs {
		_ = os.Remove(filepath.Join(s.Library.ThumbDir, filepath.Base(name)))
	}
	return EmptyTrash200JSONResponse(res), nil
}

func execCount(ctx context.Context, tx *sql.Tx, q string) (int, error) {
	res, err := tx.ExecContext(ctx, q)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return int(n), err
}

// GetPublicCover is served by servePublicCover (public.go) on the outer mux.
func (s *server) GetPublicCover(context.Context, GetPublicCoverRequestObject) (GetPublicCoverResponseObject, error) {
	return GetPublicCover404JSONResponse{notFound("not routed")}, nil
}

// AnalyzeBook marks the book outdated and queues its library's analysis,
// which picks up every outdated book.
func (s *server) AnalyzeBook(ctx context.Context, req AnalyzeBookRequestObject) (AnalyzeBookResponseObject, error) {
	var lib string
	err := s.DB.QueryRowContext(ctx, `UPDATE books SET media_status = 'outdated' WHERE id = ? AND deleted_at IS NULL RETURNING library_id`,
		req.BookId).Scan(&lib)
	if errors.Is(err, sql.ErrNoRows) {
		return AnalyzeBook404JSONResponse{notFound("本が見つかりません")}, nil
	}
	if err != nil {
		return nil, err
	}
	t, err := s.Library.QueueAnalyze(ctx, lib)
	if err != nil {
		return nil, err
	}
	return AnalyzeBook202JSONResponse(toTask(t)), nil
}
