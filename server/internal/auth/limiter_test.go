package auth

import (
	"strconv"
	"testing"
	"time"
)

func TestLimiterBlocksOnFifthFailure(t *testing.T) {
	var l Limiter
	t0 := time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)

	for i := 0; i < 4; i++ {
		l.Fail("ip:1.2.3.4", t0)
	}
	if ok, _ := l.Allow("ip:1.2.3.4", t0); !ok {
		t.Fatal("blocked after 4 failures")
	}

	l.Fail("ip:1.2.3.4", t0)
	if ok, wait := l.Allow("ip:1.2.3.4", t0); ok || wait != 15*time.Minute {
		t.Fatalf("after 5 failures: ok=%v wait=%v", ok, wait)
	}
	if ok, wait := l.Allow("ip:1.2.3.4", t0.Add(14*time.Minute+59*time.Second)); ok || wait != time.Second {
		t.Fatalf("at 14:59: ok=%v wait=%v", ok, wait)
	}
	if ok, _ := l.Allow("ip:1.2.3.4", t0.Add(15*time.Minute)); !ok {
		t.Fatal("still blocked 15 minutes after the fifth failure")
	}
}

func TestLimiterFailuresOutsideWindowDoNotCount(t *testing.T) {
	var l Limiter
	t0 := time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 4; i++ {
		l.Fail("user:bob", t0)
	}
	l.Fail("user:bob", t0.Add(16*time.Minute))
	if ok, _ := l.Allow("user:bob", t0.Add(16*time.Minute)); !ok {
		t.Fatal("old failures still counted after the window")
	}
}

func TestLimiterKeysAreIndependent(t *testing.T) {
	var l Limiter
	t0 := time.Now()
	for i := 0; i < 5; i++ {
		l.Fail("ip:a", t0)
	}
	if ok, _ := l.Allow("ip:b", t0); !ok {
		t.Fatal("other key blocked")
	}
}

func TestLimiterInFlightFailuresDoNotExtendBlock(t *testing.T) {
	var l Limiter
	t0 := time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		l.Fail("k", t0)
	}
	l.Fail("k", t0.Add(10*time.Minute))
	if ok, _ := l.Allow("k", t0.Add(15*time.Minute)); !ok {
		t.Fatal("a failure during the block extended it")
	}
}

func TestLimiterReset(t *testing.T) {
	var l Limiter
	t0 := time.Now()
	for i := 0; i < 5; i++ {
		l.Fail("user:amy", t0)
	}
	l.Reset("user:amy")
	if ok, _ := l.Allow("user:amy", t0); !ok {
		t.Fatal("still blocked after Reset")
	}
}

func TestLimiterZeroValueAllow(t *testing.T) {
	var l Limiter
	if ok, _ := l.Allow("x", time.Now()); !ok {
		t.Fatal("zero limiter blocked a key")
	}
	l.Reset("x")
}

func TestLimiterPrunesWhenFull(t *testing.T) {
	var l Limiter
	t0 := time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)
	for i := 0; i < limitMaxKeys; i++ {
		l.Fail(strconv.Itoa(i), t0)
	}
	later := t0.Add(time.Hour)
	l.Fail("fresh", later)
	if n := len(l.keys); n != 1 {
		t.Fatalf("expired keys not pruned: %d left", n)
	}
}
