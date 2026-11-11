package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// Argon2id cost parameters for new hashes. Memory is in KiB.
const (
	argonMemory  = 19456
	argonTime    = 2
	argonThreads = 1
	argonSaltLen = 16
	argonKeyLen  = 32
)

// Bounds applied to parameters read back from a stored hash. Anything
// outside them is rejected before argon2 runs, so a crafted hash cannot
// force a huge allocation or a long computation.
const (
	argonMinMemory  = 8
	argonMaxMemory  = 1048576
	argonMaxTime    = 10
	argonMaxThreads = 16
	argonMinSaltLen = 8
	argonMaxSaltLen = 64
	argonMinKeyLen  = 16
	argonMaxKeyLen  = 64
)

const (
	minPasswordRunes = 8
	maxPasswordRunes = 256
)

// hashSlots bounds how many argon2 computations run at once. Each one uses
// about 19 MiB, so the cap bounds peak memory under concurrent logins.
var hashSlots = make(chan struct{}, 2)

// hashQueue bounds how many computations may wait for a slot. Past it,
// callers get ErrBusy at once instead of piling up behind a flood.
var hashQueue = make(chan struct{}, 32)

// derive runs argon2id while holding one of the hashSlots. It gives up when
// ctx ends while waiting, and at once when the wait queue is full.
func derive(ctx context.Context, pw string, salt []byte, t, m uint32, p uint8, keyLen uint32) ([]byte, error) {
	select {
	case hashQueue <- struct{}{}:
	default:
		return nil, ErrBusy
	}
	select {
	case hashSlots <- struct{}{}:
		<-hashQueue
	case <-ctx.Done():
		<-hashQueue
		return nil, ctx.Err()
	}
	defer func() { <-hashSlots }()
	return argon2.IDKey([]byte(pw), salt, t, m, p, keyLen), nil
}

// HashPassword returns a PHC-encoded argon2id hash of pw. The password must
// be 8 to 256 characters (runes).
func HashPassword(pw string) (string, error) {
	n := utf8.RuneCountInString(pw)
	if n < minPasswordRunes || n > maxPasswordRunes {
		return "", ErrWeakPassword
	}
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: salt: %w", err)
	}
	key, err := derive(context.Background(), pw, salt, argonTime, argonMemory, uint8(argonThreads), argonKeyLen)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

type argonParams struct {
	m, t, p uint32
}

// parseHash splits a PHC-encoded argon2id string. It returns false for any
// string that is malformed, uses another algorithm or version, or has
// parameters outside the bounds above.
func parseHash(encoded string) (argonParams, []byte, []byte, bool) {
	var zero argonParams
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" ||
		parts[2] != fmt.Sprintf("v=%d", argon2.Version) {
		return zero, nil, nil, false
	}
	fields := strings.Split(parts[3], ",")
	if len(fields) != 3 {
		return zero, nil, nil, false
	}
	m, ok1 := parseParam(fields[0], "m")
	t, ok2 := parseParam(fields[1], "t")
	p, ok3 := parseParam(fields[2], "p")
	if !ok1 || !ok2 || !ok3 {
		return zero, nil, nil, false
	}
	if m < argonMinMemory || m > argonMaxMemory ||
		t < 1 || t > argonMaxTime ||
		p < 1 || p > argonMaxThreads {
		return zero, nil, nil, false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < argonMinSaltLen || len(salt) > argonMaxSaltLen {
		return zero, nil, nil, false
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(key) < argonMinKeyLen || len(key) > argonMaxKeyLen {
		return zero, nil, nil, false
	}
	return argonParams{m: m, t: t, p: p}, salt, key, true
}

// parseParam reads a "name=value" field in canonical decimal form.
func parseParam(field, name string) (uint32, bool) {
	rest, found := strings.CutPrefix(field, name+"=")
	if !found {
		return 0, false
	}
	v, err := strconv.ParseUint(rest, 10, 32)
	if err != nil || strconv.FormatUint(v, 10) != rest {
		return 0, false
	}
	return uint32(v), true
}

// CheckPassword reports whether pw matches the encoded hash. Malformed or
// out-of-bounds hashes return false.
func CheckPassword(encoded, pw string) bool {
	ok, _ := VerifyPassword(context.Background(), encoded, pw)
	return ok
}

// VerifyPassword is CheckPassword that returns an error when ctx ends while
// waiting for a slot, or when too many checks are waiting (ErrBusy).
func VerifyPassword(ctx context.Context, encoded, pw string) (bool, error) {
	p, salt, want, ok := parseHash(encoded)
	if !ok {
		return false, nil
	}
	got, err := derive(ctx, pw, salt, p.t, p.m, uint8(p.p), uint32(len(want)))
	if err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

var (
	dummyOnce sync.Once
	dummyHash string
)

// CheckDummy spends the same time as CheckPassword without matching anything.
// Call it when the user does not exist so that unknown and known accounts
// take similar time to answer.
func CheckDummy(pw string) { _ = VerifyDummy(context.Background(), pw) }

// VerifyDummy is CheckDummy with VerifyPassword's waiting rules.
func VerifyDummy(ctx context.Context, pw string) error {
	dummyOnce.Do(func() {
		var b [18]byte
		if _, err := rand.Read(b[:]); err != nil {
			panic(err) // crypto/rand never fails on supported platforms
		}
		h, err := HashPassword(base64.RawStdEncoding.EncodeToString(b[:]))
		if err != nil {
			panic(err)
		}
		dummyHash = h
	})
	_, err := VerifyPassword(ctx, dummyHash, pw)
	return err
}
