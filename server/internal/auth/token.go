package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// NewToken returns a new random token (32 bytes, base64url without padding)
// and its hash. Only the hash should be stored.
func NewToken() (token, hash string) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err) // crypto/rand never fails on supported platforms
	}
	token = base64.RawURLEncoding.EncodeToString(b[:])
	return token, HashToken(token)
}

// HashToken returns the hex SHA-256 of a token, the form stored in the database.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
