package app

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// startSearch opens the status-bar prompt for the selected document. Starting
// a new search discards any previous query and highlights.
func (m Model) startSearch() (tea.Model, tea.Cmd) {
	if m.selected == "" {
		m.notice = "no document selected"
		return m, nil
	}
	m.notice = ""
	m.searchPrompt = true
	m.focus = FocusViewer
	m.viewer.BeginSearch()
	return m, nil
}

// handleSearchPromptKey captures every key while the prompt is open. Printable
// text edits the query, so q, n, N, /, ? and spaces are literal input.
func (m Model) handleSearchPromptKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.searchPrompt = false
		m.viewer.CancelSearch()
		m.notice = ""
		return m, nil
	case "enter":
		m.searchPrompt = false
		_, cmd := m.viewer.CommitSearch()
		return m, cmd
	case "backspace", "ctrl+h":
		m.viewer.SetSearchQuery(trimLastRune(m.viewer.SearchQuery()))
		return m, nil
	case "ctrl+u":
		m.viewer.SetSearchQuery("")
		return m, nil
	}

	if msg.Mod.Contains(tea.ModCtrl) || msg.Mod.Contains(tea.ModAlt) {
		return m, nil
	}
	if msg.Text != "" {
		m.viewer.SetSearchQuery(m.viewer.SearchQuery() + msg.Text)
	}
	return m, nil
}

func trimLastRune(s string) string {
	runes := []rune(s)
	if len(runes) == 0 {
		return ""
	}
	return string(runes[:len(runes)-1])
}

// jumpSearchMatch is the n/N handler. A committed search with no matches
// leaves the status bar's no-matches feedback visible.
func (m Model) jumpSearchMatch(key string) (tea.Model, tea.Cmd) {
	m.notice = ""
	var (
		ok  bool
		cmd tea.Cmd
	)
	if key == "N" {
		ok, cmd = m.viewer.PrevMatch()
	} else {
		ok, cmd = m.viewer.NextMatch()
	}
	if !ok {
		return m, nil
	}
	m.focus = FocusViewer
	return m, cmd
}

func (m Model) searchVisible() bool {
	return m.searchPrompt || m.viewer.SearchActive()
}

func (m Model) searchPromptStatus() string {
	query := m.viewer.SearchQuery()
	total := m.viewer.SearchTotal()
	switch {
	case query == "":
		return "type to search · esc cancels"
	case total == 0:
		return "no matches · esc cancels"
	default:
		return fmt.Sprintf("%d %s · enter jumps · esc cancels", total, plural(total, "match", "matches"))
	}
}

func (m Model) searchCommittedStatus() string {
	current, total := m.viewer.SearchStatus()
	if total == 0 {
		return "no matches · esc clears"
	}
	if current < 0 {
		current = 0
	}
	return fmt.Sprintf("%d/%d · n/N · esc clears", current+1, total)
}

func (m Model) searchLabel() string {
	query := strings.TrimSpace(m.viewer.SearchQuery())
	if query == "" {
		return ""
	}
	return " · /" + query
}
