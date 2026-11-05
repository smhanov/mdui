package viewer

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/smhanov/mdui/internal/render"
)

type request struct {
	reqID uint64
	path  string
	width int
	style render.Style
	emoji bool
}

type fakeRenderer struct {
	calls    []request
	rawCalls []request

	render func(request) string
	raw    string
	large  bool
	size   int64
	err    error
}

func (f *fakeRenderer) file(reqID uint64, path string, width int, opts render.Options) tea.Cmd {
	req := request{reqID: reqID, path: path, width: width, style: opts.Style, emoji: opts.Emoji}
	f.calls = append(f.calls, req)
	return func() tea.Msg { return f.result(req) }
}

func (f *fakeRenderer) embedded(reqID uint64, name string, _ []byte, width int, opts render.Options) tea.Cmd {
	req := request{reqID: reqID, path: name, width: width, style: opts.Style, emoji: opts.Emoji}
	f.calls = append(f.calls, req)
	return func() tea.Msg { return f.result(req) }
}

func (f *fakeRenderer) rawFile(reqID uint64, path string) tea.Cmd {
	req := request{reqID: reqID, path: path}
	f.rawCalls = append(f.rawCalls, req)
	return func() tea.Msg {
		return render.RenderedMsg{ReqID: reqID, Path: path, Raw: f.raw, RawOnly: true, Err: f.err}
	}
}

func (f *fakeRenderer) result(req request) render.RenderedMsg {
	msg := render.RenderedMsg{
		ReqID: req.reqID,
		Path:  req.path,
		Width: req.width,
		Style: req.style,
		Raw:   f.raw,
		Err:   f.err,
	}
	if f.large {
		msg.Large = true
		msg.Size = f.size
		msg.Raw = ""
		return msg
	}
	msg.Rendered = "rendered: " + req.path
	if f.render != nil {
		msg.Rendered = f.render(req)
	}
	return msg
}

func newTestViewer(f *fakeRenderer) Model {
	m := New(render.Dark)
	m.fileCmd = f.file
	m.bytesCmd = f.embedded
	m.rawCmd = f.rawFile
	return m
}

func apply(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	msg := cmd()
	if msg == nil {
		return m
	}
	next, _ := m.Update(msg)
	return next
}

func press(s string) tea.KeyPressMsg {
	switch s {
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "pgup":
		return tea.KeyPressMsg{Code: tea.KeyPgUp}
	case "pgdown":
		return tea.KeyPressMsg{Code: tea.KeyPgDown}
	case "ctrl+d":
		return tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}
	case "ctrl+u":
		return tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}
	default:
		runes := []rune(s)
		return tea.KeyPressMsg{Code: runes[0], Text: s}
	}
}

func TestStaleRenderDropped(t *testing.T) {
	f := &fakeRenderer{}
	f.render = func(req request) string { return "content of " + req.path }
	m := newTestViewer(f)
	m = apply(t, m, m.SetSize(80, 24))

	cmdA := m.Open("a.md")
	cmdB := m.Open("b.md")

	m, _ = m.Update(cmdA())
	if strings.Contains(m.Content(), "a.md") {
		t.Fatalf("stale result for a.md was displayed: %q", m.Content())
	}

	m, _ = m.Update(cmdB())
	if !strings.Contains(m.Content(), "b.md") {
		t.Fatalf("fresh result for b.md missing: %q", m.Content())
	}
}

func TestScrollKeysDoNotRender(t *testing.T) {
	f := &fakeRenderer{raw: "raw"}
	f.render = func(request) string { return strings.Repeat("a line of markdown\n", 100) }
	m := newTestViewer(f)
	m = apply(t, m, m.SetSize(80, 20))
	m = apply(t, m, m.Open("notes.md"))

	beforeCalls := len(f.calls)
	beforeID := m.reqID

	for _, name := range []string{"j", "k", "down", "up", "pgdown", "pgup", "ctrl+d", "ctrl+u", "g", "G"} {
		m, _ = m.Update(press(name))
	}

	if len(f.calls) != beforeCalls {
		t.Fatalf("scrolling issued %d render requests, want 0", len(f.calls)-beforeCalls)
	}
	if m.reqID != beforeID {
		t.Fatalf("scroll changed request ID from %d to %d", beforeID, m.reqID)
	}
	if m.viewport.YOffset() == 0 {
		t.Fatal("scroll keys did not move the viewport")
	}
}

func TestPagerKeys(t *testing.T) {
	f := &fakeRenderer{}
	f.render = func(request) string { return strings.Repeat("a line of markdown\n", 100) }
	m := newTestViewer(f)
	m = apply(t, m, m.SetSize(80, 20))
	m = apply(t, m, m.Open("notes.md"))

	if !m.viewport.AtTop() {
		t.Fatal("new document should start at the top")
	}

	m, _ = m.Update(press("j"))
	if got := m.viewport.YOffset(); got != 1 {
		t.Fatalf("j moved offset to %d, want 1", got)
	}
	m, _ = m.Update(press("k"))
	if !m.viewport.AtTop() {
		t.Fatal("k did not move back to the top")
	}

	m, _ = m.Update(press("pgdown"))
	if m.viewport.AtTop() {
		t.Fatal("pgdown did not move the viewport")
	}
	m, _ = m.Update(press("pgup"))
	if !m.viewport.AtTop() {
		t.Fatal("pgup did not return to the top")
	}

	m, _ = m.Update(press("ctrl+d"))
	if m.viewport.AtTop() {
		t.Fatal("ctrl+d did not move half a page")
	}
	m, _ = m.Update(press("ctrl+u"))
	if !m.viewport.AtTop() {
		t.Fatal("ctrl+u did not move back half a page")
	}

	m, _ = m.Update(press("G"))
	if !m.viewport.AtBottom() {
		t.Fatal("G did not jump to the bottom")
	}
	m, _ = m.Update(press("g"))
	if !m.viewport.AtTop() {
		t.Fatal("g did not jump to the top")
	}
}

func TestWidthChangeRendersAgain(t *testing.T) {
	f := &fakeRenderer{}
	m := newTestViewer(f)
	m = apply(t, m, m.SetSize(80, 24))
	m = apply(t, m, m.Open("notes.md"))

	if got := f.calls[len(f.calls)-1].width; got != 78 {
		t.Fatalf("initial render width = %d, want 78", got)
	}

	before := len(f.calls)
	cmd := m.SetSize(120, 24)
	if cmd == nil {
		t.Fatal("width change did not request a render")
	}
	if len(f.calls) != before+1 {
		t.Fatalf("width change issued %d renders, want 1", len(f.calls)-before)
	}
	if got := f.calls[len(f.calls)-1].width; got != 118 {
		t.Fatalf("re-render width = %d, want 118", got)
	}
	if got := f.calls[len(f.calls)-1].style; got != render.Dark {
		t.Fatalf("render style = %q, want dark", got)
	}

	if cmd := m.SetSize(120, 30); cmd != nil {
		t.Fatal("height-only change should not request a render")
	}
}

func TestRawToggle(t *testing.T) {
	f := &fakeRenderer{raw: "# raw source\n"}
	f.render = func(request) string { return "RENDERED CONTENT" }
	m := newTestViewer(f)
	m = apply(t, m, m.SetSize(80, 24))
	m = apply(t, m, m.Open("notes.md"))

	if !strings.Contains(m.Content(), "RENDERED CONTENT") {
		t.Fatalf("rendered content missing: %q", m.Content())
	}

	m, cmd := m.Update(press("r"))
	if cmd != nil {
		t.Fatal("raw toggle should not need a raw load when source is already cached")
	}
	if !strings.Contains(m.Content(), "# raw source") {
		t.Fatalf("raw content missing after toggle: %q", m.Content())
	}

	m, _ = m.Update(press("r"))
	if !strings.Contains(m.Content(), "RENDERED CONTENT") {
		t.Fatalf("rendered content missing after second toggle: %q", m.Content())
	}
}

func TestLargeFileWarningAndRawOffer(t *testing.T) {
	f := &fakeRenderer{
		large: true,
		size:  5 * 1024 * 1024,
		raw:   "# huge source\n",
	}
	m := newTestViewer(f)
	m = apply(t, m, m.SetSize(80, 24))
	beforeCalls := len(f.calls)
	m = apply(t, m, m.Open("huge.md"))

	if !strings.Contains(m.Content(), "too large to render") {
		t.Fatalf("large-file warning missing: %q", m.Content())
	}
	if !strings.Contains(m.Content(), "Press r") {
		t.Fatalf("raw-view offer missing: %q", m.Content())
	}
	if len(f.rawCalls) != 0 {
		t.Fatal("large file should not be read before the user asks for raw")
	}

	m, cmd := m.Update(press("r"))
	if cmd == nil {
		t.Fatal("raw toggle did not start a raw load for the large file")
	}
	m = apply(t, m, cmd)
	if !strings.Contains(m.Content(), "# huge source") {
		t.Fatalf("raw content missing: %q", m.Content())
	}

	m, _ = m.Update(press("r"))
	if !strings.Contains(m.Content(), "too large to render") {
		t.Fatalf("warning missing after toggling back: %q", m.Content())
	}
	if len(f.calls) != beforeCalls+1 {
		t.Fatalf("large file issued %d render jobs, want 1 (the initial size check)", len(f.calls)-beforeCalls)
	}
}

func TestWelcomeRendersThroughPipeline(t *testing.T) {
	f := &fakeRenderer{}
	f.render = func(request) string { return "welcome body" }
	m := newTestViewer(f)

	cmd := m.SetSize(80, 24)
	if cmd == nil {
		t.Fatal("first size set did not request the welcome document")
	}
	if len(f.calls) != 1 || f.calls[0].path != welcomeSource {
		t.Fatalf("welcome request = %+v, want source %q", f.calls, welcomeSource)
	}
	m = apply(t, m, cmd)
	if !strings.Contains(m.Content(), "welcome body") {
		t.Fatalf("welcome content missing: %q", m.Content())
	}

	cmd = m.SetSize(100, 24)
	if cmd == nil {
		t.Fatal("width change did not re-render the welcome document")
	}
	if got := f.calls[len(f.calls)-1].width; got != 98 {
		t.Fatalf("welcome re-render width = %d, want 98", got)
	}
}

func TestEmptyDocument(t *testing.T) {
	f := &fakeRenderer{}
	f.render = func(request) string { return "" }
	m := newTestViewer(f)
	m = apply(t, m, m.SetSize(80, 24))
	m = apply(t, m, m.Open("empty.md"))

	if strings.Contains(m.Content(), "no file selected") {
		t.Fatalf("empty file looks like no selection: %q", m.Content())
	}
	if !strings.Contains(m.Content(), "empty") {
		t.Fatalf("empty file placeholder missing: %q", m.Content())
	}
}

func TestOpenShowsLoadingPlaceholder(t *testing.T) {
	f := &fakeRenderer{}
	m := newTestViewer(f)
	m = apply(t, m, m.SetSize(80, 24))

	cmd := m.Open("notes.md")
	if !strings.Contains(m.Content(), "rendering") {
		t.Fatalf("loading placeholder missing: %q", m.Content())
	}
	m = apply(t, m, cmd)
	if strings.Contains(m.Content(), "rendering") {
		t.Fatalf("loading placeholder still visible after render: %q", m.Content())
	}
}
