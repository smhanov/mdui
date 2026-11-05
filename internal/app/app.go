// Package app owns mdui's root Bubble Tea model and the window layout.
package app

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"mdui/internal/config"
	"mdui/internal/mdutil"
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

	// doubleClickWindow is how long a second click on the same tree row still
	// counts as a double-click.
	doubleClickWindow = 400 * time.Millisecond
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
	stats     viewer.StatsMsg

	docStyle render.Style
	emoji    bool

	overlay         overlayKind
	helpBody        string
	helpWidth       int
	searchPrompt    bool
	mouse           bool
	dragging        bool
	dragSeq         uint64
	dragTickPending bool
	lastTreeW       int
	clampRatio      bool

	tableReqID       uint64
	tableActiveReq   uint64
	tablePriorScroll int
	tablePick        int
	tableTitle       string
	tableBody        []string
	tableChoices     []mdutil.Table
	tableLoading     bool
	tableError       string
	tableClipped     bool

	links        []mdutil.Link
	headings     []mdutil.Heading
	headingLines []int
	tables       []mdutil.Table

	pendingLink   *mdutil.LinkTarget
	pendingAnchor string
	notice        string
	opener        func(string) error

	lastClickValid bool
	lastClickIndex int
	lastClickPath  string
	lastClickTime  time.Time
	now            func() time.Time
}

type options struct {
	docStyle     render.Style
	treeRatio    float64
	ratioSet     bool
	statePath    string
	statePathSet bool
	emoji        bool
	wrapWidth    int
	hiddenFiles  bool
	hideEmpty    bool
	sortBy       string
	mouse        bool
	now          func() time.Time
	opener       func(string) error
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
// once at startup; an empty style means mdui's dark stylesheet.
func WithStyle(docStyle render.Style) Option {
	return func(c *options) {
		c.docStyle = docStyle
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

// WithConfig applies the stable configuration file options that affect the
// running UI.
func WithConfig(cfg config.Config) Option {
	return func(c *options) {
		c.emoji = cfg.Emoji
		c.wrapWidth = cfg.WrapWidth
		c.hiddenFiles = cfg.HiddenFiles
		c.hideEmpty = cfg.HideEmptyDirs
		c.sortBy = cfg.SortBy
		c.mouse = cfg.Mouse
	}
}

// WithClock injects the time source used for double-click detection.
func WithClock(now func() time.Time) Option {
	return func(c *options) {
		c.now = now
	}
}

// WithOpener injects the system URL opener. Tests use it to assert dispatch
// without launching a browser; an empty opener falls back to open/xdg-open.
func WithOpener(opener func(string) error) Option {
	return func(c *options) {
		c.opener = opener
	}
}

// New returns a root model scoped to root.
func New(root string, opts ...Option) Model {
	cfg := options{
		docStyle: render.MduiDark,
		emoji:    true,
		mouse:    true,
		sortBy:   "name",
		now:      time.Now,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	m := Model{
		root:       root,
		ratio:      DefaultRatio,
		focus:      FocusTree,
		lastTreeW:  -1,
		clampRatio: true,
		docStyle:   cfg.docStyle,
		emoji:      cfg.emoji,
		mouse:      cfg.mouse,
		now:        cfg.now,
	}
	if m.docStyle == "" {
		m.docStyle = render.MduiDark
	}
	if m.now == nil {
		m.now = time.Now
	}
	if cfg.opener != nil {
		m.opener = cfg.opener
	} else {
		m.opener = systemOpener
	}
	if cfg.ratioSet {
		m.ratio = cfg.treeRatio
	}
	if cfg.statePathSet {
		m.statePath = cfg.statePath
	} else if path, err := config.DefaultPath(); err == nil {
		m.statePath = path
	}

	m.tree = tree.New(root,
		tree.WithHidden(cfg.hiddenFiles),
		tree.WithHideEmpty(cfg.hideEmpty),
		tree.WithSortBy(cfg.sortBy),
	)
	m.viewer = viewer.NewWithOptions(viewer.Options{
		Style:     m.docStyle,
		Emoji:     cfg.emoji,
		WrapWidth: cfg.wrapWidth,
	})
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
		if m.overlay == overlayHelp {
			m = m.openHelp()
		}
		resizeCmd := m.resizePanes()
		if m.overlay == overlayTable && len(m.tableChoices) > 0 {
			next, tableCmd := m.beginTableRender(m.tablePick)
			return next, chainCommands(resizeCmd, tableCmd)
		}
		return m, resizeCmd
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case tea.MouseClickMsg:
		return m.handleMouseClick(msg)
	case tea.MouseMotionMsg:
		return m.handleMouseMotion(msg)
	case tea.MouseReleaseMsg:
		return m.handleMouseRelease(msg)
	case tea.MouseWheelMsg:
		return m.handleMouseWheel(msg)
	case viewer.StatsMsg:
		if msg.Path == m.selected {
			m.stats = msg
		}
		return m, nil
	case render.RenderedMsg:
		var cmd tea.Cmd
		m.viewer, cmd = m.viewer.Update(msg)
		if msg.Path == m.selected && !msg.RawOnly && msg.Err == nil && !msg.Large && msg.Rendered == m.viewer.BaseContent() {
			cmd = chainCommands(cmd, m.applyRenderedIndex(msg))
		}
		return m, cmd
	case tableRenderedMsg:
		return m.handleTableRendered(msg)
	case dragTickMsg:
		if !m.dragging || msg.Seq != m.dragSeq {
			return m, nil
		}
		m.dragTickPending = false
		return m, m.renderSplitIfNeeded()
	case tree.SelectionMsg:
		m.selected = msg.Path
		m.stats = viewer.StatsMsg{Path: msg.Path}
		m.resetDocumentIndex()
		return m, m.viewer.Open(msg.Path)
	default:
		var cmd tea.Cmd
		m.viewer, cmd = m.viewer.Update(msg)
		return m, cmd
	}
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.pendingLink != nil {
		switch msg.String() {
		case "y", "Y":
			return m.confirmPendingLink()
		case "n", "N", "esc":
			m.pendingLink = nil
			m.notice = ""
		}
		return m, nil
	}

	switch m.overlay {
	case overlayTable:
		return m.dismissTableOverlay()
	case overlayTablePicker:
		return m.handleTablePickerKey(msg)
	case overlayHelp:
		switch msg.String() {
		case "?", "esc":
			m.overlay = overlayNone
			m.helpBody = ""
		}
		return m, nil
	}

	if m.searchPrompt {
		return m.handleSearchPromptKey(msg)
	}

	m.notice = ""
	if msg.String() == "enter" && m.focus == FocusViewer {
		return m.followVisibleLink()
	}

	switch msg.String() {
	case "q", "ctrl+c":
		m.saveState()
		return m, tea.Quit
	case "/":
		return m.startSearch()
	case "n", "N":
		if m.viewer.SearchActive() {
			return m.jumpSearchMatch(msg.String())
		}
	case "esc":
		if m.viewer.SearchActive() {
			m.viewer.CancelSearch()
			return m, nil
		}
	case "?":
		m = m.openHelp()
		return m, nil
	case "T":
		return m.openTableOverlay()
	case "m":
		return m.toggleMouse()
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
}

// View implements tea.Model.
func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "mdui — " + m.root
	if m.mouse {
		v.MouseMode = tea.MouseModeCellMotion
	} else {
		v.MouseMode = tea.MouseModeNone
	}
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

	rows := []string{m.renderTitle(innerW), body}
	if !m.mouse {
		rows = append(rows, m.renderMouseHint(innerW))
	}
	rows = append(rows, m.renderStatus(innerW))

	base := style.Frame.
		Width(m.width).
		Height(m.height).
		Render(strings.Join(rows, "\n"))

	if m.overlay == overlayHelp && m.helpBody != "" {
		return m.renderHelpOverlay(base)
	}
	if m.overlay == overlayTable {
		return m.renderTableOverlay(base)
	}
	if m.overlay == overlayTablePicker {
		return m.renderTablePickerOverlay(base)
	}
	return base
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

// resizePanes pushes the current pane dimensions into the children after a
// window resize or a mouse-capture change.
func (m *Model) resizePanes() tea.Cmd {
	m.tree.SetHeight(m.paneHeight())
	innerW := m.innerWidth()
	treeW, _, viewerW := SplitWidths(innerW, m.ratio)
	m.lastTreeW = treeW
	return m.viewer.SetSize(viewerW, m.paneHeight())
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
	if m.searchPrompt {
		return m, nil
	}
	if m.overlay == overlayTable {
		return m.dismissTableOverlay()
	}
	if m.overlay != overlayNone {
		return m, nil
	}
	if !m.mouse {
		return m, nil
	}
	mouse := msg.Mouse()
	if mouse.Button != tea.MouseLeft || mouse.Mod.Contains(tea.ModShift) {
		m.clearClick()
		return m, nil
	}

	switch m.regionAt(mouse.X, mouse.Y) {
	case regionDivider:
		m.clearClick()
		m.dragging = true
		m.dragSeq++
		m.dragTickPending = false
		return m, nil
	case regionTree:
		return m.handleTreeClick(mouse.X, mouse.Y)
	case regionViewer:
		m.clearClick()
		return m.handleViewerClick(mouse.Y)
	default:
		m.clearClick()
		return m, nil
	}
}

func (m Model) handleTreeClick(x, y int) (tea.Model, tea.Cmd) {
	row, index, ok := m.tree.RowAt(y-m.bodyTop(), m.paneHeight())
	if !ok || x < m.frameLeft() {
		m.clearClick()
		return m, nil
	}

	now := m.now()
	wasDouble := m.lastClickValid &&
		m.lastClickIndex == index &&
		m.lastClickPath == row.Path &&
		now.Sub(m.lastClickTime) <= doubleClickWindow

	m.lastClickValid = true
	m.lastClickIndex = index
	m.lastClickPath = row.Path
	m.lastClickTime = now

	if wasDouble {
		m.lastClickValid = false
		if row.IsDir {
			return m, m.tree.DescendRow(index)
		}
		return m, m.tree.SelectRow(index)
	}

	if row.IsDir {
		return m, m.tree.ToggleRow(index)
	}
	return m, m.tree.SelectRow(index)
}

func (m *Model) clearClick() {
	m.lastClickValid = false
}

func (m Model) handleMouseMotion(msg tea.MouseMotionMsg) (tea.Model, tea.Cmd) {
	if m.searchPrompt || !m.mouse || m.overlay != overlayNone || !m.dragging {
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
	if m.searchPrompt || !m.dragging {
		return m, nil
	}

	m.dragging = false
	m.dragTickPending = false
	cmd := m.renderSplitIfNeeded()
	m.saveState()
	return m, cmd
}

func (m Model) handleMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	if m.searchPrompt || !m.mouse || m.overlay != overlayNone {
		return m, nil
	}
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

// toggleMouse flips terminal mouse capture and resizes the panes to make room
// for the hint bar.
func (m Model) toggleMouse() (tea.Model, tea.Cmd) {
	m.mouse = !m.mouse
	if !m.mouse {
		m.dragging = false
		m.dragTickPending = false
	}
	return m, m.resizePanes()
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
	rows := titleRows + statusRows
	if !m.mouse {
		rows++
	}
	return m.height - style.Frame.GetVerticalFrameSize() - rows
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
	leftText := m.statusLeft()
	rightText := m.statusRight()

	left := style.StatusLeft.Render(leftText)
	right := style.StatusRight.Render(rightText)

	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 && !m.searchVisible() {
		// Keep the file info; drop the hint when there is no room for both.
		right = ""
		gap = width - lipgloss.Width(left)
	}
	if gap < 0 && m.searchVisible() {
		// Search feedback is more useful than the file label; shrink the
		// left block before dropping current/total.
		available := width - lipgloss.Width(right) - style.StatusLeft.GetHorizontalFrameSize()
		if available >= 1 {
			left = style.StatusLeft.Render(style.Truncate(leftText, available))
			gap = width - lipgloss.Width(left) - lipgloss.Width(right)
		}
	}
	if gap < 0 {
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

func (m Model) statusLeft() string {
	if m.pendingLink != nil {
		return "open " + m.pendingLink.URL + "? y open · n cancel"
	}
	if m.searchPrompt {
		return "/" + m.viewer.SearchQuery() + "▏"
	}
	if m.selected == "" {
		return "no file selected"
	}

	label := m.selectedLabel()
	if m.stats.Path == m.selected && m.stats.Ready {
		label = fmt.Sprintf("%s · %d %s · %d min",
			label, m.stats.Words, plural(m.stats.Words, "word", "words"), m.stats.ReadingMinutes)
	}
	if warning := m.viewer.Warning(); warning != "" {
		label = "⚠ " + warning + " · " + label
	}
	if m.viewer.SearchActive() {
		label += m.searchLabel()
	}
	return label
}

func (m Model) statusRight() string {
	if m.notice != "" {
		return "⚠ " + m.notice
	}
	if m.pendingLink != nil {
		return "confirm open"
	}
	if m.searchPrompt {
		return m.searchPromptStatus()
	}
	if m.viewer.SearchActive() {
		return m.searchCommittedStatus()
	}
	if m.tableClipped && m.selected != "" && !m.viewer.RawMode() {
		return "table clipped — T to view full width"
	}
	if m.degraded() {
		return "resize to ≥80×24"
	}
	hint := "? help · q quit"
	if m.stats.Path == m.selected && m.stats.Ready && m.selected != "" {
		hint = fmt.Sprintf("%d%% · %s", m.stats.ScrollPct, hint)
	}
	return hint
}

func (m Model) selectedLabel() string {
	rel, err := filepath.Rel(m.root, m.selected)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		rel = m.selected
	}
	if strings.ContainsAny(rel, " \t\n\"") {
		return strconv.Quote(rel)
	}
	return rel
}

func (m Model) renderMouseHint(width int) string {
	const text = "mouse off — native selection enabled, press m to re-enable"
	line := " " + style.Truncate(text, max(0, width-2)) + " "
	if pad := width - lipgloss.Width(line); pad > 0 {
		line += strings.Repeat(" ", pad)
	}
	return style.MouseHint.Render(line)
}

func plural(n int, singular, pluralForm string) string {
	if n == 1 {
		return singular
	}
	return pluralForm
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
