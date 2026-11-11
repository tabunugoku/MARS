package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/tabunugoku/cominavi/server/internal/ids"
)

// Collections and read lists share one visibility rule. An administrator
// sees every list, empty ones included, with all of its members. Anyone else
// sees a list only when at least one member is visible to them (a live
// series or book in a library in their scope), and then only the visible
// members are listed. collectionMembers and readListMembers are the only
// places that apply the member filter; collections and readLists build on
// them.

// collectionMembers maps each collection id to its visible member series in
// position order. Collections with no visible member have no entry.
func (s *server) collectionMembers(ctx context.Context, listIDs []string) (map[string][]string, error) {
	out := map[string][]string{}
	if len(listIDs) == 0 {
		return out, nil
	}
	scope, scopeArgs := libScope(ctx, "s.library_id")
	args := append(anyStrings(listIDs), scopeArgs...)
	rows, err := s.DB.QueryContext(ctx, `SELECT cs.collection_id, cs.series_id
		FROM collection_series cs JOIN series s ON s.id = cs.series_id
		WHERE cs.collection_id IN (`+placeholders(len(listIDs))+`) AND s.deleted_at IS NULL AND `+scope+`
		ORDER BY cs.collection_id, cs.position`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var listID, memberID string
		if err := rows.Scan(&listID, &memberID); err != nil {
			return nil, err
		}
		out[listID] = append(out[listID], memberID)
	}
	return out, rows.Err()
}

// readListMembers is collectionMembers for read lists and their books.
func (s *server) readListMembers(ctx context.Context, listIDs []string) (map[string][]string, error) {
	out := map[string][]string{}
	if len(listIDs) == 0 {
		return out, nil
	}
	scope, scopeArgs := libScope(ctx, "b.library_id")
	args := append(anyStrings(listIDs), scopeArgs...)
	rows, err := s.DB.QueryContext(ctx, `SELECT rb.read_list_id, rb.book_id
		FROM read_list_books rb JOIN books b ON b.id = rb.book_id
		WHERE rb.read_list_id IN (`+placeholders(len(listIDs))+`) AND b.deleted_at IS NULL AND `+scope+`
		ORDER BY rb.read_list_id, rb.position`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var listID, memberID string
		if err := rows.Scan(&listID, &memberID); err != nil {
			return nil, err
		}
		out[listID] = append(out[listID], memberID)
	}
	return out, rows.Err()
}

// collections returns the collections matching where (" WHERE ..." or "")
// that the caller may see, each with its visible members.
func (s *server) collections(ctx context.Context, where string, args ...any) ([]Collection, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, name, ordered FROM collections`+where+` ORDER BY name, id`, args...)
	if err != nil {
		return nil, err
	}
	all, err := collect(rows, func(r scanner) (Collection, error) {
		var c Collection
		err := r.Scan(&c.Id, &c.Name, &c.Ordered)
		return c, err
	})
	if err != nil {
		return nil, err
	}
	listIDs := make([]string, len(all))
	for i, c := range all {
		listIDs[i] = c.Id
	}
	members, err := s.collectionMembers(ctx, listIDs)
	if err != nil {
		return nil, err
	}
	admin := currentUser(ctx).IsAdmin()
	out := []Collection{}
	for _, c := range all {
		c.SeriesIds = append([]string{}, members[c.Id]...)
		if len(c.SeriesIds) == 0 && !admin {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

// readLists is collections for read lists.
func (s *server) readLists(ctx context.Context, where string, args ...any) ([]ReadList, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, name, summary FROM read_lists`+where+` ORDER BY name, id`, args...)
	if err != nil {
		return nil, err
	}
	all, err := collect(rows, func(r scanner) (ReadList, error) {
		var l ReadList
		err := r.Scan(&l.Id, &l.Name, &l.Summary)
		return l, err
	})
	if err != nil {
		return nil, err
	}
	listIDs := make([]string, len(all))
	for i, l := range all {
		listIDs[i] = l.Id
	}
	members, err := s.readListMembers(ctx, listIDs)
	if err != nil {
		return nil, err
	}
	admin := currentUser(ctx).IsAdmin()
	out := []ReadList{}
	for _, l := range all {
		l.BookIds = append([]string{}, members[l.Id]...)
		if len(l.BookIds) == 0 && !admin {
			continue
		}
		out = append(out, l)
	}
	return out, nil
}

func (s *server) ListCollections(ctx context.Context, req ListCollectionsRequestObject) (ListCollectionsResponseObject, error) {
	out, err := s.collections(ctx, "")
	if err != nil {
		return nil, err
	}
	return ListCollections200JSONResponse(out), nil
}

func (s *server) CreateCollection(ctx context.Context, req CreateCollectionRequestObject) (CreateCollectionResponseObject, error) {
	b := req.Body
	if b == nil {
		return CreateCollection400JSONResponse{badRequest("本文がありません")}, nil
	}
	name := strings.TrimSpace(b.Name)
	if name == "" {
		return CreateCollection400JSONResponse{badRequest("名前を入力してください")}, nil
	}
	var in []string
	if b.SeriesIds != nil {
		in = *b.SeriesIds
	}
	members := distinct(in)
	ordered := b.Ordered != nil && *b.Ordered
	id := ids.New()

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	ok, err := allExist(ctx, tx, "series", members)
	if err != nil {
		return nil, err
	}
	if !ok {
		return CreateCollection400JSONResponse{badRequest("存在しないシリーズが含まれています")}, nil
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO collections (id, name, ordered, created_at) VALUES (?, ?, ?, ?)`,
		id, name, ordered, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return nil, err
	}
	for i, m := range members {
		if _, err := tx.ExecContext(ctx, `INSERT INTO collection_series (collection_id, series_id, position) VALUES (?, ?, ?)`,
			id, m, i); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	// Every member is visible to the administrator who created it.
	return CreateCollection201JSONResponse{Id: id, Name: name, Ordered: ordered, SeriesIds: members}, nil
}

func (s *server) GetCollection(ctx context.Context, req GetCollectionRequestObject) (GetCollectionResponseObject, error) {
	cs, err := s.collections(ctx, " WHERE id = ?", req.CollectionId)
	if err != nil {
		return nil, err
	}
	if len(cs) == 0 {
		return GetCollection404JSONResponse{notFound("コレクションが見つかりません")}, nil
	}
	return GetCollection200JSONResponse(cs[0]), nil
}

func (s *server) UpdateCollection(ctx context.Context, req UpdateCollectionRequestObject) (UpdateCollectionResponseObject, error) {
	var b CollectionUpdate
	if req.Body != nil {
		b = *req.Body
	}
	var name *string
	if b.Name != nil {
		t := strings.TrimSpace(*b.Name)
		if t == "" {
			return UpdateCollection400JSONResponse{badRequest("名前を入力してください")}, nil
		}
		name = &t
	}
	var members []string
	if b.SeriesIds != nil {
		members = distinct(*b.SeriesIds)
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var cur string
	var ordered bool
	err = tx.QueryRowContext(ctx, `SELECT name, ordered FROM collections WHERE id = ?`, req.CollectionId).Scan(&cur, &ordered)
	if errors.Is(err, sql.ErrNoRows) {
		return UpdateCollection404JSONResponse{notFound("コレクションが見つかりません")}, nil
	}
	if err != nil {
		return nil, err
	}
	if name != nil {
		cur = *name
	}
	if b.Ordered != nil {
		ordered = *b.Ordered
	}
	if _, err := tx.ExecContext(ctx, `UPDATE collections SET name = ?, ordered = ? WHERE id = ?`,
		cur, ordered, req.CollectionId); err != nil {
		return nil, err
	}
	if b.SeriesIds != nil {
		ok, err := allExist(ctx, tx, "series", members)
		if err != nil {
			return nil, err
		}
		if !ok {
			return UpdateCollection400JSONResponse{badRequest("存在しないシリーズが含まれています")}, nil
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM collection_series WHERE collection_id = ?`, req.CollectionId); err != nil {
			return nil, err
		}
		for i, m := range members {
			if _, err := tx.ExecContext(ctx, `INSERT INTO collection_series (collection_id, series_id, position) VALUES (?, ?, ?)`,
				req.CollectionId, m, i); err != nil {
				return nil, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	cs, err := s.collections(ctx, " WHERE id = ?", req.CollectionId)
	if err != nil {
		return nil, err
	}
	if len(cs) == 0 {
		return UpdateCollection404JSONResponse{notFound("コレクションが見つかりません")}, nil
	}
	return UpdateCollection200JSONResponse(cs[0]), nil
}

func (s *server) DeleteCollection(ctx context.Context, req DeleteCollectionRequestObject) (DeleteCollectionResponseObject, error) {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM collections WHERE id = ?`, req.CollectionId)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return DeleteCollection404JSONResponse{notFound("コレクションが見つかりません")}, nil
	}
	return DeleteCollection204Response{}, nil
}

func (s *server) ListCollectionSeries(ctx context.Context, req ListCollectionSeriesRequestObject) (ListCollectionSeriesResponseObject, error) {
	cs, err := s.collections(ctx, " WHERE id = ?", req.CollectionId)
	if err != nil {
		return nil, err
	}
	if len(cs) == 0 {
		return ListCollectionSeries404JSONResponse{notFound("コレクションが見つかりません")}, nil
	}
	args := []any{req.CollectionId}
	tail := `ORDER BY s.title_sort, s.id`
	if cs[0].Ordered {
		tail = `ORDER BY (SELECT position FROM collection_series WHERE collection_id = ? AND series_id = s.id)`
		args = append(args, req.CollectionId)
	}
	out, err := s.listSeries(ctx, ` AND s.id IN (SELECT series_id FROM collection_series WHERE collection_id = ?)`, tail, args...)
	if err != nil {
		return nil, err
	}
	return ListCollectionSeries200JSONResponse(out), nil
}

func (s *server) ListReadLists(ctx context.Context, req ListReadListsRequestObject) (ListReadListsResponseObject, error) {
	out, err := s.readLists(ctx, "")
	if err != nil {
		return nil, err
	}
	return ListReadLists200JSONResponse(out), nil
}

func (s *server) CreateReadList(ctx context.Context, req CreateReadListRequestObject) (CreateReadListResponseObject, error) {
	b := req.Body
	if b == nil {
		return CreateReadList400JSONResponse{badRequest("本文がありません")}, nil
	}
	name := strings.TrimSpace(b.Name)
	if name == "" {
		return CreateReadList400JSONResponse{badRequest("名前を入力してください")}, nil
	}
	var in []string
	if b.BookIds != nil {
		in = *b.BookIds
	}
	members := distinct(in)
	summary := ""
	if b.Summary != nil {
		summary = *b.Summary
	}
	id := ids.New()

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	ok, err := allExist(ctx, tx, "books", members)
	if err != nil {
		return nil, err
	}
	if !ok {
		return CreateReadList400JSONResponse{badRequest("存在しない巻が含まれています")}, nil
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO read_lists (id, name, summary, created_at) VALUES (?, ?, ?, ?)`,
		id, name, summary, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return nil, err
	}
	for i, m := range members {
		if _, err := tx.ExecContext(ctx, `INSERT INTO read_list_books (read_list_id, book_id, position) VALUES (?, ?, ?)`,
			id, m, i); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	// Every member is visible to the administrator who created it.
	return CreateReadList201JSONResponse{Id: id, Name: name, Summary: summary, BookIds: members}, nil
}

func (s *server) GetReadList(ctx context.Context, req GetReadListRequestObject) (GetReadListResponseObject, error) {
	ls, err := s.readLists(ctx, " WHERE id = ?", req.ReadListId)
	if err != nil {
		return nil, err
	}
	if len(ls) == 0 {
		return GetReadList404JSONResponse{notFound("読書リストが見つかりません")}, nil
	}
	return GetReadList200JSONResponse(ls[0]), nil
}

func (s *server) UpdateReadList(ctx context.Context, req UpdateReadListRequestObject) (UpdateReadListResponseObject, error) {
	var b ReadListUpdate
	if req.Body != nil {
		b = *req.Body
	}
	var name *string
	if b.Name != nil {
		t := strings.TrimSpace(*b.Name)
		if t == "" {
			return UpdateReadList400JSONResponse{badRequest("名前を入力してください")}, nil
		}
		name = &t
	}
	var members []string
	if b.BookIds != nil {
		members = distinct(*b.BookIds)
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var curName, curSummary string
	err = tx.QueryRowContext(ctx, `SELECT name, summary FROM read_lists WHERE id = ?`, req.ReadListId).Scan(&curName, &curSummary)
	if errors.Is(err, sql.ErrNoRows) {
		return UpdateReadList404JSONResponse{notFound("読書リストが見つかりません")}, nil
	}
	if err != nil {
		return nil, err
	}
	if name != nil {
		curName = *name
	}
	if b.Summary != nil {
		curSummary = *b.Summary
	}
	if _, err := tx.ExecContext(ctx, `UPDATE read_lists SET name = ?, summary = ? WHERE id = ?`,
		curName, curSummary, req.ReadListId); err != nil {
		return nil, err
	}
	if b.BookIds != nil {
		ok, err := allExist(ctx, tx, "books", members)
		if err != nil {
			return nil, err
		}
		if !ok {
			return UpdateReadList400JSONResponse{badRequest("存在しない巻が含まれています")}, nil
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM read_list_books WHERE read_list_id = ?`, req.ReadListId); err != nil {
			return nil, err
		}
		for i, m := range members {
			if _, err := tx.ExecContext(ctx, `INSERT INTO read_list_books (read_list_id, book_id, position) VALUES (?, ?, ?)`,
				req.ReadListId, m, i); err != nil {
				return nil, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	ls, err := s.readLists(ctx, " WHERE id = ?", req.ReadListId)
	if err != nil {
		return nil, err
	}
	if len(ls) == 0 {
		return UpdateReadList404JSONResponse{notFound("読書リストが見つかりません")}, nil
	}
	return UpdateReadList200JSONResponse(ls[0]), nil
}

func (s *server) DeleteReadList(ctx context.Context, req DeleteReadListRequestObject) (DeleteReadListResponseObject, error) {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM read_lists WHERE id = ?`, req.ReadListId)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return DeleteReadList404JSONResponse{notFound("読書リストが見つかりません")}, nil
	}
	return DeleteReadList204Response{}, nil
}

func (s *server) ListReadListBooks(ctx context.Context, req ListReadListBooksRequestObject) (ListReadListBooksResponseObject, error) {
	ls, err := s.readLists(ctx, " WHERE id = ?", req.ReadListId)
	if err != nil {
		return nil, err
	}
	if len(ls) == 0 {
		return ListReadListBooks404JSONResponse{notFound("読書リストが見つかりません")}, nil
	}
	out, err := s.listBooks(ctx,
		` AND b.id IN (SELECT book_id FROM read_list_books WHERE read_list_id = ?)`,
		`ORDER BY (SELECT position FROM read_list_books WHERE read_list_id = ? AND book_id = b.id)`,
		req.ReadListId, req.ReadListId)
	if err != nil {
		return nil, err
	}
	return ListReadListBooks200JSONResponse(out), nil
}

// collect scans every row with scan and closes rows.
func collect[T any](rows *sql.Rows, scan func(scanner) (T, error)) ([]T, error) {
	defer rows.Close()
	out := []T{}
	for rows.Next() {
		x, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// allExist reports whether every id is a live row of table (series or
// books). Members must exist and sit outside the trash; the administrator's
// scope is everything, so no library check is needed here.
func allExist(ctx context.Context, tx *sql.Tx, table string, members []string) (bool, error) {
	if len(members) == 0 {
		return true, nil
	}
	var n int
	err := tx.QueryRowContext(ctx, `SELECT count(*) FROM `+table+` WHERE deleted_at IS NULL AND id IN (`+placeholders(len(members))+`)`,
		anyStrings(members)...).Scan(&n)
	return err == nil && n == len(members), err
}

// distinct returns in without repeats, keeping the first of each id.
func distinct(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := []string{}
	for _, id := range in {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func anyStrings(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}
