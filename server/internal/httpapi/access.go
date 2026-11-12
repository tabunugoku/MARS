package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
)

type access int

const (
	anyone access = iota + 1 // signed in, any role (sign-in itself is checked in authenticate)
	adminOnly
)

// opAccess classifies every operation. One that is missing here is refused
// for everyone (TestEveryOperationClassified catches it first), so a new
// endpoint can never be open to normal users by accident.
var opAccess = map[string]access{
	// Public or own account
	"GetHealth": anyone, "GetLoginScreen": anyone, "GetPublicCover": anyone,
	"Login": anyone, "Logout": anyone, "Setup": anyone,
	"GetMe": anyone, "UpdateMe": anyone, "ChangePassword": anyone,
	"ListApiKeys": anyone, "CreateApiKey": anyone, "DeleteApiKey": anyone,
	"GetReaderPrefs": anyone, "SetGlobalReaderPrefs": anyone,
	"SetSeriesReaderPrefs": anyone, "DeleteSeriesReaderPrefs": anyone,
	"SetBookReaderPrefs": anyone, "DeleteBookReaderPrefs": anyone,
	// Live changes; each event is filtered per user in serveEvents (eventFor).
	"StreamEvents": anyone,

	// Reading (always within libScope)
	"ListLibraries": anyone, "GetLibrary": anyone,
	"ListSeries": anyone, "ListGenres": anyone, "GetSeries": anyone, "ListSeriesBooks": anyone, "GetSeriesThumbnail": anyone,
	"GetBook": anyone, "GetBookThumbnail": anyone, "ListBookPages": anyone, "GetBookPage": anyone, "GetBookPageThumbnail": anyone,
	"GetAdjacentBooks": anyone, "GetDashboard": anyone, "Search": anyone,
	"SetBookProgress": anyone, "MarkLibraryRead": anyone, "ClearBookProgress": anyone, "MarkSeriesRead": anyone,
	"ListCollections": anyone, "GetCollection": anyone, "ListCollectionSeries": anyone,
	"ListReadLists": anyone, "GetReadList": anyone, "ListReadListBooks": anyone,
	// Hand-routed downloads: the handlers check canDownload after visibility.
	"DownloadBook": anyone, "DownloadSeries": anyone,

	// Administration
	"CreateLibrary": adminOnly, "UpdateLibrary": adminOnly, "DeleteLibrary": adminOnly, "ScanLibrary": adminOnly,
	"AnalyzeBook": adminOnly, "ListTasks": adminOnly, "CancelTask": adminOnly, "ListMediaIssues": adminOnly, "EmptyTrash": adminOnly,
	"ListUsers": adminOnly, "CreateUser": adminOnly, "UpdateUser": adminOnly, "DeleteUser": adminOnly,
	"GetSettings": adminOnly, "UpdateSettings": adminOnly,
	"UpdateSeries": adminOnly, "UpdateBook": adminOnly,
	"RegenerateBookThumbnail": adminOnly, "SetBookCover": adminOnly,
	"RegenerateSeriesThumbnails": adminOnly, "RegenerateLibraryThumbnails": adminOnly,
	"CreateCollection": adminOnly, "UpdateCollection": adminOnly, "DeleteCollection": adminOnly,
	"CreateReadList": adminOnly, "UpdateReadList": adminOnly, "DeleteReadList": adminOnly,
	"AddCollectionSeries": adminOnly, "RemoveCollectionSeries": adminOnly, "ReorderCollection": adminOnly,
	"AddReadListBook": adminOnly, "RemoveReadListBook": adminOnly, "ReorderReadList": adminOnly,
}

func requireAdmin(f StrictHandlerFunc, op string) StrictHandlerFunc {
	switch opAccess[op] {
	case anyone:
		return f
	case adminOnly:
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request, req any) (any, error) {
			if !currentUser(ctx).IsAdmin() {
				writeError(r.Context(), w, http.StatusForbidden, "forbidden.admin_only")
				return nil, nil
			}
			return f(ctx, w, r, req)
		}
	}
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request, req any) (any, error) {
		slog.Error("operation has no access rule", "op", op)
		writeError(r.Context(), w, http.StatusForbidden, "forbidden.operation")
		return nil, nil
	}
}

// libScope is the one place that decides which libraries the caller may
// see. It returns an SQL condition on col (a library id column) and its
// arguments. Every query that returns library content goes through it, and
// anything outside the scope answers 404 as if it did not exist.
func libScope(ctx context.Context, col string) (string, []any) {
	u, ok := userFrom(ctx)
	switch {
	case !ok || u.ID == "":
		return "0", nil
	case u.IsAdmin() || u.AllLibraries:
		return "1", nil
	}
	return col + ` IN (SELECT library_id FROM user_libraries WHERE user_id = ?)`, []any{u.ID}
}

// visible reports whether the row id in table (books or series) exists, is
// not in the trash and lies in a library the caller may see.
func (s *server) visible(ctx context.Context, table, id string) (bool, error) {
	cond, args := libScope(ctx, "t.library_id")
	var one int
	err := s.DB.QueryRowContext(ctx, `SELECT 1 FROM `+table+` t WHERE t.id = ? AND t.deleted_at IS NULL AND `+cond,
		append([]any{id}, args...)...).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func canStream(ctx context.Context) bool {
	u := currentUser(ctx)
	return u.IsAdmin() || u.CanStream
}

// canDownload reports whether the caller may download book files
// (ファイルのダウンロード). Administrators always may.
func canDownload(ctx context.Context) bool {
	u := currentUser(ctx)
	return u.IsAdmin() || u.CanDownload
}
