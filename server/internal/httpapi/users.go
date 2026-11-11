package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/tabunugoku/cominavi/server/internal/auth"
)

// Login, Logout and Setup set or clear the session cookie, so they are
// routed by hand (see New); these exist only to satisfy the interface.
func (s *server) Login(context.Context, LoginRequestObject) (LoginResponseObject, error) {
	return nil, errors.New("login is routed by hand")
}
func (s *server) Logout(context.Context, LogoutRequestObject) (LogoutResponseObject, error) {
	return nil, errors.New("logout is routed by hand")
}
func (s *server) Setup(context.Context, SetupRequestObject) (SetupResponseObject, error) {
	return nil, errors.New("setup is routed by hand")
}

// ─── Reading users ───────────────────────────────────────────────────────────

const userCols = `id, name, email, role, can_download, can_stream, all_libraries, last_active`

func scanUser(r scanner) (User, error) {
	var u User
	var role string
	var last sql.NullString
	err := r.Scan(&u.Id, &u.Name, &u.Email, &role, &u.CanDownload, &u.CanStream, &u.AllLibraries, &last)
	u.Role = Role(role)
	if last.Valid {
		u.LastActive = &last.String
	}
	u.LibraryIds = []string{}
	return u, err
}

func (s *server) userByID(ctx context.Context, id string) (User, error) {
	u, err := scanUser(s.DB.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id = ?`, id))
	if err != nil {
		return u, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT library_id FROM user_libraries WHERE user_id = ? ORDER BY library_id`, id)
	if err != nil {
		return u, err
	}
	defer rows.Close()
	for rows.Next() {
		var l string
		if err := rows.Scan(&l); err != nil {
			return u, err
		}
		u.LibraryIds = append(u.LibraryIds, l)
	}
	return u, rows.Err()
}

func (s *server) GetMe(ctx context.Context, _ GetMeRequestObject) (GetMeResponseObject, error) {
	u, err := s.userByID(ctx, currentUser(ctx).ID)
	if err != nil {
		return nil, err
	}
	return GetMe200JSONResponse(u), nil
}

func (s *server) UpdateMe(ctx context.Context, req UpdateMeRequestObject) (UpdateMeResponseObject, error) {
	n := strings.TrimSpace(req.Body.Name)
	if msg := checkName(n); msg != "" {
		return UpdateMe400JSONResponse{badRequest(msg)}, nil
	}
	uid := currentUser(ctx).ID
	if _, err := s.DB.ExecContext(ctx, `UPDATE users SET name = ? WHERE id = ?`, n, uid); err != nil {
		return nil, err
	}
	u, err := s.userByID(ctx, uid)
	if err != nil {
		return nil, err
	}
	return UpdateMe200JSONResponse(u), nil
}

func (s *server) ListUsers(ctx context.Context, _ ListUsersRequestObject) (ListUsersResponseObject, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+userCols+` FROM users ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := ListUsers200JSONResponse{}
	index := map[string]int{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		index[u.Id] = len(out)
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	grants, err := s.DB.QueryContext(ctx, `SELECT user_id, library_id FROM user_libraries ORDER BY library_id`)
	if err != nil {
		return nil, err
	}
	defer grants.Close()
	for grants.Next() {
		var uid, lid string
		if err := grants.Scan(&uid, &lid); err != nil {
			return nil, err
		}
		if i, ok := index[uid]; ok {
			out[i].LibraryIds = append(out[i].LibraryIds, lid)
		}
	}
	return out, grants.Err()
}

// ─── Validation ──────────────────────────────────────────────────────────────

func checkName(n string) string {
	if c := utf8.RuneCountInString(n); c < 1 || c > 64 {
		return "名前は1〜64文字にしてください"
	}
	// No "@": sign-in accepts a name or an email, so a name must never
	// read as someone else's email.
	if strings.ContainsRune(n, '@') || strings.IndexFunc(n, unicode.IsControl) >= 0 {
		return "名前に @ や制御文字は使えません"
	}
	return ""
}

func checkEmail(e string) string {
	at := strings.IndexByte(e, '@')
	if len(e) > 254 || at < 1 || at != strings.LastIndexByte(e, '@') || at == len(e)-1 ||
		strings.IndexFunc(e, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return "メールアドレスの形式が正しくありません"
	}
	return ""
}

func validRole(r Role) bool { return r == RoleAdmin || r == RoleUser }

func uniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

const takenMsg = "その名前かメールアドレスはすでに使われています"

// checkLibraries reports a message when an id does not name a library.
func checkLibraries(ctx context.Context, tx *sql.Tx, idList []string) (string, error) {
	for _, id := range idList {
		var one int
		err := tx.QueryRowContext(ctx, `SELECT 1 FROM libraries WHERE id = ?`, id).Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			return "存在しないライブラリが指定されています", nil
		}
		if err != nil {
			return "", err
		}
	}
	return "", nil
}

func setLibraries(ctx context.Context, tx *sql.Tx, userID string, idList []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_libraries WHERE user_id = ?`, userID); err != nil {
		return err
	}
	for _, id := range idList {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO user_libraries (user_id, library_id) VALUES (?, ?)`, userID, id); err != nil {
			return err
		}
	}
	return nil
}

func boolOr(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

// insertUser validates and stores a new user. A non-empty msg is a 400.
func (s *server) insertUser(ctx context.Context, c UserCreate) (id, msg string, err error) {
	c.Name, c.Email = strings.TrimSpace(c.Name), strings.TrimSpace(c.Email)
	if msg = checkName(c.Name); msg != "" {
		return "", msg, nil
	}
	if msg = checkEmail(c.Email); msg != "" {
		return "", msg, nil
	}
	if !validRole(c.Role) {
		return "", "権限の種類が正しくありません", nil
	}
	hash, err := auth.HashPassword(c.Password)
	if errors.Is(err, auth.ErrWeakPassword) {
		return "", "パスワードは8〜256文字にしてください", nil
	}
	if err != nil {
		return "", "", err
	}
	var libs []string
	if c.LibraryIds != nil {
		libs = *c.LibraryIds
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback()
	if msg, err = checkLibraries(ctx, tx, libs); msg != "" || err != nil {
		return "", msg, err
	}
	id = newID()
	_, err = tx.ExecContext(ctx, `INSERT INTO users (id, name, email, password_hash, role, can_download, can_stream, all_libraries, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, c.Name, c.Email, hash, string(c.Role), boolOr(c.CanDownload, true), boolOr(c.CanStream, true), boolOr(c.AllLibraries, true),
		time.Now().UTC().Format(time.RFC3339))
	if uniqueViolation(err) {
		return "", takenMsg, nil
	}
	if err != nil {
		return "", "", err
	}
	if err := setLibraries(ctx, tx, id, libs); err != nil {
		return "", "", err
	}
	return id, "", tx.Commit()
}

// ─── Users (administrators) ─────────────────────────────────────────────────

func (s *server) CreateUser(ctx context.Context, req CreateUserRequestObject) (CreateUserResponseObject, error) {
	id, msg, err := s.insertUser(ctx, *req.Body)
	if err != nil {
		return nil, err
	}
	if msg != "" {
		return CreateUser400JSONResponse{badRequest(msg)}, nil
	}
	u, err := s.userByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return CreateUser201JSONResponse(u), nil
}

// adminsLeft counts administrators inside tx, after a change, so that the
// last one can be neither demoted nor deleted.
func adminsLeft(ctx context.Context, tx *sql.Tx) (int, error) {
	var n int
	err := tx.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE role = 'admin'`).Scan(&n)
	return n, err
}

const lastAdminMsg = "管理者が1人もいなくなるため、この変更はできません"

func (s *server) UpdateUser(ctx context.Context, req UpdateUserRequestObject) (UpdateUserResponseObject, error) {
	b := req.Body
	var set []string
	var args []any
	if b.Name != nil {
		n := strings.TrimSpace(*b.Name)
		if msg := checkName(n); msg != "" {
			return UpdateUser400JSONResponse{badRequest(msg)}, nil
		}
		set, args = append(set, "name = ?"), append(args, n)
	}
	if b.Email != nil {
		e := strings.TrimSpace(*b.Email)
		if msg := checkEmail(e); msg != "" {
			return UpdateUser400JSONResponse{badRequest(msg)}, nil
		}
		set, args = append(set, "email = ?"), append(args, e)
	}
	if b.Role != nil {
		if !validRole(*b.Role) {
			return UpdateUser400JSONResponse{badRequest("権限の種類が正しくありません")}, nil
		}
		set, args = append(set, "role = ?"), append(args, string(*b.Role))
	}
	if b.Password != nil {
		hash, err := auth.HashPassword(*b.Password)
		if errors.Is(err, auth.ErrWeakPassword) {
			return UpdateUser400JSONResponse{badRequest("パスワードは8〜256文字にしてください")}, nil
		}
		if err != nil {
			return nil, err
		}
		set, args = append(set, "password_hash = ?"), append(args, hash)
	}
	for col, v := range map[string]*bool{"can_download": b.CanDownload, "can_stream": b.CanStream, "all_libraries": b.AllLibraries} {
		if v != nil {
			set, args = append(set, col+" = ?"), append(args, *v)
		}
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var one int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM users WHERE id = ?`, req.UserId).Scan(&one); errors.Is(err, sql.ErrNoRows) {
		return UpdateUser404JSONResponse{notFound("ユーザーが見つかりません")}, nil
	} else if err != nil {
		return nil, err
	}
	if len(set) > 0 {
		_, err := tx.ExecContext(ctx, `UPDATE users SET `+strings.Join(set, ", ")+` WHERE id = ?`, append(args, req.UserId)...)
		if uniqueViolation(err) {
			return UpdateUser400JSONResponse{badRequest(takenMsg)}, nil
		}
		if err != nil {
			return nil, err
		}
	}
	if b.LibraryIds != nil {
		if msg, err := checkLibraries(ctx, tx, *b.LibraryIds); err != nil {
			return nil, err
		} else if msg != "" {
			return UpdateUser400JSONResponse{badRequest(msg)}, nil
		}
		if err := setLibraries(ctx, tx, req.UserId, *b.LibraryIds); err != nil {
			return nil, err
		}
	}
	if n, err := adminsLeft(ctx, tx); err != nil {
		return nil, err
	} else if n == 0 {
		return UpdateUser400JSONResponse{badRequest(lastAdminMsg)}, nil
	}
	if b.Password != nil {
		// A new password signs the user out everywhere, except the
		// administrator's own session when they reset their own password.
		keep := ""
		if req.UserId == currentUser(ctx).ID {
			keep, _ = ctx.Value(tokenKey).(string)
		}
		if err := deleteSessionsTx(ctx, tx, req.UserId, keep); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	u, err := s.userByID(ctx, req.UserId)
	if err != nil {
		return nil, err
	}
	return UpdateUser200JSONResponse(u), nil
}

func deleteSessionsTx(ctx context.Context, tx *sql.Tx, userID, keepToken string) error {
	if keepToken == "" {
		_, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ? AND token_hash <> ?`, userID, auth.HashToken(keepToken))
	return err
}

func (s *server) DeleteUser(ctx context.Context, req DeleteUserRequestObject) (DeleteUserResponseObject, error) {
	if req.UserId == currentUser(ctx).ID {
		return DeleteUser400JSONResponse{badRequest("自分自身は削除できません")}, nil
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, req.UserId)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return DeleteUser404JSONResponse{notFound("ユーザーが見つかりません")}, nil
	}
	if n, err := adminsLeft(ctx, tx); err != nil {
		return nil, err
	} else if n == 0 {
		return DeleteUser400JSONResponse{badRequest(lastAdminMsg)}, nil
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return DeleteUser204Response{}, nil
}

// ─── The signed-in user ──────────────────────────────────────────────────────

func (s *server) ChangePassword(ctx context.Context, req ChangePasswordRequestObject) (ChangePasswordResponseObject, error) {
	me := currentUser(ctx)
	key := "password:" + me.ID
	if ok, _ := s.Limiter.Begin(key, time.Now()); !ok {
		return ChangePassword400JSONResponse{badRequest("失敗が続いたため、しばらく変更できません")}, nil
	}
	failed := false
	defer func() { s.Limiter.End(key, failed, time.Now()) }()
	var hash string
	if err := s.DB.QueryRowContext(ctx, `SELECT password_hash FROM users WHERE id = ?`, me.ID).Scan(&hash); err != nil {
		return nil, err
	}
	ok, err := auth.VerifyPassword(ctx, hash, req.Body.CurrentPassword)
	if errors.Is(err, auth.ErrBusy) {
		return ChangePassword400JSONResponse{badRequest("混み合っています。少し待ってからもう一度お試しください")}, nil
	}
	if err != nil {
		return nil, err
	}
	if !ok {
		failed = true
		return ChangePassword400JSONResponse{badRequest("今のパスワードが違います")}, nil
	}
	next, err := auth.HashPassword(req.Body.NewPassword)
	if errors.Is(err, auth.ErrWeakPassword) {
		return ChangePassword400JSONResponse{badRequest("パスワードは8〜256文字にしてください")}, nil
	}
	if err != nil {
		return nil, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	// Only if the hash is still the one just verified: a reset by an
	// administrator in the meantime wins.
	res, err := tx.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ? AND password_hash = ?`, next, me.ID, hash)
	if err != nil {
		return nil, err
	}
	if n, err := res.RowsAffected(); err != nil {
		return nil, err
	} else if n == 0 {
		return ChangePassword400JSONResponse{badRequest("パスワードがほかで変更されました。もう一度お試しください")}, nil
	}
	keep, _ := ctx.Value(tokenKey).(string)
	if err := deleteSessionsTx(ctx, tx, me.ID, keep); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return ChangePassword204Response{}, nil
}

func (s *server) ListApiKeys(ctx context.Context, _ ListApiKeysRequestObject) (ListApiKeysResponseObject, error) {
	keys, err := s.Auth.ListAPIKeys(ctx, currentUser(ctx).ID)
	if err != nil {
		return nil, err
	}
	out := ListApiKeys200JSONResponse{}
	for _, k := range keys {
		a := ApiKey{Id: k.ID, Name: k.Name, CreatedAt: k.CreatedAt.UTC().Format(time.RFC3339)}
		if k.LastUsed != nil {
			v := k.LastUsed.UTC().Format(time.RFC3339)
			a.LastUsed = &v
		}
		out = append(out, a)
	}
	return out, nil
}

const maxAPIKeys = 50

func (s *server) CreateApiKey(ctx context.Context, req CreateApiKeyRequestObject) (CreateApiKeyResponseObject, error) {
	me := currentUser(ctx)
	name := strings.TrimSpace(req.Body.Name)
	if c := utf8.RuneCountInString(name); c < 1 || c > 64 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return CreateApiKey400JSONResponse{badRequest("名前は1〜64文字にしてください")}, nil
	}
	var n int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM api_keys WHERE user_id = ?`, me.ID).Scan(&n); err != nil {
		return nil, err
	}
	if n >= maxAPIKeys {
		return CreateApiKey400JSONResponse{badRequest("API キーは50個までです")}, nil
	}
	id, key, err := s.Auth.CreateAPIKey(ctx, me.ID, name)
	if err != nil {
		return nil, err
	}
	return CreateApiKey201JSONResponse{Id: id, Name: name, Key: key}, nil
}

func (s *server) DeleteApiKey(ctx context.Context, req DeleteApiKeyRequestObject) (DeleteApiKeyResponseObject, error) {
	ok, err := s.Auth.DeleteAPIKey(ctx, currentUser(ctx).ID, req.KeyId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return DeleteApiKey404JSONResponse{notFound("API キーが見つかりません")}, nil
	}
	return DeleteApiKey204Response{}, nil
}
