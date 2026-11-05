package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"

	"mdui/internal/finder"
	"mdui/internal/style"
)

// renderFinderOverlay centres the finder box over the dimmed shell. The list
// height is fixed for the current terminal so the box does not jump while the
// user types or scrolls.
func (m Model) renderFinderOverlay(base string) string {
	contentW := min(max(1, m.width-6), 76)
	rows := m.finderListRows()

	content := []string{style.OverlayTitle.Render(style.Truncate(m.finderTitle(), contentW))}
	content = append(content, finderInputLine(m.finderQuery, m.finderCursor, contentW))
	content = append(content, m.finderResultLines(contentW, rows)...)
	content = append(content, m.finderFooter(contentW))
	return m.renderBoxedOverlay(base, content, contentW)
}

func (m Model) finderTitle() string {
	switch m.finderState {
	case finderLoading:
		return "Find markdown file · indexing…"
	case finderReady:
		count := 0
		if m.finderIndex != nil {
			count = len(m.finderIndex.Files)
		}
		title := fmt.Sprintf("Find markdown file · %d %s", count, plural(count, "file", "files"))
		if m.finderWarn != "" {
			title += " · ⚠ partial"
		}
		return title
	default:
		return "Find markdown file"
	}
}

func (m Model) finderResultLines(width, rows int) []string {
	lines := make([]string, 0, rows)

	state := ""
	switch {
	case m.finderState == finderLoading:
		state = "indexing markdown files…"
	case m.finderIndex == nil:
		state = "no file index"
	case len(m.finderIndex.Files) == 0:
		if m.finderWarn != "" {
			state = "no readable markdown files under this folder"
		} else {
			state = "no markdown files under this folder"
		}
	case len(m.finderResults) == 0:
		state = fmt.Sprintf("no matches for “%s”", m.finderQuery)
	}
	if state != "" {
		lines = append(lines, style.TreeEmpty.Render(style.Truncate(state, width)))
		return padFinderLines(lines, rows)
	}

	end := min(len(m.finderResults), m.finderOffset+rows)
	for i := m.finderOffset; i < end; i++ {
		result := m.finderResults[i]
		if i == m.finderSel {
			label := "▸ " + truncateFinderPath(result.Rel, max(0, width-2))
			lines = append(lines, style.TreeSelected.Render(padCells(label, width)))
			continue
		}
		lines = append(lines, "  "+finderPathLabel(result, max(0, width-2)))
	}
	return padFinderLines(lines, rows)
}

func padFinderLines(lines []string, rows int) []string {
	for len(lines) < rows {
		lines = append(lines, "")
	}
	if len(lines) > rows {
		lines = lines[:rows]
	}
	return lines
}

// finderPathLabel underlines matched runes when the complete relative path
// fits; truncated labels are plain because their rune offsets no longer map.
func finderPathLabel(result finder.Result, width int) string {
	if lipgloss.Width(result.Rel) > width {
		return truncateFinderPath(result.Rel, width)
	}
	if len(result.Positions) == 0 {
		return result.Rel
	}
	wanted := make(map[int]bool, len(result.Positions))
	for _, position := range result.Positions {
		wanted[position] = true
	}
	var b strings.Builder
	for i, r := range []rune(result.Rel) {
		if wanted[i] {
			b.WriteString(style.FinderMatch.Render(string(r)))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// truncateFinderPath shortens a relative path to width cells, keeping the
// basename when possible and marking dropped leading directories with an
// ellipsis.
func truncateFinderPath(path string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(path) <= width {
		return path
	}

	base := filepath.Base(path)
	if lipgloss.Width(base) >= width {
		return style.Truncate(base, width)
	}

	separator := string(filepath.Separator)
	parts := strings.Split(filepath.Dir(path), separator)
	suffix := base
	for i := len(parts) - 1; i >= 0; i-- {
		part := parts[i]
		if part == "" || part == "." || part == separator {
			continue
		}
		candidate := part + separator + suffix
		if lipgloss.Width("…"+separator+candidate) > width {
			break
		}
		suffix = candidate
	}
	if lipgloss.Width("…"+separator+suffix) > width {
		return base
	}
	return "…" + separator + suffix
}

func finderInputLine(query string, cursor, width int) string {
	const prefix = "> "
	available := max(1, width-lipgloss.Width(prefix))
	runes := []rune(query)
	cursor = clamp(cursor, 0, len(runes))

	start := 0
	if len(runes)+1 > available {
		start = cursor - available + 2
		if start < 0 {
			start = 0
		}
		if start > len(runes) {
			start = len(runes)
		}
	}

	text := runes[start:]
	cursor -= start
	line := string(text[:cursor]) + "▏" + string(text[cursor:])
	if start > 0 {
		line = "…" + line
	}
	return prefix + style.Truncate(line, available)
}

func (m Model) finderFooter(width int) string {
	parts := make([]string, 0, 5)
	if m.finderState == finderLoading {
		parts = append(parts, "indexing…", "esc close")
	} else {
		if len(m.finderResults) == 0 {
			parts = append(parts, "0 results")
		} else {
			parts = append(parts, fmt.Sprintf("%d/%d", m.finderSel+1, len(m.finderResults)))
		}
		parts = append(parts, "↑↓ move", "enter open", "esc close")
	}
	return style.FinderHint.Render(style.Truncate(strings.Join(parts, " · "), width))
}
