// Package style holds the Lip Gloss styles that make up mdui's chrome.
//
// Colours are deliberately sparse: ANSI colour 8 (bright black) reads as a
// muted tone on both dark and light terminals, and reverse video lets the
// status bar adapt to the terminal's own foreground/background pair.
package style

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// muted is an ANSI indexed colour understood by every colour-capable terminal.
const muted = "8"

var (
	// Frame is the rounded border around the whole application.
	Frame = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(muted))

	// TitleBar is the inset, bold row at the top of the frame.
	TitleBar = lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1)

	// Divider is the one-column rule between the tree and viewer panes.
	Divider = lipgloss.NewStyle().
		Foreground(lipgloss.Color(muted))

	// DividerActive is the brighter rule shown while the splitter is dragged.
	DividerActive = lipgloss.NewStyle().
			Bold(true)

	// Pane gives pane content a one-cell inset on each side.
	Pane = lipgloss.NewStyle().
		Padding(0, 1)

	// StatusBar is the reverse-video base for the bottom row.
	StatusBar = lipgloss.NewStyle().
			Reverse(true)

	// StatusLeft is the left status block; it carries the current file info.
	StatusLeft = StatusBar.
			Bold(true).
			Padding(0, 1)

	// StatusRight is the right status block; it carries the key hints.
	StatusRight = StatusBar.
			Padding(0, 1)

	// TooSmall centres the refusal message on terminals below the minimum size.
	TooSmall = lipgloss.NewStyle().
			Foreground(lipgloss.Color(muted))

	// TreeDir emphasises directory rows.
	TreeDir = lipgloss.NewStyle().
		Bold(true)

	// TreeFile is the default style for markdown file rows.
	TreeFile = lipgloss.NewStyle()

	// TreeWarning colours inline scan and symlink warnings.
	TreeWarning = lipgloss.NewStyle().
			Foreground(lipgloss.Color("3"))

	// TreeSelected is the full-row cursor highlight.
	TreeSelected = lipgloss.NewStyle().
			Reverse(true).
			Bold(true)

	// TreeEmpty is the muted placeholder shown for a folder with no markdown.
	TreeEmpty = lipgloss.NewStyle().
			Italic(true).
			Foreground(lipgloss.Color(muted))
)

// Truncate shortens s to at most width terminal cells, appending an ellipsis
// when it had to cut. It is intended for unstyled single-line text.
func Truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}

	var b strings.Builder
	used := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if used+rw > width-1 {
			break
		}
		b.WriteRune(r)
		used += rw
	}
	b.WriteString("…")
	return b.String()
}
