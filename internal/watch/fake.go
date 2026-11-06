package watch

import (
	"sync"

	"github.com/fsnotify/fsnotify"
)

// Fake is a channel-backed Watcher for tests. Add and Remove only record
// their calls; Send injects an event batch that Next returns to the waiting
// subscription.
type Fake struct {
	events chan []string
	done   chan struct{}
	closed sync.Once

	mu         sync.Mutex
	addCalls   []string
	removeCall []string
	watched    map[string]bool
	closeCalls int

	// AddErr, when set, decides whether an Add succeeds. Returning an error
	// simulates a watch-limit or resource failure without a real filesystem.
	AddErr func(dir string) error
}

// NewFake returns an empty fake watcher.
func NewFake() *Fake {
	return &Fake{
		events:  make(chan []string, 32),
		done:    make(chan struct{}),
		watched: make(map[string]bool),
	}
}

// Send queues one batch for Next. It is a no-op after Close.
func (f *Fake) Send(paths ...string) {
	batch := append([]string(nil), paths...)
	select {
	case f.events <- batch:
	case <-f.done:
	}
}

func (f *Fake) Add(dir string) error {
	f.mu.Lock()
	f.addCalls = append(f.addCalls, dir)
	hook := f.AddErr
	f.mu.Unlock()

	if hook != nil {
		if err := hook(dir); err != nil {
			return err
		}
	}

	f.mu.Lock()
	f.watched[dir] = true
	f.mu.Unlock()
	return nil
}

func (f *Fake) Remove(dir string) error {
	f.mu.Lock()
	f.removeCall = append(f.removeCall, dir)
	delete(f.watched, dir)
	f.mu.Unlock()
	return nil
}

func (f *Fake) Next() ([]string, error) {
	select {
	case <-f.done:
		return nil, fsnotify.ErrClosed
	default:
	}
	select {
	case paths := <-f.events:
		return append([]string(nil), paths...), nil
	case <-f.done:
		return nil, fsnotify.ErrClosed
	}
}

// Close is idempotent. The call count is still recorded so a test can assert
// the application closes the watcher on its quit path.
func (f *Fake) Close() error {
	f.mu.Lock()
	f.closeCalls++
	f.mu.Unlock()
	f.closed.Do(func() { close(f.done) })
	return nil
}

// AddAttempts returns every directory passed to Add, including failures.
func (f *Fake) AddAttempts() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.addCalls...)
}

// Removed returns every directory passed to Remove.
func (f *Fake) Removed() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.removeCall...)
}

// Watched reports whether dir is currently in the fake's watch set.
func (f *Fake) Watched(dir string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.watched[dir]
}

// CloseCalls returns the number of Close invocations.
func (f *Fake) CloseCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closeCalls
}
