package httpapi

import (
	"bufio"
	"encoding/json"
	"image/color"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tabunugoku/mars/server/internal/events"
)

// sse is one message read from a stream: an event, or a comment line
// (Type "", Comment set).
type sse struct {
	Type, Data, Comment string
}

// stream opens GET /api/v1/events on srv as c and returns its messages.
// The channel closes when the server ends the stream.
func stream(t *testing.T, srv *httptest.Server, c client) <-chan sse {
	t.Helper()
	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/events", nil)
	if c.cookie != "" {
		req.AddCookie(&http.Cookie{Name: cookieName, Value: c.cookie})
	}
	if c.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearer)
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("events: %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
	out := make(chan sse, 1024)
	go func() {
		defer close(out)
		sc := bufio.NewScanner(res.Body)
		var cur sse
		for sc.Scan() {
			line := sc.Text()
			switch {
			case line == "":
				if cur.Type != "" {
					out <- cur
				}
				cur = sse{}
			case strings.HasPrefix(line, ":"):
				out <- sse{Comment: strings.TrimSpace(line[1:])}
			case strings.HasPrefix(line, "event: "):
				cur.Type = line[len("event: "):]
			case strings.HasPrefix(line, "data: "):
				cur.Data = line[len("data: "):]
			}
		}
	}()
	return out
}

// until collects events (not comments) from ch up to and including the
// first one of type stop.
func until(t *testing.T, ch <-chan sse, stop string) []sse {
	t.Helper()
	var got []sse
	timeout := time.After(10 * time.Second)
	for {
		select {
		case m, ok := <-ch:
			if !ok {
				t.Fatalf("stream ended before %q; got %+v", stop, got)
			}
			if m.Type == "" {
				continue
			}
			got = append(got, m)
			if m.Type == stop {
				return got
			}
		case <-timeout:
			t.Fatalf("no %q event; got %+v", stop, got)
		}
	}
}

func waitSubs(t *testing.T, h *events.Hub, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for h.Len() < n {
		if time.Now().After(deadline) {
			t.Fatalf("%d streams subscribed, want %d", h.Len(), n)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestEventsFilteredPerUser(t *testing.T) {
	e := newEnv(t)
	red := pngBytes(t, 40, 60, color.RGBA{200, 0, 0, 255})
	rootA, rootB := t.TempDir(), t.TempDir()
	writeCBZ(t, filepath.Join(rootA, "Alpha", "Alpha v01.cbz"), map[string][]byte{"1.png": red})
	writeCBZ(t, filepath.Join(rootB, "Beta", "Beta v01.cbz"), map[string][]byte{"1.png": red})
	var a, b Library
	e.admin.json("POST", "/api/v1/libraries", map[string]any{"name": "A", "root": rootA, "kind": "manga"}, 201, &a)
	e.admin.json("POST", "/api/v1/libraries", map[string]any{"name": "B", "root": rootB, "kind": "manga"}, 201, &b)
	e.waitTasks()
	var reader, other User
	e.admin.json("POST", "/api/v1/users", map[string]any{"name": "読者", "email": "reader@example.com", "password": "reader pass", "role": "user",
		"allLibraries": false, "libraryIds": []string{a.Id}}, 201, &reader)
	e.admin.json("POST", "/api/v1/users", map[string]any{"name": "全部", "email": "all@example.com", "password": "reader pass", "role": "user",
		"allLibraries": true}, 201, &other)
	r, o := e.login("読者", "reader pass"), e.login("全部", "reader pass")

	srv := httptest.NewServer(e.h)
	t.Cleanup(srv.Close) // after the streams' bodies close (cleanups run last-in first-out)
	adminCh, readerCh, otherCh := stream(t, srv, e.admin), stream(t, srv, r), stream(t, srv, o)
	waitSubs(t, e.hub, 3)

	var me User
	e.admin.json("GET", "/api/v1/me", nil, 200, &me)
	pub := func(ev events.Event) { e.hub.Publish(ev) }
	pub(events.Event{Type: "library", LibraryID: b.Id, Data: events.Library{LibraryID: b.Id}})
	pub(events.Event{Type: "book", LibraryID: a.Id, Data: events.Book{LibraryID: a.Id}})
	pub(events.Event{Type: "task", AdminOnly: true, Data: map[string]string{"id": "t1"}})
	pub(events.Event{Type: "progress", UserID: reader.Id, LibraryID: a.Id, Data: events.Progress{LibraryID: a.Id}})
	// Even the owner may not see progress in a library hidden from them.
	pub(events.Event{Type: "progress", UserID: reader.Id, LibraryID: b.Id, Data: events.Progress{LibraryID: b.Id}})
	pub(events.Event{Type: "progress", UserID: me.Id, LibraryID: a.Id, Data: events.Progress{LibraryID: a.Id}})

	// Real progress writes publish to their owner only.
	var ss []Series
	var books []Book
	r.json("GET", "/api/v1/series", nil, 200, &ss)
	r.json("GET", "/api/v1/series/"+ss[0].Id+"/books", nil, 200, &books)
	r.json("PUT", "/api/v1/books/"+books[0].Id+"/progress", map[string]any{"page": 1, "writer": "tab-1", "seq": 1}, 200, nil)
	pub(events.Event{Type: "end"})

	types := func(ms []sse) string {
		var s []string
		for _, m := range ms {
			s = append(s, m.Type)
		}
		return strings.Join(s, ",")
	}
	if got := types(until(t, adminCh, "end")); got != "library,book,task,progress,end" {
		t.Errorf("admin got %s", got)
	}
	readerGot := until(t, readerCh, "end")
	if got := types(readerGot); got != "book,progress,progress,end" {
		t.Errorf("restricted reader got %s", got)
	}
	var p events.Progress
	if err := json.Unmarshal([]byte(readerGot[2].Data), &p); err != nil || p.BookID != books[0].Id || p.Writer != "tab-1" {
		t.Errorf("progress event %s (%v)", readerGot[2].Data, err)
	}
	if got := types(until(t, otherCh, "end")); got != "library,book,end" {
		t.Errorf("all-libraries user got %s", got)
	}

	// Narrowing a user's libraries applies to the open stream by the next
	// heartbeat.
	e.admin.json("PATCH", "/api/v1/users/"+reader.Id, map[string]any{"libraryIds": []string{b.Id}}, 200, nil)
	waitPings(t, readerCh, 2)
	pub(events.Event{Type: "book", LibraryID: a.Id, Data: events.Book{LibraryID: a.Id}})
	pub(events.Event{Type: "end"})
	if got := types(until(t, readerCh, "end")); got != "end" {
		t.Errorf("after narrowing, reader got %s", got)
	}
}

// waitPings waits for n heartbeats on ch.
func waitPings(t *testing.T, ch <-chan sse, n int) {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for n > 0 {
		select {
		case m, ok := <-ch:
			if !ok {
				t.Fatal("stream ended")
			}
			if m.Comment == "ping" {
				n--
			}
		case <-timeout:
			t.Fatal("no heartbeat")
		}
	}
}

func TestEventsHeartbeat(t *testing.T) {
	e := newEnv(t)
	srv := httptest.NewUnstartedServer(e.h)
	// Shorter than the stream lives: the handler must lift the deadline.
	srv.Config.ReadTimeout = 200 * time.Millisecond
	srv.Start()
	t.Cleanup(srv.Close) // after the streams' bodies close (cleanups run last-in first-out)

	ch := stream(t, srv, e.admin)
	if m := <-ch; m.Comment != "connected" {
		t.Fatalf("first message %+v", m)
	}
	start := time.Now()
	waitPings(t, ch, 10) // 10 × 50 ms, well past ReadTimeout
	if time.Since(start) < 400*time.Millisecond {
		t.Fatalf("heartbeats came too fast: %v", time.Since(start))
	}

	// Logging out ends the stream at the next heartbeat.
	e.admin.json("POST", "/api/v1/auth/logout", nil, 204, nil)
	timeout := time.After(5 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-timeout:
			t.Fatal("stream outlived its session")
		}
	}
}

func TestEventsNeedSignIn(t *testing.T) {
	e := newEnv(t)
	if rec := e.anon().do("GET", "/api/v1/events", nil); rec.Code != 401 {
		t.Fatalf("anonymous events: %d", rec.Code)
	}
}

func TestEventsFromScan(t *testing.T) {
	e := newEnv(t)
	srv := httptest.NewServer(e.h)
	t.Cleanup(srv.Close) // after the streams' bodies close (cleanups run last-in first-out)
	ch := stream(t, srv, e.admin)
	waitSubs(t, e.hub, 1)

	red := pngBytes(t, 40, 60, color.RGBA{200, 0, 0, 255})
	root := t.TempDir()
	writeCBZ(t, filepath.Join(root, "Alpha", "Alpha v01.cbz"), map[string][]byte{"1.png": red})
	writeCBZ(t, filepath.Join(root, "Alpha", "Alpha v02.cbz"), map[string][]byte{"1.png": red})
	var lib Library
	e.admin.json("POST", "/api/v1/libraries", map[string]any{"name": "A", "root": root, "kind": "manga"}, 201, &lib)
	e.waitTasks()
	e.hub.Publish(events.Event{Type: "end"})

	got := until(t, ch, "end")
	var books, scanned, doneTasks int
	for _, m := range got {
		switch m.Type {
		case "book":
			var b events.Book
			json.Unmarshal([]byte(m.Data), &b)
			if b.LibraryID == lib.Id && b.MediaStatus == "ready" {
				books++
			}
		case "library":
			var l events.Library
			json.Unmarshal([]byte(m.Data), &l)
			if l.BooksAdded == 2 && l.SeriesAdded == 1 {
				scanned++
			}
		case "task":
			var tk Task
			json.Unmarshal([]byte(m.Data), &tk)
			if tk.State == Done {
				doneTasks++
			}
		}
	}
	if books != 2 || scanned != 1 || doneTasks != 2 {
		t.Fatalf("books=%d scanned=%d done tasks=%d in %+v", books, scanned, doneTasks, got)
	}
}
