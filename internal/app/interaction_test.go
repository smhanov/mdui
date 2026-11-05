package app

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"mdui/internal/config"
)

func TestHelpOverlayOpensAndDismisses(t *testing.T) {
	m := interactionModel(t)
	m = pressKey(t, m, tea.KeyPressMsg{Code: '?', Text: "?"})
	if m.overlay != overlayHelp {
		t.Fatal("? did not open the help overlay")
	}
	out := m.render()
	if !strings.Contains(out, "\x1b[2m") {
		t.Fatal("help overlay does not dim the backdrop")
	}
	plain := ansi.Strip(out)
	for _, want := range []string{"mdui keys", "double-click", "to close this help", "mdui — ", "search document", "next / prev", "find file"} {
		if !strings.Contains(plain, want) {
			t.Errorf("help overlay missing %q:\n%s", want, plain)
		}
	}

	m = pressKey(t, m, tea.KeyPressMsg{Code: 'j', Text: "j"})
	if m.overlay != overlayHelp {
		t.Fatal("an unrelated key dismissed the help overlay")
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	m = next.(Model)
	if cmd != nil {
		t.Fatal("q quit while the help overlay was open")
	}
	if m.overlay != overlayHelp {
		t.Fatal("q dismissed the help overlay")
	}

	m = pressKey(t, m, tea.KeyPressMsg{Code: '?', Text: "?"})
	if m.overlay != overlayNone {
		t.Fatal("? did not close the help overlay")
	}
	m = pressKey(t, m, tea.KeyPressMsg{Code: '?', Text: "?"})
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.overlay != overlayNone {
		t.Fatal("esc did not close the help overlay")
	}
}

func TestHelpOverlayAt80Columns(t *testing.T) {
	m := interactionModel(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(Model)
	m = pressKey(t, m, tea.KeyPressMsg{Code: '?', Text: "?"})

	out := m.render()
	if got := maxLineWidth(out); got != 80 {
		t.Fatalf("help render width = %d, want 80", got)
	}
	if lines := strings.Count(out, "\n") + 1; lines != 24 {
		t.Fatalf("help render height = %d, want 24", lines)
	}
	if !strings.Contains(ansi.Strip(out), "mdui keys") {
		t.Fatalf("help content missing at 80 columns:\n%s", ansi.Strip(out))
	}
}

func TestClickFileRowSelectsAndPreviews(t *testing.T) {
	m := interactionModel(t)
	alpha := filepath.Join(m.root, "alpha.md")

	m = clickRow(t, m, 2)
	if m.selected != alpha {
		t.Fatalf("selected = %q, want %q", m.selected, alpha)
	}
	if !strings.Contains(m.viewer.Content(), "alpha.md") {
		t.Fatalf("viewer did not preview the clicked file:\n%s", m.viewer.Content())
	}

	status := ansi.Strip(m.renderStatus(200))
	for _, want := range []string{"alpha.md", "4 words", "1 min", "100%"} {
		if !strings.Contains(status, want) {
			t.Errorf("status bar missing %q:\n%s", want, status)
		}
	}
	if !strings.Contains(status, "? help · q quit") {
		t.Errorf("status bar hint missing:\n%s", status)
	}
}

func TestClickDirectoryToggles(t *testing.T) {
	clock := time.Unix(1000, 0)
	m := interactionModel(t, WithClock(func() time.Time { return clock }))

	m = clickRow(t, m, 1)
	if !strings.Contains(ansi.Strip(m.render()), "nested.md") {
		t.Fatal("clicking a directory did not expand it")
	}

	clock = clock.Add(time.Second)
	m = clickRow(t, m, 1)
	if strings.Contains(ansi.Strip(m.render()), "nested.md") {
		t.Fatal("second slow click did not collapse the directory")
	}
}

func TestDoubleClickDirectoryDescends(t *testing.T) {
	clock := time.Unix(1000, 0)
	m := interactionModel(t, WithClock(func() time.Time { return clock }))
	nested := filepath.Join(m.root, "projects", "nested.md")

	m = clickRow(t, m, 1) // expand
	clock = clock.Add(200 * time.Millisecond)
	m = clickRow(t, m, 1) // double-click: descend

	if m.selected != nested {
		t.Fatalf("double-click selected %q, want %q", m.selected, nested)
	}
	if !strings.Contains(m.viewer.Content(), "nested") {
		t.Fatalf("viewer did not preview the first child:\n%s", m.viewer.Content())
	}
}

func TestSlowSecondClickIsNotADoubleClick(t *testing.T) {
	clock := time.Unix(1000, 0)
	m := interactionModel(t, WithClock(func() time.Time { return clock }))

	m = clickRow(t, m, 1)
	clock = clock.Add(doubleClickWindow + time.Millisecond)
	m = clickRow(t, m, 1)

	if m.selected != "" {
		t.Fatalf("slow second click selected %q, want no file", m.selected)
	}
	if strings.Contains(ansi.Strip(m.render()), "nested.md") {
		t.Fatal("slow second click descended instead of collapsing")
	}
}

func TestClickOutsidePanesIsHarmless(t *testing.T) {
	m := interactionModel(t)
	m = clickAt(t, m, 10, 0)  // title bar
	m = clickAt(t, m, 10, 29) // status bar
	m = clickAt(t, m, 0, 10)  // frame border
	if m.selected != "" || m.dragging {
		t.Fatalf("click outside panes selected %q or started a drag", m.selected)
	}
}

func TestShiftClickAndMouseOffPassThrough(t *testing.T) {
	m := interactionModel(t)
	treeW, _, _ := SplitWidths(m.innerWidth(), m.ratio)
	dividerX := m.frameLeft() + treeW

	next, cmd := m.Update(tea.MouseClickMsg{X: dividerX, Y: 5, Button: tea.MouseLeft, Mod: tea.ModShift})
	m = next.(Model)
	if cmd != nil || m.dragging {
		t.Fatal("shift-click on the divider started a drag")
	}

	next, cmd = m.Update(tea.MouseClickMsg{X: m.frameLeft() + 3, Y: m.bodyTop() + 2, Button: tea.MouseLeft, Mod: tea.ModShift})
	m = next.(Model)
	if cmd != nil || m.selected != "" {
		t.Fatal("shift-click on a tree row selected a file")
	}

	m = pressKey(t, m, tea.KeyPressMsg{Code: 'm', Text: "m"})
	next, cmd = m.Update(tea.MouseClickMsg{X: m.frameLeft() + 3, Y: m.bodyTop() + 2, Button: tea.MouseLeft})
	m = next.(Model)
	if cmd != nil || m.selected != "" {
		t.Fatal("click selected a file while mouse capture was off")
	}
}

func TestMouseToggleHintAndMode(t *testing.T) {
	m := interactionModel(t)
	if got := m.View().MouseMode; got != tea.MouseModeCellMotion {
		t.Fatalf("initial mouse mode = %v", got)
	}
	beforeHeight := m.paneHeight()

	m = pressKey(t, m, tea.KeyPressMsg{Code: 'm', Text: "m"})
	if got := m.View().MouseMode; got != tea.MouseModeNone {
		t.Fatalf("mouse-off mode = %v, want none", got)
	}
	if m.paneHeight() != beforeHeight-1 {
		t.Fatalf("pane height = %d, want %d", m.paneHeight(), beforeHeight-1)
	}
	if !strings.Contains(ansi.Strip(m.render()), "mouse off — native selection enabled, press m to re-enable") {
		t.Fatal("mouse-off hint bar missing")
	}
	if got := len(strings.Split(m.render(), "\n")); got != 30 {
		t.Fatalf("mouse-off render height = %d, want 30", got)
	}

	m = pressKey(t, m, tea.KeyPressMsg{Code: 'm', Text: "m"})
	if got := m.View().MouseMode; got != tea.MouseModeCellMotion {
		t.Fatalf("restored mouse mode = %v", got)
	}
	if strings.Contains(ansi.Strip(m.render()), "native selection enabled") {
		t.Fatal("mouse-off hint bar remained visible")
	}
	if m.paneHeight() != beforeHeight {
		t.Fatalf("pane height after restore = %d, want %d", m.paneHeight(), beforeHeight)
	}
}

func TestDeletedOpenFileWarnsAndKeepsContent(t *testing.T) {
	m := interactionModel(t)
	alpha := filepath.Join(m.root, "alpha.md")
	beta := filepath.Join(m.root, "beta.md")

	m = clickRow(t, m, 2)
	last := m.viewer.Content()
	if !strings.Contains(last, "alpha.md") {
		t.Fatalf("initial render missing: %q", last)
	}

	if err := os.Remove(alpha); err != nil {
		t.Fatal(err)
	}
	next, cmd := m.Update(tea.WindowSizeMsg{Width: 110, Height: 30})
	m = next.(Model)
	m = applyCmd(t, m, cmd)

	if m.viewer.Warning() != "file no longer available" {
		t.Fatalf("warning = %q", m.viewer.Warning())
	}
	if m.viewer.Content() != last {
		t.Fatalf("last render was replaced after deletion:\n%q", m.viewer.Content())
	}
	status := ansi.Strip(m.renderStatus(200))
	if !strings.Contains(status, "no longer available") || !strings.Contains(status, "alpha.md") {
		t.Fatalf("status bar warning missing:\n%s", status)
	}

	m = clickRow(t, m, 3)
	if m.selected != beta {
		t.Fatalf("selected after recovery = %q, want %q", m.selected, beta)
	}
	if m.viewer.Warning() != "" {
		t.Fatalf("warning did not clear on the next selection: %q", m.viewer.Warning())
	}
	if !strings.Contains(m.viewer.Content(), "beta.md") {
		t.Fatalf("next file did not render:\n%s", m.viewer.Content())
	}
}

func TestStatusScrollPercentFollowsViewer(t *testing.T) {
	root := t.TempDir()
	var long strings.Builder
	long.WriteString("# long\n\n")
	for i := range 120 {
		long.WriteString("line ")
		long.WriteString(strconv.Itoa(i))
		long.WriteString(" of the document\n\n")
	}
	if err := os.WriteFile(filepath.Join(root, "long.md"), []byte(long.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	m := New(root, WithStatePath(filepath.Join(t.TempDir(), "state.toml")))
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m = next.(Model)
	m = clickRow(t, m, 1)
	if !m.stats.Ready {
		t.Fatal("stats were not delivered after opening the file")
	}
	if got := m.stats.ScrollPct; got != 0 {
		t.Fatalf("initial scroll = %d%%, want 0%%", got)
	}

	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyTab})
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	m = next.(Model)
	m = applyCmd(t, m, cmd)
	if got := m.stats.ScrollPct; got != 100 {
		t.Fatalf("bottom scroll = %d%%, want 100%%", got)
	}
	if status := ansi.Strip(m.renderStatus(200)); !strings.Contains(status, "100%") {
		t.Fatalf("status bar did not follow scroll:\n%s", status)
	}
}

func TestConfigOptionsReachTheUI(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "visible.md"), []byte("# visible\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".hidden.md"), []byte("# hidden\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := New(root, WithStatePath(filepath.Join(t.TempDir(), "state.toml")),
		WithConfig(config.Config{
			Mouse:       false,
			Emoji:       true,
			HiddenFiles: true,
			SortBy:      "name",
		}))
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(Model)

	if m.View().MouseMode != tea.MouseModeNone {
		t.Fatal("mouse config was not applied")
	}
	if !strings.Contains(ansi.Strip(m.render()), ".hidden.md") {
		t.Fatal("hidden_files config was not applied")
	}
}

func interactionModel(t *testing.T, opts ...Option) Model {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "projects"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "projects", "nested.md"), []byte("# nested\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "alpha.md"), []byte("# alpha.md\n\nhello world\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "beta.md"), []byte("# beta.md\n\nsecond file\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	opts = append(opts, WithStatePath(filepath.Join(t.TempDir(), "state.toml")))
	m := New(root, opts...)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return next.(Model)
}

// clickRow clicks the tree row at index (0 is the root row), then applies any
// command the click produced.
func maxLineWidth(s string) int {
	width := 0
	for _, line := range strings.Split(s, "\n") {
		width = max(width, ansi.StringWidth(line))
	}
	return width
}

func clickRow(t *testing.T, m Model, index int) Model {
	t.Helper()
	start := m.tree.VisibleStart(m.paneHeight())
	y := m.bodyTop() + index - start
	return clickAt(t, m, m.frameLeft()+3, y)
}

func clickAt(t *testing.T, m Model, x, y int) Model {
	t.Helper()
	next, cmd := m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	m = next.(Model)
	return applyCmd(t, m, cmd)
}

// applyCmd runs a command and feeds its message back through Update,
// unwrapping batched commands. It mirrors the Bubble Tea event loop closely
// enough for assertions on rendered content and status-bar stats.
func applyCmd(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	switch msg := cmd().(type) {
	case nil:
		return m
	case tea.BatchMsg:
		for _, batched := range msg {
			m = applyCmd(t, m, batched)
		}
		return m
	default:
		next, nextCmd := m.Update(msg)
		updated, ok := next.(Model)
		if !ok {
			t.Fatalf("Update(%T) returned %T", msg, next)
		}
		return applyCmd(t, updated, nextCmd)
	}
}
