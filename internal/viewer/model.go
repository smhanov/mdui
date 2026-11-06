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

	"github.com/smhanov/mdui/internal/docsearch"
	"github.com/smhanov/mdui/internal/render"
	"github.com/smhanov/mdui/internal/style"
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

	searchQuery     string
	searchIndex     *docsearch.Index
	searchCommitted bool
	searchCurrent   int

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
		viewport:      viewport.New(),
		docStyle:      opts.Style,
		emoji:         opts.Emoji,
		wrapWidth:     opts.WrapWidth,
		searchCurrent: -1,
		fileCmd:       render.NewCmdWith,
		bytesCmd:      render.BytesCmdWith,
		rawCmd:        render.RawCmd,
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
		m.resetSearch()
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

// Refresh re-renders the current document after its file changed on disk. It
// keeps the current mode, scroll offset, and search state, unlike Open which
// resets them for a new selection. The welcome document is embedded, so there
// is nothing to reload for it.
func (m *Model) Refresh() tea.Cmd {
	if m.path == "" {
		return nil
	}

	if m.rawMode {
		m.rawReqID++
		m.loading = true
		m.applyContent()
		return m.rawCmd(m.rawReqID, m.path)
	}

	m.reqID++
	m.loading = true
	m.applyContent()
	return m.fileCmd(m.reqID, m.path, max(1, m.renderWidth), m.renderOptions())
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
	m.rebuildSearch()
	m.viewport.SetContent(m.searchView())
}

// rebuildSearch is folded into applyContent so every projection change (raw
// toggle, async raw load, width re-render, new document) re-indexes the same
// query against the currently displayed content.
func (m *Model) rebuildSearch() {
	if m.searchQuery == "" {
		m.searchIndex = nil
		m.searchCurrent = -1
		return
	}

	anchor := m.viewport.YOffset()
	if m.searchCurrent >= 0 && m.searchIndex != nil && m.searchCurrent < len(m.searchIndex.Matches) {
		anchor = m.searchIndex.Matches[m.searchCurrent].Line
	}

	m.searchIndex = docsearch.New(m.content(), m.searchQuery)
	if len(m.searchIndex.Matches) == 0 {
		m.searchCurrent = -1
		return
	}
	if m.searchCommitted {
		m.searchCurrent = firstMatchAtOrAfter(m.searchIndex.Matches, anchor)
	} else {
		m.searchCurrent = -1
	}
}

func (m Model) searchView() string {
	if m.searchIndex != nil && len(m.searchIndex.Matches) > 0 {
		return m.searchIndex.Highlight()
	}
	return m.content()
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

// Content returns the text currently projected into the viewport. While a
// query is active this includes reverse-video highlight sequences.
func (m Model) Content() string {
	return m.viewport.GetContent()
}

// BaseContent returns the displayed projection before search highlighting. It
// is the string to compare against a fresh render result.
func (m Model) BaseContent() string {
	return m.content()
}

// BeginSearch clears any previous query and highlights so the status-bar
// prompt starts from an empty projection.
func (m *Model) BeginSearch() {
	m.resetSearch()
	m.applyContent()
}

// SetSearchQuery replaces the query while the prompt is open. Matches and
// highlights update immediately; the search is not committed until
// CommitSearch, so n and N remain inactive.
func (m *Model) SetSearchQuery(query string) {
	m.searchQuery = query
	m.searchCommitted = false
	m.searchCurrent = -1
	m.applyContent()
}

// CommitSearch makes the current query repeatable with n/N, chooses the first
// match at or after the current scroll position (wrapping to the first match),
// and scrolls to it. It returns the total match count and any stats command.
func (m *Model) CommitSearch() (int, tea.Cmd) {
	if m.searchQuery == "" {
		m.resetSearch()
		m.applyContent()
		return 0, nil
	}

	m.searchCommitted = true
	m.applyContent()
	total := m.SearchTotal()
	if total == 0 {
		return 0, m.statsCommand(true)
	}
	m.scrollToCurrentMatch()
	return total, m.statsCommand(true)
}

// CancelSearch clears the query, match index, and highlight projection.
func (m *Model) CancelSearch() {
	m.resetSearch()
	m.applyContent()
}

// SearchQuery returns the current query, including while the prompt is open.
func (m Model) SearchQuery() string {
	return m.searchQuery
}

// SearchActive reports whether a committed query is repeatable with n/N.
func (m Model) SearchActive() bool {
	return m.searchCommitted && m.searchQuery != ""
}

// SearchTotal returns the number of matches in the current projection.
func (m Model) SearchTotal() int {
	if m.searchIndex == nil {
		return 0
	}
	return len(m.searchIndex.Matches)
}

// SearchStatus returns the 0-based current match and the total match count.
// Current is -1 when there are no matches.
func (m Model) SearchStatus() (current, total int) {
	return m.searchCurrent, m.SearchTotal()
}

// NextMatch moves to the next match with wrap-around. It reports whether a
// match was available and returns any stats command.
func (m *Model) NextMatch() (bool, tea.Cmd) {
	total := m.SearchTotal()
	if !m.SearchActive() || total == 0 {
		return false, nil
	}
	if m.searchCurrent < 0 || m.searchCurrent >= total {
		m.searchCurrent = 0
	} else {
		m.searchCurrent = (m.searchCurrent + 1) % total
	}
	m.scrollToCurrentMatch()
	return true, m.statsCommand(true)
}

// PrevMatch moves to the previous match with wrap-around. It reports whether a
// match was available and returns any stats command.
func (m *Model) PrevMatch() (bool, tea.Cmd) {
	total := m.SearchTotal()
	if !m.SearchActive() || total == 0 {
		return false, nil
	}
	if m.searchCurrent <= 0 || m.searchCurrent >= total {
		m.searchCurrent = total - 1
	} else {
		m.searchCurrent--
	}
	m.scrollToCurrentMatch()
	return true, m.statsCommand(true)
}

func (m *Model) resetSearch() {
	m.searchQuery = ""
	m.searchIndex = nil
	m.searchCommitted = false
	m.searchCurrent = -1
}

func (m *Model) scrollToCurrentMatch() {
	if m.searchIndex == nil || m.searchCurrent < 0 || m.searchCurrent >= len(m.searchIndex.Matches) {
		return
	}
	m.viewport.SetYOffset(m.searchIndex.Matches[m.searchCurrent].Line)
}

func firstMatchAtOrAfter(matches []docsearch.Match, line int) int {
	for i, match := range matches {
		if match.Line >= line {
			return i
		}
	}
	return 0
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

// WordCount counts whitespace-separated words in raw markdown. It splits on
// Unicode whitespace like strings.Fields, so an unspaced CJK phrase counts as
// one word.
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
