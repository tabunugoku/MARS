package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/tabunugoku/cominavi/server/internal/auth"
	"github.com/tabunugoku/cominavi/server/internal/events"
	"github.com/tabunugoku/cominavi/server/internal/task"
)

// defaultHeartbeat keeps proxies and browsers from timing an idle stream
// out; most cut idle connections after 30-60 seconds.
const defaultHeartbeat = 25 * time.Second

// writeWait bounds one write to a stream; a client that stops reading is
// dropped after it.
const writeWait = 30 * time.Second

// StreamEvents is served by serveEvents (see New); the strict handler exists
// only to satisfy the generated interface.
func (s *server) StreamEvents(context.Context, StreamEventsRequestObject) (StreamEventsResponseObject, error) {
	return nil, errors.New("not routed")
}

// serveEvents is GET /events: a text/event-stream of the changes the caller
// may see. The caller is signed in (authenticate). Each heartbeat checks the
// credential again, so a logout, a deleted user or a narrowed library list
// takes effect on an open stream within one heartbeat.
func (s *server) serveEvents(w http.ResponseWriter, r *http.Request) {
	if s.Events == nil {
		writeError(w, http.StatusNotFound, "イベントは使えません")
		return
	}
	ctx := r.Context()
	u := currentUser(ctx)
	rc := http.NewResponseController(w)
	// The server's ReadTimeout would otherwise end the stream: a GET has no
	// body, but the connection's read deadline still fires.
	_ = rc.SetReadDeadline(time.Time{})

	sub, err := s.Events.Subscribe(u.ID)
	if err != nil {
		w.Header().Set("Retry-After", "30")
		writeError(w, http.StatusTooManyRequests, "開いている更新の接続が多すぎます")
		return
	}
	defer s.Events.Unsubscribe(sub)

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no") // nginx and friends: do not buffer
	w.WriteHeader(http.StatusOK)

	write := func(msg string) bool {
		_ = rc.SetWriteDeadline(time.Now().Add(writeWait))
		if _, err := fmt.Fprint(w, msg); err != nil {
			return false
		}
		if err := rc.Flush(); err != nil && !errors.Is(err, http.ErrNotSupported) {
			return false
		}
		return true
	}
	if !write("retry: 5000\n: connected\n\n") {
		return
	}

	every := s.Heartbeat
	if every <= 0 {
		every = defaultHeartbeat
	}
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-sub.C:
			if !ok {
				return // fell behind, or the server is shutting down
			}
			if !eventFor(u, ev) {
				continue
			}
			data := ev.Data
			if t, ok := data.(task.Task); ok {
				data = toTask(t)
			}
			b, err := json.Marshal(data)
			if err != nil {
				slog.Error("event", "type", ev.Type, "err", err)
				continue
			}
			if !write("event: " + ev.Type + "\ndata: " + string(b) + "\n\n") {
				return
			}
		case <-tick.C:
			nu, err := s.reauth(r)
			if errors.Is(err, auth.ErrUnauthorized) {
				return
			}
			if err == nil {
				u = nu
			} // another error (a busy database) keeps the user as it was
			if !write(": ping\n\n") {
				return
			}
		}
	}
}

// eventFor reports whether u may receive ev. Every audience field that is
// set must allow it. Library access is the same rule as libScope:
// administrators and all-library users see every library, others only the
// ones listed for them.
func eventFor(u auth.User, ev events.Event) bool {
	if u.ID == "" {
		return false
	}
	if ev.AdminOnly && !u.IsAdmin() {
		return false
	}
	if ev.UserID != "" && ev.UserID != u.ID {
		return false
	}
	if ev.LibraryID != "" && !u.IsAdmin() && !u.CanSee(ev.LibraryID) {
		return false
	}
	return true
}

// reauth looks the stream's credential up again (without sliding the
// session forward).
func (s *server) reauth(r *http.Request) (auth.User, error) {
	ctx := r.Context()
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return s.Auth.UserByAPIKey(ctx, strings.TrimPrefix(h, "Bearer "))
	}
	if token, ok := ctx.Value(tokenKey).(string); ok {
		return s.Auth.UserBySession(ctx, token)
	}
	return auth.User{}, auth.ErrUnauthorized
}

// progressChanged tells the caller's other tabs and devices that their
// reading progress changed.
func (s *server) progressChanged(ctx context.Context, p events.Progress) {
	s.Events.Publish(events.Event{Type: "progress", UserID: currentUser(ctx).ID, LibraryID: p.LibraryID, Data: p})
}
