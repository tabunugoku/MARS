package library

import (
	"context"
	"testing"
)

func TestThumbBoundsDefault(t *testing.T) {
	var s Service
	if w, h := s.thumbBounds(context.Background()); w != 400 || h != 600 {
		t.Errorf("nil ThumbSize gave %dx%d, want 400x600", w, h)
	}
	s.ThumbSize = func(context.Context) (int, int) { return 800, 1200 }
	if w, h := s.thumbBounds(context.Background()); w != 800 || h != 1200 {
		t.Errorf("ThumbSize gave %dx%d, want 800x1200", w, h)
	}
}
