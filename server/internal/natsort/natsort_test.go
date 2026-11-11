package natsort

import (
	"slices"
	"testing"
)

func TestLess(t *testing.T) {
	got := []string{"p10.jpg", "P2.jpg", "p1.jpg", "p01.jpg", "第１０巻", "第2巻", "a", "B"}
	slices.SortFunc(got, func(a, b string) int {
		if Less(a, b) {
			return -1
		}
		if Less(b, a) {
			return 1
		}
		return 0
	})
	want := []string{"a", "B", "p01.jpg", "p1.jpg", "P2.jpg", "p10.jpg", "第2巻", "第１０巻"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q", got)
	}
}
