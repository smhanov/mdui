package app

import (
	"unicode"

	tea "charm.land/bubbletea/v2"
)

func treeSearchReserved(key string) bool {
	switch key {
	case "q", "ctrl+c", "/", "?", "f", "T", ":", "m", "tab", "shift+tab",
		"H", "L", "=", "j", "k", "h", "l", "g", "G", "enter",
		"up", "down", "left", "right", "pgup", "pgdown", ".", "esc":
		return true
	default:
		return false
	}
}

func (m Model) canStartTreeSearch(msg tea.KeyPressMsg) bool {
	if m.focus != FocusTree || m.overlay != overlayNone || m.searchPrompt ||
		m.pendingLink != nil || msg.Text == "" ||
		msg.Mod.Contains(tea.ModCtrl) || msg.Mod.Contains(tea.ModAlt) ||
		treeSearchReserved(msg.String()) {
		return false
	}
	if m.viewer.SearchActive() && (msg.String() == "n" || msg.String() == "N") {
		return false
	}
	for _, r := range msg.Text {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

func (m Model) startTreeSearch(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.notice = ""
	m.treeSearching = true
	m.treeOrigin = m.tree.CursorIndex()
	m.treeQuery = msg.Text
	return m, m.applyTreeSearch()
}

func (m Model) handleTreeSearchKey(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	switch msg.String() {
	case "esc":
		return m, m.cancelTreeSearch(true), true
	case "ctrl+c":
		return m, m.cancelTreeSearch(false), false
	case "backspace":
		m.treeQuery = trimLastRune(m.treeQuery)
		return m, m.applyTreeSearch(), true
	}
	if !msg.Mod.Contains(tea.ModCtrl) && !msg.Mod.Contains(tea.ModAlt) && msg.Text != "" {
		printable := true
		for _, r := range msg.Text {
			if !unicode.IsPrint(r) {
				printable = false
				break
			}
		}
		if printable {
			m.treeQuery += msg.Text
			return m, m.applyTreeSearch(), true
		}
	}
	return m, m.cancelTreeSearch(false), false
}

func (m *Model) applyTreeSearch() tea.Cmd {
	best, count, ok := m.tree.Search(m.treeQuery)
	m.treeMatches = count
	m.treeMatchOK = ok
	if ok && best >= 0 && best != m.tree.CursorIndex() {
		return m.tree.SelectRow(best)
	}
	return nil
}

func (m *Model) cancelTreeSearch(restore bool) tea.Cmd {
	if !m.treeSearching {
		return nil
	}
	origin := m.treeOrigin
	m.treeSearching = false
	m.treeQuery = ""
	m.treeOrigin = 0
	m.treeMatches = 0
	m.treeMatchOK = false
	if restore {
		return m.tree.SelectRow(origin)
	}
	return nil
}
