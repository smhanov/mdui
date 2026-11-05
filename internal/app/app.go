// Package app owns mdui's root Bubble Tea model and the window layout.
package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"mdui/internal/config"
	"mdui/internal/render"
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

	// dragDebounce is the longest gap between markdown re-renders while the
	// splitter is being dragged.
	dragDebounce = 100 * time.Millisecond

	// nudgeColumns is the divider step for the H and L keys.
	nudgeColumns = 2

	// treeWheelRows is how many rows one wheel notch moves the tree cursor.
	treeWheelRows = 3
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

	selected  string
	tree      tree.Model
	viewer    viewer.Model
	statePath string

	dragging        bool
	dragSeq         uint64
	dragTickPending bool
	lastTreeW       int
	clampRatio      bool
}

type options struct {
	docStyle     render.Style
	treeRatio    float64
	ratioSet     bool
	statePath    string
	statePathSet bool
}

// dragTickMsg is delivered by the debounce timer during a splitter drag. Seq
// ties the tick to the drag that scheduled it so a stale timer cannot disturb
// a later drag.
type dragTickMsg struct {
	Seq uint64
}

// Option configures the root model at construction time.
type Option func(*options)

// WithStyle selects the Glamour style used by the viewer. Styles are resolved
// once at startup; an empty style means dark.
func WithStyle(style render.Style) Option {
	return func(c *options) {
		c.docStyle = style
	}
}

// WithTreeRatio restores a previously persisted split ratio. The ratio is
// clamped to the pane minimums when the first window size is known.
func WithTreeRatio(ratio float64) Option {
	return func(c *options) {
		c.treeRatio = ratio
		c.ratioSet = true
	}
}

// WithStatePath overrides where session state is written. An empty path
// disables persistence.
func WithStatePath(path string) Option {
	return func(c *options) {
		c.statePath = path
		c.statePathSet = true
	}
}

// New returns a root model scoped to root.
func New(root string, opts ...Option) Model {
	cfg := options{docStyle: render.Dark}
	for _, opt := range opts {
		opt(&cfg)
	}

	m := Model{
		root:       root,
		ratio:      DefaultRatio,
		focus:      FocusTree,
		lastTreeW:  -1,
		clampRatio: true,
	}
	if cfg.ratioSet {
		m.ratio = cfg.treeRatio
	}
	if cfg.statePathSet {
		m.statePath = cfg.statePath
	} else if path, err := config.DefaultPath(); err == nil {
		m.statePath = path
	}

	m.tree = tree.New(root)
	m.viewer = viewer.New(cfg.docStyle)
	return m
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
		if m.clampRatio {
			if innerW := m.innerWidth(); innerW > 0 {
				m.ratio = ClampRatio(innerW, m.ratio)
				m.clampRatio = false
			}
		}
		m.tree.SetHeight(m.paneHeight())
		innerW := m.innerWidth()
		treeW, _, viewerW := SplitWidths(innerW, m.ratio)
		m.lastTreeW = treeW
		return m, m.viewer.SetSize(viewerW, m.paneHeight())
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.saveState()
			return m, tea.Quit
		case "tab", "shift+tab":
			if m.focus == FocusTree {
				m.focus = FocusViewer
			} else {
				m.focus = FocusTree
			}
			return m, nil
		case "H", "L", "=":
			return m.handleSplitKey(msg.String())
		}
		if m.focus == FocusTree {
			var cmd tea.Cmd
			m.tree, cmd = m.tree.Update(msg)
			return m, cmd
		}
		var cmd tea.Cmd
		m.viewer, cmd = m.viewer.Update(msg)
		return m, cmd
	case tea.MouseClickMsg:
		return m.handleMouseClick(msg)
	case tea.MouseMotionMsg:
		return m.handleMouseMotion(msg)
	case tea.MouseReleaseMsg:
		return m.handleMouseRelease(msg)
	case tea.MouseWheelMsg:
		return m.handleMouseWheel(msg)
	case dragTickMsg:
		if !m.dragging || msg.Seq != m.dragSeq {
			return m, nil
		}
		m.dragTickPending = false
		return m, m.renderSplitIfNeeded()
	case tree.SelectionMsg:
		m.selected = msg.Path
		return m, m.viewer.Open(msg.Path)
	default:
		var cmd tea.Cmd
		m.viewer, cmd = m.viewer.Update(msg)
		return m, cmd
	}
}

// View implements tea.Model.
func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "mdui — " + m.root
	v.MouseMode = tea.MouseModeCellMotion
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

	innerW := m.innerWidth()
	panesH := m.paneHeight()
	if innerW < 1 || panesH < 1 {
		return m.renderTooSmall()
	}
	treeW, divW, viewerW := SplitWidths(innerW, m.ratio)
	body := lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.tree.View(treeW, panesH),
		renderDivider(divW, panesH, m.dragging),
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

// innerWidth is the width available inside the frame border.
func (m Model) innerWidth() int {
	return m.width - style.Frame.GetHorizontalFrameSize()
}

// frameLeft is the number of terminal columns the frame border occupies on
// the left. The rounded frame has a one-cell border and no padding.
func (m Model) frameLeft() int {
	return style.Frame.GetHorizontalFrameSize() / 2
}

// bodyTop is the first terminal row occupied by pane content (below the frame
// border and title bar).
func (m Model) bodyTop() int {
	return style.Frame.GetVerticalFrameSize()/2 + titleRows
}

// inBody reports whether terminal row y is part of the pane area.
func (m Model) inBody(y int) bool {
	top := m.bodyTop()
	return y >= top && y < top+m.paneHeight()
}

// paneRegion identifies the part of the shell under a mouse event.
type paneRegion int

const (
	regionOutside paneRegion = iota
	regionTree
	regionDivider
	regionViewer
)

// regionAt maps terminal coordinates to the pane or divider under the cursor.
// Divider coordinates include the ±DividerHitSlop drag zone.
func (m Model) regionAt(x, y int) paneRegion {
	if m.width <= 0 || m.height <= 0 || m.tooSmall() || !m.inBody(y) {
		return regionOutside
	}
	innerW := m.innerWidth()
	if innerW < 1 {
		return regionOutside
	}
	contentX := x - m.frameLeft()
	if contentX < 0 || contentX >= innerW {
		return regionOutside
	}

	treeW, _, _ := SplitWidths(innerW, m.ratio)
	switch {
	case contentX >= treeW-DividerHitSlop && contentX <= treeW+DividerHitSlop:
		return regionDivider
	case contentX < treeW:
		return regionTree
	default:
		return regionViewer
	}
}

// handleSplitKey applies the keyboard splitter controls.
func (m Model) handleSplitKey(key string) (tea.Model, tea.Cmd) {
	innerW := m.innerWidth()
	switch key {
	case "H":
		m.ratio = NudgeRatio(innerW, m.ratio, -nudgeColumns)
	case "L":
		m.ratio = NudgeRatio(innerW, m.ratio, nudgeColumns)
	case "=":
		m.ratio = ClampRatio(innerW, DefaultRatio)
	}
	return m, m.renderSplitIfNeeded()
}

// renderSplitIfNeeded asks the viewer to render at the current ratio only if
// the tree width changed since the last request. It updates the debounce
// bookkeeping and returns the viewer's asynchronous render command.
func (m *Model) renderSplitIfNeeded() tea.Cmd {
	innerW := m.innerWidth()
	if innerW < 1 || m.paneHeight() < 1 {
		return nil
	}
	treeW, _, viewerW := SplitWidths(innerW, m.ratio)
	if treeW == m.lastTreeW {
		return nil
	}
	m.lastTreeW = treeW
	return m.viewer.SetSize(viewerW, m.paneHeight())
}

// scheduleDragTick starts one debounce timer if a resize is pending and no
// timer is already in flight. Additional motion during the window only updates
// the ratio; the timer renders the latest width when it fires.
func (m *Model) scheduleDragTick() tea.Cmd {
	if m.dragTickPending {
		return nil
	}
	innerW := m.innerWidth()
	if innerW < 1 {
		return nil
	}
	treeW, _, _ := SplitWidths(innerW, m.ratio)
	if treeW == m.lastTreeW {
		return nil
	}

	m.dragTickPending = true
	seq := m.dragSeq
	return tea.Tick(dragDebounce, func(time.Time) tea.Msg {
		return dragTickMsg{Seq: seq}
	})
}

func (m Model) handleMouseClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	mouse := msg.Mouse()
	if mouse.Button != tea.MouseLeft || mouse.Mod.Contains(tea.ModShift) {
		return m, nil
	}
	if m.regionAt(mouse.X, mouse.Y) != regionDivider {
		return m, nil
	}

	m.dragging = true
	m.dragSeq++
	m.dragTickPending = false
	return m, nil
}

func (m Model) handleMouseMotion(msg tea.MouseMotionMsg) (tea.Model, tea.Cmd) {
	if !m.dragging {
		return m, nil
	}
	innerW := m.innerWidth()
	if innerW < 1 {
		return m, nil
	}
	m.ratio = RatioAtColumn(innerW, msg.Mouse().X-m.frameLeft())
	return m, m.scheduleDragTick()
}

func (m Model) handleMouseRelease(msg tea.MouseReleaseMsg) (tea.Model, tea.Cmd) {
	if !m.dragging {
		return m, nil
	}

	m.dragging = false
	m.dragTickPending = false
	cmd := m.renderSplitIfNeeded()
	m.saveState()
	return m, cmd
}

func (m Model) handleMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	mouse := msg.Mouse()
	if mouse.Mod.Contains(tea.ModShift) {
		return m, nil
	}

	switch m.regionAt(mouse.X, mouse.Y) {
	case regionTree:
		switch mouse.Button {
		case tea.MouseWheelUp:
			return m, m.tree.Scroll(-treeWheelRows)
		case tea.MouseWheelDown:
			return m, m.tree.Scroll(treeWheelRows)
		}
	case regionViewer:
		var cmd tea.Cmd
		m.viewer, cmd = m.viewer.Update(msg)
		return m, cmd
	}
	return m, nil
}

// saveState atomically writes the split ratio. It runs on drag release and
// quit, never on individual motion events.
func (m Model) saveState() {
	if m.statePath == "" {
		return
	}
	ratio := m.ratio
	if innerW := m.innerWidth(); innerW > 0 {
		ratio = ClampRatio(innerW, ratio)
	}
	_ = config.Save(m.statePath, config.State{TreeRatio: ratio})
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

// renderDivider draws the one-column rule between the panes. The glyph and
// style switch to a bright heavy rule while the splitter is being dragged.
func renderDivider(width, height int, dragging bool) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	glyph := "│"
	dividerStyle := style.Divider
	if dragging {
		glyph = "┃"
		dividerStyle = style.DividerActive
	}
	row := strings.Repeat(glyph, width)
	lines := make([]string, height)
	for i := range lines {
		lines[i] = row
	}
	return dividerStyle.Render(strings.Join(lines, "\n"))
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
