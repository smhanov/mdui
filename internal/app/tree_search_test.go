package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestTreeTypeToSearchAndCancel(t *testing.T) {
	root := notesFixture(t)
	m := New(root)

	m = pressKey(t, m, tea.KeyPressMsg{Code: 'a', Text: "a"})
	if !m.treeSearching || m.treeQuery != "a" {
		t.Fatalf("after first character searching=%v query=%q", m.treeSearching, m.treeQuery)
	}
	if m.statusLeft() != "find: a▏" || !strings.Contains(m.statusRight(), "matches · esc cancel") {
		t.Fatalf("search status = %q | %q", m.statusLeft(), m.statusRight())
	}

	m = pressKey(t, m, tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = pressKey(t, m, tea.KeyPressMsg{Code: 'p', Text: "p"})
	m = pressKey(t, m, tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = pressKey(t, m, tea.KeyPressMsg{Code: 'a', Text: "a"})
	if m.treeQuery != "alpha" || m.selected != filepath.Join(root, "alpha.md") {
		t.Fatalf("query=%q selected=%q, want alpha.md", m.treeQuery, m.selected)
	}
	origin := m.treeOrigin

	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.treeSearching || m.treeQuery != "" || m.tree.CursorIndex() != origin {
		t.Fatalf("escape left search=%v query=%q cursor=%d origin=%d",
			m.treeSearching, m.treeQuery, m.tree.CursorIndex(), origin)
	}
	if !strings.Contains(m.statusRight(), "% · f find") && !strings.Contains(m.statusRight(), "f find") {
		t.Fatalf("normal status hint not restored: %q", m.statusRight())
	}
}

func TestTreeSearchNoMatchBackspaceAndArrowExit(t *testing.T) {
	m := New(notesFixture(t))
	origin := m.tree.CursorIndex()

	m = pressKey(t, m, tea.KeyPressMsg{Code: 'a', Text: "a"})
	lastMatch := m.tree.CursorIndex()
	m = pressKey(t, m, tea.KeyPressMsg{Code: 'z', Text: "z"})
	if m.tree.CursorIndex() != lastMatch || m.statusRight() != "no matches · esc cancel" {
		t.Fatalf("no-match query moved from last match: cursor=%d last=%d status=%q",
			m.tree.CursorIndex(), lastMatch, m.statusRight())
	}
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	if m.treeQuery != "a" || m.treeMatches == 0 {
		t.Fatalf("backspace did not recover a match: query=%q matches=%d", m.treeQuery, m.treeMatches)
	}
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.tree.CursorIndex() != origin {
		t.Fatalf("escape cursor = %d, want origin %d", m.tree.CursorIndex(), origin)
	}

	m = pressKey(t, m, tea.KeyPressMsg{Code: 'z', Text: "z"})
	if !m.treeSearching || m.tree.CursorIndex() != origin || m.statusRight() != "no matches · esc cancel" {
		t.Fatalf("no-match state = searching:%v cursor:%d status:%q",
			m.treeSearching, m.tree.CursorIndex(), m.statusRight())
	}
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	if m.treeQuery != "" || m.statusRight() != "type to find · esc cancel" {
		t.Fatalf("empty query state = query:%q status:%q", m.treeQuery, m.statusRight())
	}
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
	if m.treeSearching || m.tree.CursorIndex() == origin {
		t.Fatalf("down did not end search and navigate: searching=%v cursor=%d",
			m.treeSearching, m.tree.CursorIndex())
	}
}

func TestTreeSearchEnterPerformsNormalTreeAction(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "readme.md"), []byte("# readme\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New(root)

	m = pressKey(t, m, tea.KeyPressMsg{Code: 'd', Text: "d"})
	m = pressKey(t, m, tea.KeyPressMsg{Code: 'o', Text: "o"})
	m = pressKey(t, m, tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = pressKey(t, m, tea.KeyPressMsg{Code: 's', Text: "s"})
	if !m.treeSearching || m.tree.CursorIndex() < 0 {
		t.Fatalf("tree search did not find docs: searching=%v cursor=%d", m.treeSearching, m.tree.CursorIndex())
	}
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.treeSearching {
		t.Fatal("enter did not end the tree search")
	}
	if !strings.Contains(m.tree.View(80, 10), "readme.md") {
		t.Fatal("enter did not perform the normal directory expansion")
	}
}

func TestTreeSearchReservedKeysAndControlC(t *testing.T) {
	m := New(notesFixture(t))
	m = pressKey(t, m, tea.KeyPressMsg{Code: 'j', Text: "j"})
	if m.treeSearching {
		t.Fatal("j should retain tree navigation when no session is active")
	}
	m = pressKey(t, m, tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = pressKey(t, m, tea.KeyPressMsg{Code: 'q', Text: "q"})
	if m.treeQuery != "aq" {
		t.Fatalf("reserved q did not become query text during active search: %q", m.treeQuery)
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+c did not quit")
	}
	updated, ok := next.(Model)
	if !ok || updated.treeSearching {
		t.Fatalf("ctrl+c did not clear search before quitting: %#v", next)
	}
}

func TestTreeSearchCancelsOnFocusOverlayMouseAndReload(t *testing.T) {
	root := notesFixture(t)
	m := New(root)
	m = pressKey(t, m, tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyTab})
	if m.treeSearching || m.focus != FocusViewer {
		t.Fatalf("tab did not end search and switch focus: searching=%v focus=%v", m.treeSearching, m.focus)
	}

	m.focus = FocusTree
	m = pressKey(t, m, tea.KeyPressMsg{Code: 'a', Text: "a"})
	help := m.openHelp()
	if help.treeSearching {
		t.Fatal("opening help retained the tree search")
	}
	m = help
	m.overlay = overlayNone

	m = pressKey(t, m, tea.KeyPressMsg{Code: 'a', Text: "a"})
	m.mouse = true
	next, _ := m.handleMouseWheel(tea.MouseWheelMsg{})
	wheeled := next.(Model)
	if wheeled.treeSearching {
		t.Fatal("mouse wheel retained the tree search")
	}

	m = pressKey(t, wheeled, tea.KeyPressMsg{Code: 'a', Text: "a"})
	next, _ = m.handleMouseClick(tea.MouseClickMsg{})
	clicked := next.(Model)
	if clicked.treeSearching {
		t.Fatal("mouse click retained the tree search")
	}

	m = pressKey(t, clicked, tea.KeyPressMsg{Code: 'a', Text: "a"})
	m.fsDirty = true
	m.fsPending = true
	m.fsSeq = 1
	m.fsPaths = []string{filepath.Join(root, "alpha.md")}
	next, _ = m.handleFsTick(fsTickMsg{Seq: 1})
	reloaded := next.(Model)
	if reloaded.treeSearching {
		t.Fatal("live reload retained the tree search")
	}
}
