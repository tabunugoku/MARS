package library

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/tabunugoku/cominavi/server/internal/archive"
	"github.com/tabunugoku/cominavi/server/internal/scan"
)

// watchDelay is how long a library must stay quiet after its last relevant
// file event before a scan is queued. A variable so tests can shorten it.
var watchDelay = 30 * time.Second

// After a failed start a library is not tried again until its config changes
// or the back-off elapses: watchRetryFirst after the first failure, doubling
// after each further one up to watchRetryMax. Variables so tests can shorten them.
var (
	watchRetryFirst = 10 * time.Minute
	watchRetryMax   = 6 * time.Hour
)

// watchStartHook, when set, is called for each attempt to start a library's
// watcher. Tests use it to count attempts.
var watchStartHook func(libraryID string)

// watchBackoff returns the wait after the given number of consecutive failed starts.
func watchBackoff(failures int) time.Duration {
	d := watchRetryFirst
	for i := 1; i < failures && d < watchRetryMax; i++ {
		d *= 2
	}
	if d > watchRetryMax {
		d = watchRetryMax
	}
	return d
}

const (
	watchMsgLimit   = "ファイルの監視を始められませんでした（監視できるフォルダ数の上限）。定期スキャンを使ってください"
	watchMsgGeneric = "ファイルの監視を始められませんでした。定期スキャンを使ってください"
)

// watchState is one library's watcher. It is written only by the Watch loop
// (and by its goroutine through fail, under watchSet.mu); WatchError reads err.
type watchState struct {
	sig    string             // root and exclude globs the watcher was started with
	cancel context.CancelFunc // nil when the watcher never started
	done   chan struct{}      // closed when the watcher goroutine has exited
	err    string             // WatchError message; "" when watching
	// failures counts consecutive failed starts (cancel is nil); retryAt is the
	// earliest time the start is tried again.
	failures int
	retryAt  time.Time
}

// running reports whether the watcher goroutine is still alive.
func (st *watchState) running() bool {
	if st == nil || st.done == nil {
		return false
	}
	select {
	case <-st.done:
		return false
	default:
		return true
	}
}

// halt stops the watcher and waits for it to release its resources.
func (st *watchState) halt() {
	if st != nil && st.cancel != nil {
		st.cancel()
		<-st.done
	}
}

type watchSet struct {
	mu     sync.Mutex
	states map[string]*watchState // library id -> state
}

// watchSets keeps the watcher state of each Service, so the Service struct
// does not need fields for it.
var watchSets sync.Map // *Service -> *watchSet

func (s *Service) watches() *watchSet {
	v, _ := watchSets.LoadOrStore(s, &watchSet{states: map[string]*watchState{}})
	return v.(*watchSet)
}

// WatchError says why watching the library for file changes is not working
// ("" when it works or is switched off). Interval scans run either way.
// Network mounts (SMB/NFS) usually deliver no events at all, which cannot be
// detected here; interval scans cover them.
func (s *Service) WatchError(libraryID string) string {
	ws := s.watches()
	ws.mu.Lock()
	defer ws.mu.Unlock()
	if st := ws.states[libraryID]; st != nil {
		return st.err
	}
	return ""
}

// Watch keeps a file watcher running for every library with watch on: it
// reloads the settings at start and then every minute, starts watchers that
// are missing, and stops those whose library was removed, switched off, or
// moved to another root or exclude list. It returns when ctx is done.
func (s *Service) Watch(ctx context.Context) {
	ws := s.watches()
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		s.reconcileWatches(ctx, ws)
		select {
		case <-ctx.Done():
			ws.mu.Lock()
			old := ws.states
			ws.states = map[string]*watchState{}
			ws.mu.Unlock()
			for _, st := range old {
				st.halt()
			}
			return
		case <-t.C:
		}
	}
}

func (s *Service) reconcileWatches(ctx context.Context, ws *watchSet) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, root, watch, exclude FROM libraries`)
	if err != nil {
		slog.Error("watch", "err", err)
		return
	}
	type want struct {
		id, root, excl string
	}
	var wants []want
	for rows.Next() {
		var w want
		var on int
		if err := rows.Scan(&w.id, &w.root, &on, &w.excl); err != nil {
			continue
		}
		if on != 0 {
			wants = append(wants, w)
		}
	}
	rows.Close()

	keep := map[string]bool{}
	var halt []*watchState
	for _, w := range wants {
		keep[w.id] = true
		sig := w.root + "\x00" + w.excl

		ws.mu.Lock()
		st := ws.states[w.id]
		ws.mu.Unlock()
		if st != nil && st.sig == sig {
			if st.running() {
				continue
			}
			if st.cancel == nil && time.Now().Before(st.retryAt) {
				// The last start failed and its back-off has not passed.
				continue
			}
		}
		prevFailures := 0
		if st != nil {
			if st.sig == sig && st.cancel == nil {
				prevFailures = st.failures
			}
			// Settings changed, the watcher died, or the back-off passed. A
			// failed start is logged only when its message changes.
			halt = append(halt, st)
		}
		var exclude []string
		_ = json.Unmarshal([]byte(w.excl), &exclude)
		next := &watchState{sig: sig}
		id := w.id
		scanFn := func() { s.queueWatchScan(ctx, id) }
		var fail func(error)
		fail = func(err error) { // called from the watcher goroutine
			msg := watchErrMessage(err)
			slog.Warn("watch library", "library", id, "err", err)
			ws.mu.Lock()
			next.err = msg
			ws.mu.Unlock()
		}
		if watchStartHook != nil {
			watchStartHook(w.id)
		}
		cancel, done, err := watchTree(ctx, w.root, exclude, scanFn, fail)
		if err != nil {
			msg := watchErrMessage(err)
			ws.mu.Lock()
			prev := ""
			if st != nil {
				prev = st.err
			}
			ws.mu.Unlock()
			if prev != msg {
				slog.Warn("watch library", "library", w.id, "err", err)
			}
			next.err = msg
			next.failures = prevFailures + 1
			next.retryAt = time.Now().Add(watchBackoff(next.failures))
		} else {
			next.cancel, next.done = cancel, done
		}
		ws.mu.Lock()
		ws.states[w.id] = next
		ws.mu.Unlock()
	}

	ws.mu.Lock()
	for id, st := range ws.states {
		if !keep[id] {
			halt = append(halt, st)
			delete(ws.states, id)
		}
	}
	ws.mu.Unlock()
	for _, st := range halt {
		st.halt()
	}
}

// queueWatchScan queues a library scan for a watcher. The queue already
// merges a scan that is still waiting, so at most one is pending.
func (s *Service) queueWatchScan(ctx context.Context, libraryID string) {
	if _, err := s.QueueScan(ctx, libraryID); err != nil && ctx.Err() == nil {
		slog.Error("watch scan", "library", libraryID, "err", err)
	}
}

func watchErrMessage(err error) string {
	if errors.Is(err, syscall.ENOSPC) || errors.Is(err, syscall.EMFILE) {
		return watchMsgLimit
	}
	return watchMsgGeneric
}

// watchTree watches root and every folder under it that scan would not skip,
// until the returned cancel is called or ctx ends. Each change that could
// matter to the library calls scan once the library has been quiet for
// watchDelay. A failure while watching calls fail and ends the watcher (done
// closes). A failure to start returns err.
func watchTree(ctx context.Context, root string, exclude []string, onQuiet func(), fail func(error)) (context.CancelFunc, chan struct{}, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, nil, err
	}
	if err := addTree(w, root, root, exclude); err != nil {
		w.Close()
		return nil, nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer w.Close()
		var timer *time.Timer
		var fire <-chan time.Time
		quiet := func() {
			if timer != nil {
				timer.Stop()
			}
			timer = time.NewTimer(watchDelay)
			fire = timer.C
		}
		for {
			select {
			case <-ctx.Done():
				if timer != nil {
					timer.Stop()
				}
				return
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				dirty, err := handleWatchEvent(w, root, ev, exclude)
				if err != nil && !errors.Is(err, fs.ErrNotExist) {
					fail(err)
					return
				}
				if dirty {
					quiet()
				}
			case _, ok := <-w.Errors:
				if !ok {
					return
				}
				// Events may have been lost (for example an inotify queue
				// overflow): scan to be safe.
				quiet()
			case <-fire:
				fire, timer = nil, nil
				onQuiet()
			}
		}
	}()
	return cancel, done, nil
}

// handleWatchEvent reports whether ev can change the library. A new folder is
// added to the watch set first.
func handleWatchEvent(w *fsnotify.Watcher, root string, ev fsnotify.Event, exclude []string) (bool, error) {
	rel, err := filepath.Rel(root, ev.Name)
	if err != nil || rel == "." {
		return false, nil
	}
	rel = filepath.ToSlash(rel)
	if ev.Has(fsnotify.Remove) || ev.Has(fsnotify.Rename) {
		// The entry may have been a folder, so any removal counts.
		return !scan.Skip(rel, false, exclude), nil
	}
	if ev.Has(fsnotify.Create) {
		if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
			if scan.Skip(rel, true, exclude) {
				return false, nil
			}
			return true, addTree(w, root, ev.Name, exclude)
		}
	}
	if ev.Has(fsnotify.Create) || ev.Has(fsnotify.Write) {
		if _, ok := archive.FormatOf(filepath.Base(ev.Name)); ok && !scan.Skip(rel, false, exclude) {
			return true, nil
		}
	}
	return false, nil
}

// addTree adds dir and the folders under it to the watcher, leaving out what
// scan skips. Folders that vanish meanwhile are not an error.
func addTree(w *fsnotify.Watcher, root, dir string, exclude []string) error {
	return filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == dir {
				return err
			}
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if p != root {
			rel, rerr := filepath.Rel(root, p)
			if rerr != nil {
				return nil
			}
			if scan.Skip(filepath.ToSlash(rel), true, exclude) {
				return filepath.SkipDir
			}
		}
		if err := w.Add(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	})
}
