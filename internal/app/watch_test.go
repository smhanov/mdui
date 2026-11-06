package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/smhanov/mdui/internal/config"
	"github.com/smhanov/mdui/internal/viewer"
	"github.com/smhanov/mdui/internal/watch"
)

type liveEnv struct {
	m       Model
	watcher *watch.Fake
	root    string
	docs    string
	alpha   string
}

func newLiveEnv(t *testing.T, opts ...Option) *liveEnv {
	t.Helper()
	root := t.TempDir()
	docs := filepath.Join(root, "docs")
	if err := os.Mkdir(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	env := &liveEnv{
		root:  root,
		docs:  docs,
		alpha: filepath.Join(root, "alpha.md"),
	}
	for path, body := range map[string]string{
		env.alpha:                          "# Alpha\n\n" + strings.Repeat("alpha body line\n", 30),
		filepath.Join(root, "beta.md"):     "# Beta\n\nbeta body\n",
		filepath.Join(docs, "guide.md"):    "# Guide\n\nguide body\n",
		filepath.Join(docs, "appendix.md"): "# Appendix\n\nappendix body\n",
		filepath.Join(root, "notes.txt"):   "not markdown\n",
	} {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	env.watcher = watch.NewFake()
	opts = append(opts,
		WithWatcher(env.watcher),
		WithStatePath(filepath.Join(t.TempDir(), "state.toml")),
	)
	env.m = New(root, opts...)
	next, _ := env.m.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	env.m = next.(Model)
	return env
}

func (e *liveEnv) openAlpha(t *testing.T) {
	t.Helper()
	e.m = clickRow(t, e.m, 2) // root, docs, alpha.md
	if e.m.selected != e.alpha {
		t.Fatalf("selected = %q, want %q", e.m.selected, e.alpha)
	}
}

func (e *liveEnv) change(paths ...string) tea.Cmd {
	next, cmd := e.m.Update(watch.ChangeMsg{Paths: paths})
	e.m = next.(Model)
	return cmd
}

func (e *liveEnv) fireTick(t *testing.T) tea.Cmd {
	t.Helper()
	next, cmd := e.m.Update(fsTickMsg{Seq: e.m.fsSeq})
	e.m = next.(Model)
	return cmd
}

func (e *liveEnv) expandDir(t *testing.T, name string) {
	t.Helper()
	e.m = expandNamed(t, e.m, name)
}

func expandNamed(t *testing.T, m Model, name string) Model {
	t.Helper()
	start := m.tree.VisibleStart(m.paneHeight())
	for i := 0; ; i++ {
		row, _, ok := m.tree.RowAt(i, m.paneHeight())
		if !ok {
			break
		}
		if row.IsDir && row.Name == name {
			return clickRow(t, m, start+i)
		}
	}
	t.Fatalf("directory %q not visible in the tree", name)
	return m
}

func rewriteFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
}

func runAndApply(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	return applyCmd(t, m, cmd)
}

func collectMessages(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	if cmd == nil {
		return nil
	}
	switch msg := cmd().(type) {
	case nil:
		return nil
	case tea.BatchMsg:
		var out []tea.Msg
		for _, batched := range msg {
			out = append(out, collectMessages(t, batched)...)
		}
		return out
	default:
		return []tea.Msg{msg}
	}
}

func TestLiveReloadAddsFileWithoutMovingCursor(t *testing.T) {
	env := newLiveEnv(t)
	env.expandDir(t, "docs")
	env.m = pressKey(t, env.m, tea.KeyPressMsg{Code: tea.KeyDown}) // appendix.md
	guide := filepath.Join(env.docs, "appendix.md")
	if env.m.selected != guide {
		t.Fatalf("selected = %q, want %q", env.m.selected, guide)
	}

	created := filepath.Join(env.docs, "created.md")
	if err := os.WriteFile(created, []byte("# Created\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env.change(created)
	env.m = runAndApply(t, env.m, env.fireTick(t))

	plain := ansi.Strip(env.m.render())
	if !strings.Contains(plain, "created.md") {
		t.Fatalf("new file did not appear in the tree:\n%s", plain)
	}
	if !strings.Contains(plain, "▾ docs/") {
		t.Fatalf("reload collapsed the expanded directory:\n%s", plain)
	}
	if env.m.selected != guide {
		t.Fatalf("reload moved selection from %q to %q", guide, env.m.selected)
	}
}

func TestLiveReloadDeletesOpenFileKeepsContentAndWarns(t *testing.T) {
	env := newLiveEnv(t)
	env.openAlpha(t)
	oldContent := env.m.viewer.BaseContent()

	// The cursor is on the open file; the tree reload moves its own cursor to
	// a neighbour, but the viewer must keep the deleted page and its warning.
	if err := os.Remove(env.alpha); err != nil {
		t.Fatal(err)
	}
	env.change(env.alpha)
	env.m = runAndApply(t, env.m, env.fireTick(t))

	if env.m.selected != env.alpha {
		t.Fatalf("selected changed to %q, want the deleted file", env.m.selected)
	}
	if got := env.m.viewer.BaseContent(); got != oldContent {
		t.Fatalf("deleted file replaced the last render:\nold=%q\n got=%q", oldContent, got)
	}
	if got := env.m.viewer.Warning(); got != "file no longer available" {
		t.Fatalf("warning = %q", got)
	}
	status := ansi.Strip(env.m.renderStatus(200))
	if !strings.Contains(status, "⚠ file no longer available") {
		t.Fatalf("status bar missing the deletion warning:\n%s", status)
	}
}

func TestLiveReloadWritesRerendersWithScrollAndSearch(t *testing.T) {
	env := newLiveEnv(t)
	env.openAlpha(t)

	env.m.viewer.BeginSearch()
	env.m.viewer.SetSearchQuery("alpha")
	_, cmd := env.m.viewer.CommitSearch()
	env.m = runAndApply(t, env.m, cmd)
	env.m = runAndApply(t, env.m, env.m.viewer.SetScrollOffset(6))
	offset := env.m.viewer.ScrollOffset()

	body := "# Alpha\n\n" + strings.Repeat("alpha refreshed line\n", 30)
	rewriteFile(t, env.alpha, body)
	env.change(env.alpha)
	env.m = runAndApply(t, env.m, env.fireTick(t))

	if !strings.Contains(env.m.viewer.BaseContent(), "alpha refreshed") {
		t.Fatalf("viewer did not pick up the new content:\n%s", env.m.viewer.BaseContent())
	}
	if env.m.viewer.SearchQuery() != "alpha" || !env.m.viewer.SearchActive() {
		t.Fatalf("refresh lost the search: query=%q active=%v", env.m.viewer.SearchQuery(), env.m.viewer.SearchActive())
	}
	if got := env.m.viewer.ScrollOffset(); got != offset {
		t.Fatalf("refresh moved scroll from %d to %d", offset, got)
	}
	wantWords := viewer.WordCount(body)
	if env.m.stats.Words != wantWords {
		t.Fatalf("status word count = %d, want %d", env.m.stats.Words, wantWords)
	}
	status := ansi.Strip(env.m.renderStatus(200))
	if !strings.Contains(status, fmt.Sprintf("%d words", wantWords)) {
		t.Fatalf("status bar word count did not update:\n%s", status)
	}
}

func TestLiveReloadRawModeStaysRaw(t *testing.T) {
	env := newLiveEnv(t)
	env.openAlpha(t)
	env.m = pressKey(t, env.m, tea.KeyPressMsg{Code: tea.KeyTab}) // focus viewer
	env.m = pressKey(t, env.m, tea.KeyPressMsg{Code: 'r', Text: "r"})
	if !env.m.viewer.RawMode() {
		t.Fatal("r did not switch the viewer to raw mode")
	}

	body := "# Alpha\n\nraw refreshed marker\n"
	rewriteFile(t, env.alpha, body)
	env.change(env.alpha)
	env.m = runAndApply(t, env.m, env.fireTick(t))

	if !env.m.viewer.RawMode() {
		t.Fatal("live reload left raw mode")
	}
	if !strings.Contains(ansi.Strip(env.m.viewer.BaseContent()), "raw refreshed marker") {
		t.Fatalf("raw source was not reloaded:\n%s", env.m.viewer.BaseContent())
	}
}

func TestLiveReloadBurstsCoalesce(t *testing.T) {
	env := newLiveEnv(t)
	env.openAlpha(t)

	var delays []time.Duration
	for i := range 3 {
		rewriteFile(t, env.alpha, fmt.Sprintf("# Alpha\n\nburst %d\n", i))
		env.change(env.alpha)
		delays = append(delays, env.m.fsDelay)
	}
	if got, want := delays, []time.Duration{fsDebounce, 2 * fsDebounce, 4 * fsDebounce}; len(got) != len(want) ||
		got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("burst delays = %v, want %v", got, want)
	}

	// Earlier timers from the same burst are stale and must do no work.
	for seq := uint64(1); seq < env.m.fsSeq; seq++ {
		next, cmd := env.m.Update(fsTickMsg{Seq: seq})
		env.m = next.(Model)
		if cmd != nil {
			t.Fatalf("stale tick %d produced work", seq)
		}
	}

	cmd := env.fireTick(t)
	msgs := collectMessages(t, cmd)
	renders := 0
	for _, msg := range msgs {
		if _, ok := msg.(viewer.RenderedMsg); ok {
			renders++
		}
	}
	if renders != 1 {
		t.Fatalf("coalesced burst produced %d render jobs, want 1 (%T)", renders, msgs)
	}
	for _, msg := range msgs {
		next, nextCmd := env.m.Update(msg)
		env.m = next.(Model)
		env.m = runAndApply(t, env.m, nextCmd)
	}
	if !strings.Contains(ansi.Strip(env.m.viewer.BaseContent()), "burst 2") {
		t.Fatalf("viewer did not show the final burst content:\n%s", env.m.viewer.BaseContent())
	}
	if env.m.fsPending {
		t.Fatal("tick left the debounce pending")
	}
	if env.m.fsDelay != fsDebounce {
		t.Fatalf("delay after quiet tick = %s, want %s", env.m.fsDelay, fsDebounce)
	}
}

func TestLiveReloadNoiseFiltering(t *testing.T) {
	env := newLiveEnv(t)
	env.openAlpha(t)

	env.change(filepath.Join(env.root, ".git", "index"))
	if env.m.fsPending || len(env.m.fsPaths) != 0 {
		t.Fatalf("dot noise scheduled a reload: pending=%v paths=%v", env.m.fsPending, env.m.fsPaths)
	}

	// A non-dot, non-markdown event still rescans the tree because it may be
	// a new directory; the viewer stays put when the open file is unrelated.
	fresh := filepath.Join(env.root, "fresh.md")
	if err := os.WriteFile(fresh, []byte("# Fresh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env.change(filepath.Join(env.root, "notes.txt"))
	if !env.m.fsPending {
		t.Fatal("non-markdown event did not schedule a tree reload")
	}
	cmd := env.fireTick(t)
	if msgs := collectMessages(t, cmd); len(msgs) != 0 {
		t.Fatalf("unrelated tree reload produced viewer work: %T", msgs)
	}
	if !strings.Contains(ansi.Strip(env.m.render()), "fresh.md") {
		t.Fatalf("tree did not rescan after a non-markdown event:\n%s", ansi.Strip(env.m.render()))
	}
}

func TestLiveReloadExpandedDirectoriesJoinWatchSet(t *testing.T) {
	env := newLiveEnv(t)
	if !env.watcher.Watched(env.root) {
		t.Fatal("root was not watched at startup")
	}
	if env.watcher.Watched(env.docs) {
		t.Fatal("unexpanded directory was watched")
	}

	env.expandDir(t, "docs")
	if !env.watcher.Watched(env.docs) {
		t.Fatalf("expanding docs did not add a watch (attempts %v)", env.watcher.AddAttempts())
	}

	// Collapsing keeps the loaded children cached, so the watch stays.
	env.m = pressKey(t, env.m, tea.KeyPressMsg{Code: tea.KeyLeft})
	if !env.watcher.Watched(env.docs) {
		t.Fatal("collapsing a loaded directory removed its watch")
	}

	created := filepath.Join(env.docs, "live.md")
	if err := os.WriteFile(created, []byte("# Live\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env.change(created)
	env.m = runAndApply(t, env.m, env.fireTick(t))
	env.m = pressKey(t, env.m, tea.KeyPressMsg{Code: tea.KeyRight})
	if !strings.Contains(ansi.Strip(env.m.render()), "live.md") {
		t.Fatalf("new file inside a watched collapsed directory was not loaded:\n%s", ansi.Strip(env.m.render()))
	}
}

func TestLiveReloadDropsWatchesForDeletedDirectories(t *testing.T) {
	env := newLiveEnv(t)
	env.expandDir(t, "docs")
	if !env.watcher.Watched(env.docs) {
		t.Fatal("expanded directory was not watched")
	}

	if err := os.RemoveAll(env.docs); err != nil {
		t.Fatal(err)
	}
	env.change(env.docs)
	env.m = runAndApply(t, env.m, env.fireTick(t))
	if env.watcher.Watched(env.docs) {
		t.Fatal("deleted directory remained in the watch set")
	}

	// A recreated directory must be watched again when it is expanded. The
	// stale watch entry used to make syncWatches skip the new Add.
	if err := os.Mkdir(env.docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(env.docs, "guide.md"), []byte("# Guide\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env.change(env.docs)
	env.m = runAndApply(t, env.m, env.fireTick(t))
	env.expandDir(t, "docs")
	if !env.watcher.Watched(env.docs) {
		t.Fatalf("recreated directory was not watched (attempts %v)", env.watcher.AddAttempts())
	}
}

func TestLiveReloadDisabled(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "note.md"), []byte("# note\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	withoutWatcher := New(root, WithWatcher(nil))
	if withoutWatcher.watcher != nil {
		t.Fatal("WithWatcher(nil) produced a watcher")
	}
	if cmd := withoutWatcher.Init(); cmd != nil {
		t.Fatal("WithWatcher(nil) still subscribed in Init")
	}

	cfg := config.DefaultConfig()
	cfg.LiveReload = false
	disabled := New(root, WithConfig(cfg))
	if disabled.watcher != nil {
		t.Fatal("live_reload = false produced a watcher")
	}
	if cmd := disabled.Init(); cmd != nil {
		t.Fatal("live_reload = false still subscribed in Init")
	}

	previous := newWatcher
	newWatcher = func(bool) (watch.Watcher, error) { return nil, errors.New("no watches available") }
	t.Cleanup(func() { newWatcher = previous })
	if m := New(root); m.watcher != nil {
		t.Fatal("watcher creation failure produced a watcher")
	} else if cmd := m.Init(); cmd != nil {
		t.Fatal("watcher creation failure still subscribed in Init")
	}

	// The app still works with the watcher absent.
	next, _ := disabled.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if out := ansi.Strip(next.(Model).render()); !strings.Contains(out, "note.md") {
		t.Fatalf("disabled live reload broke the tree:\n%s", out)
	}
}

func TestLiveReloadClosesWatcherOnceOnQuit(t *testing.T) {
	env := newLiveEnv(t)
	if cmd := env.m.Init(); cmd == nil {
		t.Fatal("live watcher did not subscribe in Init")
	}

	next, _ := env.m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	env.m = next.(Model)
	if got := env.watcher.CloseCalls(); got != 1 {
		t.Fatalf("watcher Close calls = %d, want 1", got)
	}

	next, _ = env.m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	env.m = next.(Model)
	if got := env.watcher.CloseCalls(); got != 1 {
		t.Fatalf("repeated quit closed the watcher %d times", got)
	}
}

func TestLiveReloadWatchCapFallsBackToScans(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"one", "two"} {
		dir := filepath.Join(root, name)
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "note.md"), []byte("# "+name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fake := watch.NewFake()
	m := New(root, WithWatcher(fake), WithStatePath(filepath.Join(t.TempDir(), "state.toml")))
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = next.(Model)
	m.watchLimit = 2

	m = expandNamed(t, m, "one")
	if !fake.Watched(filepath.Join(root, "one")) {
		t.Fatal("first directory was not watched")
	}

	m = expandNamed(t, m, "two") // expand beyond the cap
	if fake.Watched(filepath.Join(root, "two")) {
		t.Fatal("directory beyond the watch cap was watched")
	}
	if m.watchDegraded {
		t.Fatal("hitting the cap degraded the watcher")
	}
	if got := len(fake.AddAttempts()); got != 2 {
		t.Fatalf("Add attempts = %d, want root + one only", got)
	}
	if !strings.Contains(ansi.Strip(m.render()), "note.md") {
		t.Fatalf("directory beyond the cap did not scan on expansion:\n%s", ansi.Strip(m.render()))
	}
}

func TestLiveReloadAddFailureDegradesPermanently(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"one", "two", "three"} {
		dir := filepath.Join(root, name)
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "note.md"), []byte("# "+name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fake := watch.NewFake()
	two := filepath.Join(root, "two")
	fake.AddErr = func(dir string) error {
		if dir == two {
			return errors.New("too many open files")
		}
		return nil
	}
	m := New(root, WithWatcher(fake), WithStatePath(filepath.Join(t.TempDir(), "state.toml")))
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = next.(Model)

	// Expand one, then two: the second Add fails and permanently degrades
	// the watcher.
	m = expandNamed(t, m, "one")
	m = expandNamed(t, m, "two")
	if !m.watchDegraded {
		t.Fatalf("Add failure did not degrade the watcher; attempts=%v", fake.AddAttempts())
	}
	attempts := len(fake.AddAttempts())
	if attempts != 3 {
		t.Fatalf("Add attempts = %d, want root + one + failed two", attempts)
	}

	// Further tree mutations must not retry the failed watch.
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyUp})
	if got := len(fake.AddAttempts()); got != attempts {
		t.Fatalf("degraded watcher retried Add (%d attempts, want %d)", got, attempts)
	}
	if !strings.Contains(ansi.Strip(m.render()), "note.md") {
		t.Fatal("app stopped rendering after watcher degradation")
	}
}
