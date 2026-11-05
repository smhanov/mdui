package tree

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

type scanCounter struct {
	calls int
	paths []string
}

func (c *scanCounter) scan(path string, showHidden bool) ([]*Node, error) {
	c.calls++
	c.paths = append(c.paths, path)
	return ScanDir(path, showHidden)
}

func press(t *testing.T, m Model, key string) Model {
	t.Helper()
	updated, _ := m.Update(keyMsg(key))
	return updated
}

func pressWithMsg(t *testing.T, m Model, key string) (Model, tea.Msg) {
	t.Helper()
	updated, cmd := m.Update(keyMsg(key))
	if cmd == nil {
		return updated, nil
	}
	return updated, cmd()
}

func keyMsg(key string) tea.KeyPressMsg {
	switch key {
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "pgup":
		return tea.KeyPressMsg{Code: tea.KeyPgUp}
	case "pgdown":
		return tea.KeyPressMsg{Code: tea.KeyPgDown}
	default:
		runes := []rune(key)
		return tea.KeyPressMsg{Code: runes[0], Text: key}
	}
}

func TestScrollMovesCursorAndWindow(t *testing.T) {
	root := t.TempDir()
	for i := range 20 {
		writeNote(t, root, fmt.Sprintf("note%02d.md", i))
	}
	m := New(root)
	m.SetHeight(5)

	cmd := m.Scroll(3)
	if cmd == nil {
		t.Fatal("scroll onto a file emitted no selection")
	}
	if _, ok := cmd().(SelectionMsg); !ok {
		t.Fatal("scroll did not emit a SelectionMsg")
	}
	if m.cursor != 3 {
		t.Fatalf("cursor = %d, want 3", m.cursor)
	}
	assertCursorVisible(t, m)

	m.Scroll(1000)
	if m.cursor != len(m.rows)-1 {
		t.Fatalf("scroll to bottom cursor = %d, want %d", m.cursor, len(m.rows)-1)
	}
	assertCursorVisible(t, m)

	m.Scroll(-1000)
	if m.cursor != 0 {
		t.Fatalf("scroll to top cursor = %d, want 0", m.cursor)
	}
	if cmd := m.Scroll(0); cmd != nil {
		t.Fatal("zero scroll emitted a command")
	}
}

func TestNavigationKeys(t *testing.T) {
	m := treeFixture(t)

	if m.cursor != 0 {
		t.Fatalf("initial cursor = %d, want 0 (root)", m.cursor)
	}

	m, msg := pressWithMsg(t, m, "down")
	if got := m.rows[m.cursor].Name; got != "dir" {
		t.Fatalf("down selected %q, want dir", got)
	}
	if msg != nil {
		t.Fatalf("down on a directory emitted %T, want nil", msg)
	}

	m, msg = pressWithMsg(t, m, "j")
	selected, ok := msg.(SelectionMsg)
	if !ok {
		t.Fatalf("j on a file emitted %T, want SelectionMsg", msg)
	}
	if filepath.Base(selected.Path) != "first.md" {
		t.Fatalf("j selected %q, want first.md", selected.Path)
	}

	m = press(t, m, "k")
	if got := m.rows[m.cursor].Name; got != "dir" {
		t.Fatalf("k selected %q, want dir", got)
	}

	m = press(t, m, "G")
	if got := m.rows[m.cursor].Name; got != "second.md" {
		t.Fatalf("G selected %q, want second.md", got)
	}
	m = press(t, m, "g")
	if m.cursor != 0 {
		t.Fatalf("g cursor = %d, want 0", m.cursor)
	}

	m = press(t, m, "up")
	if m.cursor != 0 {
		t.Fatalf("up at top moved cursor to %d", m.cursor)
	}
}

func TestPageKeysKeepCursorVisible(t *testing.T) {
	root := t.TempDir()
	for i := range 20 {
		writeNote(t, root, fmt.Sprintf("note%02d.md", i))
	}
	m := New(root)
	m.SetHeight(5)

	m = press(t, m, "pgdown")
	if m.cursor != 5 {
		t.Fatalf("first pgdown cursor = %d, want 5", m.cursor)
	}
	assertCursorVisible(t, m)

	m = press(t, m, "pgdown")
	if m.cursor != 10 {
		t.Fatalf("second pgdown cursor = %d, want 10", m.cursor)
	}
	assertCursorVisible(t, m)

	if out := m.View(40, 5); !strings.Contains(out, "note09.md") {
		t.Fatalf("view after paging does not include selected note:\n%s", out)
	}

	m = press(t, m, "pgup")
	if m.cursor != 5 {
		t.Fatalf("pgup cursor = %d, want 5", m.cursor)
	}
	assertCursorVisible(t, m)

	m = press(t, m, "G")
	if m.cursor != len(m.rows)-1 {
		t.Fatalf("G cursor = %d, want %d", m.cursor, len(m.rows)-1)
	}
	assertCursorVisible(t, m)
	if out := m.View(40, 5); !strings.Contains(out, "note19.md") {
		t.Fatalf("view at bottom does not include last note:\n%s", out)
	}

	m = press(t, m, "g")
	if m.cursor != 0 || m.offset != 0 {
		t.Fatalf("g cursor/offset = %d/%d, want 0/0", m.cursor, m.offset)
	}
}

func TestExpandCollapseAndEnter(t *testing.T) {
	m := treeFixture(t)

	m = press(t, m, "down") // dir
	m = press(t, m, "right")
	if got := rowNames(m); !reflect.DeepEqual(got, []string{"root", "dir", "nested.md", "first.md", "second.md"}) {
		t.Fatalf("after expand rows = %v", got)
	}
	if m.cursor != 1 {
		t.Fatalf("expand moved cursor to %d, want 1", m.cursor)
	}

	m, msg := pressWithMsg(t, m, "right")
	if selected, ok := msg.(SelectionMsg); !ok || filepath.Base(selected.Path) != "nested.md" {
		t.Fatalf("right from expanded dir selected %#v, want nested.md", msg)
	}

	m = press(t, m, "left") // parent dir
	if got := m.rows[m.cursor].Name; got != "dir" {
		t.Fatalf("left from child selected %q, want dir", got)
	}
	m = press(t, m, "left") // collapse
	if m.rows[m.cursor].Expanded {
		t.Fatal("left did not collapse dir")
	}
	if got := len(m.rows); got != 4 {
		t.Fatalf("collapsed tree has %d rows, want 4", got)
	}
	m = press(t, m, "left") // parent root
	if m.cursor != 0 {
		t.Fatalf("left from collapsed dir selected row %d, want root", m.cursor)
	}

	m = press(t, m, "down")
	m = press(t, m, "enter")
	if !m.rows[m.cursor].Expanded {
		t.Fatal("enter did not expand dir")
	}
	m = press(t, m, "enter")
	if m.rows[m.cursor].Expanded {
		t.Fatal("second enter did not collapse dir")
	}

	m = press(t, m, "g")
	m = press(t, m, "enter")
	if len(m.rows) != 1 {
		t.Fatalf("collapsing root left %d rows, want 1", len(m.rows))
	}
	m = press(t, m, "enter")
	if len(m.rows) != 4 {
		t.Fatalf("expanding root left %d rows, want 4", len(m.rows))
	}
}

func TestEnterOnFileEmitsSelection(t *testing.T) {
	m := treeFixture(t)
	m = press(t, m, "down")
	m = press(t, m, "down")

	_, msg := pressWithMsg(t, m, "enter")
	selected, ok := msg.(SelectionMsg)
	if !ok {
		t.Fatalf("enter on file emitted %T, want SelectionMsg", msg)
	}
	if filepath.Base(selected.Path) != "first.md" {
		t.Fatalf("enter selected %q, want first.md", selected.Path)
	}

	_, msg = pressWithMsg(t, m, "right")
	if _, ok := msg.(SelectionMsg); !ok {
		t.Fatalf("right on file emitted %T, want SelectionMsg", msg)
	}
}

func TestLazyDirectoryLoading(t *testing.T) {
	root := t.TempDir()
	dir := mkdir(t, root, "dir")
	writeNote(t, dir, "nested.md")
	other := mkdir(t, root, "other")
	writeNote(t, other, "other.md")
	writeNote(t, root, "top.md")

	counter := &scanCounter{}
	m := newModel(root, counter.scan)
	m.SetHeight(10)

	if counter.calls != 1 {
		t.Fatalf("startup scans = %d, want 1", counter.calls)
	}
	if counter.paths[0] != root {
		t.Fatalf("startup scanned %q, want root %q", counter.paths[0], root)
	}
	if got := rowNames(m); !reflect.DeepEqual(got, []string{"root", "dir", "other", "top.md"}) {
		t.Fatalf("startup rows = %v", got)
	}

	m = press(t, m, "down") // dir
	m = press(t, m, "right")
	if counter.calls != 2 {
		t.Fatalf("after first expand scans = %d, want 2", counter.calls)
	}
	if got := counter.paths[1]; got != dir {
		t.Fatalf("second scan = %q, want %q", got, dir)
	}

	m = press(t, m, "left") // collapse dir
	m = press(t, m, "right")
	if counter.calls != 2 {
		t.Fatalf("re-expanding a loaded dir scanned again (calls = %d)", counter.calls)
	}

	m = press(t, m, "left") // collapse again so the next dir is one row down
	m = press(t, m, "down") // other
	m = press(t, m, "right")
	if counter.calls != 3 {
		t.Fatalf("expanding a second dir scans = %d, want 3", counter.calls)
	}
}

func TestHiddenToggle(t *testing.T) {
	root := t.TempDir()
	writeNote(t, root, "visible.md")
	writeNote(t, root, ".hidden.md")
	secret := mkdir(t, root, ".secret")
	writeNote(t, secret, "inside.md")
	docs := mkdir(t, root, "docs")
	writeNote(t, docs, "guide.md")

	counter := &scanCounter{}
	m := newModel(root, counter.scan)
	m.SetHeight(10)

	m = press(t, m, "down") // docs
	m = press(t, m, "right")
	if got := rowNames(m); !reflect.DeepEqual(got, []string{"root", "docs", "guide.md", "visible.md"}) {
		t.Fatalf("visible rows = %v", got)
	}
	m = press(t, m, "down") // guide
	m = press(t, m, "down") // visible
	if filepath.Base(m.cursorPath()) != "visible.md" {
		t.Fatalf("cursor path = %q, want visible.md", m.cursorPath())
	}

	m = press(t, m, ".") // show hidden
	if !containsRow(m, ".hidden.md") {
		t.Fatalf("hidden file missing after toggle: %v", rowNames(m))
	}
	if !containsRow(m, ".secret") {
		t.Fatalf("hidden directory missing after toggle: %v", rowNames(m))
	}
	if got := filepath.Base(m.cursorPath()); got != "visible.md" {
		t.Fatalf("toggle lost cursor path: got %q", got)
	}
	docsIndex := m.rowIndexByPath(pathIn(root, "docs"))
	if docsIndex < 0 {
		t.Fatalf("docs row missing after toggle: %v", rowNames(m))
	}
	if !m.rows[docsIndex].Expanded {
		t.Fatal("toggle collapsed an expanded visible dir")
	}

	secretIndex := m.rowIndexByPath(pathIn(root, ".secret"))
	if secretIndex < 0 {
		t.Fatalf(".secret row missing after toggle: %v", rowNames(m))
	}
	m.moveTo(secretIndex, true)
	m = press(t, m, "right")
	if !containsRow(m, "inside.md") {
		t.Fatalf("hidden dir did not expand: %v", rowNames(m))
	}

	m = press(t, m, ".") // hide again
	if containsRow(m, ".hidden.md") || containsRow(m, ".secret") || containsRow(m, "inside.md") {
		t.Fatalf("hidden entries remain after second toggle: %v", rowNames(m))
	}
}

func assertCursorVisible(t *testing.T, m Model) {
	t.Helper()
	if m.height <= 0 {
		t.Fatal("height not set")
	}
	if m.cursor < m.offset || m.cursor >= m.offset+m.height {
		t.Fatalf("cursor %d outside window [%d,%d)", m.cursor, m.offset, m.offset+m.height)
	}
}

func treeFixture(t *testing.T) Model {
	t.Helper()
	root := t.TempDir()
	dir := mkdir(t, root, "dir")
	writeNote(t, dir, "nested.md")
	writeNote(t, root, "first.md")
	writeNote(t, root, "second.md")

	m := New(root)
	m.SetHeight(10)
	return m
}

func rowNames(m Model) []string {
	names := make([]string, len(m.rows))
	for i, row := range m.rows {
		if row.Warning != "" {
			names[i] = "⚠"
			continue
		}
		if row.Depth == 0 {
			names[i] = "root"
			continue
		}
		names[i] = row.Name
	}
	return names
}

func containsRow(m Model, name string) bool {
	for _, row := range m.rows {
		if row.Name == name {
			return true
		}
	}
	return false
}

func pathIn(root, name string) string {
	return filepath.Join(root, name)
}

func TestRootLabel(t *testing.T) {
	if got := rootLabel("/tmp/notes"); got != "notes" {
		t.Fatalf("rootLabel(/tmp/notes) = %q", got)
	}
	if got := rootLabel("/"); got != "/" {
		t.Fatalf("rootLabel(/) = %q", got)
	}
	if got := rootLabel("."); got != "." {
		t.Fatalf("rootLabel(.) = %q", got)
	}
}

func TestSelectionMessagePathIsAbsoluteForAbsoluteRoot(t *testing.T) {
	root := t.TempDir()
	note := writeNote(t, root, "note.md")
	m := New(root)
	m = press(t, m, "down")
	_, msg := pressWithMsg(t, m, "right")
	selected, ok := msg.(SelectionMsg)
	if !ok {
		t.Fatalf("right on file emitted %T, want SelectionMsg", msg)
	}
	if selected.Path != note {
		t.Fatalf("selected path = %q, want %q", selected.Path, note)
	}
}

func TestMissingRootShowsWarning(t *testing.T) {
	counter := &scanCounter{}
	m := newModel(filepath.Join(t.TempDir(), "missing"), counter.scan)
	if counter.calls != 1 {
		t.Fatalf("startup scans = %d, want 1", counter.calls)
	}
	if m.root.Err == "" {
		t.Fatal("missing root should carry an error")
	}
	if out := m.View(40, 6); !strings.Contains(out, "⚠") {
		t.Fatalf("missing-root warning not rendered:\n%s", out)
	}
}

func TestOsStatErrorsAreReadable(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "missing")
	if _, err := os.Stat(missing); err == nil {
		t.Fatal("expected stat error")
	}
	if got := errorText(&os.PathError{Op: "open", Path: missing, Err: os.ErrNotExist}); got != os.ErrNotExist.Error() {
		t.Fatalf("errorText = %q", got)
	}
}
