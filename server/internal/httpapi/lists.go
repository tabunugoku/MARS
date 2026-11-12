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
		return CreateCollection400JSONResponse{badRequest(ctx, "bad_request.body_missing")}, nil
	}
	name := strings.TrimSpace(b.Name)
	if name == "" {
		return CreateCollection400JSONResponse{badRequest(ctx, "bad_request.name_required")}, nil
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
		return CreateCollection400JSONResponse{badRequest(ctx, "bad_request.unknown_series")}, nil
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
		return GetCollection404JSONResponse{notFound(ctx, "not_found.collection")}, nil
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
			return UpdateCollection400JSONResponse{badRequest(ctx, "bad_request.name_required")}, nil
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
		return UpdateCollection404JSONResponse{notFound(ctx, "not_found.collection")}, nil
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
			return UpdateCollection400JSONResponse{badRequest(ctx, "bad_request.unknown_series")}, nil
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
		return UpdateCollection404JSONResponse{notFound(ctx, "not_found.collection")}, nil
	}
	return UpdateCollection200JSONResponse(cs[0]), nil
}

func (s *server) DeleteCollection(ctx context.Context, req DeleteCollectionRequestObject) (DeleteCollectionResponseObject, error) {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM collections WHERE id = ?`, req.CollectionId)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return DeleteCollection404JSONResponse{notFound(ctx, "not_found.collection")}, nil
	}
	return DeleteCollection204Response{}, nil
}

func (s *server) ListCollectionSeries(ctx context.Context, req ListCollectionSeriesRequestObject) (ListCollectionSeriesResponseObject, error) {
	cs, err := s.collections(ctx, " WHERE id = ?", req.CollectionId)
	if err != nil {
		return nil, err
	}
	if len(cs) == 0 {
		return ListCollectionSeries404JSONResponse{notFound(ctx, "not_found.collection")}, nil
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
		return CreateReadList400JSONResponse{badRequest(ctx, "bad_request.body_missing")}, nil
	}
	name := strings.TrimSpace(b.Name)
	if name == "" {
		return CreateReadList400JSONResponse{badRequest(ctx, "bad_request.name_required")}, nil
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
		return CreateReadList400JSONResponse{badRequest(ctx, "bad_request.unknown_book")}, nil
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
		return GetReadList404JSONResponse{notFound(ctx, "not_found.read_list")}, nil
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
			return UpdateReadList400JSONResponse{badRequest(ctx, "bad_request.name_required")}, nil
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
		return UpdateReadList404JSONResponse{notFound(ctx, "not_found.read_list")}, nil
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
			return UpdateReadList400JSONResponse{badRequest(ctx, "bad_request.unknown_book")}, nil
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
		return UpdateReadList404JSONResponse{notFound(ctx, "not_found.read_list")}, nil
	}
	return UpdateReadList200JSONResponse(ls[0]), nil
}

func (s *server) DeleteReadList(ctx context.Context, req DeleteReadListRequestObject) (DeleteReadListResponseObject, error) {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM read_lists WHERE id = ?`, req.ReadListId)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return DeleteReadList404JSONResponse{notFound(ctx, "not_found.read_list")}, nil
	}
	return DeleteReadList204Response{}, nil
}

func (s *server) ListReadListBooks(ctx context.Context, req ListReadListBooksRequestObject) (ListReadListBooksResponseObject, error) {
	ls, err := s.readLists(ctx, " WHERE id = ?", req.ReadListId)
	if err != nil {
		return nil, err
	}
	if len(ls) == 0 {
		return ListReadListBooks404JSONResponse{notFound(ctx, "not_found.read_list")}, nil
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

// listLink describes one kind of membership: series in a collection, or
// books in a read list. Both kinds share the member operations below.
type listLink struct {
	list      string // collections or read_lists
	link      string // collection_series or read_list_books
	listCol   string // collection_id or read_list_id
	member    string // series or books
	memberCol string // series_id or book_id
	alias     string // the member's alias in queries: s or b
	notFound  string // list not found
	noMember  string // member not found or hidden from the caller
	conflict  string // the order sent no longer matches the visible members
}

var collectionLink = listLink{
	list: "collections", link: "collection_series", listCol: "collection_id",
	member: "series", memberCol: "series_id", alias: "s",
	notFound: "not_found.collection",
	noMember: "not_found.series",
	conflict: "conflict.collection_changed",
}

var readListLink = listLink{
	list: "read_lists", link: "read_list_books", listCol: "read_list_id",
	member: "books", memberCol: "book_id", alias: "b",
	notFound: "not_found.read_list",
	noMember: "not_found.member_book",
	conflict: "conflict.read_list_changed",
}

// memberSlot is one row of a list in position order.
type memberSlot struct {
	id      string
	visible bool
}

// listExists reports whether the list row exists.
func (l listLink) listExists(ctx context.Context, tx *sql.Tx, listID string) (bool, error) {
	var one int
	err := tx.QueryRowContext(ctx, `SELECT 1 FROM `+l.list+` WHERE id = ?`, listID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// memberVisible reports whether the member is live and in the caller's
// scope: the rule collectionMembers and readListMembers apply.
func (l listLink) memberVisible(ctx context.Context, tx *sql.Tx, memberID string) (bool, error) {
	cond, args := libScope(ctx, l.alias+".library_id")
	var one int
	err := tx.QueryRowContext(ctx, `SELECT 1 FROM `+l.member+` `+l.alias+` WHERE `+l.alias+`.id = ? AND `+l.alias+`.deleted_at IS NULL AND `+cond,
		append([]any{memberID}, args...)...).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// add appends memberID after every row of the list, hidden ones included.
// A member already in the list keeps its place.
func (l listLink) add(ctx context.Context, tx *sql.Tx, listID, memberID string) error {
	var one int
	err := tx.QueryRowContext(ctx, `SELECT 1 FROM `+l.link+` WHERE `+l.listCol+` = ? AND `+l.memberCol+` = ?`,
		listID, memberID).Scan(&one)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO `+l.link+` (`+l.listCol+`, `+l.memberCol+`, position)
		SELECT ?, ?, COALESCE(MAX(position) + 1, 0) FROM `+l.link+` WHERE `+l.listCol+` = ?`,
		listID, memberID, listID)
	return err
}

// remove deletes one member row. Other rows keep their positions.
func (l listLink) remove(ctx context.Context, tx *sql.Tx, listID, memberID string) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM `+l.link+` WHERE `+l.listCol+` = ? AND `+l.memberCol+` = ?`,
		listID, memberID)
	return err
}

// reorder puts the visible members of the list into the order ids gives.
// ids must be exactly the visible members, each once; otherwise it reports
// false and writes nothing. A hidden member keeps its slot in the order.
func (l listLink) reorder(ctx context.Context, tx *sql.Tx, listID string, ids []string) (bool, error) {
	cond, scopeArgs := libScope(ctx, l.alias+".library_id")
	args := append(scopeArgs, listID)
	rows, err := tx.QueryContext(ctx, `SELECT lk.`+l.memberCol+`,
		CASE WHEN `+l.alias+`.id IS NOT NULL AND `+l.alias+`.deleted_at IS NULL AND `+cond+` THEN 1 ELSE 0 END
		FROM `+l.link+` lk LEFT JOIN `+l.member+` `+l.alias+` ON `+l.alias+`.id = lk.`+l.memberCol+`
		WHERE lk.`+l.listCol+` = ? ORDER BY lk.position, lk.`+l.memberCol, args...)
	if err != nil {
		return false, err
	}
	slots, err := collect(rows, func(r scanner) (memberSlot, error) {
		var sl memberSlot
		var v int
		err := r.Scan(&sl.id, &v)
		sl.visible = v == 1
		return sl, err
	})
	if err != nil {
		return false, err
	}
	visible := 0
	for _, sl := range slots {
		if sl.visible {
			visible++
		}
	}
	if len(ids) != visible || len(distinct(ids)) != len(ids) {
		return false, nil
	}
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	for _, sl := range slots {
		if sl.visible && !want[sl.id] {
			return false, nil
		}
	}
	// Visible slots take the new order in turn; hidden slots stay put.
	order := make([]string, 0, len(slots))
	next := 0
	for _, sl := range slots {
		if sl.visible {
			order = append(order, ids[next])
			next++
		} else {
			order = append(order, sl.id)
		}
	}
	for i, id := range order {
		if _, err := tx.ExecContext(ctx, `UPDATE `+l.link+` SET position = ? WHERE `+l.listCol+` = ? AND `+l.memberCol+` = ?`,
			i, listID, id); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (s *server) AddCollectionSeries(ctx context.Context, req AddCollectionSeriesRequestObject) (AddCollectionSeriesResponseObject, error) {
	l := collectionLink
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	ok, err := l.listExists(ctx, tx, req.CollectionId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return AddCollectionSeries404JSONResponse{notFound(ctx, l.notFound)}, nil
	}
	ok, err = l.memberVisible(ctx, tx, req.SeriesId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return AddCollectionSeries404JSONResponse{notFound(ctx, l.noMember)}, nil
	}
	if err := l.add(ctx, tx, req.CollectionId, req.SeriesId); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	cs, err := s.collections(ctx, " WHERE id = ?", req.CollectionId)
	if err != nil {
		return nil, err
	}
	if len(cs) == 0 {
		return AddCollectionSeries404JSONResponse{notFound(ctx, l.notFound)}, nil
	}
	return AddCollectionSeries200JSONResponse(cs[0]), nil
}

func (s *server) RemoveCollectionSeries(ctx context.Context, req RemoveCollectionSeriesRequestObject) (RemoveCollectionSeriesResponseObject, error) {
	l := collectionLink
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	ok, err := l.listExists(ctx, tx, req.CollectionId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return RemoveCollectionSeries404JSONResponse{notFound(ctx, l.notFound)}, nil
	}
	if err := l.remove(ctx, tx, req.CollectionId, req.SeriesId); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	cs, err := s.collections(ctx, " WHERE id = ?", req.CollectionId)
	if err != nil {
		return nil, err
	}
	if len(cs) == 0 {
		return RemoveCollectionSeries404JSONResponse{notFound(ctx, l.notFound)}, nil
	}
	return RemoveCollectionSeries200JSONResponse(cs[0]), nil
}

func (s *server) ReorderCollection(ctx context.Context, req ReorderCollectionRequestObject) (ReorderCollectionResponseObject, error) {
	l := collectionLink
	var in []string
	if req.Body != nil {
		in = req.Body.SeriesIds
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	ok, err := l.listExists(ctx, tx, req.CollectionId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return ReorderCollection404JSONResponse{notFound(ctx, l.notFound)}, nil
	}
	ok, err = l.reorder(ctx, tx, req.CollectionId, in)
	if err != nil {
		return nil, err
	}
	if !ok {
		return ReorderCollection409JSONResponse{ConflictJSONResponse{Code: l.conflict, Message: msg(ctx, l.conflict)}}, nil
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	cs, err := s.collections(ctx, " WHERE id = ?", req.CollectionId)
	if err != nil {
		return nil, err
	}
	if len(cs) == 0 {
		return ReorderCollection404JSONResponse{notFound(ctx, l.notFound)}, nil
	}
	return ReorderCollection200JSONResponse(cs[0]), nil
}

func (s *server) AddReadListBook(ctx context.Context, req AddReadListBookRequestObject) (AddReadListBookResponseObject, error) {
	l := readListLink
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	ok, err := l.listExists(ctx, tx, req.ReadListId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return AddReadListBook404JSONResponse{notFound(ctx, l.notFound)}, nil
	}
	ok, err = l.memberVisible(ctx, tx, req.BookId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return AddReadListBook404JSONResponse{notFound(ctx, l.noMember)}, nil
	}
	if err := l.add(ctx, tx, req.ReadListId, req.BookId); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	ls, err := s.readLists(ctx, " WHERE id = ?", req.ReadListId)
	if err != nil {
		return nil, err
	}
	if len(ls) == 0 {
		return AddReadListBook404JSONResponse{notFound(ctx, l.notFound)}, nil
	}
	return AddReadListBook200JSONResponse(ls[0]), nil
}

func (s *server) RemoveReadListBook(ctx context.Context, req RemoveReadListBookRequestObject) (RemoveReadListBookResponseObject, error) {
	l := readListLink
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	ok, err := l.listExists(ctx, tx, req.ReadListId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return RemoveReadListBook404JSONResponse{notFound(ctx, l.notFound)}, nil
	}
	if err := l.remove(ctx, tx, req.ReadListId, req.BookId); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	ls, err := s.readLists(ctx, " WHERE id = ?", req.ReadListId)
	if err != nil {
		return nil, err
	}
	if len(ls) == 0 {
		return RemoveReadListBook404JSONResponse{notFound(ctx, l.notFound)}, nil
	}
	return RemoveReadListBook200JSONResponse(ls[0]), nil
}

func (s *server) ReorderReadList(ctx context.Context, req ReorderReadListRequestObject) (ReorderReadListResponseObject, error) {
	l := readListLink
	var in []string
	if req.Body != nil {
		in = req.Body.BookIds
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	ok, err := l.listExists(ctx, tx, req.ReadListId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return ReorderReadList404JSONResponse{notFound(ctx, l.notFound)}, nil
	}
	ok, err = l.reorder(ctx, tx, req.ReadListId, in)
	if err != nil {
		return nil, err
	}
	if !ok {
		return ReorderReadList409JSONResponse{ConflictJSONResponse{Code: l.conflict, Message: msg(ctx, l.conflict)}}, nil
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	ls, err := s.readLists(ctx, " WHERE id = ?", req.ReadListId)
	if err != nil {
		return nil, err
	}
	if len(ls) == 0 {
		return ReorderReadList404JSONResponse{notFound(ctx, l.notFound)}, nil
	}
	return ReorderReadList200JSONResponse(ls[0]), nil
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
