// Package watch turns filesystem notifications into Bubble Tea messages. A
// watcher subscribes to directories, not individual files, so one watch covers
// every entry inside a directory. Callers decide which directories to watch;
// this package only filters the event stream and batches bursts.
package watch

import (
	"path/filepath"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"
	"github.com/fsnotify/fsnotify"
)

// ChangeMsg reports a batch of changed paths. Paths are the names fsnotify
// reported, relative to or absolute like the paths passed to Add.
type ChangeMsg struct {
	Paths []string
}

// Watcher is the directory-watch set the application owns. Add and Remove are
// called from the update loop; Next blocks on a separate goroutine until an
// event or a close arrives.
type Watcher interface {
	Add(dir string) error
	Remove(dir string) error
	Next() ([]string, error)
	Close() error
}

// Subscribe returns a command that blocks on the next change batch and turns
// it into a ChangeMsg. A closed watcher (or any read error) yields a nil
// message so the application stops resubscribing instead of busy-looping.
func Subscribe(w Watcher) tea.Cmd {
	if w == nil {
		return nil
	}
	return func() tea.Msg {
		paths, err := w.Next()
		if err != nil || len(paths) == 0 {
			return nil
		}
		return ChangeMsg{Paths: paths}
	}
}

// Noisy reports whether path has a dot-prefixed component below root. The
// hidden-file rule is the same one the tree scanner uses, so watcher batches
// and application-level filters agree about what counts as noise.
func Noisy(root, path string, hiddenShown bool) bool {
	if hiddenShown {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil {
		rel = filepath.Clean(path)
	}
	if rel == "." {
		return false
	}
	return hasDotComponent(rel)
}

func hasDotComponent(path string) bool {
	for _, part := range strings.Split(path, string(filepath.Separator)) {
		if part == "." || part == ".." || part == "" {
			continue
		}
		if strings.HasPrefix(part, ".") {
			return true
		}
	}
	return false
}

// notifyWatcher wraps fsnotify. The watched set is kept here so filtered paths
// can be judged relative to the directory that produced them: a user who opens
// a dot-named folder should still receive events for that folder's children.
type notifyWatcher struct {
	notify  *fsnotify.Watcher
	hidden  bool
	mu      sync.Mutex
	watched map[string]struct{}
}

// New returns a watcher backed by fsnotify. When hiddenShown is false, events
// with a dot-prefixed component below their watch root are dropped; this keeps
// .git and editor swap-file churn from forcing rescans.
func New(hiddenShown bool) (Watcher, error) {
	notify, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &notifyWatcher{
		notify:  notify,
		hidden:  hiddenShown,
		watched: make(map[string]struct{}),
	}, nil
}

func (w *notifyWatcher) Add(dir string) error {
	clean := filepath.Clean(dir)
	w.mu.Lock()
	w.watched[clean] = struct{}{}
	w.mu.Unlock()

	if err := w.notify.Add(clean); err != nil {
		w.mu.Lock()
		delete(w.watched, clean)
		w.mu.Unlock()
		return err
	}
	return nil
}

func (w *notifyWatcher) Remove(dir string) error {
	clean := filepath.Clean(dir)
	err := w.notify.Remove(clean)
	w.mu.Lock()
	delete(w.watched, clean)
	w.mu.Unlock()
	return err
}

func (w *notifyWatcher) Close() error {
	return w.notify.Close()
}

// Next waits for one relevant event, then drains every event already queued
// without blocking so an editor save or checkout coalesces into one batch.
func (w *notifyWatcher) Next() ([]string, error) {
	for {
		select {
		case event, ok := <-w.notify.Events:
			if !ok {
				return nil, fsnotify.ErrClosed
			}
			path, keep := w.accept(event)
			if !keep {
				continue
			}
			paths := []string{path}
			for {
				select {
				case event, ok := <-w.notify.Events:
					if !ok {
						return paths, nil
					}
					if path, keep := w.accept(event); keep {
						paths = append(paths, path)
					}
				default:
					return paths, nil
				}
			}
		case err, ok := <-w.notify.Errors:
			if !ok {
				return nil, fsnotify.ErrClosed
			}
			if err == nil {
				continue
			}
			return nil, err
		}
	}
}

// accept applies the operation and noise filters. Directory Write events are
// dropped because kqueue and Windows emit them merely to say "an entry in this
// directory changed"; the corresponding child event carries the real path.
func (w *notifyWatcher) accept(event fsnotify.Event) (string, bool) {
	if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename) == 0 {
		return "", false
	}
	path := filepath.Clean(event.Name)
	if w.hidden {
		return path, true
	}
	if w.isWatchedDir(path) && event.Op == fsnotify.Write {
		return "", false
	}
	if w.noisy(path) {
		return "", false
	}
	return path, true
}

func (w *notifyWatcher) isWatchedDir(path string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, ok := w.watched[path]
	return ok
}

func (w *notifyWatcher) noisy(path string) bool {
	w.mu.Lock()
	base := ""
	for dir := range w.watched {
		if path != dir && !strings.HasPrefix(path, dir+string(filepath.Separator)) {
			continue
		}
		if len(dir) > len(base) {
			base = dir
		}
	}
	w.mu.Unlock()

	if base == "" {
		return hasDotComponent(path)
	}
	if path == base {
		return false
	}
	return hasDotComponent(strings.TrimPrefix(path, base+string(filepath.Separator)))
}
