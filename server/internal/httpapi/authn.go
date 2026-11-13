package httpapi

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tabunugoku/mars/server/internal/auth"
	"github.com/tabunugoku/mars/server/internal/ids"
)

const (
	cookieName = "cn_session"
	// A session without "keep me signed in" lasts this long after its last
	// use (the cookie itself ends with the browser session).
	shortSession        = 12 * time.Hour
	defaultRememberDays = 30
	maxBody             = 1 << 20 // every JSON body is small
)

type ctxKey int

const (
	userKey ctxKey = iota
	tokenKey
	localeKey
)

func userFrom(ctx context.Context) (auth.User, bool) {
	u, ok := ctx.Value(userKey).(auth.User)
	return u, ok
}

// currentUser is for handlers behind authenticate, where a user is always set.
func currentUser(ctx context.Context) auth.User {
	u, _ := userFrom(ctx)
	return u
}

// publicRoutes need no sign-in. Everything else under /api/v1 does.
func isPublic(r *http.Request) bool {
	switch r.URL.Path {
	case "/healthz", "/public/login-screen":
		return r.Method == http.MethodGet
	case "/auth/login", "/setup":
		return r.Method == http.MethodPost
	}
	// The cover wall; servePublicCover serves only the covers on it.
	if rest, ok := strings.CutPrefix(r.URL.Path, "/public/covers/"); ok && rest != "" && !strings.Contains(rest, "/") {
		return r.Method == http.MethodGet
	}
	return false
}

func safeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

// sameOrigin is the CSRF check for cookie-authenticated writes: browsers
// send Origin on cross-site POSTs, so a missing or foreign Origin is refused
// (Sec-Fetch-Site covers browsers that omit Origin on same-origin requests).
func sameOrigin(r *http.Request) bool {
	if o := r.Header.Get("Origin"); o != "" {
		u, err := url.Parse(o)
		return err == nil && u.Host != "" && strings.EqualFold(u.Host, r.Host)
	}
	return r.Header.Get("Sec-Fetch-Site") == "same-origin"
}

// authenticate resolves the caller from the session cookie or an API key,
// refuses unauthenticated requests to non-public routes, and applies the
// CSRF check to cookie-based writes (including sign-in itself).
// bodyCap is the most a request may send. Image uploads and CBL imports
// stream their body to an administrator-only handler that enforces its own
// limit (20 MB and 5 MB); everything else is small JSON.
func bodyCap(r *http.Request) int64 {
	p := r.URL.Path
	switch {
	case r.Method == http.MethodPut && strings.HasSuffix(p, "/thumbnail") &&
		(strings.HasPrefix(p, "/books/") || strings.HasPrefix(p, "/series/")):
		return 20<<20 + 1
	case r.Method == http.MethodPost && p == "/read-lists/import":
		return 5<<20 + 1
	}
	return maxBody
}

func (s *server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// One cap for every request body, including those the generated
		// handlers decode (before requireAdmin runs).
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, bodyCap(r))
		}
		ctx := r.Context()
		var u auth.User
		var err error = auth.ErrUnauthorized
		viaCookie := false
		if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
			u, err = s.Auth.UserByAPIKey(ctx, strings.TrimPrefix(h, "Bearer "))
		} else if c, cerr := r.Cookie(cookieName); cerr == nil && c.Value != "" {
			viaCookie = true
			u, err = s.Auth.UserBySession(ctx, c.Value)
			if err == nil {
				ctx = context.WithValue(ctx, tokenKey, c.Value)
				s.refresh(ctx, w, r, c.Value)
			}
		}
		if err != nil && !errors.Is(err, auth.ErrUnauthorized) {
			slog.Error("authenticate", "err", err)
			writeError(r.Context(), w, http.StatusInternalServerError, "server.error")
			return
		}
		signedIn := err == nil
		if !safeMethod(r.Method) && (viaCookie || isPublic(r)) && !sameOrigin(r) {
			writeError(r.Context(), w, http.StatusForbidden, "forbidden.cross_site")
			return
		}
		if !signedIn && !isPublic(r) {
			writeError(r.Context(), w, http.StatusUnauthorized, "unauthorized.sign_in")
			return
		}
		if signedIn {
			ctx = context.WithValue(ctx, userKey, u)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// refresh slides a short session forward while it is in use. Remembered
// sessions keep their fixed end.
func (s *server) refresh(ctx context.Context, w http.ResponseWriter, r *http.Request, token string) {
	var remember bool
	if err := s.DB.QueryRowContext(ctx, `SELECT remember FROM sessions WHERE token_hash = ?`, auth.HashToken(token)).Scan(&remember); err != nil || remember {
		return
	}
	if _, err := s.Auth.RefreshSession(ctx, token, shortSession); err != nil {
		slog.Warn("session refresh", "err", err)
	}
}

func secureRequest(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (s *server) setSessionCookie(w http.ResponseWriter, r *http.Request, token string, maxAge time.Duration) {
	c := &http.Cookie{
		Name: cookieName, Value: token, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: secureRequest(r),
	}
	if maxAge > 0 {
		c.MaxAge = int(maxAge.Seconds())
	}
	http.SetCookie(w, c)
}

func (s *server) rememberDays(ctx context.Context) int {
	var raw string
	if err := s.DB.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'server'`).Scan(&raw); err != nil {
		return defaultRememberDays
	}
	var v struct {
		RememberMeDays int `json:"rememberMeDays"`
	}
	if json.Unmarshal([]byte(raw), &v) != nil || v.RememberMeDays < 1 || v.RememberMeDays > 3650 {
		return defaultRememberDays
	}
	return v.RememberMeDays
}

// startSession signs userID in on this response. passwordHash is the hash
// the caller verified: the session is created only if it is still current.
func (s *server) startSession(ctx context.Context, w http.ResponseWriter, r *http.Request, userID, passwordHash string, remember bool) error {
	ttl, maxAge := shortSession, time.Duration(0)
	if remember {
		ttl = time.Duration(s.rememberDays(ctx)) * 24 * time.Hour
		maxAge = ttl
	}
	token, _, err := s.Auth.CreateSessionIfPassword(ctx, userID, passwordHash, ttl, remember)
	if err != nil {
		return err
	}
	s.setSessionCookie(w, r, token, maxAge)
	return nil
}

func retryAfter(d time.Duration) string {
	return strconv.Itoa(int(math.Ceil(d.Seconds())))
}

// clientIP is the peer address, or, when the peer is a trusted reverse
// proxy, the nearest untrusted address in X-Forwarded-For. Without trusted
// proxies configured the header is ignored, since anyone can send it.
func (s *server) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if !s.trusted(host) {
		return host
	}
	hops := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		h := strings.TrimSpace(hops[i])
		if h == "" {
			continue
		}
		if !s.trusted(h) {
			return h
		}
		host = h
	}
	return host
}

func (s *server) trusted(addr string) bool {
	ip, err := netip.ParseAddr(addr)
	if err != nil {
		return false
	}
	ip = ip.Unmap()
	for _, p := range s.TrustedProxies {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(r.Body) // capped by authenticate
	if err := dec.Decode(v); err != nil {
		writeError(r.Context(), w, http.StatusBadRequest, "bad_request.malformed")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// ─── Sign-in ─────────────────────────────────────────────────────────────────

func (s *server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if !decode(w, r, &req) {
		return
	}
	ctx := r.Context()
	name := strings.TrimSpace(req.Username)
	// Throttled per client address only. A per-account key would let anyone
	// lock a known user out; guessing from many addresses is still slowed
	// by argon2 and the 8-character minimum.
	key := "login:" + s.clientIP(r)
	if ok, wait := s.Limiter.Begin(key, time.Now()); !ok {
		w.Header().Set("Retry-After", retryAfter(wait))
		writeError(r.Context(), w, http.StatusTooManyRequests, "too_many_requests.sign_in")
		return
	}
	failed := true
	defer func() { s.Limiter.End(key, failed, time.Now()) }()

	var id, hash string
	err := s.DB.QueryRowContext(ctx,
		`SELECT id, password_hash FROM users WHERE name = ? COLLATE NOCASE OR email = ? COLLATE NOCASE LIMIT 1`, name, name).
		Scan(&id, &hash)
	ok := false
	switch {
	case errors.Is(err, sql.ErrNoRows):
		err = auth.VerifyDummy(ctx, req.Password) // same work as a real check
	case err != nil:
	default:
		ok, err = auth.VerifyPassword(ctx, hash, req.Password)
	}
	if errors.Is(err, auth.ErrBusy) || errors.Is(err, context.Canceled) {
		failed = false // not a wrong guess
		w.Header().Set("Retry-After", "5")
		writeError(r.Context(), w, http.StatusServiceUnavailable, "unavailable.busy")
		return
	}
	if err != nil {
		failed = false
		slog.Error("login", "err", err)
		writeError(r.Context(), w, http.StatusInternalServerError, "server.error")
		return
	}
	if !ok {
		writeError(r.Context(), w, http.StatusUnauthorized, "unauthorized.bad_credentials")
		return
	}
	failed = false
	if err := s.startSession(ctx, w, r, id, hash, req.Remember != nil && *req.Remember); errors.Is(err, auth.ErrUnauthorized) {
		// The password changed while we checked it.
		writeError(r.Context(), w, http.StatusUnauthorized, "unauthorized.bad_credentials")
		return
	} else if err != nil {
		slog.Error("login session", "err", err)
		writeError(r.Context(), w, http.StatusInternalServerError, "server.error")
		return
	}
	u, err := s.userByID(ctx, id)
	if err != nil {
		writeError(r.Context(), w, http.StatusInternalServerError, "server.error")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (s *server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if token, ok := r.Context().Value(tokenKey).(string); ok {
		if err := s.Auth.DeleteSession(r.Context(), token); err != nil {
			slog.Warn("logout", "err", err)
		}
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: secureRequest(r)})
	w.WriteHeader(http.StatusNoContent)
}

// ─── First-run setup ─────────────────────────────────────────────────────────

// SetupGate holds the one-time token that guards first-run setup, so that
// whoever reaches a fresh server first on the network cannot claim it.
type SetupGate struct {
	mu    sync.Mutex
	token string // "" once an administrator exists
	path  string
}

// NewSetupGate makes a token when the database has no users, logs it and
// writes it to dataDir/setup-token (readable by the server's user only).
func NewSetupGate(ctx context.Context, db *sql.DB, dataDir string) (*SetupGate, error) {
	g := &SetupGate{path: filepath.Join(dataDir, "setup-token")}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&n); err != nil {
		return nil, err
	}
	if n > 0 {
		_ = os.Remove(g.path)
		return g, nil
	}
	token, _ := auth.NewToken()
	g.token = token
	if err := os.WriteFile(g.path, []byte(token+"\n"), 0o600); err != nil {
		return nil, err
	}
	slog.Warn("waiting for first-time setup: open the web app in a browser and enter this token", "token", token, "file", g.path)
	return g, nil
}

func (g *SetupGate) open() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.token != ""
}

func (s *server) handleSetup(w http.ResponseWriter, r *http.Request) {
	var req SetupRequest
	if !decode(w, r, &req) {
		return
	}
	g := s.Gate
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.token == "" {
		writeError(r.Context(), w, http.StatusForbidden, "forbidden.setup_done")
		return
	}
	// No throttle: the token has 256 random bits, and a shared lockout
	// would let anyone on the network keep the operator out.
	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(req.Token)), []byte(g.token)) != 1 {
		writeError(r.Context(), w, http.StatusForbidden, "forbidden.setup_token")
		return
	}
	ctx := r.Context()
	id, msg, err := s.insertUser(ctx, UserCreate{Name: req.Name, Email: req.Email, Password: req.Password, Role: RoleAdmin})
	if err != nil {
		slog.Error("setup", "err", err)
		writeError(r.Context(), w, http.StatusInternalServerError, "server.error")
		return
	}
	if msg != "" {
		writeError(r.Context(), w, http.StatusBadRequest, msg)
		return
	}
	g.token = ""
	_ = os.Remove(g.path)
	var hash string
	if err := s.DB.QueryRowContext(ctx, `SELECT password_hash FROM users WHERE id = ?`, id).Scan(&hash); err != nil {
		slog.Error("setup session", "err", err)
	} else if err := s.startSession(ctx, w, r, id, hash, false); err != nil {
		slog.Error("setup session", "err", err)
	}
	u, err := s.userByID(ctx, id)
	if err != nil {
		writeError(r.Context(), w, http.StatusInternalServerError, "server.error")
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

// newID is a seam for tests.
var newID = ids.New
