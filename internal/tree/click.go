package tree

import tea "charm.land/bubbletea/v2"

// RowIndexAt maps a pane-relative row (0 is the first visible line) to an
// index in the flattened row list. It returns -1 when the row is blank.
func (m Model) RowIndexAt(y, height int) int {
	if y < 0 || height < 1 || len(m.rows) == 0 {
		return -1
	}
	start := m.windowStart(height)
	if y >= height {
		return -1
	}
	index := start + y
	if index < 0 || index >= len(m.rows) {
		return -1
	}
	return index
}

// VisibleStart returns the index of the first row rendered at height. It is
// the inverse of RowIndexAt and is useful for mouse hit-testing in callers.
func (m Model) VisibleStart(height int) int {
	return m.windowStart(height)
}

// RowAt returns the visible row at pane-relative y.
func (m Model) RowAt(y, height int) (Row, int, bool) {
	index := m.RowIndexAt(y, height)
	if index < 0 {
		return Row{}, -1, false
	}
	return m.rows[index], index, true
}

// SelectRow moves the cursor to index and returns the preview command for a
// file row.
func (m *Model) SelectRow(index int) tea.Cmd {
	if !m.validRow(index) {
		return nil
	}
	m.cursor = index
	m.ensureCursorVisible()
	return m.selectionCmd()
}

// ToggleRow selects a row and, for directories, flips its expansion state.
// Clicking a file row previews it.
func (m *Model) ToggleRow(index int) tea.Cmd {
	if !m.validRow(index) {
		return nil
	}
	m.cursor = index
	row := &m.rows[index]
	if !row.IsDir {
		m.ensureCursorVisible()
		return m.selectionCmd()
	}

	if !row.node.Expanded && !row.node.Loaded {
		m.load(row.node)
	}
	row.node.Expanded = !row.node.Expanded
	m.rebuild()
	return nil
}

// DescendRow selects a row and, for directories, ensures it is expanded and
// moves the cursor to its first selectable child. It implements the second
// click of a double-click.
func (m *Model) DescendRow(index int) tea.Cmd {
	if !m.validRow(index) {
		return nil
	}
	m.cursor = index
	row := &m.rows[index]
	if !row.IsDir {
		m.ensureCursorVisible()
		return m.selectionCmd()
	}

	depth := row.Depth
	if !row.node.Expanded || !row.node.Loaded {
		if !row.node.Loaded {
			m.load(row.node)
		}
		row.node.Expanded = true
		m.rebuild()
	}

	for i := index + 1; i < len(m.rows); i++ {
		if m.rows[i].Depth <= depth {
			break
		}
		if m.rows[i].Warning == "" {
			return m.moveTo(i, false)
		}
	}
	return nil
}

func (m Model) validRow(index int) bool {
	return index >= 0 && index < len(m.rows) && m.rows[index].Warning == ""
}
