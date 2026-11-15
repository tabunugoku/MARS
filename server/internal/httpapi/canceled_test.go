package httpapi

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A request whose client went away (its context is cancelled) stops without
// an ERROR or WARN in the log: the user's log showed "authenticate ... context
// canceled" errors for requests the browser had dropped.
func TestCanceledRequestIsNotLogged(t *testing.T) {
	e := newEnv(t)
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest("GET", "/api/v1/libraries", nil).WithContext(ctx)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: e.admin.cookie})
	e.h.ServeHTTP(httptest.NewRecorder(), req)

	if out := buf.String(); strings.Contains(out, "level=ERROR") || strings.Contains(out, "level=WARN") {
		t.Fatalf("cancelled request logged:\n%s", out)
	}
}

func TestClientGone(t *testing.T) {
	live := context.Background()
	gone, cancel := context.WithCancel(live)
	cancel()
	if !clientGone(gone, context.Canceled) {
		t.Error("cancelled request: want clientGone")
	}
	if clientGone(live, context.Canceled) {
		t.Error("live request with a stray Canceled error: want a real error")
	}
	if clientGone(gone, http.ErrHandlerTimeout) {
		t.Error("other error: want a real error")
	}
}
