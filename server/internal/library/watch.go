package library

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/tabunugoku/mars/server/internal/archive"
	"github.com/tabunugoku/mars/server/internal/scan"
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

// watchScanHook, when set, is called in place of queueing a scan for a
// library's watcher, with the scope and full flag the watcher asked for. Tests
// use it to check the scans watchers request.
var watchScanHook func(libraryID string, scope []string, full bool)

// maxScopeEntries is how many distinct top-level folders a watcher may report
// before its scan becomes a full one: past that a full walk costs about the same.
const maxScopeEntries = 50

// dirtyScope is the set of top-level entries that changed, or the mark that a
// full scan is needed (a lost event, or more than maxScopeEntries entries).
// The zero value is an empty set.
type dirtyScope struct {
	entries map[string]bool
	full    bool
}

func (d *dirtyScope) add(entry string) {
	if d.full {
		return
	}
	if d.entries == nil {
		d.entries = map[string]bool{}
	}
	d.entries[entry] = true
	if len(d.entries) > maxScopeEntries {
		d.markFull()
	}
}

func (d *dirtyScope) markFull() {
	d.full, d.entries = true, nil
}

// take returns the entries to scan, sorted, and whether a full scan is needed
// (then the entries are nil). The set is emptied.
func (d *dirtyScope) take() ([]string, bool) {
	var scope []string
	for e := range d.entries {
		scope = append(scope, e)
	}
	slices.Sort(scope)
	full := d.full
	d.full, d.entries = false, nil
	if full {
		scope = nil
	}
	return scope, full
}

// topEntry returns the first path segment of name relative to root, which is
// the top-level folder or root-level file a change belongs to ("" when name is
// the root itself).
func topEntry(root, name string) string {
	rel, err := filepath.Rel(root, name)
	if err != nil || rel == "." {
		return ""
	}
	first, _, _ := strings.Cut(filepath.ToSlash(rel), "/")
	return first
}

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

// WatchErrLimit and WatchErrGeneric are the reasons WatchError returns, as
// message codes; httpapi words them for the user.
const (
	WatchErrLimit   = "watch.limit"
	WatchErrGeneric = "watch.generic"
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
	// seen marks the libraries that have had a watcher state (running or
	// failed) since the server started. A start of such a library may have
	// missed changes, so it queues a catch-up scan.
	seen map[string]bool
}

// watchSets keeps the watcher state of each Service, so the Service struct
// does not need fields for it.
var watchSets sync.Map // *Service -> *watchSet

func (s *Service) watches() *watchSet {
	v, _ := watchSets.LoadOrStore(s, &watchSet{states: map[string]*watchState{}, seen: map[string]bool{}})
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
		seenBefore := ws.seen[w.id]
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
		scanFn := func(scope []string, full bool) { s.queueWatchScan(ctx, id, scope, full) }
		fail := func(err error) { // called from the watcher goroutine
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
		ws.seen[w.id] = true
		ws.mu.Unlock()
		if err == nil && seenBefore {
			// The library was (or was meant to be) watched before: changes
			// made while it was not watched are picked up now, by a full scan.
			scanFn(nil, true)
		}
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

// queueWatchScan asks for a scan of libraryID after a watcher's change: of the
// scope entries, or of the whole library when full. The entries are merged
// into the library's pending set first; the queued task takes them when it
// runs, and a queued one absorbs later requests.
func (s *Service) queueWatchScan(ctx context.Context, libraryID string, scope []string, full bool) {
	if watchScanHook != nil {
		watchScanHook(libraryID, scope, full)
		return
	}
	s.addWatchDirty(libraryID, scope, full)
	if _, err := s.queueWatchTask(ctx, libraryID); err != nil && ctx.Err() == nil {
		slog.Error("watch scan", "library", libraryID, "err", err)
	}
}

func watchErrMessage(err error) string {
	if errors.Is(err, syscall.ENOSPC) || errors.Is(err, syscall.EMFILE) {
		return WatchErrLimit
	}
	return WatchErrGeneric
}

// watchTree watches root and every folder under it that scan would not skip,
// until the returned cancel is called or ctx ends. Each change that could
// matter to the library calls onQuiet once the library has been quiet for
// watchDelay. onQuiet gets the top-level entries that changed (scope), or full
// when a scan of the whole library is needed: after lost events, or with more
// than maxScopeEntries entries. A failure while watching calls fail and ends
// the watcher (done closes); a change still waiting for its quiet period is
// passed to onQuiet first, so the failure does not drop it. A failure to start
// returns err.
func watchTree(ctx context.Context, root string, exclude []string, onQuiet func(scope []string, full bool), fail func(error)) (context.CancelFunc, chan struct{}, error) {
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
		var pending dirtyScope // changes since the last hand-over to onQuiet
		quiet := func() {
			if timer != nil {
				timer.Stop()
			}
			timer = time.NewTimer(watchDelay)
			fire = timer.C
		}
		// handOver passes the pending changes to onQuiet and clears them.
		handOver := func() {
			scope, full := pending.take()
			onQuiet(scope, full)
		}
		// flush queues the scan for a change still waiting for its quiet
		// period. Called on the way out of a failure, since nothing else will.
		flush := func() {
			if timer != nil {
				timer.Stop()
				timer, fire = nil, nil
				handOver()
			}
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
					flush()
					return
				}
				dirty, err := handleWatchEvent(w, root, ev, exclude)
				if err != nil && !errors.Is(err, fs.ErrNotExist) {
					flush()
					fail(err)
					return
				}
				if dirty {
					if e := topEntry(root, ev.Name); e != "" {
						pending.add(e)
					} else {
						pending.markFull()
					}
					quiet()
				}
			case _, ok := <-w.Errors:
				if !ok {
					flush()
					return
				}
				// Events may have been lost (for example an inotify queue
				// overflow): pick up folders made meanwhile, then scan the
				// whole library to be safe.
				if err := rescanTree(w, root, exclude); err != nil {
					flush()
					fail(err)
					return
				}
				pending.markFull()
				quiet()
			case <-fire:
				fire, timer = nil, nil
				handOver()
			}
		}
	}()
	return cancel, done, nil
}

// errWatchRootGone is what handleWatchEvent returns when the library root
// itself is removed or renamed. The watch set then covers nothing, so the
// watcher must fail; the reconcile loop restarts it with back-off.
var errWatchRootGone = errors.New("library root removed or renamed")

// handleWatchEvent reports whether ev can change the library. A new folder is
// added to the watch set first. A removal or rename of the root returns
// errWatchRootGone.
func handleWatchEvent(w *fsnotify.Watcher, root string, ev fsnotify.Event, exclude []string) (bool, error) {
	rel, err := filepath.Rel(root, ev.Name)
	if err != nil {
		return false, nil
	}
	if rel == "." {
		if ev.Has(fsnotify.Remove) || ev.Has(fsnotify.Rename) {
			return false, errWatchRootGone
		}
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

// rescanTree adds root and the folders under it to the watcher again. It runs
// after events may have been lost, so a folder created meanwhile gets watched.
// Folders already watched are unchanged (fsnotify's Add is idempotent). A
// missing root is not an error here: its removal event fails the watcher.
func rescanTree(w *fsnotify.Watcher, root string, exclude []string) error {
	if err := addTree(w, root, root, exclude); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
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
