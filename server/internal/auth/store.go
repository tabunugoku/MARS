package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/tabunugoku/mars/server/internal/ids"
)

// Store keeps sessions and API keys in the database.
//
// Timestamps are written in a fixed-width UTC form (nine fractional digits,
// always "Z"), which is valid RFC 3339 and sorts correctly as text. SQL
// comparisons on expires_at and last_used depend on that.
type Store struct {
	DB *sql.DB
}

const (
	timeLayout   = "2006-01-02T15:04:05.000000000Z07:00"
	touchEvery   = time.Minute
	apiKeyPrefix = "cn_"
)

func stamp(t time.Time) string { return t.UTC().Format(timeLayout) }

func parseStamp(s string) (time.Time, error) { return time.Parse(time.RFC3339Nano, s) }

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// APIKey describes a stored API key. The key itself is never stored, so it
// cannot be listed again.
type APIKey struct {
	ID        string
	Name      string
	CreatedAt time.Time
	LastUsed  *time.Time
}

// loadUser reads a user and their library grants. A missing user is ErrUnauthorized.
func (s Store) loadUser(ctx context.Context, id string) (User, error) {
	u := User{ID: id}
	var canDownload, canStream, allLibraries int
	err := s.DB.QueryRowContext(ctx,
		`SELECT name, email, role, can_download, can_stream, all_libraries FROM users WHERE id = ?`, id).
		Scan(&u.Name, &u.Email, &u.Role, &canDownload, &canStream, &allLibraries)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrUnauthorized
	}
	if err != nil {
		return User{}, fmt.Errorf("auth: load user: %w", err)
	}
	u.CanDownload = canDownload != 0
	u.CanStream = canStream != 0
	u.AllLibraries = allLibraries != 0

	rows, err := s.DB.QueryContext(ctx,
		`SELECT library_id FROM user_libraries WHERE user_id = ? ORDER BY library_id`, id)
	if err != nil {
		return User{}, fmt.Errorf("auth: load libraries: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var libID string
		if err := rows.Scan(&libID); err != nil {
			return User{}, fmt.Errorf("auth: load libraries: %w", err)
		}
		u.LibraryIDs = append(u.LibraryIDs, libID)
	}
	if err := rows.Err(); err != nil {
		return User{}, fmt.Errorf("auth: load libraries: %w", err)
	}
	return u, nil
}

// touchUser sets last_active to now, but only if it is unset or at least
// touchEvery old, so most requests do not write. The age is checked with a
// read first: an UPDATE takes the database's write lock even when it
// matches no row, so every signed-in request would otherwise queue behind
// a scan's write transactions.
func (s Store) touchUser(ctx context.Context, id string, now time.Time) error {
	cutoff := stamp(now.Add(-touchEvery))
	var last sql.NullString
	if err := s.DB.QueryRowContext(ctx, `SELECT last_active FROM users WHERE id = ?`, id).Scan(&last); err != nil {
		return fmt.Errorf("auth: touch user: %w", err)
	}
	if last.Valid && last.String > cutoff {
		return nil
	}
	_, err := s.DB.ExecContext(ctx,
		`UPDATE users SET last_active = ? WHERE id = ? AND (last_active IS NULL OR last_active <= ?)`,
		stamp(now), id, cutoff)
	if err != nil {
		return fmt.Errorf("auth: touch user: %w", err)
	}
	return nil
}

// lookupSession returns the owner and expiry of a session row. A missing row
// is ErrUnauthorized. It does not check expiry.
func (s Store) lookupSession(ctx context.Context, hash string) (string, time.Time, error) {
	var userID, expStr string
	err := s.DB.QueryRowContext(ctx,
		`SELECT user_id, expires_at FROM sessions WHERE token_hash = ?`, hash).
		Scan(&userID, &expStr)
	if errors.Is(err, sql.ErrNoRows) {
		return "", time.Time{}, ErrUnauthorized
	}
	if err != nil {
		return "", time.Time{}, fmt.Errorf("auth: session: %w", err)
	}
	exp, err := parseStamp(expStr)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("auth: session expiry: %w", err)
	}
	return userID, exp, nil
}

// CreateSession starts a session for userID that lasts ttl. It returns the
// token for the client; only its hash is stored.
func (s Store) CreateSession(ctx context.Context, userID string, ttl time.Duration, remember bool) (string, time.Time, error) {
	token, hash := NewToken()
	now := time.Now()
	exp := now.Add(ttl)
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO sessions (token_hash, user_id, expires_at, created_at, remember) VALUES (?, ?, ?, ?, ?)`,
		hash, userID, stamp(exp), stamp(now), boolInt(remember))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("auth: create session: %w", err)
	}
	return token, exp, nil
}

// CreateSessionIfPassword is CreateSession that succeeds only while the
// user's password hash is still passwordHash, checked in the same statement.
// A sign-in that verified the old password therefore cannot outlive a
// password change committed meanwhile. Otherwise it returns ErrUnauthorized.
func (s Store) CreateSessionIfPassword(ctx context.Context, userID, passwordHash string, ttl time.Duration, remember bool) (string, time.Time, error) {
	token, hash := NewToken()
	now := time.Now()
	exp := now.Add(ttl)
	res, err := s.DB.ExecContext(ctx,
		`INSERT INTO sessions (token_hash, user_id, expires_at, created_at, remember)
		 SELECT ?, ?, ?, ?, ? WHERE EXISTS (SELECT 1 FROM users WHERE id = ? AND password_hash = ?)`,
		hash, userID, stamp(exp), stamp(now), boolInt(remember), userID, passwordHash)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("auth: create session: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return "", time.Time{}, fmt.Errorf("auth: create session: %w", err)
	} else if n == 0 {
		return "", time.Time{}, ErrUnauthorized
	}
	return token, exp, nil
}

// UserBySession returns the user for a session token. Empty, unknown and
// expired tokens return ErrUnauthorized; an expired row is deleted.
func (s Store) UserBySession(ctx context.Context, token string) (User, error) {
	if token == "" {
		return User{}, ErrUnauthorized
	}
	hash := HashToken(token)
	userID, exp, err := s.lookupSession(ctx, hash)
	if err != nil {
		return User{}, err
	}
	now := time.Now()
	if !now.Before(exp) {
		if _, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, hash); err != nil {
			return User{}, fmt.Errorf("auth: delete expired session: %w", err)
		}
		return User{}, ErrUnauthorized
	}
	u, err := s.loadUser(ctx, userID)
	if err != nil {
		return User{}, err
	}
	// last_active is informational: a failed write must not fail the request.
	if err := s.touchUser(ctx, userID, now); err != nil {
		slog.Warn("touch user", "err", err)
	}
	return u, nil
}

// RefreshSession slides the expiry of a session. The expiry is reset to
// now+ttl only when less than half of ttl remains. It returns the expiry in effect.
func (s Store) RefreshSession(ctx context.Context, token string, ttl time.Duration) (time.Time, error) {
	if token == "" {
		return time.Time{}, ErrUnauthorized
	}
	hash := HashToken(token)
	_, exp, err := s.lookupSession(ctx, hash)
	if err != nil {
		return time.Time{}, err
	}
	now := time.Now()
	if !now.Before(exp) {
		if _, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, hash); err != nil {
			return time.Time{}, fmt.Errorf("auth: delete expired session: %w", err)
		}
		return time.Time{}, ErrUnauthorized
	}
	if exp.Sub(now) >= ttl/2 {
		return exp, nil
	}
	next := now.Add(ttl)
	if _, err := s.DB.ExecContext(ctx,
		`UPDATE sessions SET expires_at = ? WHERE token_hash = ?`, stamp(next), hash); err != nil {
		return time.Time{}, fmt.Errorf("auth: refresh session: %w", err)
	}
	return next, nil
}

// DeleteSession ends one session. Deleting an unknown token is not an error.
func (s Store) DeleteSession(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, HashToken(token)); err != nil {
		return fmt.Errorf("auth: delete session: %w", err)
	}
	return nil
}

// DeleteUserSessions ends all of userID's sessions except the one for
// exceptToken. An empty exceptToken ends all of them.
func (s Store) DeleteUserSessions(ctx context.Context, userID string, exceptToken string) error {
	var err error
	if exceptToken == "" {
		_, err = s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	} else {
		_, err = s.DB.ExecContext(ctx,
			`DELETE FROM sessions WHERE user_id = ? AND token_hash <> ?`, userID, HashToken(exceptToken))
	}
	if err != nil {
		return fmt.Errorf("auth: delete user sessions: %w", err)
	}
	return nil
}

// CreateAPIKey makes a new API key for userID. It returns the key id and the
// full key ("cn_" + token), which is shown once and never stored.
func (s Store) CreateAPIKey(ctx context.Context, userID, name string) (string, string, error) {
	token, hash := NewToken()
	id := ids.New()
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO api_keys (key_hash, user_id, name, created_at, id) VALUES (?, ?, ?, ?, ?)`,
		hash, userID, name, stamp(time.Now()), id)
	if err != nil {
		return "", "", fmt.Errorf("auth: create api key: %w", err)
	}
	return id, apiKeyPrefix + token, nil
}

// UserByAPIKey returns the user that owns an API key. Keys without the "cn_"
// prefix, and unknown keys, return ErrUnauthorized.
func (s Store) UserByAPIKey(ctx context.Context, key string) (User, error) {
	if !strings.HasPrefix(key, apiKeyPrefix) || len(key) == len(apiKeyPrefix) {
		return User{}, ErrUnauthorized
	}
	hash := HashToken(strings.TrimPrefix(key, apiKeyPrefix))
	var userID string
	var lastUsed sql.NullString
	err := s.DB.QueryRowContext(ctx, `SELECT user_id, last_used FROM api_keys WHERE key_hash = ?`, hash).Scan(&userID, &lastUsed)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrUnauthorized
	}
	if err != nil {
		return User{}, fmt.Errorf("auth: api key: %w", err)
	}
	now := time.Now()
	// Written only when stale, and never fatal (see touchUser).
	if cutoff := stamp(now.Add(-touchEvery)); !lastUsed.Valid || lastUsed.String <= cutoff {
		if _, err := s.DB.ExecContext(ctx,
			`UPDATE api_keys SET last_used = ? WHERE key_hash = ? AND (last_used IS NULL OR last_used <= ?)`,
			stamp(now), hash, cutoff); err != nil {
			slog.Warn("touch api key", "err", err)
		}
	}
	return s.loadUser(ctx, userID)
}

// ListAPIKeys returns userID's API keys, newest first.
func (s Store) ListAPIKeys(ctx context.Context, userID string) ([]APIKey, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT id, name, created_at, last_used FROM api_keys WHERE user_id = ? ORDER BY created_at DESC, id DESC`,
		userID)
	if err != nil {
		return nil, fmt.Errorf("auth: list api keys: %w", err)
	}
	defer rows.Close()
	keys := []APIKey{}
	for rows.Next() {
		var k APIKey
		var created string
		var lastUsed sql.NullString
		if err := rows.Scan(&k.ID, &k.Name, &created, &lastUsed); err != nil {
			return nil, fmt.Errorf("auth: list api keys: %w", err)
		}
		if k.CreatedAt, err = parseStamp(created); err != nil {
			return nil, fmt.Errorf("auth: api key time: %w", err)
		}
		if lastUsed.Valid {
			t, err := parseStamp(lastUsed.String)
			if err != nil {
				return nil, fmt.Errorf("auth: api key time: %w", err)
			}
			k.LastUsed = &t
		}
		keys = append(keys, k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("auth: list api keys: %w", err)
	}
	return keys, nil
}

// DeleteAPIKey deletes key id only if it belongs to userID. The bool reports
// whether a key was deleted.
func (s Store) DeleteAPIKey(ctx context.Context, userID, id string) (bool, error) {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM api_keys WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return false, fmt.Errorf("auth: delete api key: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("auth: delete api key: %w", err)
	}
	return n > 0, nil
}

// PurgeExpired deletes sessions that have expired and returns how many it removed.
func (s Store) PurgeExpired(ctx context.Context) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, stamp(time.Now()))
	if err != nil {
		return 0, fmt.Errorf("auth: purge sessions: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("auth: purge sessions: %w", err)
	}
	return n, nil
}
