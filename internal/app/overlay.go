package app

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"mdui/internal/render"
	"mdui/internal/style"
)

type overlayKind int

const (
	overlayNone overlayKind = iota
	overlayHelp
	overlayTable
	overlayTablePicker
	overlayFinder
)

const (
	// helpMinWidth and helpMaxWidth bound the rendered cheatsheet width.
	helpMinWidth = 36
	helpMaxWidth = 76
)

// openHelp renders the embedded cheatsheet at the current width and activates
// the overlay.
func (m Model) openHelp() Model {
	width := m.helpContentWidth()
	body, err := render.RenderBytes(style.HelpMarkdown(), width, render.Options{
		Style: m.docStyle,
		Emoji: m.emoji,
	})
	if err != nil {
		body = "help is unavailable: " + err.Error()
	}
	m.overlay = overlayHelp
	m.helpBody = body
	m.helpWidth = width
	return m
}

func (m Model) helpContentWidth() int {
	width := m.width - 10
	if width > helpMaxWidth {
		width = helpMaxWidth
	}
	if width < helpMinWidth {
		width = helpMinWidth
	}
	return width
}

// renderHelpOverlay centres the help box over a dimmed copy of the normal
// layout.
func (m Model) renderHelpOverlay(base string) string {
	innerW := max(1, m.helpWidth)
	content := strings.Split(strings.TrimRight(m.helpBody, "\n"), "\n")

	maxContentRows := max(1, m.height-2)
	truncated := false
	if len(content) > maxContentRows {
		content = content[:maxContentRows]
		truncated = true
	}
	if truncated && len(content) > 0 {
		content[len(content)-1] = "… resize the terminal to see the rest"
	}

	boxW := innerW + 4
	boxH := len(content) + 2
	if boxW > m.width {
		boxW = m.width
	}
	if boxH > m.height {
		boxH = m.height
	}

	box := make([]string, 0, boxH)
	top := "╭" + strings.Repeat("─", max(0, boxW-2)) + "╮"
	box = append(box, style.HelpBorder.Render(top))
	for _, line := range content {
		line = ansi.Truncate(line, innerW, "…")
		if pad := innerW - ansi.StringWidth(line); pad > 0 {
			line += strings.Repeat(" ", pad)
		}
		box = append(box, style.HelpBorder.Render("│")+" "+line+" "+style.HelpBorder.Render("│"))
	}
	bottom := "╰" + strings.Repeat("─", max(0, boxW-2)) + "╯"
	box = append(box, style.HelpBorder.Render(bottom))
	if len(box) > m.height {
		box = box[:m.height]
	}
	for i, line := range box {
		if w := ansi.StringWidth(line); w < boxW {
			box[i] = line + strings.Repeat(" ", boxW-w)
		} else if w > boxW {
			box[i] = ansi.Truncate(line, boxW, "")
		}
	}

	x := (m.width - boxW) / 2
	y := (m.height - boxH) / 2
	if y < 0 {
		y = 0
	}
	return overlayCenter(base, box, x, y, m.width, m.height)
}

// renderBoxedOverlay centres a bordered box over the dimmed base. content
// lines are truncated to contentW cells and the whole box is capped to the
// terminal height.
func (m Model) renderBoxedOverlay(base string, content []string, contentW int) string {
	if contentW < 1 {
		contentW = 1
	}
	if contentW+4 > m.width {
		contentW = max(1, m.width-4)
	}

	maxRows := max(1, m.height-2)
	if len(content) > maxRows {
		content = content[:maxRows]
		content[len(content)-1] = "… terminal too short"
	}

	boxW := contentW + 4
	boxH := len(content) + 2
	if boxH > m.height {
		boxH = m.height
	}

	box := make([]string, 0, boxH)
	top := "╭" + strings.Repeat("─", max(0, boxW-2)) + "╮"
	box = append(box, style.HelpBorder.Render(top))
	for _, line := range content {
		line = ansi.Truncate(line, contentW, "…")
		if pad := contentW - ansi.StringWidth(line); pad > 0 {
			line += strings.Repeat(" ", pad)
		}
		box = append(box, style.HelpBorder.Render("│")+" "+line+" "+style.HelpBorder.Render("│"))
	}
	bottom := "╰" + strings.Repeat("─", max(0, boxW-2)) + "╯"
	box = append(box, style.HelpBorder.Render(bottom))
	if len(box) > m.height {
		box = box[:m.height]
	}
	for i, line := range box {
		if w := ansi.StringWidth(line); w < boxW {
			box[i] = line + strings.Repeat(" ", boxW-w)
		} else if w > boxW {
			box[i] = ansi.Truncate(line, boxW, "")
		}
	}

	x := (m.width - boxW) / 2
	y := (m.height - boxH) / 2
	if y < 0 {
		y = 0
	}
	return overlayCenter(base, box, x, y, m.width, m.height)
}

// overlayCenter dims base and splices box into it at (x, y). All widths are
// terminal cells.
func overlayCenter(base string, box []string, x, y, width, height int) string {
	baseLines := strings.Split(base, "\n")
	rows := make([]string, 0, height)
	for row := 0; row < height; row++ {
		var line string
		if row < len(baseLines) {
			line = ansi.Strip(baseLines[row])
		}
		line = padCells(line, width)
		dim := style.Backdrop.Render(line)
		if row < y || row >= y+len(box) {
			rows = append(rows, dim)
			continue
		}

		boxLine := box[row-y]
		boxW := ansi.StringWidth(boxLine)
		left := ansi.Truncate(dim, x, "")
		right := ansi.TruncateLeft(dim, x+boxW, "")
		rows = append(rows, padCells(left+boxLine+right, width))
	}
	return strings.Join(rows, "\n")
}

func padCells(s string, width int) string {
	w := ansi.StringWidth(s)
	if w > width {
		return ansi.Truncate(s, width, "")
	}
	if w < width {
		return s + strings.Repeat(" ", width-w)
	}
	return s
}
