package tree

import (
	tea "charm.land/bubbletea/v2"
	"github.com/smhanov/mdui/internal/scrollbar"
)

// ScrollState reports the tree's current vertical scrolling geometry.
func (m Model) ScrollState() scrollbar.State {
	visible := min(max(0, m.height), len(m.rows))
	return scrollbar.State{
		Total: len(m.rows), Visible: visible,
		Offset: m.windowStart(m.height), Height: m.height,
	}
}

// ScrollTo moves the visible tree window while keeping its cursor in view.
// A changed file selection is previewed; a directory or warning row is not.
func (m *Model) ScrollTo(offset int) tea.Cmd {
	height := m.height
	if height < 1 {
		height = 1
	}
	offset = min(max(offset, 0), max(0, len(m.rows)-height))
	oldCursor := m.cursor
	if len(m.rows) > 0 {
		if m.cursor < offset {
			m.cursor = m.nearestSelectable(offset)
		} else if m.cursor >= offset+height {
			m.cursor = m.nearestSelectable(offset + height - 1)
		}
	}
	m.offset = offset
	if m.cursor != oldCursor {
		return m.selectionCmd()
	}
	return nil
}
