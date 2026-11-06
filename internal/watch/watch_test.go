package watch

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

const eventTimeout = 5 * time.Second

func TestWatcherBatchesWrites(t *testing.T) {
	dir := t.TempDir()
	w, err := New(false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })
	if err := w.Add(dir); err != nil {
		t.Fatalf("Add: %v", err)
	}

	names := []string{"one.md", "two.md", "three.md"}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("# "+name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// kqueue may deliver the writes across a couple of batches even though
	// each Next drains everything already queued; collect until all three
	// paths have arrived.
	seen := make(map[string]bool, len(names))
	deadline := time.After(eventTimeout)
	for len(seen) < len(names) {
		result := nextAsync(w)
		select {
		case got := <-result:
			if got.err != nil {
				t.Fatalf("Next: %v", got.err)
			}
			for _, path := range got.paths {
				seen[filepath.Base(path)] = true
			}
		case <-deadline:
			t.Fatalf("timed out after seeing %v, want %v", seen, names)
		}
	}
}

func TestWatcherFiltersDotNoise(t *testing.T) {
	dir := t.TempDir()
	w, err := New(false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })
	if err := w.Add(dir); err != nil {
		t.Fatalf("Add: %v", err)
	}

	result := nextAsync(w)
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".hidden.md"), []byte("# hidden\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-result:
		if got.err != nil {
			t.Fatalf("unexpected watcher error: %v", got.err)
		}
		t.Fatalf("dot-prefixed churn produced a change batch: %v", got.paths)
	case <-time.After(300 * time.Millisecond):
	}

	visible := filepath.Join(dir, "visible.md")
	if err := os.WriteFile(visible, []byte("# visible\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-result:
		if got.err != nil {
			t.Fatalf("unexpected watcher error: %v", got.err)
		}
		if len(got.paths) == 0 || got.paths[0] != visible {
			t.Fatalf("batch = %v, want the visible file", got.paths)
		}
	case <-time.After(eventTimeout):
		t.Fatal("visible file did not produce a change batch")
	}
}

func TestWatcherPassesDotPathsWhenHiddenShown(t *testing.T) {
	dir := t.TempDir()
	w, err := New(true)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })
	if err := w.Add(dir); err != nil {
		t.Fatalf("Add: %v", err)
	}

	result := nextAsync(w)
	hidden := filepath.Join(dir, ".visible.md")
	if err := os.WriteFile(hidden, []byte("# hidden\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(eventTimeout)
	for {
		select {
		case got := <-result:
			if got.err != nil {
				t.Fatalf("Next: %v", got.err)
			}
			for _, path := range got.paths {
				if path == hidden {
					return
				}
			}
			result = nextAsync(w)
		case <-deadline:
			t.Fatal("dot-prefixed path was filtered even though hidden files are shown")
		}
	}
}

func TestWatcherCloseEndsSubscription(t *testing.T) {
	dir := t.TempDir()
	w, err := New(false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := w.Add(dir); err != nil {
		t.Fatalf("Add: %v", err)
	}

	done := make(chan tea.Msg, 1)
	go func() { done <- Subscribe(w)() }()

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case msg := <-done:
		if msg != nil {
			t.Fatalf("subscription returned %T after Close, want nil", msg)
		}
	case <-time.After(eventTimeout):
		t.Fatal("Close did not end the blocked subscription")
	}
}

func TestSubscribeIgnoresClosedWatcher(t *testing.T) {
	w := NewFake()
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if msg := Subscribe(w)(); msg != nil {
		t.Fatalf("Subscribe on a closed fake returned %T, want nil", msg)
	}
	if cmd := Subscribe(nil); cmd != nil {
		t.Fatal("Subscribe(nil) returned a command, want nil")
	}
}

type nextResult struct {
	paths []string
	err   error
}

func nextAsync(w Watcher) <-chan nextResult {
	out := make(chan nextResult, 1)
	go func() {
		paths, err := w.Next()
		out <- nextResult{paths: paths, err: err}
	}()
	return out
}
