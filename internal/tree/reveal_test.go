package tree

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRevealExpandsAncestorsAndScrollsToFile(t *testing.T) {
	root := t.TempDir()
	alpha := mkdir(t, root, "alpha")
	deep := mkdir(t, alpha, "beta")
	note := writeNote(t, deep, "deep.md")
	mustWritePaths(t, root, "other.md")

	counter := &scanCounter{}
	m := newModel(root, counter.scan)
	m.SetHeight(3)
	initialScans := counter.calls

	if !m.Reveal(note) {
		t.Fatal("Reveal returned false for an existing nested file")
	}
	if m.rows[m.cursor].Path != note {
		t.Fatalf("cursor row = %q, want %q", m.rows[m.cursor].Path, note)
	}
	assertCursorVisible(t, m)
	for _, name := range []string{"alpha", "beta"} {
		if !containsRow(m, name) {
			t.Fatalf("ancestor %q is not visible after Reveal", name)
		}
	}
	if counter.calls != initialScans+2 {
		t.Fatalf("scanner calls = %d, want %d (one per newly loaded level)", counter.calls, initialScans+2)
	}

	// A second reveal of the same file must not re-scan loaded levels.
	if !m.Reveal(note) {
		t.Fatal("second Reveal failed")
	}
	if counter.calls != initialScans+2 {
		t.Fatalf("second Reveal scanned %d extra levels", counter.calls-(initialScans+2))
	}
}

func TestRevealRestoresCollapsedRootAndAncestors(t *testing.T) {
	root := t.TempDir()
	one := mkdir(t, root, "one")
	deep := mkdir(t, one, "two")
	note := writeNote(t, deep, "note.md")

	m := New(root)
	m.SetHeight(5)
	m = press(t, m, "enter") // collapse the root row
	if m.rows[0].Expanded {
		t.Fatal("root did not collapse")
	}
	if !m.Reveal(note) {
		t.Fatal("Reveal failed through a collapsed root")
	}
	if !m.rows[0].Expanded {
		t.Fatal("Reveal did not expand the root")
	}
	if m.rows[m.cursor].Path != note {
		t.Fatalf("cursor row = %q, want %q", m.rows[m.cursor].Path, note)
	}
}

func TestRevealRejectsInvalidPathsWithoutMovingCursor(t *testing.T) {
	root := t.TempDir()
	mustWritePaths(t, root, "first.md")
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("# outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := New(root)
	m.SetHeight(5)
	m = press(t, m, "down") // cursor on first.md
	cursor := m.cursor
	rows := len(m.rows)

	for _, path := range []string{
		"",
		root,
		filepath.Join(root, "missing.md"),
		outside,
		filepath.Dir(root),
	} {
		if m.Reveal(path) {
			t.Errorf("Reveal(%q) = true, want false", path)
		}
		if m.cursor != cursor || len(m.rows) != rows {
			t.Errorf("Reveal(%q) changed the tree: cursor=%d rows=%d", path, m.cursor, len(m.rows))
		}
	}
}

func TestRevealRespectsHiddenSetting(t *testing.T) {
	root := t.TempDir()
	hidden := filepath.Join(root, ".hidden.md")
	if err := os.WriteFile(hidden, []byte("# hidden\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	plain := New(root)
	if plain.Reveal(hidden) {
		t.Fatal("Reveal found a hidden file while hidden files are off")
	}

	showing := New(root, WithHidden(true))
	if !showing.Reveal(hidden) {
		t.Fatal("Reveal did not find a hidden file while hidden files are on")
	}
	if got := showing.rows[showing.cursor].Path; got != hidden {
		t.Fatalf("cursor row = %q, want %q", got, hidden)
	}
}

func TestRevealThroughSymlinkedDirectory(t *testing.T) {
	root := t.TempDir()
	real := mkdir(t, root, "real")
	note := writeNote(t, real, "note.md")
	if err := os.Symlink(real, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	m := New(root)
	if !m.Reveal(filepath.Join(root, "link", "note.md")) {
		t.Fatal("Reveal failed through a symlinked directory")
	}
	if got := m.rows[m.cursor].Path; got != filepath.Join(root, "link", "note.md") {
		t.Fatalf("cursor row = %q, want the symlink path", got)
	}
	if m.rows[m.cursor].Path == note {
		t.Fatal("cursor landed on the real path instead of the selected symlink path")
	}
}

func mustWritePaths(t *testing.T, root string, names ...string) {
	t.Helper()
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(root, name), []byte("# "+name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
