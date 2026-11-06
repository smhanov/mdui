package viewer

import (
	"io/fs"
	"strings"
	"testing"
)

func TestRefreshWelcomeIsNoop(t *testing.T) {
	f := &fakeRenderer{}
	m := newTestViewer(f)
	m = apply(t, m, m.SetSize(80, 20))
	before := len(f.calls)

	if cmd := m.Refresh(); cmd != nil {
		t.Fatalf("welcome refresh returned %T, want nil", cmd())
	}
	if len(f.calls) != before {
		t.Fatalf("welcome refresh issued %d render jobs", len(f.calls)-before)
	}
}

func TestRefreshRenderedPreservesScrollAndSearch(t *testing.T) {
	f := &fakeRenderer{raw: strings.Repeat("alpha old ", 40)}
	f.render = func(request) string { return strings.Repeat("alpha old line\n", 40) }
	m := newTestViewer(f)
	m = apply(t, m, m.SetSize(80, 10))
	m = apply(t, m, m.Open("notes.md"))

	m.BeginSearch()
	m.SetSearchQuery("alpha")
	total, _ := m.CommitSearch()
	if total != 40 {
		t.Fatalf("search total = %d, want 40", total)
	}
	m.SetScrollOffset(5)
	offset := m.ScrollOffset()
	if offset != 5 {
		t.Fatalf("scroll offset = %d, want 5", offset)
	}
	reqID := m.reqID

	f.raw = strings.Repeat("alpha refreshed ", 40)
	f.render = func(request) string { return strings.Repeat("alpha refreshed line\n", 40) }

	cmd := m.Refresh()
	if cmd == nil {
		t.Fatal("rendered refresh returned no command")
	}
	if m.reqID != reqID+1 {
		t.Fatalf("request ID = %d, want %d", m.reqID, reqID+1)
	}
	if !m.loading {
		t.Fatal("refresh did not mark the viewer loading")
	}
	if m.RawMode() {
		t.Fatal("rendered refresh switched to raw mode")
	}

	m = apply(t, m, cmd)
	if !strings.Contains(m.BaseContent(), "alpha refreshed") {
		t.Fatalf("refreshed content missing: %q", m.BaseContent())
	}
	if q := m.SearchQuery(); q != "alpha" {
		t.Fatalf("refresh lost search query: %q", q)
	}
	if total := m.SearchTotal(); total != 40 {
		t.Fatalf("refresh lost search matches: %d", total)
	}
	if got := m.ScrollOffset(); got != offset {
		t.Fatalf("refresh moved scroll from %d to %d", offset, got)
	}
}

func TestRefreshRawModeStaysRaw(t *testing.T) {
	f := &fakeRenderer{raw: strings.Repeat("old raw line\n", 40)}
	f.render = func(request) string { return "rendered" }
	m := newTestViewer(f)
	m = apply(t, m, m.SetSize(80, 10))
	m = apply(t, m, m.Open("notes.md"))

	m, _ = m.Update(press("r"))
	if !m.RawMode() {
		t.Fatal("raw toggle did not turn on")
	}
	m.SetScrollOffset(4)
	offset := m.ScrollOffset()
	rawReqID := m.rawReqID

	f.raw = strings.Repeat("new raw line\n", 40)
	cmd := m.Refresh()
	if cmd == nil {
		t.Fatal("raw refresh returned no command")
	}
	if !m.RawMode() {
		t.Fatal("raw refresh left raw mode")
	}
	if m.rawReqID != rawReqID+1 {
		t.Fatalf("raw request ID = %d, want %d", m.rawReqID, rawReqID+1)
	}
	if !m.loading {
		t.Fatal("raw refresh did not mark the viewer loading")
	}

	m = apply(t, m, cmd)
	if !strings.Contains(m.BaseContent(), "new raw line") {
		t.Fatalf("raw source was not reloaded: %q", m.BaseContent())
	}
	if strings.Contains(m.BaseContent(), "old raw line") {
		t.Fatalf("raw source still shows the old content: %q", m.BaseContent())
	}
	if got := m.ScrollOffset(); got != offset {
		t.Fatalf("raw refresh moved scroll from %d to %d", offset, got)
	}
}

func TestRefreshDeletedFileKeepsLastContent(t *testing.T) {
	f := &fakeRenderer{raw: "last good source"}
	f.render = func(request) string { return "LAST GOOD RENDER" }
	m := newTestViewer(f)
	m = apply(t, m, m.SetSize(80, 20))
	m = apply(t, m, m.Open("notes.md"))

	f.err = fs.ErrNotExist
	m = apply(t, m, m.Refresh())

	if !strings.Contains(m.BaseContent(), "LAST GOOD RENDER") {
		t.Fatalf("deleted file replaced the last render: %q", m.BaseContent())
	}
	if got := m.Warning(); got != "file no longer available" {
		t.Fatalf("warning = %q, want file-no-longer-available", got)
	}
}
