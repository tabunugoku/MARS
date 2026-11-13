package httpapi

import (
	"image/color"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tabunugoku/mars/server/internal/auth"
)

func contains(list []string, v string) bool { return slices.Contains(list, v) }

// seriesOf lists the series of library id as admin.
func seriesOf(e *env, libraryID string) []Series {
	var out []Series
	e.admin.json("GET", "/api/v1/series?libraryId="+libraryID, nil, 200, &out)
	return out
}

func booksOf(e *env, seriesID string) []Book {
	var out []Book
	e.admin.json("GET", "/api/v1/series/"+seriesID+"/books", nil, 200, &out)
	return out
}

func TestUpdateSeries(t *testing.T) {
	e := newEnv(t)
	red := pngBytes(t, 40, 60, color.RGBA{200, 0, 0, 255})
	rootA, rootB := t.TempDir(), t.TempDir()
	writeCBZ(t, filepath.Join(rootA, "Yofuke", "Yofuke v01.cbz"), map[string][]byte{"1.png": red})
	writeCBZ(t, filepath.Join(rootB, "Hidden", "Hidden v01.cbz"), map[string][]byte{"1.png": red})
	var a, b Library
	e.admin.json("POST", "/api/v1/libraries", map[string]any{"name": "A", "root": rootA, "kind": "manga"}, 201, &a)
	e.admin.json("POST", "/api/v1/libraries", map[string]any{"name": "B", "root": rootB, "kind": "manga"}, 201, &b)
	e.waitTasks()
	sa, sb := seriesOf(e, a.Id)[0], seriesOf(e, b.Id)[0]

	var got Series
	e.admin.json("PATCH", "/api/v1/series/"+sa.Id, map[string]any{"title": "新しい題名", "genres": []string{"SF", "ミステリー"}}, 200, &got)
	if got.Title != "新しい題名" || got.TitleSort != "新しい題名" || len(got.Genres) != 2 {
		t.Fatalf("update result %+v", got)
	}
	if !contains(got.LockedFields, "title") || !contains(got.LockedFields, "genres") || len(got.LockedFields) != 2 {
		t.Fatalf("locks %+v", got.LockedFields)
	}

	var again Series
	e.admin.json("GET", "/api/v1/series/"+sa.Id, nil, 200, &again)
	if again.Title != "新しい題名" || !contains(again.Genres, "SF") {
		t.Fatalf("GET after update %+v", again)
	}
	var found []Series
	e.admin.json("GET", "/api/v1/series?search="+url.QueryEscape("新しい題名"), nil, 200, &found)
	if len(found) != 1 || found[0].Id != sa.Id {
		t.Fatalf("search after rename: %+v", found)
	}

	// Explicit lockedFields replaces the list; an empty list unlocks everything.
	e.admin.json("PATCH", "/api/v1/series/"+sa.Id, map[string]any{"lockedFields": []string{"summary"}}, 200, &got)
	if len(got.LockedFields) != 1 || got.LockedFields[0] != "summary" {
		t.Fatalf("replaced locks %+v", got.LockedFields)
	}
	e.admin.json("PATCH", "/api/v1/series/"+sa.Id, map[string]any{"lockedFields": []string{}}, 200, &got)
	if len(got.LockedFields) != 0 {
		t.Fatalf("cleared locks %+v", got.LockedFields)
	}

	e.admin.json("PATCH", "/api/v1/series/"+sa.Id, map[string]any{"status": "bogus"}, 400, nil)
	e.admin.json("PATCH", "/api/v1/series/"+sa.Id, map[string]any{"title": "  "}, 400, nil)
	e.admin.json("PATCH", "/api/v1/series/"+sa.Id, map[string]any{"ageRating": 120}, 400, nil)
	e.admin.json("PATCH", "/api/v1/series/"+sa.Id, map[string]any{"lockedFields": []string{"nope"}}, 400, nil)
	e.admin.json("PATCH", "/api/v1/series/nope", map[string]any{"title": "x"}, 404, nil)

	// Restricted user: not allowed to edit at all (admin-only route), and
	// the admin sees the other library's series.
	e.admin.json("POST", "/api/v1/users", map[string]any{"name": "読者", "email": "reader@example.com", "password": "reader pass", "role": "user",
		"allLibraries": false, "libraryIds": []string{a.Id}}, 201, nil)
	r := e.login("読者", "reader pass")
	r.json("PATCH", "/api/v1/series/"+sb.Id, map[string]any{"title": "x"}, 403, nil)
	r.json("PATCH", "/api/v1/series/"+sa.Id, map[string]any{"title": "x"}, 403, nil)
	e.admin.json("PATCH", "/api/v1/series/"+sb.Id, map[string]any{"publisher": "出版社"}, 200, &got)
	if got.Publisher != "出版社" {
		t.Fatalf("admin edit of B: %+v", got)
	}
}

func TestLocksSurviveRescan(t *testing.T) {
	e := newEnv(t)
	root := t.TempDir()
	dir := filepath.Join(root, "Mystery")
	img := pngBytes(t, 40, 60, color.RGBA{0, 120, 0, 255})
	writeCBZ(t, filepath.Join(dir, "Extra.cbz"), map[string][]byte{"1.png": img})
	writeCBZ(t, filepath.Join(dir, "Bonus.cbz"), map[string][]byte{"1.png": img})
	var l Library
	e.admin.json("POST", "/api/v1/libraries", map[string]any{"name": "M", "root": root, "kind": "comics"}, 201, &l)
	e.waitTasks()
	series := seriesOf(e, l.Id)
	if len(series) != 1 {
		t.Fatalf("series %+v", series)
	}
	var extra, bonus Book
	for _, bk := range booksOf(e, series[0].Id) {
		switch {
		case strings.HasSuffix(bk.Path, "Extra.cbz"):
			extra = bk
		case strings.HasSuffix(bk.Path, "Bonus.cbz"):
			bonus = bk
		}
	}
	if extra.Number != 2 || bonus.Number != 1 {
		t.Fatalf("positional numbers extra=%v bonus=%v", extra.Number, bonus.Number)
	}

	e.admin.json("PATCH", "/api/v1/books/"+extra.Id, map[string]any{"number": 7}, 200, &extra)
	e.admin.json("PATCH", "/api/v1/books/"+bonus.Id, map[string]any{"title": "特典"}, 200, &bonus)

	// Rewrite both files with ComicInfo.xml so the rescan re-analyses them
	// and would apply a number and a title if the locks were ignored.
	writeCBZ(t, filepath.Join(dir, "Extra.cbz"), map[string][]byte{"1.png": img,
		"ComicInfo.xml": []byte(`<?xml version="1.0"?><ComicInfo><Number>3</Number></ComicInfo>`)})
	writeCBZ(t, filepath.Join(dir, "Bonus.cbz"), map[string][]byte{"1.png": img,
		"ComicInfo.xml": []byte(`<?xml version="1.0"?><ComicInfo><Title>原題</Title></ComicInfo>`)})
	e.admin.json("POST", "/api/v1/libraries/"+l.Id+"/scan", nil, 202, nil)
	e.waitTasks()

	var analyses int
	var tasks []Task
	e.admin.json("GET", "/api/v1/tasks", nil, 200, &tasks)
	for _, tk := range tasks {
		if tk.Kind == Analyze && tk.State == Done {
			analyses++
		}
	}
	if analyses < 2 {
		t.Fatalf("rescan did not re-analyse (analyses=%d)", analyses)
	}

	after := map[string]Book{}
	for _, bk := range booksOf(e, series[0].Id) {
		after[filepath.Base(bk.Path)] = bk
	}
	if got := after["Extra.cbz"]; got.Number != 7 || got.NumberSort != 7 || !contains(got.LockedFields, "number") {
		t.Fatalf("Extra after rescan %+v", got)
	}
	if got := after["Bonus.cbz"]; got.Title != "特典" || got.Number != 1 {
		t.Fatalf("Bonus after rescan %+v", got)
	}
}

func TestSettings(t *testing.T) {
	e := newEnv(t)
	var s ServerSettings
	e.admin.json("GET", "/api/v1/settings", nil, 200, &s)
	want := ServerSettings{Port: 25600, TaskThreads: 2, ThumbnailSize: ServerSettingsThumbnailSizeMedium,
		ThumbnailFormat: ServerSettingsThumbnailFormatJpeg, RememberMeDays: 30, DeleteEmptyCollections: true,
		LoginBackdrop: Planet}
	if s != want {
		t.Fatalf("defaults %+v", s)
	}

	e.admin.json("PATCH", "/api/v1/settings", map[string]any{"taskThreads": 4}, 200, &s)
	if s.TaskThreads != 4 || s.Port != 25600 || s.RememberMeDays != 30 || s.ThumbnailFormat != ServerSettingsThumbnailFormatJpeg {
		t.Fatalf("partial update %+v", s)
	}
	e.admin.json("PATCH", "/api/v1/settings", map[string]any{"port": 0}, 400, nil)
	e.admin.json("PATCH", "/api/v1/settings", map[string]any{"taskThreads": 17}, 400, nil)
	for _, f := range []string{"webp", "avif", "jpeg"} {
		e.admin.json("PATCH", "/api/v1/settings", map[string]any{"thumbnailFormat": f}, 200, &s)
		if string(s.ThumbnailFormat) != f {
			t.Fatalf("thumbnailFormat %s: got %+v", f, s)
		}
	}
	rec := e.admin.do("PATCH", "/api/v1/settings", map[string]any{"thumbnailFormat": "bmp"})
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "この形式にはまだ対応していません") {
		t.Fatalf("bmp: %d %s", rec.Code, rec.Body)
	}

	e.admin.json("PATCH", "/api/v1/settings", map[string]any{"rememberMeDays": 7}, 200, &s)
	e.admin.json("GET", "/api/v1/settings", nil, 200, &s)
	if s.RememberMeDays != 7 || s.TaskThreads != 4 {
		t.Fatalf("after read back %+v", s)
	}
	// A remembered sign-in lasts rememberMeDays.
	rec = e.anon().do("POST", "/api/v1/auth/login", map[string]any{"username": "管理人", "password": "correct horse", "remember": true})
	if rec.Code != 200 {
		t.Fatalf("login: %d %s", rec.Code, rec.Body)
	}
	var expires string
	if err := e.db.QueryRow(`SELECT expires_at FROM sessions WHERE token_hash = ?`, auth.HashToken(sessionFrom(rec))).Scan(&expires); err != nil {
		t.Fatal(err)
	}
	exp, err := time.Parse(time.RFC3339Nano, expires)
	if err != nil {
		t.Fatal(err)
	}
	if left := time.Until(exp); left < 6*24*time.Hour+23*time.Hour || left > 7*24*time.Hour+time.Hour {
		t.Fatalf("remembered session ends in %v, want about 7 days", left)
	}

	// A corrupt stored value falls back to the defaults.
	if _, err := e.db.Exec(`UPDATE settings SET value = '{"port":"x"' WHERE key = 'server'`); err != nil {
		t.Fatal(err)
	}
	e.admin.json("GET", "/api/v1/settings", nil, 200, &s)
	if s != want {
		t.Fatalf("corrupt value gave %+v", s)
	}
}

func TestEmptyTrash(t *testing.T) {
	e := newEnv(t)
	root := t.TempDir()
	img := pngBytes(t, 40, 60, color.RGBA{10, 10, 10, 255})
	writeCBZ(t, filepath.Join(root, "Kept", "Kept v01.cbz"), map[string][]byte{"1.png": img})
	writeCBZ(t, filepath.Join(root, "Kept", "Kept v02.cbz"), map[string][]byte{"1.png": img})
	writeCBZ(t, filepath.Join(root, "Gone", "Gone v01.cbz"), map[string][]byte{"1.png": img})
	writeCBZ(t, filepath.Join(root, "Gone", "Gone v02.cbz"), map[string][]byte{"1.png": img})
	writeCBZ(t, filepath.Join(root, "Odd", "Odd v01.cbz"), map[string][]byte{"1.png": img})
	var l Library
	e.admin.json("POST", "/api/v1/libraries", map[string]any{"name": "T", "root": root, "kind": "manga"}, 201, &l)
	e.waitTasks()

	byTitle := map[string]Series{}
	bookIDs := map[string]string{}
	for _, s := range seriesOf(e, l.Id) {
		byTitle[s.Title] = s
		for _, bk := range booksOf(e, s.Id) {
			bookIDs[filepath.Base(bk.Path)] = bk.Id
		}
	}
	kept, gone, odd := byTitle["Kept"], byTitle["Gone"], byTitle["Odd"]
	if kept.Id == "" || gone.Id == "" || odd.Id == "" || len(bookIDs) != 5 {
		t.Fatalf("setup: %+v %v", byTitle, bookIDs)
	}

	// One book file and one whole series folder go away.
	if err := os.Remove(filepath.Join(root, "Kept", "Kept v02.cbz")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, "Gone")); err != nil {
		t.Fatal(err)
	}
	e.admin.json("POST", "/api/v1/libraries/"+l.Id+"/scan", nil, 202, nil)
	e.waitTasks()

	// Odd is trashed directly in the database while its book stays live,
	// so emptying the trash cascades that book without counting it.
	if _, err := e.db.Exec(`UPDATE series SET deleted_at = ? WHERE id = ?`, time.Now().UTC().Format(time.RFC3339Nano), odd.Id); err != nil {
		t.Fatal(err)
	}
	var trashedBooks int
	if err := e.db.QueryRow(`SELECT count(*) FROM books WHERE deleted_at IS NOT NULL`).Scan(&trashedBooks); err != nil || trashedBooks != 3 {
		t.Fatalf("trashed books before empty: %d, %v", trashedBooks, err)
	}

	var res TrashResult
	e.admin.json("POST", "/api/v1/trash/empty", nil, 200, &res)
	if res.Books != 3 || res.Series != 2 {
		t.Fatalf("empty trash %+v, want books 3 series 2", res)
	}

	goneIDs := []string{bookIDs["Kept v02.cbz"], bookIDs["Gone v01.cbz"], bookIDs["Gone v02.cbz"], bookIDs["Odd v01.cbz"]}
	for _, id := range goneIDs {
		e.admin.json("GET", "/api/v1/books/"+id, nil, 404, nil)
		var pages int
		if err := e.db.QueryRow(`SELECT count(*) FROM pages WHERE book_id = ?`, id).Scan(&pages); err != nil || pages != 0 {
			t.Fatalf("pages of removed book %s: %d, %v", id, pages, err)
		}
	}
	e.admin.json("GET", "/api/v1/series/"+gone.Id, nil, 404, nil)
	e.admin.json("GET", "/api/v1/series/"+odd.Id, nil, 404, nil)
	e.admin.json("GET", "/api/v1/series/"+kept.Id, nil, 200, nil)
	e.admin.json("GET", "/api/v1/books/"+bookIDs["Kept v01.cbz"], nil, 200, nil)

	e.admin.json("POST", "/api/v1/trash/empty", nil, 200, &res)
	if res.Books != 0 || res.Series != 0 {
		t.Fatalf("second empty %+v", res)
	}
}

// null clears a nullable field; leaving it out keeps it.
func TestUpdateNullClears(t *testing.T) {
	e := newEnv(t)
	red := pngBytes(t, 40, 60, color.RGBA{200, 0, 0, 255})
	root := t.TempDir()
	writeCBZ(t, filepath.Join(root, "Yofuke", "Yofuke v01.cbz"), map[string][]byte{"1.png": red})
	var a Library
	e.admin.json("POST", "/api/v1/libraries", map[string]any{"name": "A", "root": root, "kind": "manga"}, 201, &a)
	e.waitTasks()
	s := seriesOf(e, a.Id)[0]
	var got Series
	e.admin.json("PATCH", "/api/v1/series/"+s.Id, map[string]any{"ageRating": 12, "totalBookCount": 5}, 200, &got)
	if got.AgeRating == nil || *got.AgeRating != 12 || got.TotalBookCount == nil || *got.TotalBookCount != 5 {
		t.Fatalf("set %+v", got)
	}
	e.admin.json("PATCH", "/api/v1/series/"+s.Id, map[string]any{"summary": "あらすじ"}, 200, &got)
	if got.AgeRating == nil || got.TotalBookCount == nil {
		t.Fatalf("absent fields changed %+v", got)
	}
	e.admin.json("PATCH", "/api/v1/series/"+s.Id, map[string]any{"ageRating": nil}, 200, &got)
	if got.AgeRating != nil || got.TotalBookCount == nil {
		t.Fatalf("null ageRating %+v", got)
	}

	var books []Book
	e.admin.json("GET", "/api/v1/series/"+s.Id+"/books", nil, 200, &books)
	var b Book
	e.admin.json("PATCH", "/api/v1/books/"+books[0].Id, map[string]any{"releaseDate": "2024-05"}, 200, &b)
	if b.ReleaseDate == nil || *b.ReleaseDate != "2024-05" {
		t.Fatalf("release date %+v", b.ReleaseDate)
	}
	e.admin.json("PATCH", "/api/v1/books/"+books[0].Id, map[string]any{"releaseDate": nil}, 200, &b)
	if b.ReleaseDate != nil {
		t.Fatalf("null release date %+v", *b.ReleaseDate)
	}
	e.admin.json("PATCH", "/api/v1/books/"+books[0].Id, map[string]any{"releaseDate": "May 2024"}, 400, nil)
}

func TestThumbBounds(t *testing.T) {
	cases := map[string][2]int{
		"small":  {300, 450},
		"medium": {400, 600},
		"large":  {600, 900},
		"xlarge": {800, 1200},
		"bogus":  {400, 600},
		"":       {400, 600},
	}
	for size, want := range cases {
		if w, h := ThumbBounds(size); w != want[0] || h != want[1] {
			t.Errorf("ThumbBounds(%q) = %dx%d, want %dx%d", size, w, h, want[0], want[1])
		}
	}
}

// TestBookMediaErrorAndWatch covers the fields added to books and libraries
// for the web app, and that a failed analysis hides its stale pages.
func TestBookMediaErrorAndWatch(t *testing.T) {
	e := newEnv(t)
	root := t.TempDir()
	red := pngBytes(t, 20, 30, color.RGBA{200, 0, 0, 255})
	writeCBZ(t, filepath.Join(root, "A", "A v01.cbz"), map[string][]byte{"1.png": red})
	l := e.libraryWith("A", root)
	if l.Watch || l.WatchError == nil || *l.WatchError != "" {
		t.Fatalf("new library: watch %v, watchError %v", l.Watch, l.WatchError)
	}
	_, books := e.onlySeries(l.Id)
	b := books[0]
	if b.MediaError == nil || *b.MediaError != "" {
		t.Fatalf("ready book mediaError: %v", b.MediaError)
	}
	var pages []Page
	e.admin.json("GET", "/api/v1/books/"+b.Id+"/pages", nil, 200, &pages)
	if len(pages) != 1 {
		t.Fatalf("pages of a ready book: %+v", pages)
	}

	// Watch round-trips through PATCH, and on create.
	var up Library
	e.admin.json("PATCH", "/api/v1/libraries/"+l.Id, map[string]any{"watch": true}, 200, &up)
	if !up.Watch {
		t.Fatalf("patch watch: %+v", up)
	}
	var got Library
	e.admin.json("GET", "/api/v1/libraries/"+l.Id, nil, 200, &got)
	if !got.Watch || got.WatchError == nil {
		t.Fatalf("get after patch: %+v", got)
	}
	var created Library
	e.admin.json("POST", "/api/v1/libraries", map[string]any{"name": "B", "root": t.TempDir(), "kind": "manga", "watch": true}, 201, &created)
	if !created.Watch {
		t.Fatalf("create watch: %+v", created)
	}

	// A failed analysis: the error shows, and the stale pages do not.
	if _, err := e.db.Exec(`UPDATE books SET media_status = 'error', media_error = 'broken archive' WHERE id = ?`, b.Id); err != nil {
		t.Fatal(err)
	}
	var failed Book
	e.admin.json("GET", "/api/v1/books/"+b.Id, nil, 200, &failed)
	if failed.MediaError == nil || *failed.MediaError != "ファイルを読み込めませんでした" {
		t.Fatalf("mediaError after failure: %v", failed.MediaError)
	}
	// An archive's own reason shows without its prefix; one naming a path
	// does not show at all (paths are for administrators).
	for raw, want := range map[string]string{
		"archive: unsupported: PDF page 1 is not a single image": "PDF の 1 ページは1枚の画像ではありません",
		"open /srv/comics/A/b.cbz: permission denied":           "ファイルを読み込めませんでした",
		`archive: unsupported: "C:\lib\x.foo"`:            "ファイルを読み込めませんでした",
	} {
		if _, err := e.db.Exec(`UPDATE books SET media_error = ? WHERE id = ?`, raw, b.Id); err != nil {
			t.Fatal(err)
		}
		e.admin.json("GET", "/api/v1/books/"+b.Id, nil, 200, &failed)
		if failed.MediaError == nil || *failed.MediaError != want {
			t.Fatalf("mediaError for %q: %v", raw, failed.MediaError)
		}
	}
	e.admin.json("GET", "/api/v1/books/"+b.Id+"/pages", nil, 200, &pages)
	if len(pages) != 0 {
		t.Fatalf("pages of an errored book: %+v", pages)
	}
}
