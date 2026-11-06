package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

func TestValidateDir(t *testing.T) {
	root := t.TempDir()
	if err := validateDir(root); err != nil {
		t.Fatalf("validateDir(%q) = %v, want nil", root, err)
	}

	file := filepath.Join(root, "note.md")
	if err := os.WriteFile(file, []byte("# note\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateDir(file); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("validateDir(file) = %v, want not-a-directory error", err)
	}

	missing := filepath.Join(root, "missing")
	if err := validateDir(missing); err == nil {
		t.Fatal("validateDir(missing) = nil, want error")
	}

	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Log("skipping unreadable-directory check (Windows or root)")
		return
	}

	locked := filepath.Join(root, "locked")
	if err := os.Mkdir(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })
	if err := validateDir(locked); err == nil {
		t.Fatal("validateDir(unreadable) = nil, want error")
	}
}

func TestUserTTYError(t *testing.T) {
	// openTTY builds the error chain bubbletea creates when OpenTTY fails:
	// two wrapped messages around the *fs.PathError from the open itself.
	openTTY := func(errno error) error {
		return fmt.Errorf("bubbletea: error opening TTY: %w",
			fmt.Errorf("bubbletea: could not open TTY: %w",
				&fs.PathError{Op: "open", Path: "/dev/tty", Err: errno}))
	}

	msg, ok := userTTYError(openTTY(syscall.ENXIO))
	if !ok {
		t.Fatal("userTTYError(ENXIO chain) = ok false, want true")
	}
	if want := "no interactive terminal available (mdui needs a TTY to display its interface)"; msg != want {
		t.Fatalf("userTTYError(ENXIO chain) = %q, want %q", msg, want)
	}
	if strings.Contains(msg, "bubbletea") || strings.Contains(msg, "/dev/tty") {
		t.Fatalf("userTTYError(ENXIO chain) message %q leaks internals", msg)
	}

	if _, ok := userTTYError(openTTY(syscall.ENOENT)); !ok {
		t.Fatal("userTTYError(ENOENT chain) = ok false, want true")
	}

	// The message fallback keeps matching if a future bubbletea wraps the
	// same wording differently.
	if _, ok := userTTYError(fmt.Errorf("repl: %w", errors.New("bubbletea: could not open TTY: oops"))); !ok {
		t.Fatal("userTTYError(message fallback) = ok false, want true")
	}

	// Unrelated errors pass through so main can report them verbatim.
	file := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(file, []byte("# note\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := validateDir(file)
	if err == nil {
		t.Fatal("validateDir(file) = nil, want not-a-directory error for passthrough case")
	}
	if _, ok := userTTYError(err); ok {
		t.Fatalf("userTTYError(%v) = ok true, want false", err)
	}
	if _, ok := userTTYError(fmt.Errorf("read %s: %w", file, os.ErrNotExist)); ok {
		t.Fatal("userTTYError(os.ErrNotExist) = ok true, want false")
	}
}

func TestReplayInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keys")
	if err := os.WriteFile(path, []byte("jk"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	// The replayed prefix comes out first, then the underlying file; reads
	// smaller than the pending bytes split across calls.
	r := newReplayInput(f, []byte("qp"))
	buf := make([]byte, 1)
	var got strings.Builder
	for {
		n, err := r.Read(buf)
		got.Write(buf[:n])
		if err != nil {
			break
		}
	}
	if got.String() != "qpjk" {
		t.Fatalf("replayInput produced %q, want %q", got.String(), "qpjk")
	}

	// The wrapper keeps the real descriptor so bubbletea can put the tty
	// into raw mode.
	if r.Fd() != f.Fd() {
		t.Fatal("replayInput lost the underlying file descriptor")
	}
}
