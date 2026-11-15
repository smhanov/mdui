package tree

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSearchRanksVisibleRowsAndCountsMatches(t *testing.T) {
	root := t.TempDir()
	writeSearchFile(t, root, "readme.md")
	writeSearchFile(t, root, "docs/readme.md")
	writeSearchFile(t, root, "notes/project.md")
	writeSearchFile(t, root, "collapsed/hidden-note.md")
	writeSearchFile(t, root, ".secret.md")

	m := New(root)
	expandDir(t, &m, "docs")
	expandDir(t, &m, "notes")
	if _, _, ok := m.Search("secret"); ok {
		t.Fatal("hidden file matched while hidden")
	}

	best, count, ok := m.Search("readme")
	if !ok || count != 2 {
		t.Fatalf("Search(readme) = (%d, %d, %v), want two matches", best, count, ok)
	}
	if got := m.rows[best].Path; got != filepath.Join(root, "readme.md") {
		t.Fatalf("best readme match = %q, want root readme", got)
	}

	best, count, ok = m.Search("rdm")
	if !ok || count != 2 {
		t.Fatalf("Search(rdm) = (%d, %d, %v), want two matches", best, count, ok)
	}
	if got := m.rows[best].Path; got != filepath.Join(root, "readme.md") {
		t.Fatalf("best path-segment match = %q, want root readme", got)
	}

	best, count, ok = m.Search("project")
	if !ok || count != 1 || m.rows[best].Name != "project.md" {
		t.Fatalf("Search(project) = (%d, %d, %v), want the visible project row", best, count, ok)
	}
}

func TestSearchUsesTreeOrderForTiesAndIgnoresCollapsedRows(t *testing.T) {
	root := t.TempDir()
	writeSearchFile(t, root, "alpha/abc.md")
	writeSearchFile(t, root, "beta/abc.md")
	writeSearchFile(t, root, "collapsed/abc.md")
	m := New(root)
	expandDir(t, &m, "alpha")
	expandDir(t, &m, "beta")

	best, count, ok := m.Search("abc")
	if !ok || count != 2 {
		t.Fatalf("Search(abc) = (%d, %d, %v), want two visible matches", best, count, ok)
	}
	if got, want := m.rows[best].Path, filepath.Join(root, "alpha", "abc.md"); got != want {
		t.Fatalf("tied best path = %q, want first visible row %q", got, want)
	}

	if _, _, ok := m.Search("collapsedabc"); ok {
		t.Fatal("file inside a collapsed directory matched")
	}
	if best, count, ok := m.Search("missing"); ok || count != 0 || best != -1 {
		t.Fatalf("Search(missing) = (%d, %d, %v), want (-1, 0, false)", best, count, ok)
	}
	if best, count, ok := m.Search(""); ok || count != 0 || best != -1 {
		t.Fatalf("Search(empty) = (%d, %d, %v), want (-1, 0, false)", best, count, ok)
	}
}

func writeSearchFile(t *testing.T, root, rel string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# note\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func expandDir(t *testing.T, m *Model, name string) {
	t.Helper()
	for i, row := range m.rows {
		if row.IsDir && row.Name == name {
			m.SelectRow(i)
			m.toggleOrSelect()
			return
		}
	}
	t.Fatalf("directory %q not found", name)
}
