package auth

import (
	"encoding/base64"
	"encoding/hex"
	"testing"
)

func TestNewTokenUniqueAndShaped(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		tok, hash := NewToken()
		raw, err := base64.RawURLEncoding.DecodeString(tok)
		if err != nil || len(raw) != 32 {
			t.Fatalf("token %q: decode %v, len %d", tok, err, len(raw))
		}
		if len(tok) != 43 {
			t.Fatalf("token length %d, want 43", len(tok))
		}
		if seen[tok] {
			t.Fatalf("duplicate token %q", tok)
		}
		seen[tok] = true
		if hash != HashToken(tok) {
			t.Fatal("hash does not match HashToken")
		}
		if b, err := hex.DecodeString(hash); err != nil || len(b) != 32 {
			t.Fatalf("hash %q is not hex sha256", hash)
		}
	}
}

func TestHashTokenKnownValue(t *testing.T) {
	// sha256("abc")
	const want = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got := HashToken("abc"); got != want {
		t.Fatalf("HashToken(abc) = %s", got)
	}
}
