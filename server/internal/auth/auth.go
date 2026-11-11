// Package auth holds the credentials side of sign-in: password hashes
// (argon2id), session tokens, API keys, and the user record a request runs
// as. Authorization decisions (who may see what) live with the callers.
package auth

import "errors"

// User is the signed-in account a request acts as.
type User struct {
	ID           string
	Name         string
	Email        string
	Role         string // admin | user
	CanDownload  bool
	CanStream    bool
	AllLibraries bool
	LibraryIDs   []string // libraries the user may see when AllLibraries is false
}

func (u User) IsAdmin() bool { return u.Role == "admin" }

// CanSee reports whether the user may see a library.
func (u User) CanSee(libraryID string) bool {
	if u.AllLibraries {
		return true
	}
	for _, id := range u.LibraryIDs {
		if id == libraryID {
			return true
		}
	}
	return false
}

var (
	ErrUnauthorized = errors.New("auth: not signed in")
	ErrWeakPassword = errors.New("auth: password must be 8 to 256 characters")
	ErrBusy         = errors.New("auth: too many password checks waiting")
)
