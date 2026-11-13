//go:build perf

// Large-library measurements (docs/perf-large-library.md). Not part of the
// normal test run: build with -tags perf.
//
//	PERF_LIB=D:/perf/lib go test -tags perf -run TestPerfGenerate -timeout 2h ./internal/httpapi
//	PERF_LIB=D:/perf/lib PERF_DATA=D:/perf/data PERF_OUT=D:/perf/report.md \
//	  go test -tags perf -run TestPerfLargeLibrary -timeout 4h -v ./internal/httpapi
//
// TestPerfGenerate writes a synthetic library of tiny CBZs (Japanese titles,
// ComicInfo.xml with authors and a summary, ~200 one-colour pages each).
// TestPerfLargeLibrary scans it through the real API with readers writing
// progress the whole time, then times the main read endpoints.
package httpapi

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tabunugoku/mars/server/internal/scan"
)

func envInt(name string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(name)); err == nil && v > 0 {
		return v
	}
	return def
}

var (
	titleA = []string{"蒼き", "黄昏の", "最果ての", "星降る", "銀河の", "鋼の", "放課後の", "真夜中の", "風の", "月下の", "紅の", "白銀の", "名もなき", "遥かなる", "眠れる", "嵐の"}
	titleB = []string{"騎士", "魔法使い", "探偵", "料理人", "剣士", "旅人", "図書館", "学園", "王国", "竜", "猫", "姫", "錬金術師", "海賊", "天使", "怪盗"}
	titleC = []string{"物語", "戦記", "日誌", "譚", "録", "", "伝説", "奇譚", "の日常", "クロニクル"}
	people = []string{"山田太郎", "佐藤花子", "鈴木一郎", "高橋美咲", "田中健", "伊藤さくら", "渡辺翔", "中村葵", "小林蓮", "加藤結衣", "吉田大輔", "山本陽菜"}
	pubs   = []string{"架空書房", "星海社文庫", "青空コミックス", "月光出版", "風見書店"}
	genres = []string{"ファンタジー", "ミステリー", "SF", "日常", "冒険", "恋愛", "ホラー", "歴史"}
)

func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xmlEscapeTo(&b, s)
	return b.String()
}

func xmlEscapeTo(b *bytes.Buffer, s string) error {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	_, err := b.WriteString(r.Replace(s))
	return err
}

// TestPerfGenerate writes PERF_BOOKS (37000) books into PERF_LIB.
func TestPerfGenerate(t *testing.T) {
	root := os.Getenv("PERF_LIB")
	if root == "" {
		t.Skip("PERF_LIB not set")
	}
	total := envInt("PERF_BOOKS", 37000)
	meanPages := envInt("PERF_PAGES", 200)

	img := image.NewGray(image.Rect(0, 0, 16, 24))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	var pb bytes.Buffer
	if err := png.Encode(&pb, img); err != nil {
		t.Fatal(err)
	}
	page := pb.Bytes()

	rng := rand.New(rand.NewPCG(1, 2))
	used := map[string]int{}
	books, series := 0, 0
	start := time.Now()
	summary := strings.Repeat("これは架空の物語のあらすじです。主人公は旅に出て、仲間と出会い、困難に立ち向かう。", 6)
	for books < total {
		var n int
		switch r := rng.Float64(); {
		case r < 0.5:
			n = 1 + rng.IntN(5)
		case r < 0.85:
			n = 6 + rng.IntN(15)
		default:
			n = 21 + rng.IntN(80)
		}
		n = min(n, total-books)
		title := titleA[rng.IntN(len(titleA))] + titleB[rng.IntN(len(titleB))] + titleC[rng.IntN(len(titleC))]
		if c := used[title]; c > 0 {
			used[title]++
			title = fmt.Sprintf("%s %d", title, c+1)
		} else {
			used[title] = 1
		}
		series++
		writer := people[rng.IntN(len(people))]
		artist := people[rng.IntN(len(people))]
		pub := pubs[rng.IntN(len(pubs))]
		genre := genres[rng.IntN(len(genres))] + "," + genres[rng.IntN(len(genres))]
		dir := filepath.Join(root, title)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for v := 1; v <= n; v++ {
			pages := meanPages - 20 + rng.IntN(41)
			ci := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<ComicInfo><Title>%s 第%d巻</Title><Series>%s</Series><Number>%d</Number><Count>%d</Count>
<Summary>%s</Summary><Year>%d</Year><Month>%d</Month><Writer>%s</Writer><Penciller>%s</Penciller>
<Publisher>%s</Publisher><Genre>%s</Genre><LanguageISO>ja</LanguageISO><Manga>YesAndRightToLeft</Manga></ComicInfo>`,
				xmlEscape(title), v, xmlEscape(title), v, n, summary, 2000+rng.IntN(25), 1+rng.IntN(12), writer, artist, pub, genre)
			p := filepath.Join(dir, fmt.Sprintf("%s 第%02d巻.cbz", title, v))
			if err := writeTinyCBZ(p, []byte(ci), page, pages); err != nil {
				t.Fatal(err)
			}
			books++
		}
	}
	t.Logf("wrote %d series, %d books in %s", series, books, time.Since(start))
}

func writeTinyCBZ(p string, ci, page []byte, pages int) error {
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)
	add := func(name string, body []byte) error {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
		if err != nil {
			return err
		}
		_, err = w.Write(body)
		return err
	}
	if err := add("ComicInfo.xml", ci); err != nil {
		f.Close()
		return err
	}
	for i := 1; i <= pages; i++ {
		if err := add(fmt.Sprintf("p%03d.png", i), page); err != nil {
			f.Close()
			return err
		}
	}
	if err := zw.Close(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// latencies collects request timings and failures for one phase.
type latencies struct {
	mu    sync.Mutex
	d     []time.Duration
	fails map[string]int
}

func (l *latencies) add(d time.Duration, fail string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.d = append(l.d, d)
	if fail != "" {
		if l.fails == nil {
			l.fails = map[string]int{}
		}
		l.fails[fail]++
	}
}

func pct(d []time.Duration, p float64) time.Duration {
	if len(d) == 0 {
		return 0
	}
	s := slices.Clone(d)
	slices.Sort(s)
	return s[min(len(s)-1, int(float64(len(s))*p))]
}

func ms(d time.Duration) string { return fmt.Sprintf("%.1f", float64(d.Microseconds())/1000) }

func (l *latencies) summary() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	nf := 0
	var fails []string
	for k, v := range l.fails {
		nf += v
		fails = append(fails, fmt.Sprintf("%s×%d", k, v))
	}
	slices.Sort(fails)
	return fmt.Sprintf("n=%d fail=%d p50=%sms p99=%sms max=%sms %s", len(l.d), nf,
		ms(pct(l.d, .5)), ms(pct(l.d, .99)), ms(pct(l.d, 1)), strings.Join(fails, " "))
}

// load runs progress writers, page-list readers and a write-lock probe
// until stop is called.
type load struct {
	writes, reads, probe latencies
	stop                 func()
}

func startLoad(t *testing.T, e *env, reader client, dbPath, bookID string, pageCount int) *load {
	l := &load{}
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	var seq atomic.Int64
	call := func(c client, method, u string, body any) (int, string) {
		var r *bytes.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			r = bytes.NewReader(b)
		} else {
			r = bytes.NewReader(nil)
		}
		req := httptest.NewRequest(method, u, r)
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: cookieName, Value: c.cookie})
		if !safeMethod(method) {
			req.Header.Set("Origin", "http://"+req.Host)
		}
		rec := httptest.NewRecorder()
		c.h.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}
	classify := func(code int, body string) string {
		if code == 200 {
			return ""
		}
		return strconv.Itoa(code)
	}
	for w := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			writer := fmt.Sprintf("perf-%d", w)
			for ctx.Err() == nil {
				s := int(seq.Add(1))
				t0 := time.Now()
				code, body := call(reader, "PUT", "/api/v1/books/"+bookID+"/progress",
					map[string]any{"page": 1 + s%pageCount, "writer": writer, "seq": s})
				l.writes.add(time.Since(t0), classify(code, body))
				time.Sleep(50 * time.Millisecond)
			}
		}()
	}
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				t0 := time.Now()
				code, body := call(reader, "GET", "/api/v1/books/"+bookID+"/pages", nil)
				l.reads.add(time.Since(t0), classify(code, body))
				time.Sleep(100 * time.Millisecond)
			}
		}()
	}
	// The probe takes the write lock on its own connection (waiting as long
	// as needed) and releases it at once: the wait is how long some other
	// write transaction held the lock.
	probeDB, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?_pragma=busy_timeout(3600000)")
	if err != nil {
		t.Fatal(err)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer probeDB.Close()
		conn, err := probeDB.Conn(context.Background())
		if err != nil {
			l.probe.add(0, err.Error())
			return
		}
		defer conn.Close()
		for ctx.Err() == nil {
			t0 := time.Now()
			_, err := conn.ExecContext(context.Background(), "BEGIN IMMEDIATE")
			d := time.Since(t0)
			fail := ""
			if err != nil {
				fail = err.Error()
			} else {
				conn.ExecContext(context.Background(), "ROLLBACK")
			}
			l.probe.add(d, fail)
			if d > 300*time.Millisecond {
				t.Logf("probe: write lock busy for %s until %s", d.Round(time.Millisecond), time.Now().Format("15:04:05.000"))
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()
	l.stop = func() { cancel(); wg.Wait() }
	return l
}

type taskRow struct {
	kind, state string
	dur         time.Duration
}

// waitIdle waits until every task created after since has finished, and
// returns them.
func waitIdle(t *testing.T, d *sql.DB, since time.Time, want int) []taskRow {
	t.Helper()
	cut := since.UTC().Format(time.RFC3339Nano)
	for {
		rows, err := d.Query(`SELECT kind, state, COALESCE(started_at, ''), COALESCE(finished_at, '') FROM tasks WHERE created_at >= ? ORDER BY created_at`, cut)
		if err != nil {
			t.Fatal(err)
		}
		var out []taskRow
		busy := false
		for rows.Next() {
			var r taskRow
			var st, fin string
			rows.Scan(&r.kind, &r.state, &st, &fin)
			if r.state == "running" || r.state == "queued" {
				busy = true
			}
			if a, err := time.Parse(time.RFC3339Nano, st); err == nil {
				if b, err := time.Parse(time.RFC3339Nano, fin); err == nil {
					r.dur = b.Sub(a)
				}
			}
			out = append(out, r)
		}
		rows.Close()
		if !busy && len(out) >= want {
			for _, r := range out {
				if r.state != "done" {
					t.Fatalf("task %s ended %s", r.kind, r.state)
				}
			}
			return out
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func fileSize(p string) int64 {
	st, err := os.Stat(p)
	if err != nil {
		return 0
	}
	return st.Size()
}

func mb(n int64) string { return fmt.Sprintf("%.1f MB", float64(n)/(1<<20)) }

func TestPerfLargeLibrary(t *testing.T) {
	libRoot := os.Getenv("PERF_LIB")
	data := os.Getenv("PERF_DATA")
	if libRoot == "" || data == "" {
		t.Skip("PERF_LIB and PERF_DATA not set")
	}
	var report strings.Builder
	out := func(format string, args ...any) {
		line := fmt.Sprintf(format, args...)
		t.Log(line)
		report.WriteString(line + "\n")
	}
	defer func() {
		if p := os.Getenv("PERF_OUT"); p != "" {
			os.WriteFile(p, []byte(report.String()), 0o644)
		}
	}()

	if err := os.RemoveAll(data); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	e := newEnvAt(t, data)
	dbPath := filepath.Join(data, "t.db")

	// A small library whose book the readers write progress on.
	small := filepath.Join(data, "small")
	pg := pngBytes(t, 8, 8, color.Gray{Y: 128})
	for i := 1; i <= 3; i++ {
		writeCBZ(t, filepath.Join(small, "既読シリーズ", fmt.Sprintf("既読シリーズ 第%d巻.cbz", i)),
			map[string][]byte{"1.png": pg, "2.png": pg, "3.png": pg, "4.png": pg})
	}
	var smallLib, bigLib Library
	t0 := time.Now()
	e.admin.json("POST", "/api/v1/libraries", map[string]any{"name": "小", "root": small, "kind": "manga"}, 201, &smallLib)
	waitIdle(t, e.db, t0, 2)
	var smallBook string
	if err := e.db.QueryRow(`SELECT id FROM books WHERE library_id = ? ORDER BY path LIMIT 1`, smallLib.Id).Scan(&smallBook); err != nil {
		t.Fatal(err)
	}
	e.admin.json("POST", "/api/v1/users", map[string]any{"name": "perf-reader", "email": "perf-reader@example.com",
		"password": "correct horse", "role": "user", "allLibraries": true}, 201, nil)
	reader := e.login("perf-reader", "correct horse")

	walkStart := time.Now()
	listing, err := scan.Walk(context.Background(), libRoot, nil)
	if err != nil {
		t.Fatal(err)
	}
	nb := 0
	for _, s := range listing.Series {
		nb += len(s.Books)
	}
	out("## Library\n\n%d series, %d books (walk alone: %s)\n", len(listing.Series), nb, time.Since(walkStart).Round(time.Millisecond))

	phase := func(name string, run func() int) {
		l := startLoad(t, e, reader, dbPath, smallBook, 4)
		start := time.Now()
		want := run()
		tasks := waitIdle(t, e.db, start, want)
		wall := time.Since(start)
		l.stop()
		out("### %s\n", name)
		out("- wall time: %s", wall.Round(time.Millisecond))
		for _, tk := range tasks {
			out("- task %s: %s", tk.kind, tk.dur.Round(time.Millisecond))
		}
		out("- progress PUT (4 writers): %s", l.writes.summary())
		out("- page list GET (2 readers): %s", l.reads.summary())
		out("- write-lock wait (probe): %s", l.probe.summary())
		out("- db %s, wal %s, peak RSS %s\n", mb(fileSize(dbPath)), mb(fileSize(dbPath+"-wal")), mb(peakRSS()))
	}

	out("## Scans\n")
	phase("First scan + analysis", func() int {
		e.admin.json("POST", "/api/v1/libraries", map[string]any{"name": "大", "root": libRoot, "kind": "manga"}, 201, &bigLib)
		return 2
	})
	if os.Getenv("PERF_FIRST_ONLY") != "" {
		return
	}
	phase("Rescan, nothing changed", func() int {
		e.admin.json("POST", "/api/v1/libraries/"+bigLib.Id+"/scan", nil, 202, nil)
		return 1
	})
	var touched string
	if err := e.db.QueryRow(`SELECT path FROM books WHERE library_id = ? ORDER BY path LIMIT 1 OFFSET ?`, bigLib.Id, nb/2).Scan(&touched); err != nil {
		t.Fatal(err)
	}
	phase("Rescan, one book changed", func() int {
		now := time.Now()
		os.Chtimes(filepath.Join(libRoot, filepath.FromSlash(touched)), now, now)
		e.admin.json("POST", "/api/v1/libraries/"+bigLib.Id+"/scan", nil, 202, nil)
		return 2
	})

	// Reading history like a migrated Komga user: every 4th series read
	// halfway, every 16th with the next book in progress.
	type ser struct {
		id    string
		books []string
	}
	var all []ser
	rows, err := e.db.Query(`SELECT series_id, id FROM books WHERE library_id = ? AND deleted_at IS NULL ORDER BY series_id, number_sort, path`, bigLib.Id)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var sid, bid string
		rows.Scan(&sid, &bid)
		if len(all) == 0 || all[len(all)-1].id != sid {
			all = append(all, ser{id: sid})
		}
		all[len(all)-1].books = append(all[len(all)-1].books, bid)
	}
	rows.Close()
	var uid string
	e.db.QueryRow(`SELECT id FROM users WHERE name = 'perf-reader'`).Scan(&uid)
	tx, _ := e.db.Begin()
	completed, inProgress := 0, 0
	for i, s := range all {
		if i%4 != 0 {
			continue
		}
		half := (len(s.books) + 1) / 2
		for j, b := range s.books[:half] {
			tx.Exec(`INSERT INTO read_progress (user_id, book_id, page, completed, read_at) VALUES (?, ?, 180, 1, ?)`,
				uid, b, time.Now().Add(-time.Duration(i*100+j)*time.Minute).UTC().Format(time.RFC3339))
			completed++
		}
		if i%16 == 0 && half < len(s.books) {
			tx.Exec(`INSERT INTO read_progress (user_id, book_id, page, completed, read_at) VALUES (?, ?, 20, 0, ?)`,
				uid, s.books[half], time.Now().Add(-time.Duration(i)*time.Minute).UTC().Format(time.RFC3339))
			inProgress++
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	out("## Reads\n\nReader history: %d books read, %d in progress. 20 runs each, sequential, no other load.\n", completed, inProgress)

	mid := all[len(all)/2]
	longest := all[0]
	for _, s := range all {
		if len(s.books) > len(longest.books) {
			longest = s
		}
	}
	bookID := longest.books[len(longest.books)/2]
	out("| request | p50 ms | p95 ms | max ms | bytes |")
	out("|---|---:|---:|---:|---:|")
	measure := func(name string, c client, u string) {
		var d []time.Duration
		size := 0
		for range 20 {
			t0 := time.Now()
			rec := c.do("GET", u, nil)
			d = append(d, time.Since(t0))
			if rec.Code != 200 {
				t.Errorf("%s: %d %s", u, rec.Code, rec.Body)
			}
			size = rec.Body.Len()
		}
		out("| %s | %s | %s | %s | %d |", name, ms(pct(d, .5)), ms(pct(d, .95)), ms(pct(d, 1)), size)
	}
	q := url.QueryEscape
	measure("GET /series (all, as the web app asks)", reader, "/api/v1/series")
	measure("GET /series?libraryId", reader, "/api/v1/series?libraryId="+bigLib.Id)
	measure("GET /series?limit=50", reader, "/api/v1/series?limit=50")
	measure("GET /series?sort=bookCount&limit=50", reader, "/api/v1/series?sort=bookCount&desc=true&limit=50")
	measure("GET /series?sort=releaseDate&limit=50", reader, "/api/v1/series?sort=releaseDate&limit=50")
	measure("GET /series?readState=unread&limit=50", reader, "/api/v1/series?readState=unread&limit=50")
	measure("GET /series?readState=inProgress&limit=50", reader, "/api/v1/series?readState=inProgress&limit=50")
	measure("GET /series?readState=read&limit=50", reader, "/api/v1/series?readState=read&limit=50")
	measure("GET /series?genres=SF&limit=50", reader, "/api/v1/series?genres=SF&limit=50")
	measure("GET /series?search=騎士", reader, "/api/v1/series?search="+q("騎士"))
	measure("GET /dashboard", reader, "/api/v1/dashboard")
	measure("GET /search?q=竜 (1 char)", reader, "/api/v1/search?q="+q("竜"))
	measure("GET /search?q=騎士 (2 chars)", reader, "/api/v1/search?q="+q("騎士"))
	measure("GET /search?q=魔法使い", reader, "/api/v1/search?q="+q("魔法使い"))
	measure("GET /search?q=山田太郎 (author)", reader, "/api/v1/search?q="+q("山田太郎"))
	measure("GET /series/{id}", reader, "/api/v1/series/"+mid.id)
	measure(fmt.Sprintf("GET /series/{id}/books (%d books)", len(longest.books)), reader, "/api/v1/series/"+longest.id+"/books")
	measure("GET /books/{id}", reader, "/api/v1/books/"+bookID)
	measure("GET /books/{id}/pages", reader, "/api/v1/books/"+bookID+"/pages")
	measure("GET /books/{id}/adjacent", reader, "/api/v1/books/"+bookID+"/adjacent")
	measure("GET /libraries", reader, "/api/v1/libraries")

	// Edits that rebuild search, timed with the write lock in mind.
	t1 := time.Now()
	e.admin.json("PATCH", "/api/v1/series/"+mid.id, map[string]any{"summary": "編集"}, 200, nil)
	out("\nPATCH /series/{id}: %s", ms(time.Since(t1)))
	t1 = time.Now()
	e.admin.json("PATCH", "/api/v1/books/"+bookID, map[string]any{"summary": "編集"}, 200, nil)
	out("PATCH /books/{id}: %s ms", ms(time.Since(t1)))
	out("\nFinal: db %s, wal %s, peak RSS %s", mb(fileSize(dbPath)), mb(fileSize(dbPath+"-wal")), mb(peakRSS()))
}
