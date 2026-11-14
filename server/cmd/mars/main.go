// Command mars runs the MARS server: the REST API under /api/v1 and
// the embedded web UI everywhere else.
package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/tabunugoku/mars/server/internal/auth"
	"github.com/tabunugoku/mars/server/internal/config"
	"github.com/tabunugoku/mars/server/internal/db"
	"github.com/tabunugoku/mars/server/internal/events"
	"github.com/tabunugoku/mars/server/internal/httpapi"
	"github.com/tabunugoku/mars/server/internal/library"
	"github.com/tabunugoku/mars/server/internal/media"
	"github.com/tabunugoku/mars/server/internal/task"
	"github.com/tabunugoku/mars/server/internal/webui"
)

// version is set at build time: -ldflags "-X main.version=v1.0.0".
var version = "dev"

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(log)
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		cfg, err := config.Load()
		if err == nil {
			err = healthcheck(context.Background(), healthTarget(cfg))
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

// minDecodeBudget is the smallest decode budget set from a container limit.
const minDecodeBudget = 64 << 20

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Before any work starts: the Go memory target and the decode budget follow
	// the container's memory limit when there is one.
	if limit := applyMemoryLimit(os.Getenv, defaultCgroupFiles); limit > 0 {
		budget := max(limit/3, minDecodeBudget)
		media.SetDecodeBudget(budget)
		slog.Info("decode budget", "bytes", budget)
	}

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

	// Settings read at startup: the task thread count (restart applies it)
	// and the port unless MARS_PORT is set.
	settings, err := httpapi.ReadServerSettings(ctx, database)
	if err != nil {
		slog.Warn("stored server settings could not be read, using defaults", "err", err)
	}
	if !cfg.PortSet {
		cfg.Addr = cfg.AddrWithPort(settings.Port)
	}
	threads := settings.TaskThreads
	if threads < 1 {
		threads = runtime.NumCPU()/2 + 1
	}

	hub := events.New()
	queue := task.New(database, threads)
	queue.Events = hub
	if err := queue.Run(ctx); err != nil {
		return err
	}
	cache := &thumbCache{db: database}
	lib := &library.Service{
		DB:           database,
		Queue:        queue,
		ThumbDir:     cfg.ThumbnailDir(),
		Pages:        &media.PageCache{Dir: cfg.PageCacheDir(), MaxBytes: 2 << 30},
		PageThumbDir: cfg.PageThumbDir(),
		Events:       hub,

		// Books of one library are analysed in parallel, up to 4 at once: a
		// library usually sits on one hard disk, which gains little beyond that.
		AnalyzeWorkers: min(threads, 4),
		// Read at most every thumbCacheTTL, so a new size or format applies
		// without a restart.
		ThumbSize:   cache.bounds,
		ThumbFormat: cache.format,
	}
	go lib.Schedule(ctx)
	go lib.Watch(ctx)

	gate, err := httpapi.NewSetupGate(ctx, database, cfg.DataDir)
	if err != nil {
		return err
	}
	store := auth.Store{DB: database}
	go purgeSessions(ctx, store)

	deps := httpapi.Deps{
		DB: database, Version: version, Library: lib, Tasks: queue,
		Auth: store, Limiter: &auth.Limiter{}, Gate: gate, TrustedProxies: cfg.TrustedProxies,
		Events: hub, JPEGCacheDir: filepath.Join(cfg.ThumbnailDir(), library.JPEGCopyDir),
	}
	mux := http.NewServeMux()
	mux.Handle("/api/v1/", http.StripPrefix("/api/v1", httpapi.New(deps)))
	// OPDS and the Komga-shaped API: full paths, no prefix stripping. Longer
	// than "/", so they win over the web UI.
	compat := httpapi.NewCompat(deps)
	mux.Handle("/opds/", compat)
	mux.Handle("/komga/", compat)
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
	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return err
	}
	if err := writeListenAddr(cfg.ListenAddrFile(), cfg.Addr); err != nil {
		// The healthcheck then falls back to the default port; serving goes on.
		slog.Warn("record listen address", "err", err)
	}
	errc := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", cfg.Addr, "data", cfg.DataDir, "version", version)
		errc <- srv.Serve(ln)
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

// writeListenAddr replaces path with addr. The write goes to a temporary file
// in the same folder first, so a reader sees either the old or the new address.
func writeListenAddr(path, addr string) error {
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp) // a no-op once the rename has moved it
	if _, err := f.WriteString(addr); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// thumbCacheTTL is how long the thumbnail settings read from the database are
// kept.
const thumbCacheTTL = 30 * time.Second

// thumbCache holds the thumbnail box and format of the stored settings, so that
// making a thumbnail does not query the database. A new size or format applies
// within thumbCacheTTL.
type thumbCache struct {
	db       *sql.DB
	mu       sync.Mutex
	w, h     int
	thumbFmt media.Format
	at       time.Time
}

// refresh reads the settings again once the cached values are older than
// thumbCacheTTL. The caller holds c.mu.
func (c *thumbCache) refresh(ctx context.Context) {
	if !c.at.IsZero() && time.Since(c.at) < thumbCacheTTL {
		return
	}
	s, err := httpapi.ReadServerSettings(ctx, c.db)
	if err != nil {
		slog.Warn("stored server settings could not be read, using defaults", "err", err)
	}
	c.w, c.h = httpapi.ThumbBounds(string(s.ThumbnailSize))
	c.thumbFmt = media.ParseFormat(string(s.ThumbnailFormat))
	c.at = time.Now()
}

// bounds returns the thumbnail box (width, height) from the settings.
func (c *thumbCache) bounds(ctx context.Context) (int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.refresh(ctx)
	return c.w, c.h
}

// format returns the thumbnail format from the settings: JPEG unless the stored
// value is webp or avif.
func (c *thumbCache) format(ctx context.Context) media.Format {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.refresh(ctx)
	return c.thumbFmt
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
