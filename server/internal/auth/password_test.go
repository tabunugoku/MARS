package auth

import (
	"strings"
	"testing"
)

func TestHashCheckRoundTrip(t *testing.T) {
	h, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("unexpected encoding %q", h)
	}
	if !CheckPassword(h, "correct horse battery") {
		t.Fatal("right password rejected")
	}
	if CheckPassword(h, "correct horse batter") {
		t.Fatal("wrong password accepted")
	}
	if CheckPassword(h, "") {
		t.Fatal("empty password accepted")
	}
}

func TestHashesDiffer(t *testing.T) {
	a, err := HashPassword("same password")
	if err != nil {
		t.Fatal(err)
	}
	b, err := HashPassword("same password")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("two hashes of the same password are equal (salt reused?)")
	}
}

func TestHashPasswordLength(t *testing.T) {
	tests := []struct {
		name string
		pw   string
		want error
	}{
		{"7 chars", strings.Repeat("a", 7), ErrWeakPassword},
		{"8 chars", strings.Repeat("a", 8), nil},
		{"256 chars", strings.Repeat("a", 256), nil},
		{"257 chars", strings.Repeat("a", 257), ErrWeakPassword},
		{"8 runes, 24 bytes", strings.Repeat("あ", 8), nil},
		{"7 runes, 21 bytes", strings.Repeat("あ", 7), ErrWeakPassword},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := HashPassword(tt.pw)
			if err != tt.want {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestCheckPasswordMalformed(t *testing.T) {
	valid, err := HashPassword("valid password")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(valid, "$")
	salt, key := parts[4], parts[5]

	tests := map[string]string{
		"empty":           "",
		"garbage":         "not a hash",
		"wrong algorithm": "$argon2i$v=19$m=19456,t=2,p=1$" + salt + "$" + key,
		"wrong version":   "$argon2id$v=18$m=19456,t=2,p=1$" + salt + "$" + key,
		"no params":       "$argon2id$v=19$" + salt + "$" + key,
		"extra field":     "$argon2id$v=19$m=19456,t=2,p=1$x$" + salt + "$" + key,
		"bad base64 salt": "$argon2id$v=19$m=19456,t=2,p=1$!!!$" + key,
		"bad base64 key":  "$argon2id$v=19$m=19456,t=2,p=1$" + salt + "$@@@",
		"absurd memory":   "$argon2id$v=19$m=4294967295,t=2,p=1$" + salt + "$" + key,
		"memory overflow": "$argon2id$v=19$m=4294967296,t=2,p=1$" + salt + "$" + key,
		"zero time":       "$argon2id$v=19$m=19456,t=0,p=1$" + salt + "$" + key,
		"huge time":       "$argon2id$v=19$m=19456,t=11,p=1$" + salt + "$" + key,
		"zero threads":    "$argon2id$v=19$m=19456,t=2,p=0$" + salt + "$" + key,
		"too many thread": "$argon2id$v=19$m=19456,t=2,p=17$" + salt + "$" + key,
		"short key":       "$argon2id$v=19$m=19456,t=2,p=1$" + salt + "$" + "AAAAAAAAAAAAAAAA",
		"signed param":    "$argon2id$v=19$m=+19456,t=2,p=1$" + salt + "$" + key,
		"empty key":       "$argon2id$v=19$m=19456,t=2,p=1$" + salt + "$",
	}
	for name, enc := range tests {
		t.Run(name, func(t *testing.T) {
			if CheckPassword(enc, "valid password") {
				t.Fatalf("malformed hash %q accepted", enc)
			}
		})
	}
}

func TestCheckDummy(t *testing.T) {
	CheckDummy("anything at all")
	CheckDummy("")
}
