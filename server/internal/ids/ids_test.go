package ids

import (
	"strings"
	"testing"
	"time"
)

func TestNewSortsByTime(t *testing.T) {
	a := New()
	time.Sleep(2 * time.Millisecond)
	b := New()
	if len(a) != 26 || len(b) != 26 {
		t.Fatalf("lengths %d %d", len(a), len(b))
	}
	if !(a < b) {
		t.Fatalf("%s !< %s", a, b)
	}
	if strings.Trim(a, alphabet) != "" {
		t.Fatalf("bad alphabet in %s", a)
	}
}
