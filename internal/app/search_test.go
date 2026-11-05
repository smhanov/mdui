package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestSearchPromptCapturesQAndCommits(t *testing.T) {
	m := interactionModel(t)
	m = clickRow(t, m, 2) // alpha.md

	m = pressKey(t, m, tea.KeyPressMsg{Code: '/', Text: "/"})
	if !m.searchPrompt {
		t.Fatal("/ did not open the search prompt")
	}
	if m.focus != FocusViewer {
		t.Fatal("search prompt did not focus the viewer")
	}

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	m = next.(Model)
	if cmd != nil {
		t.Fatal("q quit while the search prompt was open")
	}
	if !m.searchPrompt {
		t.Fatal("q dismissed the search prompt")
	}
	if got := m.viewer.SearchQuery(); got != "q" {
		t.Fatalf("query = %q, want q", got)
	}
	if status := ansi.Strip(m.renderStatus(120)); !strings.Contains(status, "/q") {
		t.Fatalf("search prompt is not visible in the status bar:\n%s", status)
	}

	m = typeText(t, m, "?/ ")
	if got := m.viewer.SearchQuery(); got != "q?/ " {
		t.Fatalf("prompt did not capture ?/ and space: query = %q", got)
	}
	if m.overlay != overlayNone {
		t.Fatal("? opened the help overlay while the search prompt was open")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m = next.(Model)
	if got := m.viewer.SearchQuery(); got != "" {
		t.Fatalf("ctrl+u left query %q", got)
	}
	m = typeText(t, m, "ab")
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = next.(Model)
	if got := m.viewer.SearchQuery(); got != "a" {
		t.Fatalf("backspace left query %q, want a", got)
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m = next.(Model)

	m = typeText(t, m, "alpha")
	if status := ansi.Strip(m.renderStatus(120)); !strings.Contains(status, "match") {
		t.Fatalf("incremental match count missing:\n%s", status)
	}
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.searchPrompt {
		t.Fatal("enter did not close the search prompt")
	}
	if !m.viewer.SearchActive() {
		t.Fatal("enter did not commit the search")
	}
	if total := m.viewer.SearchTotal(); total == 0 {
		t.Fatal("alpha search found no matches")
	}
	if !strings.Contains(m.viewer.Content(), "\x1b[7m") {
		t.Fatalf("committed search is not highlighted: %q", m.viewer.Content())
	}
	status := ansi.Strip(m.renderStatus(160))
	if !strings.Contains(status, "/alpha") {
		t.Fatalf("committed query missing from status:\n%s", status)
	}
	if !strings.Contains(status, "1/") || !strings.Contains(status, "esc clears") {
		t.Fatalf("committed search count missing from status:\n%s", status)
	}
}

func TestSearchEscCancelsAndClearsHighlights(t *testing.T) {
	m := interactionModel(t)
	m = clickRow(t, m, 2)

	m = pressKey(t, m, tea.KeyPressMsg{Code: '/', Text: "/"})
	m = typeText(t, m, "alpha")
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.viewer.SearchActive() {
		t.Fatal("search did not commit")
	}

	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.viewer.SearchActive() || m.searchPrompt {
		t.Fatalf("esc left search state: active=%v prompt=%v", m.viewer.SearchActive(), m.searchPrompt)
	}
	if strings.Contains(m.viewer.Content(), "\x1b[7m") {
		t.Fatalf("esc left highlights: %q", m.viewer.Content())
	}
	if status := ansi.Strip(m.renderStatus(160)); strings.Contains(status, "/alpha") {
		t.Fatalf("esc left the query in the status bar:\n%s", status)
	}
}

func TestSearchPromptEscAndCtrlCCancel(t *testing.T) {
	m := interactionModel(t)
	m = clickRow(t, m, 2)

	m = pressKey(t, m, tea.KeyPressMsg{Code: '/', Text: "/"})
	m = typeText(t, m, "alpha")
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(Model)
	if cmd != nil {
		t.Fatal("esc from the prompt quit the app")
	}
	if m.searchPrompt || m.viewer.SearchActive() || m.viewer.SearchQuery() != "" {
		t.Fatalf("esc left search state: prompt=%v active=%v query=%q",
			m.searchPrompt, m.viewer.SearchActive(), m.viewer.SearchQuery())
	}
	if strings.Contains(m.viewer.Content(), "\x1b[7m") {
		t.Fatalf("esc from the prompt left highlights: %q", m.viewer.Content())
	}

	m = pressKey(t, m, tea.KeyPressMsg{Code: '/', Text: "/"})
	next, cmd = m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m = next.(Model)
	if cmd != nil {
		t.Fatal("ctrl+c from the prompt quit the app")
	}
	if m.searchPrompt {
		t.Fatal("ctrl+c did not cancel the prompt")
	}
}

func TestSearchNoMatchFeedback(t *testing.T) {
	m := interactionModel(t)
	m = clickRow(t, m, 2)

	m = pressKey(t, m, tea.KeyPressMsg{Code: '/', Text: "/"})
	m = typeText(t, m, "zzzz-not-here")
	if status := ansi.Strip(m.renderStatus(160)); !strings.Contains(status, "no matches") {
		t.Fatalf("typing a no-match query lacks feedback:\n%s", status)
	}
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.viewer.SearchActive() {
		t.Fatal("no-match search should stay active for esc")
	}
	if status := ansi.Strip(m.renderStatus(160)); !strings.Contains(status, "no matches · esc clears") {
		t.Fatalf("committed no-match feedback missing:\n%s", status)
	}
	if strings.Contains(m.viewer.Content(), "\x1b[7m") {
		t.Fatalf("no-match search left highlights: %q", m.viewer.Content())
	}

	m = pressKey(t, m, tea.KeyPressMsg{Code: 'n', Text: "n"})
	if status := ansi.Strip(m.renderStatus(160)); !strings.Contains(status, "no matches") {
		t.Fatalf("n on a no-match search lost feedback:\n%s", status)
	}
}

func TestSearchNextPreviousWrapAndStatus(t *testing.T) {
	root := searchFixture(t)
	m := New(root, WithStatePath(filepath.Join(t.TempDir(), "state.toml")))
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m = next.(Model)
	m = clickRow(t, m, 1) // needle.md

	m = pressKey(t, m, tea.KeyPressMsg{Code: '/', Text: "/"})
	m = typeText(t, m, "needle")
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := m.viewer.SearchTotal(); got != 3 {
		t.Fatalf("total matches = %d, want 3", got)
	}
	if status := ansi.Strip(m.renderStatus(160)); !strings.Contains(status, "1/3") {
		t.Fatalf("initial current/total missing:\n%s", status)
	}

	m = pressKey(t, m, tea.KeyPressMsg{Code: 'n', Text: "n"})
	if status := ansi.Strip(m.renderStatus(160)); !strings.Contains(status, "2/3") {
		t.Fatalf("n did not advance:\n%s", status)
	}
	m = pressKey(t, m, tea.KeyPressMsg{Code: 'n', Text: "n"})
	if status := ansi.Strip(m.renderStatus(160)); !strings.Contains(status, "3/3") {
		t.Fatalf("second n did not advance:\n%s", status)
	}
	m = pressKey(t, m, tea.KeyPressMsg{Code: 'n', Text: "n"})
	if status := ansi.Strip(m.renderStatus(160)); !strings.Contains(status, "1/3") {
		t.Fatalf("n did not wrap:\n%s", status)
	}
	m = pressKey(t, m, tea.KeyPressMsg{Code: 'N', Text: "N"})
	if status := ansi.Strip(m.renderStatus(160)); !strings.Contains(status, "3/3") {
		t.Fatalf("N did not wrap backwards:\n%s", status)
	}
}

func TestSearchRawToggleReappliesQuery(t *testing.T) {
	m := interactionModel(t)
	m = clickRow(t, m, 2) // alpha.md

	m = pressKey(t, m, tea.KeyPressMsg{Code: '/', Text: "/"})
	m = typeText(t, m, "alpha")
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m = pressKey(t, m, tea.KeyPressMsg{Code: 'r', Text: "r"})

	if !m.viewer.RawMode() {
		t.Fatal("r did not switch to raw mode")
	}
	if !m.viewer.SearchActive() || m.viewer.SearchQuery() != "alpha" {
		t.Fatalf("raw toggle lost search: active=%v query=%q", m.viewer.SearchActive(), m.viewer.SearchQuery())
	}
	if !strings.Contains(m.viewer.Content(), "\x1b[7m") {
		t.Fatalf("raw projection was not highlighted: %q", m.viewer.Content())
	}
	if !strings.Contains(ansi.Strip(m.viewer.Content()), "# alpha.md") {
		t.Fatalf("raw projection missing source: %q", ansi.Strip(m.viewer.Content()))
	}
	if status := ansi.Strip(m.renderStatus(160)); !strings.Contains(status, "1/") {
		t.Fatalf("raw mode lost search status:\n%s", status)
	}

	m = pressKey(t, m, tea.KeyPressMsg{Code: 'r', Text: "r"})
	if m.viewer.RawMode() {
		t.Fatal("second r did not restore rendered mode")
	}
	if !m.viewer.SearchActive() || !strings.Contains(m.viewer.Content(), "\x1b[7m") {
		t.Fatalf("rendered projection was not re-highlighted: %q", m.viewer.Content())
	}
}

func TestSearchClearsOnNewFileSelection(t *testing.T) {
	m := interactionModel(t)
	m = clickRow(t, m, 2) // alpha.md

	m = pressKey(t, m, tea.KeyPressMsg{Code: '/', Text: "/"})
	m = typeText(t, m, "alpha")
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.viewer.SearchActive() {
		t.Fatal("search did not commit")
	}

	m = clickRow(t, m, 3) // beta.md
	if m.searchPrompt || m.viewer.SearchActive() || m.viewer.SearchQuery() != "" {
		t.Fatalf("new file kept search: prompt=%v active=%v query=%q",
			m.searchPrompt, m.viewer.SearchActive(), m.viewer.SearchQuery())
	}
	if strings.Contains(m.viewer.Content(), "\x1b[7m") {
		t.Fatalf("new file kept highlights: %q", m.viewer.Content())
	}
	if status := ansi.Strip(m.renderStatus(160)); strings.Contains(status, "/alpha") {
		t.Fatalf("new file kept the query in the status bar:\n%s", status)
	}
}

func TestSearchRehighlightsAfterWidthChange(t *testing.T) {
	m := interactionModel(t)
	m = clickRow(t, m, 2)

	m = pressKey(t, m, tea.KeyPressMsg{Code: '/', Text: "/"})
	m = typeText(t, m, "alpha")
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	before := m.viewer.SearchTotal()
	if before == 0 {
		t.Fatal("initial search found no matches")
	}

	next, cmd := m.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	m = next.(Model)
	m = applyCmd(t, m, cmd)

	if !m.viewer.SearchActive() || m.viewer.SearchTotal() == 0 {
		t.Fatalf("width change lost search: active=%v total=%d", m.viewer.SearchActive(), m.viewer.SearchTotal())
	}
	if !strings.Contains(m.viewer.Content(), "\x1b[7m") {
		t.Fatalf("width change did not re-highlight: %q", m.viewer.Content())
	}
}

func TestSearchStatusSurvivesNarrowBar(t *testing.T) {
	m := interactionModel(t)
	m = clickRow(t, m, 2)

	m = pressKey(t, m, tea.KeyPressMsg{Code: '/', Text: "/"})
	m = typeText(t, m, "alpha")
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})

	status := ansi.Strip(m.renderStatus(60))
	if !strings.Contains(status, "1/") || !strings.Contains(status, "esc clears") {
		t.Fatalf("narrow status bar dropped search feedback:\n%s", status)
	}
}

func TestSearchPromptIgnoresMouse(t *testing.T) {
	m := interactionModel(t)
	m = clickRow(t, m, 2)
	selected := m.selected

	m = pressKey(t, m, tea.KeyPressMsg{Code: '/', Text: "/"})
	next, cmd := m.Update(tea.MouseClickMsg{X: m.frameLeft() + 3, Y: m.bodyTop() + 3, Button: tea.MouseLeft})
	m = next.(Model)
	if cmd != nil {
		t.Fatal("mouse click while searching produced a command")
	}
	if !m.searchPrompt {
		t.Fatal("mouse click dismissed the search prompt")
	}
	if m.selected != selected {
		t.Fatalf("mouse click changed selection from %q to %q", selected, m.selected)
	}
}

func searchFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	var doc strings.Builder
	doc.WriteString("# start document\n\n")
	for i := 0; i < 3; i++ {
		doc.WriteString(fmt.Sprintf("needle paragraph %d\n\n", i))
		for j := 0; j < 12; j++ {
			doc.WriteString(fmt.Sprintf("filler line %d of paragraph %d\n\n", j, i))
		}
	}
	if err := os.WriteFile(filepath.Join(root, "needle.md"), []byte(doc.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func typeText(t *testing.T, m Model, text string) Model {
	t.Helper()
	for _, r := range text {
		m = pressKey(t, m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}
