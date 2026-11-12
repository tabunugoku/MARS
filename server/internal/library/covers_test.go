package library

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"path/filepath"
	"testing"
	"time"

	"github.com/tabunugoku/cominavi/server/internal/db"
	"github.com/tabunugoku/cominavi/server/internal/events"
)

func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for x := range 8 {
		for y := range 8 {
			img.Set(x, y, color.RGBA{R: 200, G: 40, B: 40, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// waitLibraryEvent reads the next event from sub and checks it is a
// "library" event for libraryID.
func waitLibraryEvent(t *testing.T, sub *events.Sub, libraryID string) {
	t.Helper()
	select {
	case ev := <-sub.C:
		if ev.Type != "library" || ev.LibraryID != libraryID {
			t.Fatalf("event = %s for %q, want library for %q", ev.Type, ev.LibraryID, libraryID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no event published")
	}
}

func TestSeriesCoverChangesPublishLibraryEvent(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	const ts = "2026-01-01T00:00:00Z"
	if _, err := conn.ExecContext(ctx,
		`INSERT INTO libraries (id, name, root, kind, watch, created_at) VALUES ('lib1', 'L', ?, 'manga', 0, ?)`,
		t.TempDir(), ts); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx,
		`INSERT INTO series (id, library_id, dir, title, title_sort, created_at, updated_at) VALUES ('s1', 'lib1', 'Series', 'Series', 'Series', ?, ?)`,
		ts, ts); err != nil {
		t.Fatal(err)
	}

	hub := events.New()
	defer hub.Close()
	sub, err := hub.Subscribe("test")
	if err != nil {
		t.Fatal(err)
	}
	defer hub.Unsubscribe(sub)
	s := &Service{DB: conn, ThumbDir: t.TempDir(), Events: hub}

	if err := s.SetSeriesUpload(ctx, "s1", testPNG(t)); err != nil {
		t.Fatal(err)
	}
	waitLibraryEvent(t, sub, "lib1")

	if err := s.DropSeriesUpload(ctx, "s1"); err != nil {
		t.Fatal(err)
	}
	waitLibraryEvent(t, sub, "lib1")
}
