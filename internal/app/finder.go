package app

import (
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/smhanov/mdui/internal/finder"
	"github.com/smhanov/mdui/internal/viewer"
)

// finderState is the index lifecycle while the overlay is open.
type finderState int

const (
	finderIdle finderState = iota
	finderLoading
	finderReady
)

// finderIndexMsg is the asynchronous index result. Seq guards against stale
// builds after a hidden-file toggle or a replacement build.
type finderIndexMsg struct {
	Seq   uint64
	Index finder.Index
	Err   error
}

func finderIndexCmd(seq uint64, root string, showHidden bool, build finder.Builder) tea.Cmd {
	if build == nil {
		build = finder.Build
	}
	return func() tea.Msg {
		index, err := build(root, showHidden)
		return finderIndexMsg{Seq: seq, Index: index, Err: err}
	}
}

// openFinder resets the query, reuses a valid cached index, joins an in-flight
// build for the same hidden setting, or starts one in the background.
func (m Model) openFinder() (tea.Model, tea.Cmd) {
	hidden := m.tree.Hidden()
	m.notice = ""
	m.overlay = overlayFinder
	m.finderQuery = ""
	m.finderCursor = 0
	m.finderSel = 0
	m.finderOffset = 0
	m.finderResults = nil

	if m.finderIndex != nil && m.finderIndex.Hidden == hidden {
		m.finderState = finderReady
		m.refreshFinderResults()
		return m, nil
	}
	if m.finderBuilding && m.finderBuildHidden == hidden {
		m.finderState = finderLoading
		return m, nil
	}

	m.finderIndex = nil
	m.finderWarn = ""
	m.finderState = finderLoading
	m.finderBuilding = true
	m.finderBuildHidden = hidden
	m.finderSeq++
	return m, finderIndexCmd(m.finderSeq, m.root, hidden, m.finderBuild)
}

func (m Model) handleFinderIndex(msg finderIndexMsg) (tea.Model, tea.Cmd) {
	if msg.Seq != m.finderSeq {
		return m, nil
	}
	m.finderBuilding = false
	index := msg.Index
	m.finderIndex = &index
	if msg.Err != nil {
		m.finderWarn = msg.Err.Error()
		if m.overlay != overlayFinder {
			m.notice = "finder: " + msg.Err.Error()
		}
	} else {
		m.finderWarn = ""
	}
	if m.overlay == overlayFinder {
		m.finderState = finderReady
		m.refreshFinderResults()
	}
	return m, nil
}

// handleFinderKey captures every key while the finder is open, so q, ?, / and
// spaces are query text. Navigation is arrow/ctrl+jk/page based; Esc and
// ctrl+c close, Enter opens the current result.
func (m Model) handleFinderKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		return m.closeFinder(), nil
	case "enter":
		return m.openFinderSelection()
	case "up", "ctrl+k":
		m.moveFinder(-1)
		return m, nil
	case "down", "ctrl+j":
		m.moveFinder(1)
		return m, nil
	case "pgup":
		m.moveFinder(-m.finderListRows())
		return m, nil
	case "pgdown":
		m.moveFinder(m.finderListRows())
		return m, nil
	case "left", "ctrl+b":
		if m.finderCursor > 0 {
			m.finderCursor--
		}
		return m, nil
	case "right", "ctrl+f":
		if m.finderCursor < len([]rune(m.finderQuery)) {
			m.finderCursor++
		}
		return m, nil
	case "home", "ctrl+a":
		m.finderCursor = 0
		return m, nil
	case "end", "ctrl+e":
		m.finderCursor = len([]rune(m.finderQuery))
		return m, nil
	case "backspace", "ctrl+h":
		m.deleteFinderRune(-1)
		return m, nil
	case "delete":
		m.deleteFinderRune(1)
		return m, nil
	case "ctrl+u":
		m.finderQuery = ""
		m.finderCursor = 0
		m.refreshFinderResults()
		return m, nil
	case "ctrl+w":
		m.deleteFinderWord()
		return m, nil
	}

	if msg.Mod.Contains(tea.ModCtrl) || msg.Mod.Contains(tea.ModAlt) {
		return m, nil
	}
	if msg.Text != "" {
		m.insertFinderText(msg.Text)
	}
	return m, nil
}

func (m *Model) insertFinderText(text string) {
	runes := []rune(m.finderQuery)
	position := clamp(m.finderCursor, 0, len(runes))
	insert := []rune(strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\r', '\t':
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, text))
	if len(insert) == 0 {
		return
	}
	next := make([]rune, 0, len(runes)+len(insert))
	next = append(next, runes[:position]...)
	next = append(next, insert...)
	next = append(next, runes[position:]...)
	m.finderQuery = string(next)
	m.finderCursor = position + len(insert)
	m.refreshFinderResults()
}

func (m *Model) deleteFinderRune(direction int) {
	runes := []rune(m.finderQuery)
	position := clamp(m.finderCursor, 0, len(runes))
	remove := -1
	switch {
	case direction < 0 && position > 0:
		remove = position - 1
	case direction > 0 && position < len(runes):
		remove = position
	}
	if remove < 0 {
		return
	}
	runes = append(runes[:remove], runes[remove+1:]...)
	m.finderQuery = string(runes)
	if remove < m.finderCursor {
		m.finderCursor--
	}
	m.refreshFinderResults()
}

func (m *Model) deleteFinderWord() {
	runes := []rune(m.finderQuery)
	position := clamp(m.finderCursor, 0, len(runes))
	start := position
	for start > 0 && unicode.IsSpace(runes[start-1]) {
		start--
	}
	for start > 0 && !unicode.IsSpace(runes[start-1]) {
		start--
	}
	if start == position {
		return
	}
	runes = append(runes[:start], runes[position:]...)
	m.finderQuery = string(runes)
	m.finderCursor = start
	m.refreshFinderResults()
}

func (m *Model) refreshFinderResults() {
	m.finderSel = 0
	m.finderOffset = 0
	if m.finderIndex == nil {
		m.finderResults = nil
		return
	}
	m.finderResults = finder.Filter(*m.finderIndex, m.finderQuery)
}

func (m *Model) moveFinder(delta int) {
	if len(m.finderResults) == 0 {
		return
	}
	m.finderSel = clamp(m.finderSel+delta, 0, len(m.finderResults)-1)
	m.ensureFinderSelectionVisible()
}

func (m *Model) ensureFinderSelectionVisible() {
	rows := m.finderListRows()
	if len(m.finderResults) == 0 {
		m.finderSel = 0
		m.finderOffset = 0
		return
	}
	m.finderSel = clamp(m.finderSel, 0, len(m.finderResults)-1)
	if m.finderSel < m.finderOffset {
		m.finderOffset = m.finderSel
	}
	if m.finderSel >= m.finderOffset+rows {
		m.finderOffset = m.finderSel - rows + 1
	}
	maxOffset := max(0, len(m.finderResults)-rows)
	if m.finderOffset > maxOffset {
		m.finderOffset = maxOffset
	}
}

func (m *Model) finderListRows() int {
	return max(1, min(12, m.height-8))
}

func (m Model) closeFinder() Model {
	m.overlay = overlayNone
	m.finderState = finderIdle
	m.finderQuery = ""
	m.finderCursor = 0
	m.finderResults = nil
	m.finderSel = 0
	m.finderOffset = 0
	if m.finderWarn != "" {
		m.notice = "finder: " + m.finderWarn
	} else {
		m.notice = ""
	}
	return m
}

// openFinderSelection opens the highlighted file in the viewer and reveals it
// in the tree. A reveal failure (vanished, hidden, outside root) still opens
// the path so the viewer's error state can explain the failure.
func (m Model) openFinderSelection() (tea.Model, tea.Cmd) {
	if m.finderSel < 0 || m.finderSel >= len(m.finderResults) {
		return m, nil
	}
	path := m.finderResults[m.finderSel].Path
	m = m.closeFinder()
	m.notice = ""
	m.focus = FocusViewer
	m.selected = path
	m.stats = viewer.StatsMsg{Path: path}
	m.resetDocumentIndex()
	if !m.tree.Reveal(path) {
		m.notice = "finder: file not visible in the tree"
	}
	m.syncWatches()
	return m, m.viewer.Open(path)
}

// invalidateFinder drops the cached index after the tree's hidden-file rule
// changes. In-flight results are rejected by the bumped sequence.
func (m *Model) invalidateFinder() {
	m.finderIndex = nil
	m.finderBuilding = false
	m.finderSeq++
	m.finderResults = nil
	m.finderWarn = ""
	if m.finderState != finderLoading {
		m.finderState = finderIdle
	}
}

func clamp(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}
