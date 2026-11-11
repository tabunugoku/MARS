package auth

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/tabunugoku/cominavi/server/internal/db"
)

// newTestStore opens a fresh database and inserts a library and two users,
// "u1" (with access to lib1) and "u2".
func newTestStore(t *testing.T) Store {
	t.Helper()
	ctx := context.Background()
	d, err := db.Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	now := stamp(time.Now())
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := d.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	mustExec(`INSERT INTO libraries (id, name, root, kind, created_at) VALUES ('lib1', 'Lib', '/lib1', 'manga', ?)`, now)
	mustExec(`INSERT INTO users (id, name, email, password_hash, role, all_libraries, created_at) VALUES ('u1', 'Alice', 'a@example.test', 'x', 'user', 0, ?)`, now)
	mustExec(`INSERT INTO users (id, name, email, password_hash, role, created_at) VALUES ('u2', 'Bob', 'b@example.test', 'x', 'admin', ?)`, now)
	mustExec(`INSERT INTO user_libraries (user_id, library_id) VALUES ('u1', 'lib1')`)
	return Store{DB: d}
}

func countSessions(t *testing.T, s Store) int {
	t.Helper()
	var n int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSessionLifecycle(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	tok, exp, err := s.CreateSession(ctx, "u1", time.Hour, false)
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Until(exp); d < 59*time.Minute || d > time.Hour {
		t.Fatalf("expiry %v not about an hour away", d)
	}

	u, err := s.UserBySession(ctx, tok)
	if err != nil {
		t.Fatal(err)
	}
	if u.ID != "u1" || u.Name != "Alice" || u.Email != "a@example.test" || u.AllLibraries ||
		len(u.LibraryIDs) != 1 || u.LibraryIDs[0] != "lib1" || !u.CanSee("lib1") || u.CanSee("lib2") {
		t.Fatalf("unexpected user %+v", u)
	}

	if err := s.DeleteSession(ctx, tok); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UserBySession(ctx, tok); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("after delete: %v", err)
	}
}

func TestSessionRejectsEmptyAndUnknown(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	for _, tok := range []string{"", "nope"} {
		if _, err := s.UserBySession(ctx, tok); !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("token %q: %v", tok, err)
		}
	}
}

func TestSessionExpiredIsRemoved(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	tok, _, err := s.CreateSession(ctx, "u1", -time.Second, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UserBySession(ctx, tok); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expired session accepted: %v", err)
	}
	if n := countSessions(t, s); n != 0 {
		t.Fatalf("expired row not deleted: %d rows", n)
	}
}

func TestPurgeExpired(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	for i := 0; i < 2; i++ {
		if _, _, err := s.CreateSession(ctx, "u1", -time.Second, false); err != nil {
			t.Fatal(err)
		}
	}
	live, _, err := s.CreateSession(ctx, "u1", time.Hour, true)
	if err != nil {
		t.Fatal(err)
	}
	n, err := s.PurgeExpired(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("purged %d, want 2", n)
	}
	if _, err := s.UserBySession(ctx, live); err != nil {
		t.Fatalf("live session purged: %v", err)
	}
}

func TestDeleteUserSessions(t *testing.T) {
	ctx := context.Background()

	s := newTestStore(t)
	keep, _, _ := s.CreateSession(ctx, "u1", time.Hour, false)
	other1, _, _ := s.CreateSession(ctx, "u1", time.Hour, false)
	other2, _, _ := s.CreateSession(ctx, "u1", time.Hour, false)
	bobTok, _, _ := s.CreateSession(ctx, "u2", time.Hour, false)
	if err := s.DeleteUserSessions(ctx, "u1", keep); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UserBySession(ctx, keep); err != nil {
		t.Fatalf("kept session gone: %v", err)
	}
	for _, tok := range []string{other1, other2} {
		if _, err := s.UserBySession(ctx, tok); !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("other session survived: %v", err)
		}
	}
	if _, err := s.UserBySession(ctx, bobTok); err != nil {
		t.Fatalf("another user's session deleted: %v", err)
	}

	if err := s.DeleteUserSessions(ctx, "u1", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UserBySession(ctx, keep); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("delete-all left a session: %v", err)
	}
}

func TestRefreshSessionSlidesOnlyUnderHalf(t *testing.T) {
	ctx := context.Background()
	const ttl = time.Hour
	tests := []struct {
		name      string
		remaining time.Duration
		extend    bool
	}{
		{"fresh session untouched", 55 * time.Minute, false},
		{"just over half untouched", 31 * time.Minute, false},
		{"under half extended", 20 * time.Minute, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore(t)
			tok, _, err := s.CreateSession(ctx, "u1", ttl, false)
			if err != nil {
				t.Fatal(err)
			}
			want := time.Now().Add(tt.remaining)
			if _, err := s.DB.ExecContext(ctx, `UPDATE sessions SET expires_at = ?`, stamp(want)); err != nil {
				t.Fatal(err)
			}
			got, err := s.RefreshSession(ctx, tok, ttl)
			if err != nil {
				t.Fatal(err)
			}
			if tt.extend {
				if d := time.Until(got); d < 59*time.Minute || d > ttl {
					t.Fatalf("not extended to about an hour: %v", d)
				}
			} else if d := got.Sub(want); d < -time.Second || d > time.Second {
				t.Fatalf("expiry changed: %v", got.Sub(want))
			}
		})
	}
}

func TestRefreshSessionUnknownAndExpired(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	if _, err := s.RefreshSession(ctx, "missing", time.Hour); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("unknown: %v", err)
	}
	tok, _, _ := s.CreateSession(ctx, "u1", -time.Second, false)
	if _, err := s.RefreshSession(ctx, tok, time.Hour); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expired: %v", err)
	}
}

func TestLastActiveTouchedAtMostOncePerMinute(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	tok, _, _ := s.CreateSession(ctx, "u1", time.Hour, false)

	lastActive := func() string {
		var v string
		if err := s.DB.QueryRow(`SELECT last_active FROM users WHERE id = 'u1'`).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}

	if _, err := s.UserBySession(ctx, tok); err != nil {
		t.Fatal(err)
	}
	first := lastActive()
	if first == "" {
		t.Fatal("last_active not set")
	}
	if _, err := s.UserBySession(ctx, tok); err != nil {
		t.Fatal(err)
	}
	if lastActive() != first {
		t.Fatal("last_active rewritten within a minute")
	}

	old := stamp(time.Now().Add(-2 * time.Minute))
	if _, err := s.DB.ExecContext(ctx, `UPDATE users SET last_active = ? WHERE id = 'u1'`, old); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UserBySession(ctx, tok); err != nil {
		t.Fatal(err)
	}
	if lastActive() == old {
		t.Fatal("last_active not updated after a minute")
	}
}

func TestAPIKeyLifecycle(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	id, key, err := s.CreateAPIKey(ctx, "u1", "phone")
	if err != nil {
		t.Fatal(err)
	}
	if len(key) < 4 || key[:3] != "cn_" {
		t.Fatalf("key %q lacks cn_ prefix", key)
	}
	u, err := s.UserByAPIKey(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if u.ID != "u1" {
		t.Fatalf("got user %s", u.ID)
	}

	keys, err := s.ListAPIKeys(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0].ID != id || keys[0].Name != "phone" || keys[0].LastUsed == nil {
		t.Fatalf("list = %+v", keys)
	}

	if ok, err := s.DeleteAPIKey(ctx, "u1", id); err != nil || !ok {
		t.Fatalf("delete own key: ok=%v err=%v", ok, err)
	}
	if _, err := s.UserByAPIKey(ctx, key); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("deleted key still works: %v", err)
	}
}

func TestAPIKeyOwnership(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	id, key, err := s.CreateAPIKey(ctx, "u1", "alice's")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := s.DeleteAPIKey(ctx, "u2", id); err != nil || ok {
		t.Fatalf("other user deleted the key: ok=%v err=%v", ok, err)
	}
	if _, err := s.UserByAPIKey(ctx, key); err != nil {
		t.Fatalf("key gone after failed delete: %v", err)
	}
	if keys, _ := s.ListAPIKeys(ctx, "u2"); len(keys) != 0 {
		t.Fatalf("u2 sees u1's keys: %+v", keys)
	}
}

func TestAPIKeyRejectsBadKeys(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	_, key, err := s.CreateAPIKey(ctx, "u1", "k")
	if err != nil {
		t.Fatal(err)
	}
	bad := map[string]string{
		"no prefix":          key[3:],
		"other prefix":       "sk_" + key[3:],
		"prefix only":        "cn_",
		"empty":              "",
		"unknown":            "cn_" + "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"altered bare token": key[3:] + "x",
	}
	for name, k := range bad {
		if _, err := s.UserByAPIKey(ctx, k); !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestListAPIKeysNewestFirst(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	var ids []string
	for _, name := range []string{"first", "second", "third"} {
		id, _, err := s.CreateAPIKey(ctx, "u1", name)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
		time.Sleep(2 * time.Millisecond)
	}
	keys, err := s.ListAPIKeys(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 3 || keys[0].ID != ids[2] || keys[2].ID != ids[0] {
		t.Fatalf("not newest first: %+v", keys)
	}
	if keys[0].LastUsed != nil {
		t.Fatal("LastUsed set for a key never used")
	}
}
