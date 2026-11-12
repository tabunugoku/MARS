package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image/color"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/tabunugoku/cominavi/server/internal/events"
	"github.com/tabunugoku/cominavi/server/internal/library"
)

var (
	redCover  = color.RGBA{220, 0, 0, 255}
	blueCover = color.RGBA{0, 0, 220, 255}
	greenCov  = color.RGBA{0, 200, 0, 255}
)

// rawPut sends body as the administrator, as a PUT of an octet stream.
func rawPut(e *env, url string, body []byte) *httptest.ResponseRecorder {
	e.t.Helper()
	req := httptest.NewRequest(http.MethodPut, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/octet-stream")
	req.AddCookie(&http.Cookie{Name: cookieName, Value: e.admin.cookie})
	req.Header.Set("Origin", "http://"+req.Host)
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

// answerCode returns the code of an error answer.
func answerCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var a struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &a); err != nil {
		t.Fatalf("answer %d: %s", rec.Code, rec.Body)
	}
	return a.Code
}

func isRed(r, g, b uint32) bool  { return r >= 150 && b <= 80 }
func isBlue(r, g, b uint32) bool { return b >= 150 && r <= 80 }
func isGreen(r, g, b uint32) bool {
	return g >= 150 && r <= 80 && b <= 80
}

// rescan queues a scan of the library and waits for it.
func (e *env) rescan(libraryID string) {
	e.t.Helper()
	e.admin.json("POST", "/api/v1/libraries/"+libraryID+"/scan", nil, 202, nil)
	e.waitTasks()
}

func TestSeriesUploadCover(t *testing.T) {
	e := newEnv(t)
	root := t.TempDir()
	writeCBZ(t, filepath.Join(root, "S", "S v01.cbz"), map[string][]byte{"1.png": pngBytes(t, 20, 30, redCover)})
	l := e.libraryWith("漫画", root)
	s, _ := e.onlySeries(l.Id)
	if s.ThumbnailSource != SeriesThumbnailSourceBook || s.CoverBookId != nil {
		t.Fatalf("before upload: source %q, cover book %v", s.ThumbnailSource, s.CoverBookId)
	}
	before := s.ThumbnailUrl
	url := "/api/v1/series/" + s.Id + "/thumbnail"

	// Bytes that are not an image, an unsupported format and an image too
	// large for the limits are all refused.
	if code := answerCode(t, rawPutCode(e, url, []byte("not an image"), 400)); code != "bad_request.image_format" {
		t.Fatalf("bad bytes: %s", code)
	}
	gif := []byte("GIF89a\x01\x00\x01\x00\x80\x00\x00\x00\x00\x00\xff\xff\xff!\xf9\x04\x01\x00\x00\x00\x00,\x00\x00\x00\x00\x01\x00\x01\x00\x00\x02\x02D\x01\x00;")
	if code := answerCode(t, rawPutCode(e, url, gif, 400)); code != "bad_request.image_format" {
		t.Fatalf("gif: %s", code)
	}
	if code := answerCode(t, rawPutCode(e, url, make([]byte, maxCoverBytes+1), 400)); code != "bad_request.image_too_large" {
		t.Fatalf("too large: %s", code)
	}
	if code := answerCode(t, rawPutCode(e, url, pngBytes(t, 20001, 1, blueCover), 400)); code != "bad_request.image_size" {
		t.Fatalf("too wide: %s", code)
	}
	e.admin.json("PUT", "/api/v1/series/nope/thumbnail", nil, 404, nil)

	// A PNG is served as a JPEG cover, marked as an upload, under a new URL.
	var got Series
	rec := rawPut(e, url, pngBytes(t, 40, 60, blueCover))
	if rec.Code != 200 {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ThumbnailSource != SeriesThumbnailSourceUpload || got.ThumbnailUrl == before {
		t.Fatalf("after upload: source %q url %q (was %q)", got.ThumbnailSource, got.ThumbnailUrl, before)
	}
	if r, g, b := thumbColor(t, e.admin, got.ThumbnailUrl); !isBlue(r, g, b) {
		t.Fatalf("uploaded cover not blue: %d %d %d", r, g, b)
	}
	if ct := e.admin.do("GET", url, nil).Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Fatalf("cover content type %q", ct)
	}

	// A scan keeps an upload.
	e.rescan(l.Id)
	e.admin.json("GET", "/api/v1/series/"+s.Id, nil, 200, &got)
	if got.ThumbnailSource != SeriesThumbnailSourceUpload {
		t.Fatalf("upload lost in a scan: %q", got.ThumbnailSource)
	}

	// Dropping it goes back to the book's cover, under the URL it had.
	e.admin.json("DELETE", url, nil, 200, &got)
	if got.ThumbnailSource != SeriesThumbnailSourceBook || got.ThumbnailUrl != before {
		t.Fatalf("after delete: source %q url %q (want %q)", got.ThumbnailSource, got.ThumbnailUrl, before)
	}
	if r, _, b := thumbColor(t, e.admin, got.ThumbnailUrl); !isRed(r, 0, b) {
		t.Fatalf("book cover not red again: %d %d", r, b)
	}
	e.admin.json("DELETE", "/api/v1/series/nope/thumbnail", nil, 404, nil)
}

// rawPutCode is rawPut that also checks the status.
func rawPutCode(e *env, url string, body []byte, want int) *httptest.ResponseRecorder {
	e.t.Helper()
	rec := rawPut(e, url, body)
	if rec.Code != want {
		e.t.Fatalf("PUT %s: %d, want %d: %s", url, rec.Code, want, rec.Body)
	}
	return rec
}

func TestBookUploadThumbnail(t *testing.T) {
	e := newEnv(t)
	root := t.TempDir()
	writeCBZ(t, filepath.Join(root, "S", "S v01.cbz"), map[string][]byte{"1.png": pngBytes(t, 20, 30, redCover)})
	l := e.libraryWith("漫画", root)
	_, books := e.onlySeries(l.Id)
	id := books[0].Id
	url := "/api/v1/books/" + id + "/thumbnail"
	if books[0].ThumbnailUploaded {
		t.Fatal("a new book reports an upload")
	}
	if r, _, b := thumbColor(t, e.admin, books[0].ThumbnailUrl); !isRed(r, 0, b) {
		t.Fatalf("first cover not red: %d %d", r, b)
	}

	rawPutCode(e, url, []byte("nope"), 400)
	rawPutCode(e, "/api/v1/books/nope/thumbnail", pngBytes(t, 10, 10, greenCov), 404)

	var b Book
	rec := rawPutCode(e, url, pngBytes(t, 40, 60, greenCov), 200)
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	if !b.ThumbnailUploaded || b.ThumbnailUrl == books[0].ThumbnailUrl {
		t.Fatalf("after upload: uploaded %v url %q", b.ThumbnailUploaded, b.ThumbnailUrl)
	}
	if r, g, bl := thumbColor(t, e.admin, b.ThumbnailUrl); !isGreen(r, g, bl) {
		t.Fatalf("uploaded thumbnail not green: %d %d %d", r, g, bl)
	}

	// Re-analysis keeps the upload.
	e.admin.json("POST", "/api/v1/libraries/"+l.Id+"/scan?deep=true", nil, 202, nil)
	e.waitTasks()
	e.admin.json("GET", "/api/v1/books/"+id, nil, 200, &b)
	if !b.ThumbnailUploaded {
		t.Fatal("deep scan dropped the uploaded thumbnail")
	}
	if r, g, bl := thumbColor(t, e.admin, b.ThumbnailUrl); !isGreen(r, g, bl) {
		t.Fatalf("deep scan replaced the upload: %d %d %d", r, g, bl)
	}

	// Regeneration makes the thumbnail from the page again.
	e.admin.json("POST", "/api/v1/books/"+id+"/regenerate-thumbnail", nil, 204, nil)
	e.admin.json("GET", "/api/v1/books/"+id, nil, 200, &b)
	if b.ThumbnailUploaded {
		t.Fatal("regeneration kept the upload flag")
	}
	if r, _, bl := thumbColor(t, e.admin, b.ThumbnailUrl); !isRed(r, 0, bl) {
		t.Fatalf("regenerated thumbnail not from the page: %d %d", r, bl)
	}
}

func TestCoverBookChoice(t *testing.T) {
	e := newEnv(t)
	root := t.TempDir()
	writeCBZ(t, filepath.Join(root, "S", "S v01.cbz"), map[string][]byte{"1.png": pngBytes(t, 20, 30, redCover)})
	writeCBZ(t, filepath.Join(root, "S", "S v02.cbz"), map[string][]byte{"1.png": pngBytes(t, 20, 30, blueCover)})
	writeCBZ(t, filepath.Join(root, "T", "T v01.cbz"), map[string][]byte{"1.png": pngBytes(t, 20, 30, redCover)})
	l := e.libraryWith("漫画", root)
	var ss []Series
	e.admin.json("GET", "/api/v1/series?libraryId="+l.Id, nil, 200, &ss)
	var s, other Series
	for _, x := range ss {
		if x.Title == "S" {
			s = x
		} else {
			other = x
		}
	}
	bs := booksOf(e, s.Id)
	ts := booksOf(e, other.Id)
	if s.CoverBookId != nil {
		t.Fatalf("cover book before any choice: %v", *s.CoverBookId)
	}
	if r, _, b := thumbColor(t, e.admin, s.ThumbnailUrl); !isRed(r, 0, b) {
		t.Fatalf("default series cover not the first book: %d %d", r, b)
	}

	var got Series
	// A book of another series is not a cover of this one.
	e.admin.json("PATCH", "/api/v1/series/"+s.Id, map[string]any{"coverBookId": ts[0].Id}, 400, nil)
	e.admin.json("PATCH", "/api/v1/series/"+s.Id, map[string]any{"coverBookId": "nope"}, 400, nil)
	e.admin.json("PATCH", "/api/v1/series/"+s.Id, map[string]any{"coverBookId": bs[1].Id}, 200, &got)
	if got.CoverBookId == nil || *got.CoverBookId != bs[1].Id {
		t.Fatalf("cover book after choice: %v", got.CoverBookId)
	}
	if got.ThumbnailUrl == s.ThumbnailUrl {
		t.Fatal("series URL did not change with its cover book")
	}
	if r, _, b := thumbColor(t, e.admin, got.ThumbnailUrl); !isBlue(r, 0, b) {
		t.Fatalf("chosen cover not blue: %d %d", r, b)
	}

	// Deleting the chosen book reports no cover book and falls back to the first.
	if _, err := e.db.Exec(`UPDATE books SET deleted_at = ? WHERE id = ?`, time.Now().UTC().Format(time.RFC3339Nano), bs[1].Id); err != nil {
		t.Fatal(err)
	}
	e.admin.json("GET", "/api/v1/series/"+s.Id, nil, 200, &got)
	if got.CoverBookId != nil {
		t.Fatalf("deleted cover book still reported: %v", *got.CoverBookId)
	}
	if r, _, b := thumbColor(t, e.admin, got.ThumbnailUrl); !isRed(r, 0, b) {
		t.Fatalf("cover after the chosen book went: %d %d", r, b)
	}

	// Null clears the choice; it is not a locked field.
	if _, err := e.db.Exec(`UPDATE books SET deleted_at = NULL WHERE id = ?`, bs[1].Id); err != nil {
		t.Fatal(err)
	}
	e.admin.json("PATCH", "/api/v1/series/"+s.Id, map[string]any{"coverBookId": bs[1].Id}, 200, &got)
	e.admin.json("PATCH", "/api/v1/series/"+s.Id, map[string]any{"coverBookId": nil}, 200, &got)
	if got.CoverBookId != nil || len(got.LockedFields) != 0 {
		t.Fatalf("after null: cover %v locks %v", got.CoverBookId, got.LockedFields)
	}
}

func TestLocalArtwork(t *testing.T) {
	e := newEnv(t)
	root := t.TempDir()
	writeCBZ(t, filepath.Join(root, "S", "S v01.cbz"), map[string][]byte{"1.png": pngBytes(t, 20, 30, redCover)})
	art := filepath.Join(root, "S", "Cover.PNG")
	if err := os.WriteFile(art, pngBytes(t, 40, 60, blueCover), 0o644); err != nil {
		t.Fatal(err)
	}
	var l Library
	e.admin.json("POST", "/api/v1/libraries", map[string]any{"name": "漫画", "root": root, "kind": "manga", "importLocalArtwork": true}, 201, &l)
	e.waitTasks()
	if !l.ImportLocalArtwork {
		t.Fatal("library created without local artwork")
	}
	s, _ := e.onlySeries(l.Id)
	if s.ThumbnailSource != SeriesThumbnailSourceLocal {
		t.Fatalf("scan did not pick up the artwork: %q", s.ThumbnailSource)
	}
	if r, _, b := thumbColor(t, e.admin, s.ThumbnailUrl); !isBlue(r, 0, b) {
		t.Fatalf("local artwork not blue: %d %d", r, b)
	}

	// Removing the file drops the artwork at the next scan.
	if err := os.Remove(art); err != nil {
		t.Fatal(err)
	}
	e.rescan(l.Id)
	e.admin.json("GET", "/api/v1/series/"+s.Id, nil, 200, &s)
	if s.ThumbnailSource != SeriesThumbnailSourceBook {
		t.Fatalf("artwork kept after its file went: %q", s.ThumbnailSource)
	}

	// Back again; with the option off, a scan drops it.
	if err := os.WriteFile(art, pngBytes(t, 40, 60, blueCover), 0o644); err != nil {
		t.Fatal(err)
	}
	e.rescan(l.Id)
	e.admin.json("GET", "/api/v1/series/"+s.Id, nil, 200, &s)
	if s.ThumbnailSource != SeriesThumbnailSourceLocal {
		t.Fatalf("artwork not picked up again: %q", s.ThumbnailSource)
	}
	e.admin.json("PATCH", "/api/v1/libraries/"+l.Id, map[string]any{"importLocalArtwork": false}, 200, &l)
	if l.ImportLocalArtwork {
		t.Fatal("importLocalArtwork not updated")
	}
	e.rescan(l.Id)
	e.admin.json("GET", "/api/v1/series/"+s.Id, nil, 200, &s)
	if s.ThumbnailSource != SeriesThumbnailSourceBook {
		t.Fatalf("artwork kept with the option off: %q", s.ThumbnailSource)
	}

	// An upload is never replaced by artwork, and dropping it brings the
	// artwork back at once.
	e.admin.json("PATCH", "/api/v1/libraries/"+l.Id, map[string]any{"importLocalArtwork": true}, 200, &l)
	e.rescan(l.Id)
	rawPutCode(e, "/api/v1/series/"+s.Id+"/thumbnail", pngBytes(t, 40, 60, greenCov), 200)
	if err := os.Remove(art); err != nil {
		t.Fatal(err)
	}
	e.rescan(l.Id)
	e.admin.json("GET", "/api/v1/series/"+s.Id, nil, 200, &s)
	if s.ThumbnailSource != SeriesThumbnailSourceUpload {
		t.Fatalf("scan replaced the upload: %q", s.ThumbnailSource)
	}
	if err := os.WriteFile(art, pngBytes(t, 40, 60, blueCover), 0o644); err != nil {
		t.Fatal(err)
	}
	e.admin.json("DELETE", "/api/v1/series/"+s.Id+"/thumbnail", nil, 200, &s)
	if s.ThumbnailSource != SeriesThumbnailSourceLocal {
		t.Fatalf("dropping the upload did not bring the artwork back: %q", s.ThumbnailSource)
	}
}

// thumbFiles lists the file names in the thumbnail folder.
func thumbFiles(t *testing.T, dir string) []string {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range ents {
		names = append(names, e.Name())
	}
	return names
}

// trashedInside is a ThumbSize hook that trashes a row. The upload reads the
// row before it asks for the thumbnail box and stores the file after, so this
// is a scan trashing the row in between.
func trashedInside(t *testing.T, e *env, table, id string) func(context.Context) (int, int) {
	return func(context.Context) (int, int) {
		if _, err := e.db.Exec("UPDATE "+table+" SET deleted_at = 'x' WHERE id = ?", id); err != nil {
			t.Fatal(err)
		}
		return 400, 600
	}
}

// An upload whose row a scan trashes meanwhile answers not found, keeps the
// old thumbnail file and leaves no new file behind.
func TestUploadRowTrashedMeanwhile(t *testing.T) {
	e := newEnv(t)
	root := t.TempDir()
	writeCBZ(t, filepath.Join(root, "S", "S v01.cbz"), map[string][]byte{"1.png": pngBytes(t, 20, 30, redCover)})
	l := e.libraryWith("漫画", root)
	s, books := e.onlySeries(l.Id)
	rawPutCode(e, "/api/v1/series/"+s.Id+"/thumbnail", pngBytes(t, 40, 60, blueCover), 200)
	thumbs := filepath.Join(e.data, "thumbs")

	svc := &library.Service{DB: e.db, ThumbDir: thumbs, ThumbSize: trashedInside(t, e, "series", s.Id)}
	before := thumbFiles(t, thumbs)
	if err := svc.SetSeriesUpload(context.Background(), s.Id, pngBytes(t, 40, 60, greenCov)); !errors.Is(err, library.ErrNotFound) {
		t.Fatalf("series upload to a trashed row: %v", err)
	}
	if after := thumbFiles(t, thumbs); !slices.Equal(before, after) {
		t.Fatalf("series files changed: before %v, after %v", before, after)
	}

	svc.ThumbSize = trashedInside(t, e, "books", books[0].Id)
	before = thumbFiles(t, thumbs)
	if err := svc.SetBookUpload(context.Background(), books[0].Id, pngBytes(t, 40, 60, greenCov)); !errors.Is(err, library.ErrNotFound) {
		t.Fatalf("book upload to a trashed row: %v", err)
	}
	if after := thumbFiles(t, thumbs); !slices.Equal(before, after) {
		t.Fatalf("book files changed: before %v, after %v", before, after)
	}
}

// libraryEvents counts the library events waiting on sub.
func libraryEvents(sub *events.Sub) int {
	n := 0
	for {
		select {
		case ev := <-sub.C:
			if ev.Type == "library" {
				n++
			}
		default:
			return n
		}
	}
}

// A scan that changes only a series cover still publishes the library event
// that open screens listen to; a scan that changes nothing publishes none.
func TestArtworkOnlyScanPublishes(t *testing.T) {
	e := newEnv(t)
	root := t.TempDir()
	writeCBZ(t, filepath.Join(root, "S", "S v01.cbz"), map[string][]byte{"1.png": pngBytes(t, 20, 30, redCover)})
	var l Library
	e.admin.json("POST", "/api/v1/libraries", map[string]any{"name": "漫画", "root": root, "kind": "manga", "importLocalArtwork": true}, 201, &l)
	e.waitTasks()
	sub, err := e.hub.Subscribe("artwork-test")
	if err != nil {
		t.Fatal(err)
	}
	defer e.hub.Unsubscribe(sub)

	e.rescan(l.Id)
	if n := libraryEvents(sub); n != 0 {
		t.Fatalf("an idle scan published %d library events", n)
	}
	if err := os.WriteFile(filepath.Join(root, "S", "cover.png"), pngBytes(t, 40, 60, blueCover), 0o644); err != nil {
		t.Fatal(err)
	}
	e.rescan(l.Id)
	s, _ := e.onlySeries(l.Id)
	if s.ThumbnailSource != SeriesThumbnailSourceLocal {
		t.Fatalf("artwork not picked up: %q", s.ThumbnailSource)
	}
	if n := libraryEvents(sub); n != 1 {
		t.Fatalf("an artwork-only scan published %d library events, want 1", n)
	}
}
