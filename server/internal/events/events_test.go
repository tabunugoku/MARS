package events

import (
	"errors"
	"testing"
)

func TestLimits(t *testing.T) {
	h := New()
	var mine []*Sub
	for range MaxPerKey {
		s, err := h.Subscribe("u1")
		if err != nil {
			t.Fatal(err)
		}
		mine = append(mine, s)
	}
	if _, err := h.Subscribe("u1"); !errors.Is(err, ErrTooMany) {
		t.Fatalf("stream past the per-user limit: %v", err)
	}
	if _, err := h.Subscribe("u2"); err != nil {
		t.Fatalf("another user: %v", err)
	}
	h.Unsubscribe(mine[0])
	h.Unsubscribe(mine[0]) // twice frees one slot only
	if _, err := h.Subscribe("u1"); err != nil {
		t.Fatalf("after a disconnect: %v", err)
	}
	if _, err := h.Subscribe("u1"); !errors.Is(err, ErrTooMany) {
		t.Fatal("a double unsubscribe freed two slots")
	}

	// A stream dropped for falling behind frees its slot too.
	for range Buffer + 1 {
		h.Publish(Event{Type: "task"})
	}
	if h.Len() != 0 {
		t.Fatalf("%d slow streams kept", h.Len())
	}
	for _, s := range mine[1:] {
		h.Unsubscribe(s) // the handler still unsubscribes after the drop
	}
	if _, err := h.Subscribe("u1"); err != nil {
		t.Fatalf("after slow streams were dropped: %v", err)
	}
	if n := h.perKey["u1"]; n != 1 {
		t.Fatalf("perKey u1 = %d, want 1", n)
	}
}

func TestTotalLimit(t *testing.T) {
	h := New()
	for i := range MaxTotal {
		if _, err := h.Subscribe(string(rune('a' + i%26)) + string(rune(i))); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := h.Subscribe("late"); !errors.Is(err, ErrTooMany) {
		t.Fatalf("stream past the total: %v", err)
	}
}
