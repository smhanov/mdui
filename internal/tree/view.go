package tree

import (
	"strings"
	"unicode"

	"charm.land/lipgloss/v2"

	"mdui/internal/style"
)

// View renders the visible window of rows at exactly width by height cells.
func (m Model) View(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}

	innerWidth := width - style.Pane.GetHorizontalFrameSize()
	if innerWidth < 1 {
		innerWidth = 1
	}

	lines := m.visibleLines(innerWidth, height)
	return style.Pane.
		Width(width).
		Height(height).
		MaxHeight(height).
		Render(strings.Join(lines, "\n"))
}

func (m Model) visibleLines(width, height int) []string {
	if len(m.rows) == 0 || m.root == nil {
		return nil
	}
	if m.empty() {
		return []string{style.TreeEmpty.Render(style.Truncate("no markdown files found", width))}
	}

	start := m.windowStart(height)
	end := min(start+height, len(m.rows))
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		lines = append(lines, m.renderRow(i, width, i == m.cursor))
	}
	return lines
}

func (m Model) empty() bool {
	return m.root.Err == "" && len(m.root.Children) == 0
}

// windowStart recomputes the first visible row from the cursor and the actual
// view height, so direct View calls stay correct even before SetHeight.
func (m Model) windowStart(height int) int {
	if height < 1 {
		return 0
	}
	start := m.offset
	if m.cursor < start {
		start = m.cursor
	}
	if m.cursor >= start+height {
		start = m.cursor - height + 1
	}
	if start > len(m.rows)-height {
		start = len(m.rows) - height
	}
	if start < 0 {
		start = 0
	}
	return start
}

func (m Model) renderRow(index, width int, selected bool) string {
	row := m.rows[index]

	var text strings.Builder
	text.WriteString(strings.Repeat("  ", row.Depth))

	switch {
	case row.Warning != "":
		text.WriteString("⚠ ")
		text.WriteString(row.Warning)
	case row.IsDir:
		if row.Expanded {
			text.WriteString("▾ ")
		} else {
			text.WriteString("▸ ")
		}
		text.WriteString(displayName(row))
	default:
		text.WriteString("• ")
		text.WriteString(row.Name)
	}

	plain := style.Truncate(sanitizeText(text.String()), width)
	if pad := width - lipgloss.Width(plain); pad > 0 {
		plain += strings.Repeat(" ", pad)
	}

	var rowStyle lipgloss.Style
	switch {
	case selected:
		rowStyle = style.TreeSelected
	case row.Warning != "":
		rowStyle = style.TreeWarning
	case row.IsDir:
		rowStyle = style.TreeDir
	default:
		rowStyle = style.TreeFile
	}
	return rowStyle.Render(plain)
}

// sanitizeText keeps filenames with control characters or terminal escape
// sequences on one visible line.
func sanitizeText(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\r', '\t':
			return ' '
		}
		if unicode.IsControl(r) {
			return '?'
		}
		return r
	}, s)
}
