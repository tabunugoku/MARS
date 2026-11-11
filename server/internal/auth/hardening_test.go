package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

// In-flight attempts count: a burst cannot outrun the first failures.
func TestLimiterBeginReservesBudget(t *testing.T) {
	var l Limiter
	now := time.Now()
	for i := range limitMaxFails {
		if ok, _ := l.Begin("k", now); !ok {
			t.Fatalf("attempt %d refused", i)
		}
	}
	if ok, wait := l.Begin("k", now); ok || wait <= 0 {
		t.Fatalf("6th parallel attempt allowed (wait %v)", wait)
	}
	for range limitMaxFails {
		l.End("k", true, now)
	}
	if ok, wait := l.Begin("k", now.Add(time.Minute)); ok || wait < 10*time.Minute {
		t.Fatalf("not blocked after 5 failures (wait %v)", wait)
	}
}

func TestLimiterEndSuccessClears(t *testing.T) {
	var l Limiter
	now := time.Now()
	for range limitMaxFails - 1 {
		l.Begin("k", now)
		l.End("k", true, now)
	}
	l.Begin("k", now)
	l.End("k", false, now)
	for i := range limitMaxFails {
		if ok, _ := l.Begin("k", now); !ok {
			t.Fatalf("attempt %d after success refused", i)
		}
	}
}

func TestVerifyPasswordCancelled(t *testing.T) {
	h, err := HashPassword("a long password")
	if err != nil {
		t.Fatal(err)
	}
	// Hold both slots so the check has to wait, then cancel it.
	hashSlots <- struct{}{}
	hashSlots <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = VerifyPassword(ctx, h, "a long password")
	<-hashSlots
	<-hashSlots
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v, want context.Canceled", err)
	}
	if ok, err := VerifyPassword(context.Background(), h, "a long password"); !ok || err != nil {
		t.Fatalf("after release: %v %v", ok, err)
	}
}

func TestVerifyPasswordQueueFull(t *testing.T) {
	for range cap(hashQueue) {
		hashQueue <- struct{}{}
	}
	defer func() {
		for range cap(hashQueue) {
			<-hashQueue
		}
	}()
	if _, err := VerifyPassword(context.Background(), "$argon2id$v=19$m=19456,t=2,p=1$c2FsdHNhbHRzYWx0$a2V5a2V5a2V5a2V5a2V5a2V5a2V5a2V5a2V5a2V5a2U", "x"); !errors.Is(err, ErrBusy) {
		t.Fatalf("err %v, want ErrBusy", err)
	}
}

func TestCreateSessionIfPassword(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	var hash string
	if err := s.DB.QueryRowContext(ctx, `SELECT password_hash FROM users WHERE id = 'u1'`).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CreateSessionIfPassword(ctx, "u1", hash, time.Hour, false); err != nil {
		t.Fatalf("current hash: %v", err)
	}
	if _, err := s.DB.ExecContext(ctx, `UPDATE users SET password_hash = 'changed' WHERE id = 'u1'`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CreateSessionIfPassword(ctx, "u1", hash, time.Hour, false); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("stale hash: %v", err)
	}
}
