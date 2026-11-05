// Package viewer renders the selected markdown document in the right pane.
package viewer

import (
	_ "embed"
	"fmt"
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

// Model is the markdown viewer pane. It owns the viewport, the raw and
// rendered copies of the current document, and the request counter that keeps
// stale asynchronous renders out of the pane.
type Model struct {
	viewport viewport.Model
	docStyle render.Style

	renderWidth int

	path   string
	source string

	reqID    uint64
	rawReqID uint64

	rendered  string
	raw       string
	rawMode   bool
	large     bool
	largeSize int64
	loading   bool
	err       error

	fileCmd  func(reqID uint64, path string, width int, style render.Style) tea.Cmd
	bytesCmd func(reqID uint64, name string, data []byte, width int, style render.Style) tea.Cmd
	rawCmd   func(reqID uint64, path string) tea.Cmd
}

// New returns a viewer. An optional style selects the Glamour stylesheet; the
// default is dark.
func New(docStyles ...render.Style) Model {
	docStyle := render.Dark
	if len(docStyles) > 0 && docStyles[0] != "" {
		docStyle = docStyles[0]
	}
	m := Model{
		viewport: viewport.New(),
		docStyle: docStyle,
		fileCmd:  render.NewCmd,
		bytesCmd: render.BytesCmd,
		rawCmd:   render.RawCmd,
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
	if innerW == m.renderWidth {
		return nil
	}
	m.renderWidth = innerW

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
		m.viewport.GotoTop()
	}

	m.path = path
	m.source = path
	m.rawMode = false
	m.reqID++
	m.rawReqID++
	m.loading = true
	m.applyContent()

	return m.fileCmd(m.reqID, path, max(1, m.renderWidth), m.docStyle)
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
		return m, cmd
	}
}

func (m Model) handleRendered(msg render.RenderedMsg) (Model, tea.Cmd) {
	if msg.RawOnly {
		if msg.ReqID != m.rawReqID || msg.Path != m.source {
			return m, nil
		}
		m.loading = false
		m.err = msg.Err
		if msg.Err == nil {
			m.raw = msg.Raw
		}
		m.applyContent()
		return m, nil
	}

	if msg.ReqID != m.reqID || msg.Path != m.source {
		return m, nil
	}

	m.loading = false
	m.err = msg.Err
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
	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "g":
		m.viewport.GotoTop()
		return m, nil
	case "G":
		m.viewport.GotoBottom()
		return m, nil
	case "r":
		return m.toggleRaw()
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m Model) toggleRaw() (Model, tea.Cmd) {
	if m.path == "" {
		return m, nil
	}

	if m.rawMode {
		m.rawMode = false
		m.applyContent()
		return m, nil
	}

	m.rawMode = true
	if m.raw == "" && !m.loading {
		m.rawReqID++
		m.loading = true
		m.applyContent()
		return m, m.rawCmd(m.rawReqID, m.path)
	}
	m.applyContent()
	return m, nil
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
	m.loading = true
	m.applyContent()

	return m.bytesCmd(m.reqID, welcomeSource, welcomeMarkdown, m.renderWidth, m.docStyle)
}

func (m *Model) requestRender() tea.Cmd {
	m.reqID++
	m.loading = true
	m.applyContent()
	return m.fileCmd(m.reqID, m.path, m.renderWidth, m.docStyle)
}

func (m *Model) applyContent() {
	m.viewport.SetContent(m.content())
}

func (m Model) content() string {
	switch {
	case m.err != nil:
		return "⚠ " + m.err.Error()
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

// ScrollOffset returns the first visible line in the viewport.
func (m Model) ScrollOffset() int {
	return m.viewport.YOffset()
}

func (m Model) innerWidth(width int) int {
	return width - style.Pane.GetHorizontalFrameSize()
}

func (m Model) innerHeight(height int) int {
	return height - style.Pane.GetVerticalFrameSize()
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
