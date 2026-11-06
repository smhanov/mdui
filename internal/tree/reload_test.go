package tree

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestReloadPicksUpCreatedAndDeletedFiles(t *testing.T) {
	root := t.TempDir()
	writeNote(t, root, "a.md")
	bPath := writeNote(t, root, "b.md")

	counter := &scanCounter{}
	m := newModel(root, counter.scan)
	m.SetHeight(2)
	if got := rowNames(m); !reflect.DeepEqual(got, []string{"root", "a.md", "b.md"}) {
		t.Fatalf("startup rows = %v", got)
	}

	m.moveTo(2, false) // b.md
	if got := m.cursorPath(); got != bPath {
		t.Fatalf("cursor path = %q, want %q", got, bPath)
	}
	offset := m.offset

	newPath := writeNote(t, root, "c.md")
	if cmd := m.Reload(newPath); cmd != nil {
		t.Fatalf("reload re-selected the unchanged cursor row: %T", cmd())
	}
	if counter.calls != 2 {
		t.Fatalf("reload scans = %d, want 2", counter.calls)
	}
	if got := rowNames(m); !reflect.DeepEqual(got, []string{"root", "a.md", "b.md", "c.md"}) {
		t.Fatalf("rows after create = %v", got)
	}
	if got := m.cursorPath(); got != bPath {
		t.Fatalf("create lost cursor path: got %q", got)
	}
	if m.offset != offset {
		t.Fatalf("create changed scroll offset from %d to %d", offset, m.offset)
	}

	aPath := pathIn(root, "a.md")
	if err := os.Remove(aPath); err != nil {
		t.Fatal(err)
	}
	if cmd := m.Reload(aPath); cmd != nil {
		t.Fatalf("reload re-selected the unchanged cursor row: %T", cmd())
	}
	if got := rowNames(m); !reflect.DeepEqual(got, []string{"root", "b.md", "c.md"}) {
		t.Fatalf("rows after delete = %v", got)
	}
	if got := m.cursorPath(); got != bPath {
		t.Fatalf("delete lost cursor path: got %q", got)
	}

	// Removing the file under the cursor moves to the nearest row and emits
	// a fresh selection so the viewer follows.
	if err := os.Remove(bPath); err != nil {
		t.Fatal(err)
	}
	cmd := m.Reload(bPath)
	if cmd == nil {
		t.Fatal("deleting the cursor file emitted no selection")
	}
	selected, ok := cmd().(SelectionMsg)
	if !ok {
		t.Fatalf("selection command returned %T, want SelectionMsg", cmd())
	}
	if selected.Path != pathIn(root, "c.md") {
		t.Fatalf("selection moved to %q, want c.md", selected.Path)
	}
	if got := rowNames(m); !reflect.DeepEqual(got, []string{"root", "c.md"}) {
		t.Fatalf("rows after cursor delete = %v", got)
	}
}

func TestReloadPreservesExpansionAndLoadedDirs(t *testing.T) {
	root := t.TempDir()
	docs := mkdir(t, root, "docs")
	writeNote(t, docs, "guide.md")
	unloaded := mkdir(t, root, "other")
	writeNote(t, unloaded, "later.md")
	for i := range 10 {
		writeNote(t, root, "note"+string(rune('0'+i))+".md")
	}

	m := New(root)
	m.SetHeight(4)
	m.moveTo(m.rowIndexByPath(docs), false)
	m = press(t, m, "right")
	if index := m.rowIndexByPath(docs); index < 0 || !m.rows[index].Expanded {
		t.Fatalf("docs did not expand: %v", rowNames(m))
	}

	loaded := m.LoadedDirs()
	if !containsString(loaded, root) || !containsString(loaded, docs) {
		t.Fatalf("LoadedDirs = %v, want root and docs", loaded)
	}
	if containsString(loaded, unloaded) {
		t.Fatalf("LoadedDirs includes unloaded directory: %v", loaded)
	}

	m.moveTo(m.rowIndexByPath(pathIn(docs, "guide.md")), false)
	offset := m.offset
	newNote := writeNote(t, docs, "new.md")
	if cmd := m.Reload(newNote); cmd != nil {
		t.Fatalf("reload re-selected the unchanged cursor row: %T", cmd())
	}
	if index := m.rowIndexByPath(docs); index < 0 || !m.rows[index].Expanded {
		t.Fatalf("reload collapsed docs: %v", rowNames(m))
	}
	if !containsRow(m, "new.md") {
		t.Fatalf("reload did not pick up the new file: %v", rowNames(m))
	}
	if got := m.cursorPath(); got != pathIn(docs, "guide.md") {
		t.Fatalf("reload lost cursor path: got %q", got)
	}
	if m.offset != offset {
		t.Fatalf("reload changed scroll offset from %d to %d", offset, m.offset)
	}

	// Collapsing keeps the directory loaded, and therefore watched.
	m.moveTo(m.rowIndexByPath(docs), false)
	m = press(t, m, "left")
	if index := m.rowIndexByPath(docs); index < 0 || m.rows[index].Expanded {
		t.Fatalf("docs did not collapse: %v", rowNames(m))
	}
	if !containsString(m.LoadedDirs(), docs) {
		t.Fatalf("collapsed directory dropped from LoadedDirs: %v", m.LoadedDirs())
	}
}

func TestReloadHideEmptyScopedToChangedPath(t *testing.T) {
	root := t.TempDir()
	docs := mkdir(t, root, "docs")
	writeNote(t, docs, "guide.md")
	empty := mkdir(t, root, "empty")
	if err := os.Mkdir(filepath.Join(empty, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}

	m := New(root, WithHideEmpty(true))
	if containsRow(m, "empty") {
		t.Fatalf("empty directory visible at startup: %v", rowNames(m))
	}

	// Populating a visible directory keeps it visible, and a scoped reload
	// still picks up the file.
	newNote := writeNote(t, docs, "more.md")
	if cmd := m.Reload(newNote); cmd != nil {
		t.Fatalf("reload re-selected the unchanged cursor row: %T", cmd())
	}
	if !containsRow(m, "docs") {
		t.Fatalf("docs disappeared after a scoped reload: %v", rowNames(m))
	}
	if containsRow(m, "empty") {
		t.Fatalf("unrelated empty directory appeared after a scoped reload: %v", rowNames(m))
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
