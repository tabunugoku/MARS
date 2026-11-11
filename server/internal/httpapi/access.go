package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
)

// adminOnly lists the operations a normal user may not call. They get 403.
var adminOnly = map[string]bool{
	"CreateLibrary": true, "UpdateLibrary": true, "DeleteLibrary": true, "ScanLibrary": true,
	"ListTasks": true, "CancelTask": true, "ListMediaIssues": true,
	"ListUsers": true, "CreateUser": true, "UpdateUser": true, "DeleteUser": true,
}

func requireAdmin(f StrictHandlerFunc, op string) StrictHandlerFunc {
	if !adminOnly[op] {
		return f
	}
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request, req any) (any, error) {
		if !currentUser(ctx).IsAdmin() {
			writeError(w, http.StatusForbidden, "管理者だけが使える操作です")
			return nil, nil
		}
		return f(ctx, w, r, req)
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
