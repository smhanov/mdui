package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"mdui/internal/mdutil"
)

const appWideTable = `# Wide table

Intro text that explains the table.

| Service | Owner | Status | Latency | Cost | Notes |
|:--|:--|:--|--:|--:|:--|
| gateway | platform team | healthy | 23 ms | $120 | routes all public traffic through the edge |
| billing | finance squad | degraded | 450 ms | $80 | retrying invoices after the outage |
| search | discovery | healthy | 18 ms | $300 | vector index warmup still running |
| identity | security | healthy | 9 ms | $45 | keys rotated weekly |

After the table.
`

func appFixtureModel(t *testing.T, files map[string]string, width, height int, opts ...Option) Model {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	opts = append(opts, WithStatePath(filepath.Join(t.TempDir(), "state.toml")))
	m := New(root, opts...)
	next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return next.(Model)
}

func TestCrushedTableHint(t *testing.T) {
	m := appFixtureModel(t, map[string]string{"a-wide.md": appWideTable}, 100, 30)
	m = clickRow(t, m, 1)
	if !m.tableClipped {
		t.Fatalf("wide table was not flagged\n%s", ansi.Strip(m.viewer.Content()))
	}
	status := ansi.Strip(m.renderStatus(300))
	if !strings.Contains(status, "table clipped — T to view full width") {
		t.Fatalf("status bar missing crushed-table hint:\n%s", status)
	}

	small := appFixtureModel(t, map[string]string{
		"a-small.md": "# Small\n\n| A | B |\n|---|---|\n| one | two |\n",
	}, 140, 30)
	small = clickRow(t, small, 1)
	if small.tableClipped {
		t.Fatal("normal table was flagged as crushed")
	}
	if strings.Contains(ansi.Strip(small.renderStatus(300)), "table clipped") {
		t.Fatal("crushed hint shown for a normal table")
	}
}

func TestTableOverlayRendersFullWidthAndRestoresScroll(t *testing.T) {
	longDoc := appWideTable + strings.Repeat("filler paragraph that keeps the document scrollable\n\n", 80)
	m := appFixtureModel(t, map[string]string{"a-wide.md": longDoc}, 100, 30)
	m = clickRow(t, m, 1)

	m.viewer.SetScrollOffset(4)
	prior := m.viewer.ScrollOffset()
	if prior == 0 {
		t.Fatal("could not set up a non-zero scroll position")
	}

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'T', Text: "T"})
	m = next.(Model)
	if m.overlay != overlayTable {
		t.Fatalf("T overlay = %v, want table", m.overlay)
	}
	if cmd == nil {
		t.Fatal("T did not start the full-width render")
	}
	m = applyCmd(t, m, cmd)
	if m.tableLoading {
		t.Fatal("table overlay is still loading after its render landed")
	}
	body := ansi.Strip(strings.Join(m.tableBody, "\n"))
	if !strings.Contains(body, "Service") || !strings.Contains(body, "gateway") {
		t.Fatalf("overlay does not contain the table block:\n%s", body)
	}
	out := ansi.Strip(m.render())
	if !strings.Contains(out, "Table 1 · 6 cols × 4 rows") {
		t.Fatalf("overlay title missing:\n%s", out)
	}
	if got := maxLineWidth(m.render()); got != 100 {
		t.Fatalf("overlay render width = %d, want 100", got)
	}
	if !strings.Contains(ansi.Strip(m.render()), "any key closes") {
		t.Fatal("overlay footer missing")
	}

	next, cmd = m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = next.(Model)
	if m.overlay != overlayNone {
		t.Fatal("unrelated key did not dismiss the table overlay")
	}
	m = applyCmd(t, m, cmd)
	if got := m.viewer.ScrollOffset(); got != prior {
		t.Fatalf("scroll offset after dismiss = %d, want %d", got, prior)
	}
}

func TestTableOverlayPickerChoosesSecondTable(t *testing.T) {
	doc := `# Two tables

| A | B |
|---|---|
| one | two |
| three | four |

| C | D |
|---|---|
| five | six |
| seven | eight |
`
	m := appFixtureModel(t, map[string]string{"a-two.md": doc}, 100, 30)
	m = clickRow(t, m, 1)

	next, _ := m.Update(tea.KeyPressMsg{Code: 'T', Text: "T"})
	m = next.(Model)
	if m.overlay != overlayTablePicker {
		t.Fatalf("overlay = %v, want picker", m.overlay)
	}
	picker := ansi.Strip(m.render())
	if !strings.Contains(picker, "1. Table 2 cols × 2 rows") ||
		!strings.Contains(picker, "2. Table 2 cols × 2 rows") {
		t.Fatalf("picker does not list both tables:\n%s", picker)
	}

	next, cmd := m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	m = next.(Model)
	if m.overlay != overlayTable {
		t.Fatalf("choosing table 2 left overlay at %v", m.overlay)
	}
	if m.tablePick != 1 {
		t.Fatalf("tablePick = %d, want 1", m.tablePick)
	}
	m = applyCmd(t, m, cmd)
	body := ansi.Strip(strings.Join(m.tableBody, "\n"))
	if !strings.Contains(body, "C") || strings.Contains(body, "one") {
		t.Fatalf("overlay shows the wrong table:\n%s", body)
	}
	if !strings.Contains(ansi.Strip(m.render()), "Table 2 · 2 cols × 2 rows") {
		t.Fatalf("second table title missing:\n%s", ansi.Strip(m.render()))
	}
}

func TestTableOverlayWithoutTablesShowsNotice(t *testing.T) {
	m := appFixtureModel(t, map[string]string{"a-plain.md": "# Plain\n\nno tables\n"}, 100, 30)
	m = clickRow(t, m, 1)
	next, _ := m.Update(tea.KeyPressMsg{Code: 'T', Text: "T"})
	m = next.(Model)
	if m.overlay != overlayNone {
		t.Fatalf("T opened %v without a table", m.overlay)
	}
	if !strings.Contains(m.notice, "no table") {
		t.Fatalf("notice = %q", m.notice)
	}
	if !strings.Contains(ansi.Strip(m.renderStatus(300)), "no table") {
		t.Fatal("status bar does not show the no-table notice")
	}
}

func TestClickRelativeMarkdownLinkOpensInApp(t *testing.T) {
	m := appFixtureModel(t, map[string]string{
		"a-links.md": "# Links\n\n| Name | Target |\n|:--|:--|\n| other | [other](./b-other.md) |\n",
		"b-other.md": "# Other document\n\nbody\n",
	}, 100, 30)
	m = clickRow(t, m, 1)
	links := mdutil.ParseLinkFootnotes(m.viewer.Content())
	if len(links) != 1 {
		t.Fatalf("links = %d, want 1", len(links))
	}

	m = clickLink(t, m, links[0])
	want := filepath.Join(m.root, "b-other.md")
	if m.selected != want {
		t.Fatalf("selected = %q, want %q", m.selected, want)
	}
	if !strings.Contains(ansi.Strip(m.viewer.Content()), "Other document") {
		t.Fatalf("in-app navigation did not render the target:\n%s", m.viewer.Content())
	}
}

func TestClickAnchorLinkScrollsToHeading(t *testing.T) {
	var body strings.Builder
	for i := 0; i < 60; i++ {
		fmt.Fprintf(&body, "paragraph %d with enough text to occupy a rendered line\n\n", i)
	}
	doc := "# Anchors\n\n| Name | Target |\n|:--|:--|\n| jump | [jump](#bottom) |\n\n" +
		body.String() + "## Bottom\n\narrived\n\n" + strings.Repeat("trailing filler paragraph\n\n", 40)
	m := appFixtureModel(t, map[string]string{"a-anchors.md": doc}, 100, 30)
	m = clickRow(t, m, 1)

	links := mdutil.ParseLinkFootnotes(m.viewer.Content())
	if len(links) != 1 {
		t.Fatalf("links = %d, want 1", len(links))
	}
	m = clickLink(t, m, links[0])
	if m.viewer.ScrollOffset() == 0 {
		t.Fatal("anchor click did not scroll")
	}
	lines := strings.Split(ansi.Strip(m.viewer.Content()), "\n")
	offset := m.viewer.ScrollOffset()
	if offset >= len(lines) || !strings.Contains(lines[offset], "Bottom") {
		t.Fatalf("offset %d does not point at the heading: %q", offset, lines[min(offset, len(lines)-1)])
	}
}

func TestRemoteLinkConfirmAndCancel(t *testing.T) {
	var opened []string
	m := appFixtureModel(t, map[string]string{
		"a-remote.md": "# Remote\n\n| Name | Target |\n|:--|:--|\n| site | [site](https://example.com/path) |\n",
	}, 100, 30, WithOpener(func(url string) error {
		opened = append(opened, url)
		return nil
	}))
	m = clickRow(t, m, 1)
	links := mdutil.ParseLinkFootnotes(m.viewer.Content())
	if len(links) != 1 {
		t.Fatalf("links = %d, want 1", len(links))
	}

	m = clickLink(t, m, links[0])
	if m.pendingLink == nil {
		t.Fatal("remote link did not ask for confirmation")
	}
	if !strings.Contains(ansi.Strip(m.statusLeft()), "y open") {
		t.Fatalf("confirmation prompt missing: %q", m.statusLeft())
	}
	if len(opened) != 0 {
		t.Fatal("click opened the remote URL without confirmation")
	}

	m = pressKey(t, m, tea.KeyPressMsg{Code: 'n', Text: "n"})
	if m.pendingLink != nil {
		t.Fatal("n did not cancel the remote-link prompt")
	}
	if len(opened) != 0 {
		t.Fatal("n opened the remote URL")
	}

	m = clickLink(t, m, links[0])
	m = pressKey(t, m, tea.KeyPressMsg{Code: 'y', Text: "y"})
	if len(opened) != 1 || opened[0] != "https://example.com/path" {
		t.Fatalf("opened = %v, want the confirmed URL", opened)
	}
	if m.pendingLink != nil {
		t.Fatal("confirmation prompt remained after y")
	}
}

func TestRemoteLinkOpenerErrorIsReported(t *testing.T) {
	m := appFixtureModel(t, map[string]string{
		"a-remote.md": "# Remote\n\n| Name | Target |\n|:--|:--|\n| site | [site](https://example.com/x) |\n",
	}, 100, 30, WithOpener(func(string) error {
		return fmt.Errorf("no browser")
	}))
	m = clickRow(t, m, 1)
	links := mdutil.ParseLinkFootnotes(m.viewer.Content())
	m = clickLink(t, m, links[0])
	m = pressKey(t, m, tea.KeyPressMsg{Code: 'y', Text: "y"})
	if !strings.Contains(m.notice, "could not open link") {
		t.Fatalf("opener error notice = %q", m.notice)
	}
	if !strings.Contains(ansi.Strip(m.renderStatus(300)), "could not open link") {
		t.Fatal("status bar does not show the opener error")
	}
}

func TestUnsupportedSchemeIsRefused(t *testing.T) {
	calls := 0
	m := appFixtureModel(t, map[string]string{
		"a-mail.md": "# Mail\n\n| Name | Target |\n|:--|:--|\n| mail | [mail](mailto:a@example.com) |\n",
	}, 100, 30, WithOpener(func(string) error {
		calls++
		return nil
	}))
	m = clickRow(t, m, 1)
	links := mdutil.ParseLinkFootnotes(m.viewer.Content())
	m = clickLink(t, m, links[0])
	if m.pendingLink != nil {
		t.Fatal("mailto link asked for remote confirmation")
	}
	if calls != 0 {
		t.Fatal("mailto link reached the system opener")
	}
	if !strings.Contains(m.notice, "unsupported link") {
		t.Fatalf("notice = %q", m.notice)
	}
}

func TestEnterFollowsFirstVisibleLink(t *testing.T) {
	m := appFixtureModel(t, map[string]string{
		"a-links.md": "# Links\n\n| Name | Target |\n|:--|:--|\n| other | [other](./b-other.md) |\n",
		"b-other.md": "# Other\n",
	}, 100, 30)
	m = clickRow(t, m, 1)
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyTab})
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if filepath.Base(m.selected) != "b-other.md" {
		t.Fatalf("enter selected %q, want b-other.md", m.selected)
	}
}

func TestMalformedTableDocumentDoesNotCrash(t *testing.T) {
	doc := "# Broken\n\n| A | B |\n| --- |\n| one | two |\n\nrest of the document\n"
	m := appFixtureModel(t, map[string]string{"a-broken.md": doc}, 100, 30)
	m = clickRow(t, m, 1)
	if m.viewer.Content() == "" {
		t.Fatal("malformed table document rendered empty")
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: 'T', Text: "T"})
	m = next.(Model)
	if m.overlay != overlayNone {
		t.Fatalf("T opened %v for a malformed table", m.overlay)
	}
}

func clickLink(t *testing.T, m Model, link mdutil.Link) Model {
	t.Helper()
	y := m.bodyTop() + link.Line - m.viewer.ScrollOffset()
	if y < m.bodyTop() || y >= m.bodyTop()+m.paneHeight() {
		t.Fatalf("link line %d is outside the viewport (offset %d)", link.Line, m.viewer.ScrollOffset())
	}
	next, cmd := m.Update(tea.MouseClickMsg{X: m.width - 5, Y: y, Button: tea.MouseLeft})
	m = next.(Model)
	return applyCmd(t, m, cmd)
}

func TestTableOverlayPickerWhenViewportHasNoTable(t *testing.T) {
	doc := "# Two tables\n\n| A | B |\n|---|---|\n| one | two |\n\n" +
		"| C | D |\n|---|---|\n| three | four |\n\n" +
		strings.Repeat("filler paragraph that pushes the tables off screen\n\n", 60)
	m := appFixtureModel(t, map[string]string{"a-two.md": doc}, 100, 30)
	m = clickRow(t, m, 1)
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyTab})
	m = pressKey(t, m, tea.KeyPressMsg{Code: 'G', Text: "G"})
	if m.viewer.ScrollOffset() == 0 {
		t.Fatal("could not scroll away from the tables")
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: 'T', Text: "T"})
	m = next.(Model)
	if m.overlay != overlayTablePicker {
		t.Fatalf("overlay = %v, want picker when no table is visible", m.overlay)
	}
}
