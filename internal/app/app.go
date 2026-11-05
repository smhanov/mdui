// Package app owns mdui's root Bubble Tea model and the window layout.
package app

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"mdui/internal/style"
	"mdui/internal/tree"
	"mdui/internal/viewer"
)

const (
	// minTermW and minTermH are the hard-refusal size from the spec: below
	// this the app shows a placeholder rather than mangled panes.
	minTermW = 60
	minTermH = 15

	// comfortW and comfortH are the soft minimum. Between the hard and soft
	// minimums the app renders normally but tells the user to resize.
	comfortW = 80
	comfortH = 24

	titleRows  = 1
	statusRows = 1
)

// Focus identifies which pane receives routed input.
type Focus int

const (
	// FocusTree is the left (file tree) pane.
	FocusTree Focus = iota
	// FocusViewer is the right (markdown viewer) pane.
	FocusViewer
)

// Model is the root application model. It owns the window size, the split
// ratio, focus, and the chrome; the tree and viewer children own their
// content.
type Model struct {
	root   string
	width  int
	height int
	ratio  float64
	focus  Focus

	selected string
	tree     tree.Model
	viewer   viewer.Model
}

// New returns a root model scoped to root.
func New(root string) Model {
	return Model{
		root:   root,
		ratio:  DefaultRatio,
		focus:  FocusTree,
		tree:   tree.New(root),
		viewer: viewer.New(),
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model. Layout-level messages are handled here; pane
// content messages are routed to the child that owns them.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.tree.SetHeight(m.paneHeight())
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab", "shift+tab":
			if m.focus == FocusTree {
				m.focus = FocusViewer
			} else {
				m.focus = FocusTree
			}
			return m, nil
		}
		if m.focus == FocusTree {
			var cmd tea.Cmd
			m.tree, cmd = m.tree.Update(msg)
			return m, cmd
		}
	case tree.SelectionMsg:
		m.selected = msg.Path
	}
	return m, nil
}

// View implements tea.Model.
func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "mdui — " + m.root
	return v
}

// render produces the full shell as a styled string. It is split out from
// View so tests can assert on the rendered content without a terminal.
func (m Model) render() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	if m.tooSmall() {
		return m.renderTooSmall()
	}

	innerW := m.width - style.Frame.GetHorizontalFrameSize()
	panesH := m.paneHeight()
	if innerW < 1 || panesH < 1 {
		return m.renderTooSmall()
	}
	treeW, divW, viewerW := SplitWidths(innerW, m.ratio)
	body := lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.tree.View(treeW, panesH),
		renderDivider(divW, panesH),
		m.viewer.View(viewerW, panesH),
	)

	content := strings.Join([]string{
		m.renderTitle(innerW),
		body,
		m.renderStatus(innerW),
	}, "\n")

	return style.Frame.
		Width(m.width).
		Height(m.height).
		Render(content)
}

func (m Model) tooSmall() bool {
	return m.width < minTermW || m.height < minTermH
}

// paneHeight is the number of rows available to the tree and viewer panes.
func (m Model) paneHeight() int {
	return m.height - style.Frame.GetVerticalFrameSize() - titleRows - statusRows
}

func (m Model) degraded() bool {
	return !m.tooSmall() && (m.width < comfortW || m.height < comfortH)
}

func (m Model) renderTitle(width int) string {
	plain := "mdui — " + m.root
	available := width - style.TitleBar.GetHorizontalFrameSize()
	return style.TitleBar.
		Width(width).
		Render(style.Truncate(plain, available))
}

func (m Model) renderStatus(width int) string {
	leftText := "no file selected"
	if m.selected != "" {
		leftText = strconv.Quote(m.selected)
	}
	rightText := "tab focus · q quit"
	if m.degraded() {
		rightText = "resize to ≥80×24"
	}

	left := style.StatusLeft.Render(leftText)
	right := style.StatusRight.Render(rightText)

	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		// Keep the file info; drop the hint when there is no room for both.
		right = ""
		gap = width - lipgloss.Width(left)
	}
	if gap < 0 {
		available := width - style.StatusLeft.GetHorizontalFrameSize()
		left = style.StatusLeft.Render(style.Truncate(leftText, available))
		gap = width - lipgloss.Width(left)
	}

	var b strings.Builder
	b.WriteString(left)
	if gap > 0 {
		b.WriteString(style.StatusBar.Render(strings.Repeat(" ", gap)))
	}
	b.WriteString(right)
	return b.String()
}

// renderDivider draws the one-column rule between the panes.
func renderDivider(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	row := strings.Repeat("│", width)
	lines := make([]string, height)
	for i := range lines {
		lines[i] = row
	}
	return style.Divider.Render(strings.Join(lines, "\n"))
}

// renderTooSmall replaces the shell below the hard minimum size.
func (m Model) renderTooSmall() string {
	w, h := max(m.width, 0), max(m.height, 0)
	if w == 0 || h == 0 {
		return ""
	}

	lines := []string{
		"terminal too small",
		fmt.Sprintf("need at least %d×%d (have %d×%d)", minTermW, minTermH, m.width, m.height),
	}
	if h < len(lines) {
		lines = lines[:h]
	}
	for i, line := range lines {
		lines[i] = style.Truncate(line, w)
	}

	return lipgloss.Place(
		w, h,
		lipgloss.Center, lipgloss.Center,
		style.TooSmall.Render(strings.Join(lines, "\n")),
	)
}
