// Package viewer renders the selected markdown document in the right pane.
package viewer

import (
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"mdui/internal/render"
	"mdui/internal/style"
)

const welcomeSource = "<welcome>"

//go:embed welcome.md
var welcomeMarkdown []byte

// RenderedMsg is the render result handled by the viewer.
type RenderedMsg = render.RenderedMsg

// StatsMsg reports the status-bar figures for the open document.
type StatsMsg struct {
	Path           string
	Words          int
	ReadingMinutes int
	ScrollPct      int
	Ready          bool
}

// Options configures a viewer.
type Options struct {
	Style     render.Style
	Emoji     bool
	WrapWidth int
}

// Model is the markdown viewer pane. It owns the viewport, the raw and
// rendered copies of the current document, and the request counter that keeps
// stale asynchronous renders out of the pane.
type Model struct {
	viewport  viewport.Model
	docStyle  render.Style
	emoji     bool
	wrapWidth int

	renderWidth int

	path    string
	source  string
	raw     string
	warning string

	reqID    uint64
	rawReqID uint64

	rendered  string
	rawMode   bool
	large     bool
	largeSize int64
	loading   bool
	err       error

	stats StatsMsg

	fileCmd  func(reqID uint64, path string, width int, opts render.Options) tea.Cmd
	bytesCmd func(reqID uint64, name string, data []byte, width int, opts render.Options) tea.Cmd
	rawCmd   func(reqID uint64, path string) tea.Cmd
}

// New returns a viewer. An optional style selects the Glamour stylesheet; the
// default is dark.
func New(docStyles ...render.Style) Model {
	opts := Options{Style: render.Dark}
	if len(docStyles) > 0 && docStyles[0] != "" {
		opts.Style = docStyles[0]
	}
	return NewWithOptions(opts)
}

// NewWithOptions returns a viewer with the full renderer configuration.
func NewWithOptions(opts Options) Model {
	if opts.Style == "" {
		opts.Style = render.Dark
	}
	if opts.WrapWidth < 0 {
		opts.WrapWidth = 0
	}
	m := Model{
		viewport:  viewport.New(),
		docStyle:  opts.Style,
		emoji:     opts.Emoji,
		wrapWidth: opts.WrapWidth,
		fileCmd:   render.NewCmdWith,
		bytesCmd:  render.BytesCmdWith,
		rawCmd:    render.RawCmd,
	}
	m.viewport.SoftWrap = true
	m.viewport.FillHeight = true
	m.viewport.SetContent("no file selected")
	return m
}

// SetSize updates the viewer's outer dimensions and returns a command that
// re-renders at the new inner width. Height-only changes do not re-render.
func (m *Model) SetSize(width, height int) tea.Cmd {
	innerW := max(0, m.innerWidth(max(0, width)))
	innerH := max(0, m.innerHeight(max(0, height)))
	m.viewport.SetWidth(innerW)
	m.viewport.SetHeight(innerH)

	if innerW < 1 || innerH < 1 {
		return nil
	}
	renderW := m.renderWidthFor(innerW)
	if renderW == m.renderWidth {
		return nil
	}
	m.renderWidth = renderW

	if m.path == "" {
		return m.requestWelcome()
	}
	return m.requestRender()
}

// Open selects path and starts an asynchronous render. The viewer keeps its
// current content while a re-render of the same file is in flight, but clears
// it immediately when the selection changes so another file's content is
// never shown.
func (m *Model) Open(path string) tea.Cmd {
	if path == "" {
		return nil
	}

	if path != m.path {
		m.rendered = ""
		m.raw = ""
		m.large = false
		m.largeSize = 0
		m.err = nil
		m.warning = ""
		m.stats = StatsMsg{Path: path}
		m.viewport.GotoTop()
	}

	m.path = path
	m.source = path
	m.rawMode = false
	m.reqID++
	m.rawReqID++
	m.loading = true
	m.applyContent()

	return m.fileCmd(m.reqID, path, max(1, m.renderWidth), m.renderOptions())
}

// Update handles render results and viewer keybindings. Scroll keys are
// forwarded to the viewport and never start a render.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case render.RenderedMsg:
		return m.handleRendered(msg)
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	default:
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, batch(cmd, m.statsCommand(false))
	}
}

func (m Model) handleRendered(msg render.RenderedMsg) (Model, tea.Cmd) {
	if msg.RawOnly {
		if msg.ReqID != m.rawReqID || msg.Path != m.source {
			return m, nil
		}
		m.loading = false
		if msg.Err != nil {
			m.handleRenderError(msg.Err)
			return m, nil
		}
		m.err = nil
		m.warning = ""
		m.raw = msg.Raw
		m.applyContent()
		return m, m.statsCommand(true)
	}

	if msg.ReqID != m.reqID || msg.Path != m.source {
		return m, nil
	}

	m.loading = false
	if msg.Err != nil {
		m.handleRenderError(msg.Err)
		return m, nil
	}

	m.err = nil
	m.warning = ""
	if msg.Large {
		m.large = true
		m.largeSize = msg.Size
		m.rendered = ""
		m.raw = ""
	} else {
		m.large = false
		m.largeSize = 0
		m.rendered = msg.Rendered
		m.raw = msg.Raw
	}
	m.applyContent()
	return m, m.statsCommand(true)
}

// handleRenderError distinguishes a file that vanished after it was rendered
// (keep the last good content and warn) from a file that could not be read at
// all (show the error inline).
func (m *Model) handleRenderError(err error) {
	if errors.Is(err, fs.ErrNotExist) && (m.rendered != "" || m.raw != "") {
		m.warning = "file no longer available"
		m.err = nil
		m.applyContent()
		return
	}
	m.warning = ""
	m.err = err
	m.applyContent()
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "g":
		m.viewport.GotoTop()
		return m, batch(nil, m.statsCommand(false))
	case "G":
		m.viewport.GotoBottom()
		return m, batch(nil, m.statsCommand(false))
	case "r":
		return m.toggleRaw()
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, batch(cmd, m.statsCommand(false))
}

func (m Model) toggleRaw() (Model, tea.Cmd) {
	if m.path == "" {
		return m, nil
	}

	if m.rawMode {
		m.rawMode = false
		m.applyContent()
		return m, batch(nil, m.statsCommand(false))
	}

	m.rawMode = true
	if m.raw == "" && !m.loading {
		m.rawReqID++
		m.loading = true
		m.applyContent()
		return m, m.rawCmd(m.rawReqID, m.path)
	}
	m.applyContent()
	return m, batch(nil, m.statsCommand(false))
}

func (m *Model) requestWelcome() tea.Cmd {
	m.reqID++
	m.path = ""
	m.source = welcomeSource
	m.raw = ""
	m.rawMode = false
	m.large = false
	m.largeSize = 0
	m.err = nil
	m.warning = ""
	m.loading = true
	m.applyContent()

	return m.bytesCmd(m.reqID, welcomeSource, welcomeMarkdown, m.renderWidth, m.renderOptions())
}

func (m *Model) requestRender() tea.Cmd {
	m.reqID++
	m.loading = true
	m.applyContent()
	return m.fileCmd(m.reqID, m.path, m.renderWidth, m.renderOptions())
}

func (m *Model) applyContent() {
	m.viewport.SetContent(m.content())
}

func (m Model) content() string {
	switch {
	case m.err != nil:
		return "⚠ " + cleanError(m.err)
	case m.rawMode:
		if m.raw != "" {
			return expandTabs(m.raw)
		}
		if m.loading {
			return "loading raw source…"
		}
		return "no content"
	case m.large:
		name := filepath.Base(m.path)
		return fmt.Sprintf("⚠ %s is %s — too large to render.\n\nPress r to view the raw source.", name, humanSize(m.largeSize))
	case m.rendered != "":
		return m.rendered
	case m.loading:
		return "rendering…"
	default:
		if m.path == "" {
			return "no file selected"
		}
		return "empty document"
	}
}

// View renders the viewport at exactly width by height cells, including the
// pane's one-cell horizontal padding.
func (m Model) View(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}

	m.viewport.SetWidth(max(1, m.innerWidth(width)))
	m.viewport.SetHeight(max(1, m.innerHeight(height)))

	return style.Pane.
		Width(width).
		Height(height).
		MaxHeight(height).
		Render(m.viewport.View())
}

// Content returns the text currently projected into the viewport.
func (m Model) Content() string {
	return m.viewport.GetContent()
}

// RawMode reports whether the pane is showing raw markdown.
func (m Model) RawMode() bool {
	return m.rawMode
}

// ScrollOffset returns the first visible line in the viewport.
func (m Model) ScrollOffset() int {
	return m.viewport.YOffset()
}

// SetScrollOffset restores a previously captured viewport offset and refreshes
// the status-bar figures.
func (m *Model) SetScrollOffset(offset int) tea.Cmd {
	m.viewport.SetYOffset(offset)
	return m.statsCommand(true)
}

// ScrollToLine puts the given rendered line at the top of the viewport and
// refreshes the status-bar figures.
func (m *Model) ScrollToLine(line int) tea.Cmd {
	m.viewport.SetYOffset(line)
	return m.statsCommand(true)
}

// Warning returns the status-bar notice for a file that vanished after its
// last successful render.
func (m Model) Warning() string {
	return m.warning
}

// Stats returns the current status-bar figures. Word counts come from the raw
// markdown and the scroll percentage from the viewport offset.
func (m Model) Stats() StatsMsg {
	words := WordCount(m.raw)
	pct := int(math.Round(m.viewport.ScrollPercent() * 100))
	return StatsMsg{
		Path:           m.path,
		Words:          words,
		ReadingMinutes: ReadingMinutes(words),
		ScrollPct:      clampPct(pct),
		Ready:          true,
	}
}

// WordCount counts whitespace-separated words in raw markdown. It matches
// wc -w for the whitespace that appears in ordinary documents.
func WordCount(raw string) int {
	return len(strings.Fields(raw))
}

// ReadingMinutes estimates reading time at 200 words per minute, with a
// minimum of one minute.
func ReadingMinutes(words int) int {
	if words <= 0 {
		return 1
	}
	return (words + 199) / 200
}

func (m *Model) statsCommand(force bool) tea.Cmd {
	if m.loading {
		return nil
	}
	stats := m.Stats()
	if stats.Path == "" {
		return nil
	}
	changed := stats != m.stats
	m.stats = stats
	if !force && !changed {
		return nil
	}
	return func() tea.Msg { return stats }
}

func (m Model) renderOptions() render.Options {
	return render.Options{Style: m.docStyle, Emoji: m.emoji}
}

func (m Model) renderWidthFor(inner int) int {
	if m.wrapWidth > 0 && m.wrapWidth < inner {
		return m.wrapWidth
	}
	return inner
}

func (m Model) innerWidth(width int) int {
	return width - style.Pane.GetHorizontalFrameSize()
}

func (m Model) innerHeight(height int) int {
	return height - style.Pane.GetVerticalFrameSize()
}

func cleanError(err error) string {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Err.Error()
	}
	return err.Error()
}

func clampPct(pct int) int {
	if pct < 0 {
		return 0
	}
	if pct > 100 {
		return 100
	}
	return pct
}

func batch(first, second tea.Cmd) tea.Cmd {
	switch {
	case first == nil:
		return second
	case second == nil:
		return first
	default:
		return tea.Batch(first, second)
	}
}

func expandTabs(s string) string {
	return strings.ReplaceAll(s, "\t", "    ")
}

func humanSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGT"[exp])
}
