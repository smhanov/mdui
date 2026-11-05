package tree

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSymlinkCycleShowsWarning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs extra privileges on Windows")
	}
	root := t.TempDir()
	writeNote(t, root, "note.md")
	if err := os.Symlink(root, filepath.Join(root, "loop")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	m := New(root)
	m.SetHeight(10)
	m = press(t, m, "down") // loop, directories sort first
	if got := m.rows[m.cursor].Name; got != "loop" {
		t.Fatalf("down selected %q, want loop", got)
	}
	m = press(t, m, "right")

	if len(m.rows) > 10 {
		t.Fatalf("cycle produced %d rows", len(m.rows))
	}
	var warning string
	for _, row := range m.rows {
		if row.Warning != "" {
			warning = row.Warning
		}
	}
	if !strings.Contains(warning, "cycle") {
		t.Fatalf("warning row = %q, want a cycle warning", warning)
	}

	out := m.View(40, 10)
	if !strings.Contains(out, "⚠") || !strings.Contains(out, "cycle") {
		t.Fatalf("cycle warning not rendered:\n%s", out)
	}

	// Toggling hidden files must not bypass cycle detection and graft the
	// target's children onto the looping node.
	m = press(t, m, ".")
	if len(m.rows) > 10 {
		t.Fatalf("toggle after cycle produced %d rows", len(m.rows))
	}
	if out := m.View(40, 10); !strings.Contains(out, "cycle") {
		t.Fatalf("cycle warning lost after hidden toggle:\n%s", out)
	}
}

func TestMutualSymlinkCycleTerminates(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs extra privileges on Windows")
	}
	root := t.TempDir()
	a := filepath.Join(root, "a")
	b := filepath.Join(root, "b")
	if err := os.Symlink(b, a); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	if err := os.Symlink(a, b); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	// Neither link can be resolved, so they are not directories; the point is
	// that scanning terminates instead of following the loop forever.
	m := New(root)
	if got := len(m.rows); got != 1 {
		t.Fatalf("unresolvable symlinks produced %d rows, want just the root", got)
	}
	if out := m.View(30, 5); !strings.Contains(out, "no markdown files found") {
		t.Fatalf("unexpected view:\n%s", out)
	}
}

func TestNestedSymlinkCycleWarns(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs extra privileges on Windows")
	}
	root := t.TempDir()
	dir := mkdir(t, root, "dir")
	writeNote(t, dir, "note.md")
	if err := os.Symlink(root, filepath.Join(dir, "back")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	m := New(root)
	m.SetHeight(10)
	m = press(t, m, "down") // dir
	m = press(t, m, "right")
	m = press(t, m, "down") // back
	if got := m.rows[m.cursor].Name; got != "back" {
		t.Fatalf("selected %q, want back", got)
	}
	m = press(t, m, "right")

	if !strings.Contains(m.View(40, 10), "cycle") {
		t.Fatalf("nested cycle warning not rendered:\n%s", m.View(40, 10))
	}
}

func TestUnreadableDirShowsWarning(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("directory permissions are not enforced for this user")
	}
	root := t.TempDir()
	locked := mkdir(t, root, "locked")
	writeNote(t, locked, "inside.md")
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })

	m := New(root)
	m.SetHeight(10)
	m = press(t, m, "down")
	m = press(t, m, "right")

	out := m.View(40, 10)
	if !strings.Contains(out, "⚠") || !strings.Contains(out, "permission denied") {
		t.Fatalf("unreadable directory warning not rendered:\n%s", out)
	}
}
