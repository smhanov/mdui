package app

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/smhanov/mdui/internal/config"
	"github.com/smhanov/mdui/internal/tree"
)

func TestViewEnablesCellMotion(t *testing.T) {
	m := New("/tmp/notes")
	m.width, m.height = 100, 30

	if got := m.View().MouseMode; got != tea.MouseModeCellMotion {
		t.Fatalf("mouse mode = %v, want cell motion", got)
	}
}

func TestDividerRegions(t *testing.T) {
	m := sizedTestModel(t)
	treeW, _, _ := SplitWidths(m.innerWidth(), m.ratio)
	dividerX := m.frameLeft() + treeW

	tests := []struct {
		name string
		x    int
		want paneRegion
	}{
		{name: "tree edge of hit zone", x: dividerX - 1, want: regionDivider},
		{name: "divider", x: dividerX, want: regionDivider},
		{name: "viewer edge of hit zone", x: dividerX + 1, want: regionDivider},
		{name: "tree", x: dividerX - 2, want: regionTree},
		{name: "viewer", x: dividerX + 2, want: regionViewer},
		{name: "left border", x: m.frameLeft() - 1, want: regionOutside},
		{name: "right border", x: m.width - 1, want: regionOutside},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := m.regionAt(tt.x, 5); got != tt.want {
				t.Fatalf("regionAt(%d, 5) = %v, want %v", tt.x, got, tt.want)
			}
		})
	}

	if got := m.regionAt(dividerX, 0); got != regionOutside {
		t.Fatalf("title-bar row mapped to %v, want outside", got)
	}

	lines := strings.Split(m.render(), "\n")
	if len(lines) <= 5 {
		t.Fatalf("rendered shell has %d lines, want more than 5", len(lines))
	}
	plain := []rune(ansi.Strip(lines[5]))
	if dividerX >= len(plain) || plain[dividerX] != '│' {
		t.Fatalf("rendered divider column does not match hit-test column %d", dividerX)
	}
}

func TestMouseDragDebounceAndFinalRender(t *testing.T) {
	m := sizedTestModel(t)
	treeW, _, _ := SplitWidths(m.innerWidth(), m.ratio)
	dividerX := m.frameLeft() + treeW

	next, cmd := m.Update(tea.MouseClickMsg{X: dividerX, Y: 5, Button: tea.MouseLeft})
	m = next.(Model)
	if !m.dragging {
		t.Fatal("mouse down in the divider zone did not start a drag")
	}
	if cmd != nil {
		t.Fatalf("mouse down emitted %T, want no immediate work", cmd)
	}
	if out := m.render(); !strings.Contains(out, "┃") {
		t.Fatal("dragging divider is not highlighted")
	}

	// Twenty motions inside one debounce window must schedule exactly one
	// timer, not one render per event.
	scheduled := 0
	for i := range 20 {
		x := dividerX + 1 + i%10
		next, cmd = m.Update(tea.MouseMotionMsg{X: x, Y: 5, Button: tea.MouseLeft})
		m = next.(Model)
		if cmd != nil {
			scheduled++
		}
	}
	if scheduled != 1 {
		t.Fatalf("20 motions scheduled %d timers, want 1", scheduled)
	}
	if !m.dragTickPending {
		t.Fatal("debounce timer is not marked pending")
	}

	// Fire the injected tick: exactly one render, at the latest width.
	renderedBefore := m.lastTreeW
	next, cmd = m.Update(dragTickMsg{Seq: m.dragSeq})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("debounce tick did not request a render")
	}
	if m.dragTickPending {
		t.Fatal("debounce tick left the timer pending")
	}
	if m.lastTreeW == renderedBefore {
		t.Fatal("debounce tick did not update the rendered width")
	}

	// A further motion starts another window; releasing before its tick must
	// still produce exactly one final render at the release width.
	next, cmd = m.Update(tea.MouseMotionMsg{X: dividerX + 20, Y: 5, Button: tea.MouseLeft})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("motion after a tick did not schedule another timer")
	}
	finalWidth, _, _ := SplitWidths(m.innerWidth(), m.ratio)

	next, cmd = m.Update(tea.MouseReleaseMsg{X: dividerX + 20, Y: 5, Button: tea.MouseLeft})
	m = next.(Model)
	if m.dragging {
		t.Fatal("release did not end the drag")
	}
	if cmd == nil {
		t.Fatal("release did not request a final render")
	}
	if m.dragTickPending {
		t.Fatal("release left a debounce timer pending")
	}
	if m.lastTreeW != finalWidth {
		t.Fatalf("final render width = %d, want %d", m.lastTreeW, finalWidth)
	}
	if out := m.render(); strings.Contains(out, "┃") {
		t.Fatal("released divider is still highlighted")
	}

	state := config.Load(m.statePath)
	if math.Abs(state.TreeRatio-m.ratio) > 1e-9 {
		t.Fatalf("persisted ratio = %v, want %v", state.TreeRatio, m.ratio)
	}

	next, cmd = m.Update(tea.MouseReleaseMsg{X: dividerX + 20, Y: 5, Button: tea.MouseLeft})
	m = next.(Model)
	if cmd != nil {
		t.Fatalf("second release emitted %T, want nothing", cmd)
	}
}

func TestMouseDragClampsAtPaneMinimums(t *testing.T) {
	m := sizedTestModel(t)
	treeW, _, _ := SplitWidths(m.innerWidth(), m.ratio)
	dividerX := m.frameLeft() + treeW

	next, _ := m.Update(tea.MouseClickMsg{X: dividerX, Y: 5, Button: tea.MouseLeft})
	m = next.(Model)
	next, _ = m.Update(tea.MouseMotionMsg{X: -100, Y: 5, Button: tea.MouseLeft})
	m = next.(Model)
	next, _ = m.Update(tea.MouseReleaseMsg{X: -100, Y: 5, Button: tea.MouseLeft})
	m = next.(Model)

	gotTree, _, gotViewer := SplitWidths(m.innerWidth(), m.ratio)
	if gotTree != TreeMinW {
		t.Fatalf("left clamp tree width = %d, want %d", gotTree, TreeMinW)
	}
	if gotViewer < ViewerMinW {
		t.Fatalf("left clamp viewer width = %d, want >= %d", gotViewer, ViewerMinW)
	}

	treeW, _, _ = SplitWidths(m.innerWidth(), m.ratio)
	dividerX = m.frameLeft() + treeW
	next, _ = m.Update(tea.MouseClickMsg{X: dividerX, Y: 5, Button: tea.MouseLeft})
	m = next.(Model)
	next, _ = m.Update(tea.MouseMotionMsg{X: m.width + 100, Y: 5, Button: tea.MouseLeft})
	m = next.(Model)
	next, _ = m.Update(tea.MouseReleaseMsg{X: m.width + 100, Y: 5, Button: tea.MouseLeft})
	m = next.(Model)

	gotTree, _, gotViewer = SplitWidths(m.innerWidth(), m.ratio)
	if gotViewer != ViewerMinW {
		t.Fatalf("right clamp viewer width = %d, want %d", gotViewer, ViewerMinW)
	}
	if gotTree != m.innerWidth()-DividerW-ViewerMinW {
		t.Fatalf("right clamp tree width = %d, want %d", gotTree, m.innerWidth()-DividerW-ViewerMinW)
	}
}

func TestMouseClickOutsideDividerIsIgnored(t *testing.T) {
	m := sizedTestModel(t)
	treeW, _, _ := SplitWidths(m.innerWidth(), m.ratio)
	dividerX := m.frameLeft() + treeW

	for _, x := range []int{dividerX - 2, dividerX + 2, dividerX} {
		y := 5
		if x == dividerX {
			y = 0 // title bar is outside the pane body
		}
		next, cmd := m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
		m = next.(Model)
		if m.dragging {
			t.Fatalf("click at (%d, %d) started a drag", x, y)
		}
		if cmd != nil {
			t.Fatalf("click at (%d, %d) emitted %T", x, y, cmd)
		}
	}
}

func TestWheelOverTreeMovesCursor(t *testing.T) {
	root := wheelFixture(t)
	m, _ := newSizedModel(t, root)

	next, cmd := m.Update(tea.MouseWheelMsg{X: 5, Y: 5, Button: tea.MouseWheelDown})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("wheel down over the tree did not move the cursor")
	}
	msg := cmd()
	selection, ok := msg.(tree.SelectionMsg)
	if !ok {
		t.Fatalf("tree wheel emitted %T, want tree.SelectionMsg", msg)
	}
	if filepath.Base(selection.Path) != "c.md" {
		t.Fatalf("wheel down selected %q, want c.md", selection.Path)
	}

	next, _ = m.Update(selection)
	m = next.(Model)
	if filepath.Base(m.selected) != "c.md" {
		t.Fatalf("root selection = %q, want c.md", m.selected)
	}

	next, cmd = m.Update(tea.MouseWheelMsg{X: 5, Y: 5, Button: tea.MouseWheelDown})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("second wheel down over the tree did not move the cursor")
	}
	selection, ok = cmd().(tree.SelectionMsg)
	if !ok || filepath.Base(selection.Path) != "f.md" {
		t.Fatalf("second wheel down selected %v, want f.md", selection.Path)
	}

	next, cmd = m.Update(tea.MouseWheelMsg{X: 5, Y: 5, Button: tea.MouseWheelUp})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("wheel up over the tree did not move the cursor")
	}
	selection, ok = cmd().(tree.SelectionMsg)
	if !ok || filepath.Base(selection.Path) != "c.md" {
		t.Fatalf("wheel up selected %v, want c.md", selection.Path)
	}
}

func TestWheelOverViewerScrollsDocument(t *testing.T) {
	root := wheelFixture(t)
	long := filepath.Join(root, "long.md")
	m, _ := newSizedModel(t, root)

	cmd := m.viewer.Open(long)
	m = applyViewerCmd(t, m, cmd)
	if m.viewer.ScrollOffset() != 0 {
		t.Fatalf("viewer offset = %d, want 0", m.viewer.ScrollOffset())
	}

	next, cmd := m.Update(tea.MouseWheelMsg{X: m.width - 10, Y: 5, Button: tea.MouseWheelDown})
	m = next.(Model)
	if cmd != nil {
		m = applyViewerCmd(t, m, cmd)
	}
	if m.viewer.ScrollOffset() == 0 {
		t.Fatal("wheel down over the viewer did not scroll the document")
	}

	before := m.viewer.ScrollOffset()
	treeW, _, _ := SplitWidths(m.innerWidth(), m.ratio)
	dividerX := m.frameLeft() + treeW
	for _, button := range []tea.MouseButton{tea.MouseWheelUp, tea.MouseWheelDown} {
		next, cmd = m.Update(tea.MouseWheelMsg{X: dividerX, Y: 5, Button: button})
		m = next.(Model)
		if cmd != nil {
			t.Fatalf("wheel over divider emitted %T, want nothing", cmd)
		}
		if m.dragging {
			t.Fatal("wheel over divider started a drag")
		}
	}
	if got := m.viewer.ScrollOffset(); got != before {
		t.Fatalf("wheel over divider changed viewer offset from %d to %d", before, got)
	}
}

func TestKeyboardSplitNudgesAndReset(t *testing.T) {
	m := sizedTestModel(t)
	innerW := m.innerWidth()

	next, _ := m.Update(tea.KeyPressMsg{Code: 'L', Text: "L"})
	m = next.(Model)
	next, _ = m.Update(tea.KeyPressMsg{Code: 'L', Text: "L"})
	m = next.(Model)
	if got, _, _ := SplitWidths(innerW, m.ratio); got != 33 {
		t.Fatalf("L twice produced tree width %d, want 33", got)
	}

	next, _ = m.Update(tea.KeyPressMsg{Code: 'H', Text: "H"})
	m = next.(Model)
	next, _ = m.Update(tea.KeyPressMsg{Code: 'H', Text: "H"})
	m = next.(Model)
	if got, _, _ := SplitWidths(innerW, m.ratio); got != 29 {
		t.Fatalf("H twice returned tree width %d, want 29", got)
	}

	next, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = next.(Model)
	// Lowercase l belongs to the tree pane, not the splitter.
	if got, _, _ := SplitWidths(innerW, m.ratio); got != 29 {
		t.Fatalf("lowercase l changed tree width to %d", got)
	}

	next, _ = m.Update(tea.KeyPressMsg{Code: '=', Text: "="})
	m = next.(Model)
	if math.Abs(m.ratio-DefaultRatio) > 1e-9 {
		t.Fatalf("= set ratio to %v, want %v", m.ratio, DefaultRatio)
	}
	if got, _, _ := SplitWidths(innerW, m.ratio); got != 29 {
		t.Fatalf("= reset tree width to %d, want 29", got)
	}

	for range 100 {
		next, _ = m.Update(tea.KeyPressMsg{Code: 'L', Text: "L"})
		m = next.(Model)
	}
	treeW, _, viewerW := SplitWidths(innerW, m.ratio)
	if treeW != innerW-DividerW-ViewerMinW || viewerW != ViewerMinW {
		t.Fatalf("L clamp split = (%d, %d), want viewer minimum", treeW, viewerW)
	}

	for range 100 {
		next, _ = m.Update(tea.KeyPressMsg{Code: 'H', Text: "H"})
		m = next.(Model)
	}
	treeW, _, _ = SplitWidths(innerW, m.ratio)
	if treeW != TreeMinW {
		t.Fatalf("H clamp tree width = %d, want %d", treeW, TreeMinW)
	}
}

func TestQuitPersistsSplitRatio(t *testing.T) {
	m := sizedTestModel(t)
	next, _ := m.Update(tea.KeyPressMsg{Code: 'L', Text: "L"})
	m = next.(Model)
	next, _ = m.Update(tea.KeyPressMsg{Code: 'L', Text: "L"})
	m = next.(Model)

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("q did not return the quit command")
	}

	state := config.Load(m.statePath)
	if math.Abs(state.TreeRatio-m.ratio) > 1e-9 {
		t.Fatalf("state on quit = %v, want %v", state.TreeRatio, m.ratio)
	}
}

func TestTreeRatioRestoredAndClampedAtStartup(t *testing.T) {
	root := notesFixture(t)
	path := filepath.Join(t.TempDir(), "state.toml")
	if err := config.Save(path, config.State{TreeRatio: 0.5}); err != nil {
		t.Fatal(err)
	}
	state := config.Load(path)

	m := New(root, WithTreeRatio(state.TreeRatio), WithStatePath(path))
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(Model)
	if math.Abs(m.ratio-0.5) > 1e-9 {
		t.Fatalf("restored ratio = %v, want 0.5", m.ratio)
	}
	if got, _, _ := SplitWidths(m.innerWidth(), m.ratio); got != 49 {
		t.Fatalf("restored tree width = %d, want 49", got)
	}

	wide := New(root, WithTreeRatio(1), WithStatePath(path))
	next, _ = wide.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	wide = next.(Model)
	if want := ClampRatio(98, 1); math.Abs(wide.ratio-want) > 1e-9 {
		t.Fatalf("startup clamp ratio = %v, want %v", wide.ratio, want)
	}
	if _, _, viewerW := SplitWidths(wide.innerWidth(), wide.ratio); viewerW != ViewerMinW {
		t.Fatalf("startup clamp viewer width = %d, want %d", viewerW, ViewerMinW)
	}
}

func sizedTestModel(t *testing.T, opts ...Option) Model {
	t.Helper()
	m, _ := newSizedModel(t, notesFixture(t), opts...)
	return m
}

func newSizedModel(t *testing.T, root string, opts ...Option) (Model, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.toml")
	opts = append(opts, WithStatePath(path))
	m := New(root, opts...)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return next.(Model), path
}

func wheelFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"a.md", "b.md", "c.md", "d.md", "e.md", "f.md", "g.md", "h.md"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("# "+name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var long strings.Builder
	for i := range 100 {
		fmt.Fprintf(&long, "line %d of the long document\n\n", i)
	}
	if err := os.WriteFile(filepath.Join(root, "long.md"), []byte(long.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func applyViewerCmd(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	msg := cmd()
	if msg == nil {
		return m
	}
	var next tea.Cmd
	m.viewer, next = m.viewer.Update(msg)
	if next != nil {
		return applyViewerCmd(t, m, next)
	}
	return m
}
