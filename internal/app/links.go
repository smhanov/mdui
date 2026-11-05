package app

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	tea "charm.land/bubbletea/v2"

	"github.com/smhanov/mdui/internal/mdutil"
	"github.com/smhanov/mdui/internal/viewer"
)

// handleViewerClick maps a terminal row in the viewer pane to a rendered line
// and follows the footnote link on that line, if any.
func (m Model) handleViewerClick(y int) (tea.Model, tea.Cmd) {
	if m.viewer.RawMode() {
		return m, nil
	}
	row := y - m.bodyTop()
	if row < 0 || row >= m.paneHeight() {
		return m, nil
	}
	line := m.viewer.ScrollOffset() + row
	link, ok := mdutil.LinkAtLine(m.links, line)
	if !ok {
		return m, nil
	}
	return m.followLink(link)
}

// followVisibleLink is the Enter path for footnote links: it follows the
// first link line currently visible in the viewport. Clicking remains the
// primary interaction; Enter is the keyboard reachability fallback.
func (m Model) followVisibleLink() (tea.Model, tea.Cmd) {
	if m.viewer.RawMode() {
		return m, nil
	}
	top := m.viewer.ScrollOffset()
	bottom := top + max(1, m.paneHeight())
	for _, link := range m.links {
		if link.Line >= top && link.Line < bottom {
			return m.followLink(link)
		}
	}
	m.notice = "no visible link"
	return m, nil
}

func (m Model) followLink(link mdutil.Link) (tea.Model, tea.Cmd) {
	if link.Truncated {
		m.notice = "link is truncated — widen the pane"
		return m, nil
	}

	current := m.selected
	target, err := mdutil.ResolveLink(current, link.URL)
	if err != nil {
		m.notice = err.Error()
		return m, nil
	}

	switch target.Kind {
	case mdutil.LinkRemote:
		pending := target
		m.pendingLink = &pending
		m.notice = ""
		return m, nil
	case mdutil.LinkAnchor:
		return m.scrollToAnchor(target.Anchor)
	case mdutil.LinkFile:
		return m.openLinkedFile(target)
	default:
		m.notice = "unsupported link — only .md, #anchors, and http(s) open"
		return m, nil
	}
}

func (m Model) scrollToAnchor(anchor string) (tea.Model, tea.Cmd) {
	line, ok := m.lineForAnchor(anchor)
	if !ok {
		m.notice = "heading not found: #" + anchor
		return m, nil
	}
	return m, m.viewer.ScrollToLine(line)
}

func (m Model) openLinkedFile(target mdutil.LinkTarget) (tea.Model, tea.Cmd) {
	info, err := os.Stat(target.Path)
	if err != nil {
		m.notice = "link target not found: " + target.Path
		return m, nil
	}
	if info.IsDir() {
		m.notice = "link target is a directory"
		return m, nil
	}

	m.selected = target.Path
	m.stats = viewer.StatsMsg{Path: target.Path}
	m.resetDocumentIndex()
	m.pendingAnchor = target.Anchor
	return m, m.viewer.Open(target.Path)
}

// confirmPendingLink is the y handler for the status-bar remote-link prompt.
func (m Model) confirmPendingLink() (tea.Model, tea.Cmd) {
	if m.pendingLink == nil {
		return m, nil
	}
	target := *m.pendingLink
	m.pendingLink = nil

	// Re-check the scheme immediately before exec: only user-confirmed
	// http(s) URLs ever reach the system opener.
	if mdutil.ClassifyLink(target.URL) != mdutil.LinkRemote {
		m.notice = "unsupported link — nothing was opened"
		return m, nil
	}
	if m.opener == nil {
		m.notice = "no system opener available"
		return m, nil
	}
	if err := m.opener(target.URL); err != nil {
		m.notice = "could not open link: " + err.Error()
		return m, nil
	}
	m.notice = "opened link"
	return m, nil
}

// systemOpener launches the platform URL handler. It is only ever called for
// user-confirmed http(s) URLs, with the URL passed as a single argv element.
func systemOpener(target string) error {
	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return fmt.Errorf("%s not found", name)
	}
	cmd := exec.Command(path, target)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		_ = cmd.Wait()
	}()
	return nil
}
