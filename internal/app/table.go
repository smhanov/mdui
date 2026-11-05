package app

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"mdui/internal/mdutil"
	"mdui/internal/render"
	"mdui/internal/style"
)

// tableRenderedMsg is the result of rendering the current document at the
// overlay width. It is kept separate from render.RenderedMsg so the viewer's
// request counter and the overlay's never interfere.
type tableRenderedMsg struct {
	ReqID    uint64
	Path     string
	Rendered string
	Err      error
}

// tableRenderCmd runs the existing asynchronous render pipeline at a new
// width. The render cache is keyed by path, mtime, width, style, and emoji, so
// the overlay costs one extra render per width.
func tableRenderCmd(reqID uint64, path string, width int, opts render.Options) tea.Cmd {
	return func() tea.Msg {
		msg, ok := render.NewCmdWith(reqID, path, width, opts)().(render.RenderedMsg)
		if !ok {
			return tableRenderedMsg{ReqID: reqID, Path: path, Err: fmt.Errorf("render returned an unexpected message")}
		}
		return tableRenderedMsg{
			ReqID:    reqID,
			Path:     msg.Path,
			Rendered: msg.Rendered,
			Err:      msg.Err,
		}
	}
}

func validTables(tables []mdutil.Table) []mdutil.Table {
	valid := make([]mdutil.Table, 0, len(tables))
	for _, table := range tables {
		if !table.Malformed {
			valid = append(valid, table)
		}
	}
	return valid
}

func tableLabel(number int, table mdutil.Table) string {
	return fmt.Sprintf("Table %d · %d cols × %d rows", number, table.Columns, table.Rows)
}

// openTableOverlay is the T handler. It captures the scroll position, then
// either opens the table under the viewport or offers a picker when the choice
// is ambiguous or the viewport does not intersect a table.
func (m Model) openTableOverlay() (tea.Model, tea.Cmd) {
	m.tablePriorScroll = m.viewer.ScrollOffset()
	m.notice = ""
	if m.selected == "" {
		m.notice = "no document selected"
		return m, nil
	}
	if m.viewer.RawMode() {
		m.notice = "tables are not available in raw mode"
		return m, nil
	}

	valid := validTables(m.tables)
	if len(valid) == 0 {
		m.notice = "no table in this document"
		return m, nil
	}
	m.tableChoices = valid

	if len(valid) == 1 {
		return m.beginTableRender(0)
	}

	spans := mdutil.TableSpans(m.viewer.Content())
	if len(spans) == 1 && len(valid) > 1 {
		// The rendered document exposed only one table block. Showing it is
		// more useful than a picker that cannot be mapped to a span.
		return m.beginTableRender(0)
	}

	top := m.viewer.ScrollOffset()
	bottom := top + max(1, m.paneHeight()) - 1
	var intersecting []int
	for i, span := range spans {
		if span.Start <= bottom && span.End > top {
			intersecting = append(intersecting, i)
		}
	}
	if len(intersecting) == 1 && len(spans) == len(valid) {
		return m.beginTableRender(intersecting[0])
	}

	m.overlay = overlayTablePicker
	return m, nil
}

// beginTableRender starts the full-width render and shows the loading state.
func (m Model) beginTableRender(index int) (tea.Model, tea.Cmd) {
	if index < 0 || index >= len(m.tableChoices) {
		index = 0
	}
	if len(m.tableChoices) == 0 {
		m.notice = "no table in this document"
		return m, nil
	}
	m.overlay = overlayTable
	m.tablePick = index
	m.tableTitle = tableLabel(index+1, m.tableChoices[index])
	m.tableLoading = true
	m.tableBody = nil
	m.tableError = ""
	m.tableReqID++
	m.tableActiveReq = m.tableReqID

	width := max(1, m.width-4)
	return m, tableRenderCmd(m.tableReqID, m.selected, width, render.Options{
		Style: m.docStyle,
		Emoji: m.emoji,
	})
}

func (m Model) handleTablePickerKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc", "q", "?":
		return m.dismissTableOverlay()
	case "enter":
		if len(m.tableChoices) > 0 {
			return m.beginTableRender(0)
		}
		return m.dismissTableOverlay()
	}
	if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
		index := int(key[0] - '1')
		if index < len(m.tableChoices) {
			return m.beginTableRender(index)
		}
	}
	return m, nil
}

func (m Model) handleTableRendered(msg tableRenderedMsg) (tea.Model, tea.Cmd) {
	if msg.ReqID != m.tableActiveReq || m.overlay != overlayTable {
		return m, nil
	}
	m.tableLoading = false
	if msg.Err != nil {
		m.tableError = msg.Err.Error()
		m.tableBody = nil
		return m, nil
	}
	m.tableBody = m.sliceTable(msg.Rendered)
	return m, nil
}

// sliceTable extracts the chosen table block (and its footnote lines) from
// the full-width render. If the heuristic cannot locate the block it returns
// the document from the table's approximate start, which still shows the
// table at full width.
func (m Model) sliceTable(rendered string) []string {
	lines := strings.Split(rendered, "\n")
	spans := mdutil.TableSpans(rendered)
	index := m.tablePick
	if index >= 0 && index < len(spans) {
		span := mdutil.TableFootnotes(rendered, spans[index])
		if span.Start >= 0 && span.End <= len(lines) && span.Start < span.End {
			return trimTrailingBlank(lines[span.Start:span.End])
		}
	}

	start := 0
	if index >= 0 && index < len(spans) {
		start = spans[index].Start
	}
	if start >= len(lines) {
		start = 0
	}
	return trimTrailingBlank(lines[start:])
}

func trimTrailingBlank(lines []string) []string {
	end := len(lines)
	for end > 0 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return lines[:end]
}

// dismissTableOverlay closes the table overlay, invalidates an in-flight
// render, and restores the exact scroll position captured by T.
func (m Model) dismissTableOverlay() (tea.Model, tea.Cmd) {
	m.overlay = overlayNone
	m.tableReqID++
	m.tableActiveReq = 0
	m.tableLoading = false
	m.tableBody = nil
	m.tableError = ""
	cmd := m.viewer.SetScrollOffset(m.tablePriorScroll)
	return m, cmd
}

func (m Model) renderTableOverlay(base string) string {
	contentW := max(1, m.width-4)
	title := m.tableTitle
	if title == "" {
		title = "Table"
	}

	var body []string
	switch {
	case m.tableError != "":
		body = []string{"⚠ " + m.tableError}
	case m.tableLoading:
		body = []string{"rendering table at full width…"}
	case len(m.tableBody) > 0:
		body = m.tableBody
	default:
		body = []string{"table unavailable"}
	}

	// Reserve the title, spacer, footer, spacer, and two border rows.
	maxBody := max(1, m.height-6)
	if len(body) > maxBody {
		body = body[:maxBody]
		body[len(body)-1] = "… table continues"
	}

	content := []string{style.OverlayTitle.Render(title), ""}
	content = append(content, body...)
	content = append(content, "", "any key closes · scroll position restored")
	return m.renderBoxedOverlay(base, content, contentW)
}

func (m Model) renderTablePickerOverlay(base string) string {
	contentW := min(max(1, m.width-4), 64)
	content := []string{style.OverlayTitle.Render("Choose a table"), ""}
	for i, table := range m.tableChoices {
		line := fmt.Sprintf("%d. Table %d cols × %d rows", i+1, table.Columns, table.Rows)
		content = append(content, line)
	}
	content = append(content, "", "press 1–9 or enter · esc cancels")
	return m.renderBoxedOverlay(base, content, contentW)
}

// applyRenderedIndex rebuilds every coordinate-dependent index after a render
// lands: table footnotes, headings, source tables, and the crushed-table hint.
func (m *Model) applyRenderedIndex(msg render.RenderedMsg) tea.Cmd {
	src := []byte(msg.Raw)
	m.links = mdutil.ParseLinkFootnotes(msg.Rendered)
	m.headings = mdutil.FindHeadings(src)
	m.headingLines = mdutil.LocateHeadings(msg.Rendered, m.headings)
	tables, _ := mdutil.FindTables(src)
	m.tables = tables
	valid := validTables(tables)
	rowCounts := make([]int, len(valid))
	for i, table := range valid {
		rowCounts[i] = table.Rows
	}
	m.tableClipped = len(mdutil.CrushedTablesWithRows(msg.Rendered, rowCounts, 3)) > 0
	m.notice = ""

	if m.pendingAnchor != "" {
		anchor := m.pendingAnchor
		m.pendingAnchor = ""
		if line, ok := m.lineForAnchor(anchor); ok {
			return m.viewer.ScrollToLine(line)
		}
		m.notice = "heading not found: #" + anchor
	}
	return nil
}

func (m *Model) resetDocumentIndex() {
	m.links = nil
	m.headings = nil
	m.headingLines = nil
	m.tables = nil
	m.tableClipped = false
	m.notice = ""
	m.pendingAnchor = ""
	m.searchPrompt = false
	m.viewer.CancelSearch()
}

func (m Model) lineForAnchor(anchor string) (int, bool) {
	heading, ok := mdutil.HeadingBySlug(m.headings, anchor)
	if !ok {
		return 0, false
	}
	for i, candidate := range m.headings {
		if candidate.Slug == heading.Slug && i < len(m.headingLines) && m.headingLines[i] >= 0 {
			return m.headingLines[i], true
		}
	}
	return 0, false
}

func chainCommands(first, second tea.Cmd) tea.Cmd {
	switch {
	case first == nil:
		return second
	case second == nil:
		return first
	default:
		return tea.Batch(first, second)
	}
}
