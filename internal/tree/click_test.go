package tree

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRowAtMapsVisibleWindow(t *testing.T) {
	root := t.TempDir()
	for i := range 20 {
		writeNote(t, root, "note"+string(rune('a'+i))+".md")
	}
	m := New(root)
	m.SetHeight(5)
	m.SelectRow(10)

	start := m.windowStart(5)
	for y := range 5 {
		row, index, ok := m.RowAt(y, 5)
		if !ok {
			t.Fatalf("RowAt(%d) = false, want a row", y)
		}
		if index != start+y {
			t.Fatalf("RowAt(%d) index = %d, want %d", y, index, start+y)
		}
		if index < len(m.rows) && row.Path != m.rows[index].Path {
			t.Fatalf("RowAt(%d) path = %q, want %q", y, row.Path, m.rows[index].Path)
		}
	}

	if got := m.RowIndexAt(-1, 5); got != -1 {
		t.Fatalf("RowIndexAt(-1) = %d, want -1", got)
	}
	if got := m.RowIndexAt(5, 5); got != -1 {
		t.Fatalf("RowIndexAt(past window) = %d, want -1", got)
	}
	if got := m.RowIndexAt(0, 0); got != -1 {
		t.Fatalf("RowIndexAt with no height = %d, want -1", got)
	}
}

func TestToggleRowExpandsAndCollapsesDirectory(t *testing.T) {
	m := treeFixture(t)
	dirIndex := m.rowIndexByPath(filepath.Join(m.root.Path, "dir"))
	if dirIndex < 0 {
		t.Fatal("dir row missing")
	}

	if cmd := m.ToggleRow(dirIndex); cmd != nil {
		t.Fatal("expanding a directory emitted a selection")
	}
	if !containsRow(m, "nested.md") {
		t.Fatalf("expanded directory is missing its child: %v", rowNames(m))
	}

	if cmd := m.ToggleRow(dirIndex); cmd != nil {
		t.Fatal("collapsing a directory emitted a selection")
	}
	if containsRow(m, "nested.md") {
		t.Fatalf("collapsed directory still shows its child: %v", rowNames(m))
	}
}

func TestSelectRowFileEmitsPreview(t *testing.T) {
	m := treeFixture(t)
	fileIndex := m.rowIndexByPath(filepath.Join(m.root.Path, "first.md"))

	cmd := m.SelectRow(fileIndex)
	if cmd == nil {
		t.Fatal("selecting a file emitted no command")
	}
	msg, ok := cmd().(SelectionMsg)
	if !ok || filepath.Base(msg.Path) != "first.md" {
		t.Fatalf("selection = %#v, want first.md", msg)
	}
}

func TestDescendRowEntersDirectory(t *testing.T) {
	m := treeFixture(t)
	dirIndex := m.rowIndexByPath(filepath.Join(m.root.Path, "dir"))
	if cmd := m.ToggleRow(dirIndex); cmd != nil {
		t.Fatal("first click emitted a selection")
	}

	cmd := m.DescendRow(dirIndex)
	if cmd == nil {
		t.Fatal("descending into an expanded directory emitted no selection")
	}
	msg, ok := cmd().(SelectionMsg)
	if !ok || filepath.Base(msg.Path) != "nested.md" {
		t.Fatalf("descend selection = %#v, want nested.md", msg)
	}
	if got := filepath.Base(m.cursorPath()); got != "nested.md" {
		t.Fatalf("cursor = %q, want nested.md", got)
	}
}

func TestDescendRowExpandsCollapsedDirectory(t *testing.T) {
	m := treeFixture(t)
	dirIndex := m.rowIndexByPath(filepath.Join(m.root.Path, "dir"))

	cmd := m.DescendRow(dirIndex)
	if cmd == nil {
		t.Fatal("descending into a collapsed directory emitted no selection")
	}
	msg, ok := cmd().(SelectionMsg)
	if !ok || filepath.Base(msg.Path) != "nested.md" {
		t.Fatalf("descend selection = %#v, want nested.md", msg)
	}
	if !containsRow(m, "nested.md") {
		t.Fatalf("descend did not expand the directory: %v", rowNames(m))
	}
}

func TestTreeConfigurationOptions(t *testing.T) {
	root := t.TempDir()
	writeNote(t, root, "visible.md")
	writeNote(t, root, ".secret.md")

	if m := New(root); containsRow(m, ".secret.md") {
		t.Fatal("hidden file shown by default")
	}
	if m := New(root, WithHidden(true)); !containsRow(m, ".secret.md") {
		t.Fatal("WithHidden did not include the dot file")
	}

	large := t.TempDir()
	writeNote(t, large, "a.md")
	writeNote(t, large, "b.md")
	if err := writeFileSized(t, filepath.Join(large, "a.md"), 1); err != nil {
		t.Fatal(err)
	}
	if err := writeFileSized(t, filepath.Join(large, "b.md"), 20); err != nil {
		t.Fatal(err)
	}
	if m := New(large, WithSortBy("size")); rowFileOrder(m) != "b.md,a.md" {
		t.Fatalf("size order = %s, want b.md,a.md", rowFileOrder(m))
	}

	emptyRoot := t.TempDir()
	mkdir(t, emptyRoot, "empty")
	docs := mkdir(t, emptyRoot, "docs")
	writeNote(t, docs, "nested.md")
	plain := New(emptyRoot, WithHideEmpty(true))
	if containsRow(plain, "empty") {
		t.Fatalf("empty directory was not hidden: %v", rowNames(plain))
	}
	if !containsRow(plain, "docs") {
		t.Fatalf("directory with nested markdown was hidden: %v", rowNames(plain))
	}
}

func TestSortByMtime(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "old.md")
	newer := filepath.Join(root, "new.md")
	writeNote(t, root, "old.md")
	writeNote(t, root, "new.md")
	if err := setMtime(t, old, 1000); err != nil {
		t.Fatal(err)
	}
	if err := setMtime(t, newer, 2000); err != nil {
		t.Fatal(err)
	}

	m := New(root, WithSortBy("MTIME"))
	if got := rowFileOrder(m); got != "new.md,old.md" {
		t.Fatalf("mtime order = %s, want new.md,old.md", got)
	}
}

func TestRowNamesFixtureIsStable(t *testing.T) {
	m := treeFixture(t)
	want := []string{"root", "dir", "first.md", "second.md"}
	if got := rowNames(m); !reflect.DeepEqual(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
}

func writeFileSized(t *testing.T, path string, size int) error {
	t.Helper()
	return os.WriteFile(path, []byte(strings.Repeat("x", size)), 0o644)
}

func setMtime(t *testing.T, path string, unix int64) error {
	t.Helper()
	stamp := time.Unix(unix, 0)
	return os.Chtimes(path, stamp, stamp)
}

func rowFileOrder(m Model) string {
	var names []string
	for _, row := range m.rows {
		if !row.IsDir && row.Warning == "" {
			names = append(names, row.Name)
		}
	}
	return strings.Join(names, ",")
}
