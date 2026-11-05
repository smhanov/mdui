package viewer

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestSearchIncrementalCountAndHighlight(t *testing.T) {
	content := "alpha one\nbeta alpha\ngamma"
	_, m := searchViewer(t, content, 80, 10)

	m.BeginSearch()
	if m.SearchActive() {
		t.Fatal("BeginSearch made the query repeatable before commit")
	}
	m.SetSearchQuery("alpha")
	if got := m.SearchTotal(); got != 2 {
		t.Fatalf("total matches = %d, want 2", got)
	}
	if m.SearchActive() {
		t.Fatal("typing committed the search")
	}
	if !strings.Contains(m.Content(), "\x1b[7m") {
		t.Fatalf("matches are not highlighted: %q", m.Content())
	}
	if got, want := ansi.Strip(m.Content()), content; got != want {
		t.Fatalf("highlight changed visible text:\n got %q\nwant %q", got, want)
	}
}

func TestCommitSearchChoosesMatchAtScrollPosition(t *testing.T) {
	var lines []string
	for i := 0; i < 30; i++ {
		if i == 0 || i == 10 || i == 20 {
			lines = append(lines, fmt.Sprintf("needle line %d", i))
		} else {
			lines = append(lines, fmt.Sprintf("filler line %d", i))
		}
	}
	content := strings.Join(lines, "\n")
	_, m := searchViewer(t, content, 80, 8)

	m.BeginSearch()
	m.SetSearchQuery("needle")
	m.ScrollToLine(12)
	total, _ := m.CommitSearch()
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}
	current, gotTotal := m.SearchStatus()
	if current != 2 || gotTotal != 3 {
		t.Fatalf("status = %d/%d, want 2/3", current, gotTotal)
	}
	if got := m.ScrollOffset(); got != 20 {
		t.Fatalf("scroll offset = %d, want 20", got)
	}
}

func TestCommitSearchWrapsToFirstMatch(t *testing.T) {
	content := "needle top\nfiller\nneedle middle\nfiller\nneedle bottom\nfiller\nfiller\nfiller\nfiller"
	_, m := searchViewer(t, content, 80, 3)

	m.ScrollToLine(8)
	m.BeginSearch()
	m.SetSearchQuery("needle")
	_, _ = m.CommitSearch()
	current, total := m.SearchStatus()
	if current != 0 || total != 3 {
		t.Fatalf("status = %d/%d, want wrapped 0/3", current, total)
	}
	if got := m.ScrollOffset(); got != 0 {
		t.Fatalf("scroll offset = %d, want 0", got)
	}
}

func TestNextPreviousMatchWrap(t *testing.T) {
	content := "needle 0\nfiller\nneedle 1\nfiller\nneedle 2\nfiller\nfiller\nfiller"
	_, m := searchViewer(t, content, 80, 3)

	m.ScrollToLine(2)
	m.BeginSearch()
	m.SetSearchQuery("needle")
	m.CommitSearch()

	if current, _ := m.SearchStatus(); current != 1 {
		t.Fatalf("initial current = %d, want 1", current)
	}
	if ok, _ := m.NextMatch(); !ok {
		t.Fatal("next match unavailable")
	}
	if current, _ := m.SearchStatus(); current != 2 {
		t.Fatalf("after n current = %d, want 2", current)
	}
	if got := m.ScrollOffset(); got != 4 {
		t.Fatalf("after n offset = %d, want 4", got)
	}
	if ok, _ := m.NextMatch(); !ok {
		t.Fatal("next match unavailable at end")
	}
	if current, _ := m.SearchStatus(); current != 0 {
		t.Fatalf("wrapped next current = %d, want 0", current)
	}
	if got := m.ScrollOffset(); got != 0 {
		t.Fatalf("wrapped next offset = %d, want 0", got)
	}
	if ok, _ := m.PrevMatch(); !ok {
		t.Fatal("previous match unavailable")
	}
	if current, _ := m.SearchStatus(); current != 2 {
		t.Fatalf("wrapped previous current = %d, want 2", current)
	}
	if got := m.ScrollOffset(); got != 4 {
		t.Fatalf("wrapped previous offset = %d, want 4", got)
	}
}

func TestCancelSearchClearsHighlights(t *testing.T) {
	content := "alpha\nbeta"
	_, m := searchViewer(t, content, 80, 8)

	m.BeginSearch()
	m.SetSearchQuery("alpha")
	m.CommitSearch()
	m.CancelSearch()

	if m.SearchActive() || m.SearchQuery() != "" {
		t.Fatalf("cancel left search state: active=%v query=%q", m.SearchActive(), m.SearchQuery())
	}
	if strings.Contains(m.Content(), "\x1b[7m") {
		t.Fatalf("cancel left highlights: %q", m.Content())
	}
	if got := ansi.Strip(m.Content()); got != content {
		t.Fatalf("cancel changed content: %q", got)
	}
}

func TestSearchNoMatches(t *testing.T) {
	content := "alpha\nbeta"
	_, m := searchViewer(t, content, 80, 8)

	m.BeginSearch()
	m.SetSearchQuery("zzz")
	total, _ := m.CommitSearch()
	if total != 0 {
		t.Fatalf("total = %d, want 0", total)
	}
	if !m.SearchActive() {
		t.Fatal("committed empty-result search should remain active for feedback")
	}
	if current, total := m.SearchStatus(); current != -1 || total != 0 {
		t.Fatalf("status = %d/%d, want -1/0", current, total)
	}
	if ok, _ := m.NextMatch(); ok {
		t.Fatal("next matched in an empty result set")
	}
	if ok, _ := m.PrevMatch(); ok {
		t.Fatal("previous matched in an empty result set")
	}
	if strings.Contains(m.Content(), "\x1b[7m") {
		t.Fatalf("no-match search left highlights: %q", m.Content())
	}
}

func TestSearchEmptyQueryAndDocument(t *testing.T) {
	_, m := searchViewer(t, "alpha", 80, 8)
	m.BeginSearch()
	m.SetSearchQuery("")
	if total, _ := m.CommitSearch(); total != 0 {
		t.Fatalf("empty query total = %d, want 0", total)
	}
	if m.SearchActive() || m.SearchQuery() != "" {
		t.Fatal("empty query should be a no-op")
	}

	_, empty := searchViewer(t, "", 80, 8)
	empty.BeginSearch()
	empty.SetSearchQuery("anything")
	if total, _ := empty.CommitSearch(); total != 0 {
		t.Fatalf("empty document total = %d, want 0", total)
	}
}

func TestRawToggleReappliesSearch(t *testing.T) {
	f := &fakeRenderer{raw: "# raw needle\n"}
	f.render = func(request) string { return "rendered needle" }
	m := newTestViewer(f)
	m = apply(t, m, m.SetSize(80, 8))
	m = apply(t, m, m.Open("notes.md"))

	m.BeginSearch()
	m.SetSearchQuery("needle")
	m.CommitSearch()
	if !m.SearchActive() {
		t.Fatal("search did not commit")
	}

	m, cmd := m.Update(press("r"))
	if cmd != nil {
		t.Fatal("raw toggle unexpectedly needed a raw load")
	}
	if !m.RawMode() {
		t.Fatal("raw mode did not turn on")
	}
	if !m.SearchActive() || m.SearchQuery() != "needle" {
		t.Fatalf("raw toggle lost search: active=%v query=%q", m.SearchActive(), m.SearchQuery())
	}
	if !strings.Contains(m.Content(), "\x1b[7m") || !strings.Contains(ansi.Strip(m.Content()), "raw needle") {
		t.Fatalf("raw projection was not highlighted: %q", m.Content())
	}

	m, _ = m.Update(press("r"))
	if m.RawMode() {
		t.Fatal("raw mode did not turn off")
	}
	if !strings.Contains(m.Content(), "\x1b[7m") || !strings.Contains(ansi.Strip(m.Content()), "rendered needle") {
		t.Fatalf("rendered projection was not re-highlighted: %q", m.Content())
	}
}

func TestWidthRenderReappliesSearch(t *testing.T) {
	f := &fakeRenderer{}
	f.render = func(req request) string {
		if req.width == 78 {
			return "needle here\n" + strings.Repeat("filler\n", 4)
		}
		return strings.Repeat("filler\n", 5) + "needle there"
	}
	m := newTestViewer(f)
	m = apply(t, m, m.SetSize(80, 10))
	m = apply(t, m, m.Open("notes.md"))

	m.BeginSearch()
	m.SetSearchQuery("needle")
	m.CommitSearch()
	if current, total := m.SearchStatus(); current != 0 || total != 1 {
		t.Fatalf("initial status = %d/%d, want 0/1", current, total)
	}

	cmd := m.SetSize(120, 10)
	if cmd == nil {
		t.Fatal("width change did not request a render")
	}
	m = apply(t, m, cmd)
	if !m.SearchActive() || m.SearchTotal() != 1 {
		t.Fatalf("width change lost search: active=%v total=%d", m.SearchActive(), m.SearchTotal())
	}
	if len(m.searchIndex.Matches) != 1 || m.searchIndex.Matches[0].Line != 5 {
		t.Fatalf("re-highlighted match lines = %+v, want line 5", m.searchIndex.Matches)
	}
	if !strings.Contains(m.Content(), "\x1b[7m") {
		t.Fatalf("width change did not re-highlight: %q", m.Content())
	}
	if got, want := ansi.StringWidth(m.Content()), ansi.StringWidth(m.BaseContent()); got != want {
		t.Fatalf("highlight changed width: %d, want %d", got, want)
	}
}

func TestNewFileClearsSearch(t *testing.T) {
	f := &fakeRenderer{raw: "raw"}
	f.render = func(req request) string { return "needle in " + req.path }
	m := newTestViewer(f)
	m = apply(t, m, m.SetSize(80, 8))
	m = apply(t, m, m.Open("first.md"))

	m.BeginSearch()
	m.SetSearchQuery("needle")
	m.CommitSearch()
	if !m.SearchActive() {
		t.Fatal("search did not commit")
	}

	m = apply(t, m, m.Open("second.md"))
	if m.SearchActive() || m.SearchQuery() != "" || m.SearchTotal() != 0 {
		t.Fatalf("new file kept search: active=%v query=%q total=%d", m.SearchActive(), m.SearchQuery(), m.SearchTotal())
	}
	if strings.Contains(m.Content(), "\x1b[7m") {
		t.Fatalf("new file kept highlights: %q", m.Content())
	}
}

func TestSearchDoesNotRender(t *testing.T) {
	content := strings.Repeat("needle filler\n", 40)
	f := &fakeRenderer{}
	f.render = func(request) string { return content }
	m := newTestViewer(f)
	m = apply(t, m, m.SetSize(80, 10))
	m = apply(t, m, m.Open("notes.md"))

	beforeCalls := len(f.calls)
	beforeID := m.reqID

	m.BeginSearch()
	m.SetSearchQuery("needle")
	m.CommitSearch()
	m.NextMatch()
	m.NextMatch()
	m.PrevMatch()
	m.CancelSearch()
	m.SetScrollOffset(5)
	m.ScrollToLine(10)

	if len(f.calls) != beforeCalls {
		t.Fatalf("search issued %d render requests, want 0", len(f.calls)-beforeCalls)
	}
	if m.reqID != beforeID {
		t.Fatalf("search changed request ID from %d to %d", beforeID, m.reqID)
	}
}

func searchViewer(t *testing.T, content string, width, height int) (*fakeRenderer, Model) {
	t.Helper()
	f := &fakeRenderer{raw: content}
	f.render = func(request) string { return content }
	m := newTestViewer(f)
	m = apply(t, m, m.SetSize(width, height))
	m = apply(t, m, m.Open("notes.md"))
	return f, m
}
