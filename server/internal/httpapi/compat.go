package httpapi

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/tabunugoku/cominavi/server/internal/auth"
)

// The compat handler serves what other apps speak, outside /api/v1:
//
//   - /opds/   OPDS 1.2 (with OPDS-PSE page streaming) for reader apps
//     (opds.go, behind ServerSettings.opdsEnabled)
//   - /komga/  a Komga-shaped API for the Mihon/Tachiyomi Komga extension and
//     Mihon's Komga tracker (komga*.go, behind komgaApiEnabled)
//
// Both are read through the same query builders as the main API, so a user
// sees exactly the libraries they may see there. A switched-off one answers
// 404 to everyone.
//
// Authentication is what those apps send: HTTP Basic (user name or email and
// password, or any name with an API key as the password), an API key in
// X-API-Key or "Authorization: Bearer", or the compat cookie. A request
// without valid credentials gets 401 with a Basic challenge: OkHttp (Mihon)
// sends Basic only after one. The first Basic sign-in starts a long session
// and sets it as the compat cookie, which Mihon's tracker relies on (it sends
// no credentials of its own) and which spares clients that keep cookies a
// password check per request. The compat cookie is not the web app's
// session cookie, so it cannot drive /api/v1.

const (
	compatCookie  = "cn_compat"
	compatSession = 30 * 24 * time.Hour
	// compatBody caps request bodies (only small JSON progress updates).
	compatBody = 64 << 10
	// basicCacheTTL is how long a checked Basic credential maps to its
	// session, so clients without a cookie jar are not re-hashed (argon2)
	// on every page request.
	basicCacheTTL = 10 * time.Minute
	basicCacheMax = 1024
)

// NewCompat returns the handler for /opds/ and /komga/. Mount it at those two
// prefixes without stripping them: routes are registered with full paths.
func NewCompat(d Deps) http.Handler {
	s := &server{d}
	opds := http.NewServeMux()
	s.opdsRoutes(opds)
	komga := http.NewServeMux()
	s.komgaRoutes(komga)
	cache := &basicCache{m: map[[32]byte]basicEntry{}}
	mux := http.NewServeMux()
	mux.Handle("/opds/", s.compatGate(func(v ServerSettings) bool { return v.OpdsEnabled }, cache, opds))
	mux.Handle("/komga/", s.compatGate(func(v ServerSettings) bool { return v.KomgaApiEnabled }, cache, komga))
	return withLocale(mux)
}

// compatGate answers 404 while the feature is off, then authenticates.
func (s *server) compatGate(on func(ServerSettings) bool, cache *basicCache, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v, err := loadSettings(r.Context(), s.DB)
		if err != nil {
			slog.Error("compat settings", "err", err)
			writeError(r.Context(), w, http.StatusInternalServerError, "server.error")
			return
		}
		if !on(v) {
			writeError(r.Context(), w, http.StatusNotFound, "not_found.route")
			return
		}
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, compatBody)
		}
		u, ok := s.compatUser(w, r, cache)
		if !ok {
			return // compatUser answered
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	})
}

// compatUser finds the caller, or answers (401, 429, 503, 500) and returns
// false.
func (s *server) compatUser(w http.ResponseWriter, r *http.Request, cache *basicCache) (auth.User, bool) {
	ctx := r.Context()
	try := func(u auth.User, err error) (auth.User, bool, bool) {
		if err == nil {
			return u, true, false
		}
		if !errors.Is(err, auth.ErrUnauthorized) {
			slog.Error("compat auth", "err", err)
			writeError(ctx, w, http.StatusInternalServerError, "server.error")
			return u, false, true
		}
		return u, false, false
	}
	if c, err := r.Cookie(compatCookie); err == nil && c.Value != "" {
		if u, ok, done := try(s.Auth.UserBySession(ctx, c.Value)); ok || done {
			return u, ok
		}
	}
	if k := r.Header.Get("X-API-Key"); k != "" {
		if u, ok, done := try(s.Auth.UserByAPIKey(ctx, k)); ok || done {
			return u, ok
		}
	}
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "Bearer ") {
		k := h[7:]
		if u, ok, done := try(s.Auth.UserByAPIKey(ctx, k)); ok || done {
			return u, ok
		}
	}
	if name, pass, found := r.BasicAuth(); found {
		// An API key as the password; anything else (including a real
		// password that happens to look like a key) is checked as a password.
		if strings.HasPrefix(pass, "cn_") {
			if u, ok, done := try(s.Auth.UserByAPIKey(ctx, pass)); ok || done {
				return u, ok
			}
		}
		if u, ok, done := s.compatBasic(w, r, cache, strings.TrimSpace(name), pass); ok || done {
			return u, ok
		}
	}
	w.Header().Set("WWW-Authenticate", `Basic realm="ComiNavi", charset="UTF-8"`)
	writeError(ctx, w, http.StatusUnauthorized, "unauthorized.sign_in")
	return auth.User{}, false
}

// compatBasic checks a user name (or email) and password like the sign-in
// form does, throttled per client address, and remembers the result as a
// session. ok: signed in. done: it has already answered (429, 503, 500).
// Neither: wrong credentials (the caller sends the 401 challenge).
func (s *server) compatBasic(w http.ResponseWriter, r *http.Request, cache *basicCache, name, pass string) (u auth.User, ok, done bool) {
	ctx := r.Context()
	answered := func(status int, code string) (auth.User, bool, bool) {
		writeError(ctx, w, status, code)
		return auth.User{}, false, true
	}
	key := sha256.Sum256([]byte(name + "\x00" + pass))
	if token, ok := cache.get(key, time.Now()); ok {
		u, err := s.Auth.UserBySession(ctx, token)
		if err == nil {
			s.setCompatCookie(w, r, token)
			return u, true, false
		}
		cache.drop(key) // signed out, password changed, or expired
		if !errors.Is(err, auth.ErrUnauthorized) {
			slog.Error("compat session", "err", err)
			return answered(http.StatusInternalServerError, "server.error")
		}
	}

	limit := "login:" + s.clientIP(r)
	if ok, wait := s.Limiter.Begin(limit, time.Now()); !ok {
		w.Header().Set("Retry-After", retryAfter(wait))
		return answered(http.StatusTooManyRequests, "too_many_requests.sign_in")
	}
	failed := true
	defer func() { s.Limiter.End(limit, failed, time.Now()) }()

	var id, hash string
	err := s.DB.QueryRowContext(ctx,
		`SELECT id, password_hash FROM users WHERE name = ? COLLATE NOCASE OR email = ? COLLATE NOCASE LIMIT 1`, name, name).
		Scan(&id, &hash)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		err = auth.VerifyDummy(ctx, pass)
	case err != nil:
	default:
		ok, err = auth.VerifyPassword(ctx, hash, pass)
	}
	if errors.Is(err, auth.ErrBusy) || errors.Is(err, context.Canceled) {
		failed = false
		w.Header().Set("Retry-After", "5")
		return answered(http.StatusServiceUnavailable, "unavailable.busy")
	}
	if err != nil {
		failed = false
		slog.Error("compat login", "err", err)
		return answered(http.StatusInternalServerError, "server.error")
	}
	if !ok {
		return auth.User{}, false, false // 401 with a challenge
	}
	failed = false
	token, _, err := s.Auth.CreateSessionIfPassword(ctx, id, hash, compatSession, true)
	if errors.Is(err, auth.ErrUnauthorized) {
		return auth.User{}, false, false // the password changed while we checked it
	}
	if err != nil {
		slog.Error("compat session", "err", err)
		return answered(http.StatusInternalServerError, "server.error")
	}
	u, err = s.Auth.UserBySession(ctx, token)
	if err != nil {
		slog.Error("compat session", "err", err)
		return answered(http.StatusInternalServerError, "server.error")
	}
	cache.put(key, token, time.Now())
	s.setCompatCookie(w, r, token)
	return u, true, false
}

// setCompatCookie hands the session to clients that keep cookies, unless the
// request already carries it.
func (s *server) setCompatCookie(w http.ResponseWriter, r *http.Request, token string) {
	if c, err := r.Cookie(compatCookie); err == nil && c.Value == token {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: compatCookie, Value: token, Path: "/", MaxAge: int(compatSession / time.Second),
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: secureRequest(r),
	})
}

// basicCache maps a checked Basic credential (hashed) to the session it
// started. A hit is still checked against the session store, so signing
// out everywhere or changing the password ends it.
type basicCache struct {
	mu sync.Mutex
	m  map[[32]byte]basicEntry
}

type basicEntry struct {
	token string
	until time.Time
}

func (c *basicCache) get(k [32]byte, now time.Time) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[k]
	if !ok || now.After(e.until) {
		delete(c.m, k)
		return "", false
	}
	return e.token, true
}

func (c *basicCache) put(k [32]byte, token string, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.m) >= basicCacheMax {
		for key, e := range c.m {
			if now.After(e.until) {
				delete(c.m, key)
			}
		}
		if len(c.m) >= basicCacheMax {
			clear(c.m)
		}
	}
	c.m[k] = basicEntry{token: token, until: now.Add(basicCacheTTL)}
}

func (c *basicCache) drop(k [32]byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, k)
}
