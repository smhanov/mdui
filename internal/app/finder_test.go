package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/smhanov/mdui/internal/finder"
)

type countingFinderBuilder struct {
	mu      sync.Mutex
	calls   int
	hidden  []bool
	visible finder.Index
	withDot finder.Index
	err     error
}

func newCountingFinderBuilder(t *testing.T, root string) *countingFinderBuilder {
	t.Helper()
	visible, err := finder.Build(root, false)
	if err != nil {
		t.Fatalf("build visible index: %v", err)
	}
	withDot, err := finder.Build(root, true)
	if err != nil {
		t.Fatalf("build hidden index: %v", err)
	}
	return &countingFinderBuilder{visible: visible, withDot: withDot}
}

func (b *countingFinderBuilder) build(root string, showHidden bool) (finder.Index, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls++
	b.hidden = append(b.hidden, showHidden)
	if b.err != nil {
		return finder.Index{Root: root, Hidden: showHidden}, b.err
	}
	if showHidden {
		return b.withDot, nil
	}
	return b.visible, nil
}

func (b *countingFinderBuilder) callCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.calls
}

func finderFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	deep := filepath.Join(root, "projects", "deep")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join(deep, "guide.md"):          "# guide\n\nnested document\n",
		filepath.Join(root, "alpha.md"):          "# alpha\n\nfirst document\n",
		filepath.Join(root, "beta.md"):           "# beta\n\nsecond document\n",
		filepath.Join(root, ".hidden.md"):        "# hidden\n",
		filepath.Join(root, "notes.txt"):         "not markdown",
		filepath.Join(root, "projects", "x.txt"): "not markdown either",
	}
	for path, body := range files {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func finderTestModel(t *testing.T, opts ...Option) (Model, *countingFinderBuilder, string) {
	t.Helper()
	root := finderFixture(t)
	builder := newCountingFinderBuilder(t, root)
	opts = append(opts,
		WithFinderBuilder(builder.build),
		WithStatePath(filepath.Join(t.TempDir(), "state.toml")),
	)
	m := New(root, opts...)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return next.(Model), builder, root
}

func openFinderWithoutBuild(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	updated := next.(Model)
	if updated.overlay != overlayFinder {
		t.Fatalf("f did not open the finder overlay")
	}
	return updated, cmd
}

func runFinderBuild(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		t.Fatal("first f returned no index command")
	}
	next, _ := m.Update(cmd())
	return next.(Model)
}

func TestFinderOpensLoadingAndBuildsIndex(t *testing.T) {
	m, builder, _ := finderTestModel(t)

	m, cmd := openFinderWithoutBuild(t, m)
	if m.finderState != finderLoading || !m.finderBuilding {
		t.Fatalf("finder state = %v building=%v, want loading", m.finderState, m.finderBuilding)
	}
	plain := ansi.Strip(m.render())
	if !strings.Contains(plain, "indexing markdown files…") {
		t.Fatalf("loading state missing from overlay:\n%s", plain)
	}
	if !strings.Contains(plain, "Find markdown file") {
		t.Fatalf("finder title missing:\n%s", plain)
	}

	m = runFinderBuild(t, m, cmd)
	if m.finderState != finderReady {
		t.Fatalf("finder state = %v, want ready", m.finderState)
	}
	if builder.callCount() != 1 {
		t.Fatalf("builder calls = %d, want 1", builder.callCount())
	}
	if len(m.finderResults) != 3 {
		t.Fatalf("results = %d, want 3 visible markdown files", len(m.finderResults))
	}
	if plain := ansi.Strip(m.render()); !strings.Contains(plain, "3 files") {
		t.Fatalf("ready title does not report the file count:\n%s", plain)
	}
}

func TestFinderTypingDoesNoAdditionalWalksAndEscPreservesState(t *testing.T) {
	m, builder, _ := finderTestModel(t)
	m = clickRow(t, m, 2) // alpha.md
	selected := m.selected
	content := m.viewer.Content()
	visibleStart := m.tree.VisibleStart(m.paneHeight())

	m, cmd := openFinderWithoutBuild(t, m)
	m = runFinderBuild(t, m, cmd)
	if builder.callCount() != 1 {
		t.Fatalf("builder calls = %d, want 1", builder.callCount())
	}

	m = typeText(t, m, "guide")
	if got := m.finderQuery; got != "guide" {
		t.Fatalf("query = %q, want guide", got)
	}
	if len(m.finderResults) != 1 || filepath.Base(m.finderResults[0].Rel) != "guide.md" {
		t.Fatalf("filtered results = %+v, want guide.md", m.finderResults)
	}
	m = typeText(t, m, "x")
	if len(m.finderResults) != 0 {
		t.Fatalf("query guidex returned %d results", len(m.finderResults))
	}
	if builder.callCount() != 1 {
		t.Fatalf("typing performed %d builder calls, want 1", builder.callCount())
	}

	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.overlay != overlayNone {
		t.Fatal("esc did not close the finder")
	}
	if m.selected != selected {
		t.Fatalf("esc changed selection to %q, want %q", m.selected, selected)
	}
	if m.viewer.Content() != content {
		t.Fatal("esc changed the viewer content")
	}
	if got := m.tree.VisibleStart(m.paneHeight()); got != visibleStart {
		t.Fatalf("esc changed the tree window start from %d to %d", visibleStart, got)
	}

	// Reopening uses the cache: no new walk and a fresh empty query.
	m = pressKey(t, m, tea.KeyPressMsg{Code: 'f', Text: "f"})
	if m.finderState != finderReady {
		t.Fatalf("reopened finder state = %v, want ready", m.finderState)
	}
	if m.finderQuery != "" {
		t.Fatalf("reopened query = %q, want empty", m.finderQuery)
	}
	if builder.callCount() != 1 {
		t.Fatalf("reopen performed a walk: calls = %d", builder.callCount())
	}

	// Toggling hidden files invalidates once; the next open builds again.
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	m = pressKey(t, m, tea.KeyPressMsg{Code: '.', Text: "."})
	if builder.callCount() != 1 {
		t.Fatalf("hidden toggle walked before the next open: calls = %d", builder.callCount())
	}
	m = pressKey(t, m, tea.KeyPressMsg{Code: 'f', Text: "f"})
	if builder.callCount() != 2 {
		t.Fatalf("post-toggle open calls = %d, want 2", builder.callCount())
	}
	if len(m.finderResults) != 4 {
		t.Fatalf("hidden index results = %d, want 4", len(m.finderResults))
	}
}

func TestFinderWindowingAndNavigation(t *testing.T) {
	root := t.TempDir()
	files := make([]finder.File, 30)
	for i := range files {
		rel := fmt.Sprintf("dir/file-%02d.md", i)
		files[i] = finder.File{Path: filepath.Join(root, rel), Rel: rel}
	}
	builder := func(string, bool) (finder.Index, error) {
		return finder.Index{Root: root, Files: files}, nil
	}
	m := New(root, WithFinderBuilder(builder), WithStatePath(filepath.Join(t.TempDir(), "state.toml")))
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(Model)

	m, cmd := openFinderWithoutBuild(t, m)
	m = runFinderBuild(t, m, cmd)
	rows := m.finderListRows()
	if rows != 12 {
		t.Fatalf("list rows = %d, want 12 at height 30", rows)
	}

	lines := m.finderResultLines(60, rows)
	if len(lines) != rows {
		t.Fatalf("rendered result rows = %d, want %d", len(lines), rows)
	}
	if !strings.Contains(ansi.Strip(lines[0]), "file-00.md") {
		t.Fatalf("first window row = %q, want file-00.md", ansi.Strip(lines[0]))
	}

	for range 15 {
		m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if m.finderSel != 15 {
		t.Fatalf("selection = %d, want 15", m.finderSel)
	}
	if m.finderOffset != 4 {
		t.Fatalf("window offset = %d, want 4", m.finderOffset)
	}
	lines = m.finderResultLines(60, rows)
	if !strings.Contains(ansi.Strip(lines[0]), "file-04.md") {
		t.Fatalf("scrolled first row = %q, want file-04.md", ansi.Strip(lines[0]))
	}
	if !strings.Contains(ansi.Strip(lines[m.finderSel-m.finderOffset]), "file-15.md") {
		t.Fatalf("selected row missing file-15.md: %q", ansi.Strip(lines[m.finderSel-m.finderOffset]))
	}

	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyPgDown})
	if m.finderSel != 27 || m.finderOffset != 16 {
		t.Fatalf("after pgdown selection=%d offset=%d, want 27/16", m.finderSel, m.finderOffset)
	}
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyPgDown})
	if m.finderSel != 29 {
		t.Fatalf("selection clamped to %d, want 29", m.finderSel)
	}
	m = pressKey(t, m, tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})
	if m.finderSel != 28 {
		t.Fatalf("ctrl+k selection = %d, want 28", m.finderSel)
	}
	m = pressKey(t, m, tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl})
	if m.finderSel != 29 {
		t.Fatalf("ctrl+j selection = %d, want 29", m.finderSel)
	}
}

func TestFinderEnterOpensViewerAndRevealsTree(t *testing.T) {
	m, builder, root := finderTestModel(t)
	m, cmd := openFinderWithoutBuild(t, m)
	m = runFinderBuild(t, m, cmd)
	m = typeText(t, m, "guide")

	guide := filepath.Join(root, "projects", "deep", "guide.md")
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.overlay != overlayNone {
		t.Fatal("enter left the finder open")
	}
	if m.selected != guide {
		t.Fatalf("selected = %q, want %q", m.selected, guide)
	}
	if m.focus != FocusViewer {
		t.Fatalf("focus = %v, want viewer", m.focus)
	}
	if !strings.Contains(m.viewer.Content(), "guide") {
		t.Fatalf("viewer did not open guide.md:\n%s", m.viewer.Content())
	}

	plain := ansi.Strip(m.render())
	for _, want := range []string{"projects", "deep", "guide.md"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("revealed tree is missing %q:\n%s", want, plain)
		}
	}
	if builder.callCount() != 1 {
		t.Fatalf("enter performed %d builds, want 1", builder.callCount())
	}
}

func TestFinderRevealFailureStillOpensFile(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "gone", "missing.md")
	builder := func(string, bool) (finder.Index, error) {
		return finder.Index{Root: root, Files: []finder.File{{Path: missing, Rel: "gone/missing.md"}}}, nil
	}
	m := New(root, WithFinderBuilder(builder), WithStatePath(filepath.Join(t.TempDir(), "state.toml")))
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(Model)

	m, cmd := openFinderWithoutBuild(t, m)
	m = runFinderBuild(t, m, cmd)
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.selected != missing {
		t.Fatalf("selected = %q, want %q", m.selected, missing)
	}
	if !strings.Contains(m.notice, "not visible in the tree") {
		t.Fatalf("notice = %q, want a reveal failure notice", m.notice)
	}
	if !strings.Contains(m.viewer.Content(), "⚠") {
		t.Fatalf("viewer did not show an error for the missing file:\n%s", m.viewer.Content())
	}
}

func TestFinderEmptyAndNoMatchStates(t *testing.T) {
	root := t.TempDir()
	builder := func(string, bool) (finder.Index, error) {
		return finder.Index{Root: root}, nil
	}
	m := New(root, WithFinderBuilder(builder), WithStatePath(filepath.Join(t.TempDir(), "state.toml")))
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(Model)

	m, cmd := openFinderWithoutBuild(t, m)
	m = runFinderBuild(t, m, cmd)
	if plain := ansi.Strip(m.render()); !strings.Contains(plain, "no markdown files under this folder") {
		t.Fatalf("empty index state missing:\n%s", plain)
	}
	if plain := ansi.Strip(m.render()); !strings.Contains(plain, "0 results") {
		t.Fatalf("empty footer missing:\n%s", plain)
	}

	files := []finder.File{
		{Path: filepath.Join(root, "alpha.md"), Rel: "alpha.md"},
		{Path: filepath.Join(root, "beta.md"), Rel: "beta.md"},
	}
	builder = func(string, bool) (finder.Index, error) {
		return finder.Index{Root: root, Files: files}, nil
	}
	m = New(root, WithFinderBuilder(builder), WithStatePath(filepath.Join(t.TempDir(), "state.toml")))
	next, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(Model)
	m, cmd = openFinderWithoutBuild(t, m)
	m = runFinderBuild(t, m, cmd)
	m = typeText(t, m, "zzz")
	if plain := ansi.Strip(m.render()); !strings.Contains(plain, "no matches for “zzz”") {
		t.Fatalf("no-match state missing:\n%s", plain)
	}
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if cmd != nil {
		t.Fatal("enter on a no-match result emitted a command")
	}
	if m.overlay != overlayFinder {
		t.Fatal("enter on a no-match result closed the finder")
	}
}

func TestFinderPartialIndexNotice(t *testing.T) {
	root := t.TempDir()
	files := []finder.File{{Path: filepath.Join(root, "alpha.md"), Rel: "alpha.md"}}
	builder := func(_ string, _ bool) (finder.Index, error) {
		return finder.Index{Root: root, Files: files}, &finder.BuildError{Count: 1, First: errors.New("permission denied")}
	}
	m := New(root, WithFinderBuilder(builder), WithStatePath(filepath.Join(t.TempDir(), "state.toml")))
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(Model)

	m, cmd := openFinderWithoutBuild(t, m)
	m = runFinderBuild(t, m, cmd)
	if plain := ansi.Strip(m.render()); !strings.Contains(plain, "⚠ partial") {
		t.Fatalf("partial marker missing:\n%s", plain)
	}
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if !strings.Contains(m.notice, "partial index") {
		t.Fatalf("notice = %q, want the partial index warning", m.notice)
	}
}

func TestFinderCapturesQUsesCtrlCToCloseAndIgnoresMouse(t *testing.T) {
	m, _, _ := finderTestModel(t)
	m, cmd := openFinderWithoutBuild(t, m)
	m = runFinderBuild(t, m, cmd)

	m = typeText(t, m, "q?")
	if m.finderQuery != "q?" {
		t.Fatalf("query = %q, want q?", m.finderQuery)
	}
	if m.overlay != overlayFinder {
		t.Fatal("query text dismissed the finder")
	}

	before := m.finderSel
	next, clickCmd := m.Update(tea.MouseClickMsg{X: 1, Y: 1, Button: tea.MouseLeft})
	m = next.(Model)
	if clickCmd != nil {
		t.Fatal("finder click produced a command")
	}
	if m.overlay != overlayFinder || m.finderSel != before {
		t.Fatal("finder click changed overlay state")
	}

	next, quitCmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m = next.(Model)
	if quitCmd != nil {
		t.Fatal("ctrl+c quit the app instead of closing the finder")
	}
	if m.overlay != overlayNone {
		t.Fatal("ctrl+c did not close the finder")
	}
}

func TestFinderOverlayPrecedence(t *testing.T) {
	m, _, _ := finderTestModel(t)

	m = pressKey(t, m, tea.KeyPressMsg{Code: '?', Text: "?"})
	m = pressKey(t, m, tea.KeyPressMsg{Code: 'f', Text: "f"})
	if m.overlay != overlayHelp {
		t.Fatal("f replaced the help overlay")
	}

	m.overlay = overlayTablePicker
	m = pressKey(t, m, tea.KeyPressMsg{Code: 'f', Text: "f"})
	if m.overlay != overlayTablePicker {
		t.Fatal("f replaced the table picker")
	}

	m.overlay = overlayTable
	m = pressKey(t, m, tea.KeyPressMsg{Code: 'f', Text: "f"})
	if m.overlay != overlayNone {
		t.Fatal("f did not dismiss the full-width table overlay")
	}

	m = clickRow(t, m, 2) // alpha.md
	m = pressKey(t, m, tea.KeyPressMsg{Code: '/', Text: "/"})
	if !m.searchPrompt {
		t.Fatal("search prompt did not open")
	}
	m = pressKey(t, m, tea.KeyPressMsg{Code: 'f', Text: "f"})
	if m.overlay == overlayFinder {
		t.Fatal("f opened the finder over the search prompt")
	}
	if got := m.viewer.SearchQuery(); got != "f" {
		t.Fatalf("search query = %q, want f", got)
	}
}

func TestFinderInputEditing(t *testing.T) {
	m, _, _ := finderTestModel(t)
	m, cmd := openFinderWithoutBuild(t, m)
	m = runFinderBuild(t, m, cmd)

	m = typeText(t, m, "ac")
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyLeft})
	m = typeText(t, m, "b")
	if m.finderQuery != "abc" || m.finderCursor != 2 {
		t.Fatalf("after insert: query=%q cursor=%d, want abc/2", m.finderQuery, m.finderCursor)
	}
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	if m.finderQuery != "ac" || m.finderCursor != 1 {
		t.Fatalf("after backspace: query=%q cursor=%d, want ac/1", m.finderQuery, m.finderCursor)
	}
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyDelete})
	if m.finderQuery != "a" || m.finderCursor != 1 {
		t.Fatalf("after delete: query=%q cursor=%d, want a/1", m.finderQuery, m.finderCursor)
	}
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyHome})
	m = typeText(t, m, "b")
	if m.finderQuery != "ba" || m.finderCursor != 1 {
		t.Fatalf("after home insert: query=%q cursor=%d, want ba/1", m.finderQuery, m.finderCursor)
	}
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnd})
	m = typeText(t, m, " tail")
	if m.finderQuery != "ba tail" {
		t.Fatalf("query = %q, want ba tail", m.finderQuery)
	}
	m = pressKey(t, m, tea.KeyPressMsg{Code: 'w', Mod: tea.ModCtrl})
	if m.finderQuery != "ba " {
		t.Fatalf("ctrl+w query = %q, want ba ", m.finderQuery)
	}
	m = pressKey(t, m, tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	if m.finderQuery != "" || m.finderCursor != 0 {
		t.Fatalf("ctrl+u query=%q cursor=%d, want empty/0", m.finderQuery, m.finderCursor)
	}
}

func TestTruncateFinderPathKeepsBasename(t *testing.T) {
	path := filepath.Join("a", "very", "long", "directory", "structure", "readme.md")
	got := truncateFinderPath(path, 20)
	if !strings.Contains(got, "readme.md") {
		t.Fatalf("truncated path %q lost the basename", got)
	}
	if !strings.HasPrefix(got, "…") {
		t.Fatalf("truncated path %q has no leading ellipsis", got)
	}
	if width := ansi.StringWidth(got); width > 20 {
		t.Fatalf("truncated width = %d, want <= 20", width)
	}
	if got := truncateFinderPath("short.md", 20); got != "short.md" {
		t.Fatalf("short path = %q, want unchanged", got)
	}
	if got := truncateFinderPath("really-long-basename.md", 8); !strings.HasSuffix(got, "…") {
		t.Fatalf("basename truncation = %q, want a trailing ellipsis", got)
	}
}

func TestFinderLongPathRendersBasename(t *testing.T) {
	root := t.TempDir()
	rel := filepath.Join("one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten", "readme.md")
	builder := func(string, bool) (finder.Index, error) {
		return finder.Index{Root: root, Files: []finder.File{{Path: filepath.Join(root, rel), Rel: rel}}}, nil
	}
	m := New(root, WithFinderBuilder(builder), WithStatePath(filepath.Join(t.TempDir(), "state.toml")))
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(Model)
	m, cmd := openFinderWithoutBuild(t, m)
	m = runFinderBuild(t, m, cmd)

	plain := ansi.Strip(m.render())
	if !strings.Contains(plain, "readme.md") {
		t.Fatalf("long path did not preserve the basename:\n%s", plain)
	}
	if !strings.Contains(plain, "…") {
		t.Fatalf("long path did not truncate with an ellipsis:\n%s", plain)
	}
}

func TestFinderInputLineKeepsCursorInsideWidth(t *testing.T) {
	query := "a-very-long-query-that-does-not-fit"
	for _, cursor := range []int{0, 5, len([]rune(query))} {
		line := finderInputLine(query, cursor, 20)
		if width := ansi.StringWidth(line); width > 20 {
			t.Fatalf("cursor %d: width = %d, want <= 20 (%q)", cursor, width, line)
		}
		if !strings.Contains(line, "▏") {
			t.Fatalf("cursor %d: marker missing from %q", cursor, line)
		}
	}
}
