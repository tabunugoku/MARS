// Command cominavi runs the ComiNavi server: the REST API under /api/v1 and
// the embedded web UI everywhere else.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/tabunugoku/cominavi/server/internal/auth"
	"github.com/tabunugoku/cominavi/server/internal/config"
	"github.com/tabunugoku/cominavi/server/internal/db"
	"github.com/tabunugoku/cominavi/server/internal/events"
	"github.com/tabunugoku/cominavi/server/internal/httpapi"
	"github.com/tabunugoku/cominavi/server/internal/library"
	"github.com/tabunugoku/cominavi/server/internal/media"
	"github.com/tabunugoku/cominavi/server/internal/task"
	"github.com/tabunugoku/cominavi/server/internal/webui"
)

// version is set at build time: -ldflags "-X main.version=v0.1.0".
var version = "dev"

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(log)
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		cfg, err := config.Load()
		if err == nil {
			err = healthcheck(context.Background(), cfg.Addr)
		}
		if err != nil {
			log.Error("unhealthy", "err", err)
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return err
	}
	database, err := db.Open(ctx, cfg.DBPath())
	if err != nil {
		return err
	}
	defer database.Close()

	hub := events.New()
	queue := task.New(database, runtime.NumCPU()/2+1)
	queue.Events = hub
	if err := queue.Run(ctx); err != nil {
		return err
	}
	lib := &library.Service{
		DB:           database,
		Queue:        queue,
		ThumbDir:     cfg.ThumbnailDir(),
		Pages:        &media.PageCache{Dir: cfg.PageCacheDir(), MaxBytes: 2 << 30},
		PageThumbDir: cfg.PageThumbDir(),
		Events:       hub,
	}
	go lib.Schedule(ctx)

	gate, err := httpapi.NewSetupGate(ctx, database, cfg.DataDir)
	if err != nil {
		return err
	}
	store := auth.Store{DB: database}
	go purgeSessions(ctx, store)

	mux := http.NewServeMux()
	mux.Handle("/api/v1/", http.StripPrefix("/api/v1", httpapi.New(httpapi.Deps{
		DB: database, Version: version, Library: lib, Tasks: queue,
		Auth: store, Limiter: &auth.Limiter{}, Gate: gate, TrustedProxies: cfg.TrustedProxies,
		Events: hub,
	})))
	if !cfg.Dev {
		mux.Handle("/", webui.Handler())
	}

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       time.Minute,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    64 << 10,
		// No WriteTimeout: /events streams for hours and sets its own
		// per-write deadline (ReadTimeout it clears for itself).
	}
	// Shutdown waits for connections to go idle, which a stream never does.
	srv.RegisterOnShutdown(hub.Close)
	errc := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", cfg.Addr, "data", cfg.DataDir, "version", version)
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		slog.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
	return nil
}

// purgeSessions deletes expired sessions once an hour.
func purgeSessions(ctx context.Context, store auth.Store) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		if n, err := store.PurgeExpired(ctx); err != nil {
			slog.Warn("purge sessions", "err", err)
		} else if n > 0 {
			slog.Info("purged expired sessions", "count", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
