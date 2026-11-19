# T017: Command mode with `cd <folder>` (GitHub issue #6)

Readiness: ready

## Request

From https://github.com/smhanov/mdui/issues #6: mdui needs a way to change the
viewed folder by typing. A key combination enters a "command mode" showing an
input box; `esc` exits; `enter` runs the command; a failed command shows an
error message. The first command is `cd <folder>` — an mdui-specific command,
not a shell: `cd ..` moves to the parent folder, `cd ~/` goes home, relative
paths resolve against the current folder. Design it so more commands can be
added later.

## Outcome and scope

**In scope**

- New key `:` opens a command prompt in the status bar (same UI pattern as the
  existing `/` document-search prompt — the status bar's left block becomes the
  input box, the right block shows hints).
- While the prompt is open: printable runes append to the input, `backspace`
  (and `ctrl+h`) delete the last rune, `ctrl+u` clears the line, `enter`
  executes, `esc`/`ctrl+c` close the prompt without running anything. Mouse
  events are ignored (exactly like the search prompt).
- `cd <folder>` re-roots the whole app at the resolved folder: new tree, fresh
  viewer (welcome screen), cleared selection/searches/finder cache, watch set
  moved to the new folder, title bar updated.
- Failures show an error in the status bar's right block via the existing
  `m.notice` "⚠" mechanism: bad path, not-a-directory, unreadable directory,
  unknown command, missing argument.
- `:` must not be swallowed by the tree's type-to-search; it must always open
  the command prompt.
- Help cheatsheet (`?`) and README key table gain the new binding.

**Out of scope**

- Any other commands (`cd` is the only one; leave a clean dispatch point),
  path completion, shell execution, `~user` home expansion, quoting rules,
  remembering recent folders, changing the CLI `[folder]` argument behavior.

## Context and prerequisites

All file/symbol references below were verified against master `0d5c201`
(2026-11-15). Run everything from the repo root `/Users/smhanov/mdui`.
Module path is `github.com/smhanov/mdui` (go.mod). Tests: `go test ./...`
currently passes fully — keep it that way after every step.

Key existing patterns to copy (do not invent new ones):

- **Status-bar prompt**: `internal/app/search.go` — `startSearch`,
  `handleSearchPromptKey`, and `statusLeft`/`statusRight` branches in
  `internal/app/app.go` (lines ~1190–1252). The prompt is two Model fields
  (`searchPrompt bool` on app.Model; query text lives in the viewer). Rendering:
  `statusLeft` returns `"/" + query + "▏"`; `statusRight` returns hints.
- **Key routing order** in `internal/app/app.go` `handleKey` (line ~434):
  pendingLink → overlay switch → `searchPrompt` → tree type-to-search
  (`treeSearching` / `canStartTreeSearch`) → global key switch (`q`, `/`, `?`,
  `f`, `T`, `m`, `tab`, `H`/`L`/`=`) → focused pane. Command mode slots in
  right after `searchPrompt`.
- **Type-to-search reservation**: `treeSearchReserved` in
  `internal/app/tree_search.go` (line 9) lists keys that never seed the tree
  search. `:` must be added there.
- **Tree construction**: `tree.New(root, tree.WithHidden(...),
  tree.WithHideEmpty(...), tree.WithSortBy(...))` in `internal/tree/model.go`
  (line 79). Runtime hidden state is readable via `m.tree.Hidden()`
  (`internal/tree/reveal.go` line 10); `hideEmpty`/`sortBy` have no getters —
  store them on app.Model at construction.
- **Viewer construction**: `viewer.NewWithOptions(viewer.Options{...})` in
  `internal/viewer/model.go` (line 96). A fresh viewer shows "no file
  selected"; the first `SetSize` (path still `""`) triggers the welcome
  document render (`requestWelcome`). app.New builds it at line ~337 with
  `{Style, Emoji, WrapWidth}` — store that exact Options value on app.Model so
  `cd` can rebuild the viewer.
- **Re-root bookkeeping already centralized**: `syncWatches`
  (app.go line ~859, reconciles watcher to the tree's `LoadedDirs()`),
  `invalidateFinder` (`internal/app/finder.go` line 302, drops the fuzzy index
  which is rooted at the old folder), `resetDocumentIndex`
  (`internal/app/table.go` line 282, clears link/heading/table indexes),
  `cancelTreeSearch` (`internal/app/tree_search.go` line 82),
  `resizePanes` (app.go line ~668, pushes pane sizes into tree+viewer and
  returns the viewer render command).
- **Pending fs-change hazard**: `handleFsTick` (app.go line ~766) reloads the
  tree for paths batched before the `cd`. Guard by bumping `m.fsSeq` (stale
  ticks are ignored by sequence check) and clearing `fsDirty/fsPending/fsPaths`.
- **Errors/notices**: `m.notice` string; rendered as `"⚠ " + notice` in
  `statusRight` (app.go line ~1219). Cleared by `m.notice = ""` on next normal
  key (app.go line ~476).
- **Tests**: `internal/app/app_test.go` has `pressKey(t, m, tea.KeyPressMsg)`
  (line 183, feeds Update and applies the returned command) and
  `notesFixture(t)` (line 206, temp dir with `alpha.md`, `beta.md` — no
  subdirectory; command tests need their own fixture with a subfolder).
  Key-press literal example: `tea.KeyPressMsg{Code: ':', Text: ":"}`;
  `tea.KeyPressMsg{Code: tea.KeyEnter}` for enter.
- **Help sheet**: `internal/style/help.md` (embedded; table rows like
  ``| `/` | search document | `n` `N` | next / prev |``). Its test
  `TestHelpMarkdownListsImplementedKeys` in `internal/style/sheets_test.go`
  asserts certain tokens exist — extend the wanted list there.
- **README key table**: rows around `README.md` lines 61–78.

## Implementation overview

Add a modal text-input state to the existing app.Model, cloning the proven
`/`-search prompt pattern: two new fields (`commandPrompt`, `commandInput`),
a dedicated key handler that captures all input while open, and status-bar
rendering branches. `:` opens it (reserved from type-to-search). On `enter`,
a small parser splits the line into command name + argument, and a dispatch
switch runs it — `cd` is the first (and initially only) command. `cd` resolves
its argument to an absolute cleaned path (`~` expansion, then join with the
current root unless absolute), validates it is a readable directory, and calls
a new `changeRoot` method that rebuilds every piece of state that was
constructed against the old root: tree model, viewer model, selection,
document index, finder cache, watch set, and pending filesystem-change
bookkeeping (invalidated by sequence bump). Errors never mutate state; they
set `m.notice`, which the status bar already renders as a ⚠ message. Docs
(help.md, README) gain one line each. Everything lives in `internal/app`
except the doc edits; no changes to tree/viewer/finder/watch packages.

## Implementation steps

Work in small increments; after each, run `go build ./... && go test ./...`.

### Step 1 — Model fields and construction wiring (`internal/app/app.go`)

1. Add fields to `Model` (near `searchPrompt`, line ~103):

   ```go
   commandPrompt bool
   commandInput  string
   treeHideEmpty bool
   treeSortBy    string
   viewerOpts    viewer.Options
   ```

2. In `New` (line ~280), after `m.docStyle` defaulting (line ~308) and before
   building the tree/viewer, store the config for later rebuilds:

   ```go
   m.treeHideEmpty = cfg.hideEmpty
   m.treeSortBy = cfg.sortBy
   m.viewerOpts = viewer.Options{
       Style:     m.docStyle,
       Emoji:     cfg.emoji,
       WrapWidth: cfg.wrapWidth,
   }
   ```

   Then build the viewer from it: replace the literal at line ~337 with
   `m.viewer = viewer.NewWithOptions(m.viewerOpts)` (field order/values are
   identical to today's literal; do not change the tree construction).

   Checkpoint: `go build ./...` and the whole suite still pass (pure wiring).

### Step 2 — New file `internal/app/command.go` with the prompt + `cd`

Create `internal/app/command.go` containing exactly these functions (copy the
editing behavior from `handleSearchPromptKey` in `internal/app/search.go`
lines 27–53; reuse `trimLastRune` from search.go — do not duplicate it):

```go
package app

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
)

// startCommandPrompt opens the status-bar command prompt (":" key).
func (m Model) startCommandPrompt() (tea.Model, tea.Cmd) {
	m.cancelTreeSearch(false)
	m.notice = ""
	m.commandPrompt = true
	m.commandInput = ""
	return m, nil
}

// handleCommandKey captures every key while the command prompt is open, so
// q, /, ? and spaces are literal input. enter runs the line; esc/ctrl+c
// close the prompt without running anything.
func (m Model) handleCommandKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.commandPrompt = false
		m.commandInput = ""
		m.notice = ""
		return m, nil
	case "enter":
		m.commandPrompt = false
		line := m.commandInput
		m.commandInput = ""
		return m.runCommand(line)
	case "backspace", "ctrl+h":
		m.commandInput = trimLastRune(m.commandInput)
		return m, nil
	case "ctrl+u":
		m.commandInput = ""
		return m, nil
	}
	if msg.Mod.Contains(tea.ModCtrl) || msg.Mod.Contains(tea.ModAlt) {
		return m, nil
	}
	if msg.Text != "" {
		m.commandInput += msg.Text
	}
	return m, nil
}

// parseCommand splits a command line into a name and the remaining argument
// text. Paths keep their internal spaces; only the first whitespace run
// separates name from argument.
func parseCommand(line string) (name, rest string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", ""
	}
	i := strings.IndexFunc(line, unicode.IsSpace)
	if i < 0 {
		return line, ""
	}
	return line[:i], strings.TrimSpace(line[i+1:])
}

// runCommand executes one command line. cd is the only command so far;
// anything else reports an error through the notice line.
func (m Model) runCommand(line string) (tea.Model, tea.Cmd) {
	name, rest := parseCommand(line)
	switch name {
	case "":
		return m, nil
	case "cd":
		target, err := resolveCdPath(m.root, rest)
		if err != nil {
			m.notice = err.Error()
			return m, nil
		}
		if err := validateCdTarget(target); err != nil {
			m.notice = "cd: " + rest + ": " + err.Error()
			return m, nil
		}
		if target == m.root {
			return m, nil
		}
		m.notice = ""
		return m, m.changeRoot(target)
	default:
		m.notice = fmt.Sprintf("unknown command %q — try cd <folder>", name)
		return m, nil
	}
}

// resolveCdPath resolves a cd argument against the current root: "~" and
// "~/" expand to the user's home, absolute paths pass through, and anything
// else is relative to root. The result is cleaned and absolute.
func resolveCdPath(root, arg string) (string, error) {
	if arg == "" {
		return "", errors.New("usage: cd <folder>")
	}
	path := arg
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", errors.New("cd: cannot determine home directory")
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~"))
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	return filepath.Clean(path), nil
}

// validateCdTarget reports whether path is an existing, listable directory.
func validateCdTarget(path string) error {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return errors.New("no such directory")
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("not a directory")
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Readdirnames(1); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

// changeRoot re-scopes the app to a new root folder: fresh tree and viewer,
// cleared selection and caches, watch set moved, and any queued filesystem
// change batch from the old root discarded.
func (m *Model) changeRoot(root string) tea.Cmd {
	m.root = root
	m.cancelTreeSearch(false)
	if m.viewer.SearchActive() {
		m.viewer.CancelSearch()
	}
	m.tree = tree.New(root,
		tree.WithHidden(m.tree.Hidden()),
		tree.WithHideEmpty(m.treeHideEmpty),
		tree.WithSortBy(m.treeSortBy),
	)
	m.viewer = viewer.NewWithOptions(m.viewerOpts)
	m.selected = ""
	m.stats = viewer.StatsMsg{}
	m.focus = FocusTree
	m.pendingLink = nil
	m.tableClipped = false
	m.resetDocumentIndex()
	m.invalidateFinder()
	m.syncWatches()
	m.fsDirty = false
	m.fsPending = false
	m.fsPaths = nil
	m.fsSeq++
	return m.resizePanes()
}
```

Notes for the implementer:

- Imports needed in app.go are already present (`tree`, `viewer`); command.go
  needs the imports listed above — remove any the compiler reports unused.
- `m.viewer.SearchActive()` is safe on the old viewer; `CancelSearch` before
  discarding it is belt-and-braces (fresh viewer has no search anyway).
- Do not reorder: `syncWatches` must run after the new tree exists (it reads
  `m.tree.LoadedDirs()`); `resizePanes` last (it returns the welcome-render
  command and pushes pane heights into the new children).
- `changeRoot` is a pointer method called on the local copy inside the
  value-receiver `runCommand`, mirroring how `handleKey` already calls
  `m.resizePanes()` — that is the established pattern in this codebase.

Checkpoint: `go build ./...` passes; no behavior change yet (nothing calls
these functions).

### Step 3 — Routing and reservation

1. `internal/app/tree_search.go`: add `":"` to the key list in
   `treeSearchReserved` (line 11–13), e.g. after `"f", "T",` add `":"`.
2. `internal/app/app.go` `handleKey` (line ~434): immediately after the
   `if m.searchPrompt { ... }` block (line ~462), add:

   ```go
   if m.commandPrompt {
       return m.handleCommandKey(msg)
   }
   ```

3. In the global key switch (line ~481), add a case next to `"/"`:

   ```go
   case ":":
       return m.startCommandPrompt()
   ```

4. Mouse guards — extend the `m.searchPrompt` early-returns to
   `m.searchPrompt || m.commandPrompt` in: `handleMouseClick` (line ~916),
   `handleMouseMotion` (line ~1036), `handleMouseRelease` (line ~1059), and
   `handleMouseWheel` (line ~1079).

Checkpoint: manual smoke test possible: `go run . specs.md`'s folder or any
folder — actually run `go run . .` in a temp folder with two markdown files,
press `:` (prompt appears), type garbage, `enter` (⚠ unknown command), `:cd /`
+ enter (tree shows `/`, title bar updates), `esc` closes prompt.

### Step 4 — Status bar rendering (`internal/app/app.go`)

1. In `statusLeft` (line ~1190), right after the `pendingLink` branch, add:

   ```go
   if m.commandPrompt {
       return ":" + m.commandInput + "▏"
   }
   ```

2. In `statusRight` (line ~1218), before the `searchPrompt` branch, add:

   ```go
   if m.commandPrompt {
       return "type a command · enter runs · esc cancels"
   }
   ```

### Step 5 — Tests (`internal/app/command_test.go`, new file)

Write these tests in package `app`, using the existing helpers `pressKey`,
`New` from `app_test.go`, and `t.TempDir()`. Build a fixture with a
subdirectory (notesFixture has none):

```go
func cdFixture(t *testing.T) (root, sub string) {
	t.Helper()
	root = t.TempDir()
	sub = filepath.Join(root, "docs")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		filepath.Join(root, "top.md"),
		filepath.Join(sub, "inner.md"),
	} {
		if err := os.WriteFile(name, []byte("# doc\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root, sub
}
```

Required tests (exact names):

1. `TestCommandPromptEditAndEscape` — `New(root)`, size it
   (`m.width, m.height = 100, 30`), press `:` via
   `tea.KeyPressMsg{Code: ':', Text: ":"}`; assert `m.commandPrompt` is true,
   `m.treeSearching` false, and `m.render()` contains `":"` + `"▏"` and
   `"enter runs"`. Append text `c`, `d` (KeyPressMsg with Text) →
   `m.commandInput == "cd"`. `backspace` → `"c"`. `ctrl+u` → `""`. `esc` →
   prompt closed, render no longer contains `"▏"`.
2. `TestCommandCdChangesRoot` — from `cdFixture`, press `:`, type
   `cd docs` (as separate KeyPressMsgs with Text, including a
   `tea.KeyPressMsg{Code: ' ', Text: " "}` for the space), `enter`. Assert:
   `m.root == sub` (both are already absolute+clean: TempDir is), `m.selected
   == ""`, `m.commandPrompt` false, and `m.render()` contains `"mdui — " + sub`
   and `"inner.md"` and `"top.md"` is absent.
3. `TestCommandCdParentAndHome` — from `cdFixture`: `cd ..` moves root to
   `filepath.Dir(root)`; then `cd ~` moves root to the value returned by
   `os.UserHomeDir()` (skip the home assertion if it errors — CI containers
   may lack a home; then assert only `cd ..`).
4. `TestCommandCdErrors` — from `cdFixture`, `cd nope` → prompt closed,
   `m.notice` contains `"no such directory"`, `m.root` unchanged, render
   contains `"⚠"`. `cd top.md` → notice contains `"not a directory"`.
   Bare `cd` (no arg) → notice contains `"usage: cd <folder>"`.
   `frobnicate x` → notice contains `"unknown command"`, root unchanged.
5. `TestCommandColonReservedFromTreeSearch` — `New(root)` (tree focus is
   default), press `:` → `m.commandPrompt` true AND `m.treeSearching` false
   (proves the reservation works; fails if Step 3.1 is skipped).
6. `TestCommandMouseIgnoredWhilePromptOpen` — open prompt, send
   `m.Update(tea.MouseClickMsg{...})` with a left click in the tree region
   (see `mouse_test.go` for construction); assert no selection happened
   (`m.selected == ""`) and prompt still open.
7. `TestResolveCdPath` — table test for `resolveCdPath`: `("", …) → error`;
   `("/abs/base", "x") → /abs/base/x`; `("x/../y")` cleaning; `("~")` and
   `"~/sub"` home expansion; absolute passthrough `("/etc", "/var") → /var`.

Also extend `TestHelpMarkdownListsImplementedKeys`
(`internal/style/sheets_test.go`) wanted list with `"`:`"` and
`"command mode"` (matching whatever wording help.md uses).

### Step 6 — Documentation

1. `internal/style/help.md`: add a row to the key table, e.g.
   ``| `:` | command mode | `cd <folder>` | change folder |``
   and a trailing prose line: "`:` opens the command line: `cd <folder>`
   changes the viewed folder (`cd ..`, `cd ~/`, absolute and relative paths
   work); enter runs it, esc cancels."
2. `README.md` key table (after the `m` row around line 78): add
   ``| `:` | command mode: `cd <folder>` changes the viewed folder |``

## Verification

Run from the repo root:

- `go build ./...` — compiles.
- `go vet ./...` — clean.
- `go test ./...` — all packages pass, including the new
  `internal/app/command_test.go` tests (run verbosely once:
  `go test ./internal/app/ -run 'TestCommand|TestResolveCd' -v`).
- Manual TUI check (`go run . <some folder with .md files>`):
  1. `:` opens prompt; typing echo works; `esc` closes.
  2. `:cd <subfolder>` + enter swaps the tree, title bar shows the new root,
     viewer shows the welcome screen.
  3. `:cd ..` returns; `:cd ~` goes home; `:cd /` works.
  4. `:cd /definitely/not/here` shows ⚠ message and keeps the current folder.
  5. `:wat` shows ⚠ unknown command.
  6. After `cd`, `f` (finder) still works and lists only the new root's files
     (finder cache was invalidated); live-reload still updates the new tree
     (edit a file under the new root, tree/viewer refresh).
  7. Type `:` while tree type-to-search is idle — command prompt must open,
     not seed a `:` search.
- repo-todo gate (before marking done):
  `git grep -nIE '(^|[^A-Za-z0-9_])T[0-9]{3,}([^0-9]|$)' -- . ':!.plans'`
  must print nothing.

## Acceptance criteria

- [x] AC1: Pressing `:` with the tree focused opens a command prompt in the
      status bar (`:` + input + `▏`), never a tree search. (Test 5, manual 7)
- [x] AC2: While the prompt is open, printable keys edit the input
      (backspace/ctrl+h/ctrl+u behave as specified), mouse events change
      nothing, `esc`/`ctrl+c` close it without running. (Tests 1, 6)
- [x] AC3: `cd docs` re-roots: `m.root` becomes the subfolder, render shows
      the new root in the title and its files, selection cleared. (Test 2)
- [x] AC4: `cd ..` goes to the parent; `cd ~`/`cd ~/x` expands home;
      absolute paths pass through; all results cleaned+absolute. (Tests 3, 7)
- [x] AC5: Failed commands (missing dir, file-as-dir, missing arg, unknown
      command) show a ⚠ notice naming the problem, change no state, and close
      the prompt. (Test 4)
- [x] AC6: After `cd`, the watcher, finder (`f`), and document indexes reset
      to the new root (fs-tick sequence bump prevents stale reloads).
      (Code inspection of changeRoot; manual 6)
- [x] AC7: `go build ./... && go vet ./... && go test ./...` all pass, and
      help.md/README document `:` and `cd`. (sheets_test extended)

## Risks and recovery

- **Stale fs tick after cd** would reload the fresh tree with old-root paths:
  mitigated by `fsSeq++` + clearing the batch; covered by inspection and
  manual 6.
- **Tree/viewer option drift** (hidden toggled at runtime, sort/hideEmpty from
  config): hidden is read live from `m.tree.Hidden()` before rebuild;
  sort/hideEmpty stored once in `New`. If a future option is added to
  `tree.New`/`viewer.NewWithOptions`, `changeRoot` must pass it too.
- **Search-prompt interplay**: `/` and `:` are mutually exclusive modes by
  routing order; each handler captures all keys, so neither can open while
  the other is active. No conflict.
- **Render width**: a very long command input can push the status bar layout;
  the same truncation logic as the search prompt applies (`renderStatus`
  handles negative gaps) — acceptable, matches existing behavior.
- Rollback: the feature is additive (one new file + localized branches);
  `git revert` of the implementing commit restores prior behavior cleanly.

## Progress and handoff

- Completed: command prompt (`:`), `cd <folder>` re-root, reservation from
  tree type-to-search, status-bar rendering, help/README bindings, tests.
- Changed files:
  - `internal/app/command.go` (new)
  - `internal/app/command_test.go` (new)
  - `internal/app/app.go` (model fields, New wiring, key routing, mouse guards, status bar)
  - `internal/app/tree_search.go` (`:` reserved)
  - `internal/style/help.md`
  - `internal/style/sheets_test.go`
  - `README.md`
- Evidence:
  - `go build ./...` — pass
  - `go vet ./...` — pass
  - `go test ./internal/app/ -run 'TestCommand|TestResolveCd' -count=1` — pass
  - `go test ./...` — pass (worker, then coordinator re-run 2026-11-19)
  - `git grep -nIE '(^|[^A-Za-z0-9_])T[0-9]{3,}([^0-9]|$)' -- . ':!.plans'` — no matches
  - AC6 by inspection of `changeRoot`: new tree then `syncWatches`, `invalidateFinder`, `resetDocumentIndex`, `fsSeq++` and cleared `fsDirty`/`fsPending`/`fsPaths`.
- Deviation: help.md prose notes were joined into fewer paragraphs, and the
  trailing command sentence was shortened to "`:` opens command mode.
  `cd <folder>` changes the folder; enter runs, esc cancels." The plan's
  longer sentence (`cd ..`, `cd ~/`) overflowed the 100x30 help overlay and
  replaced the last row with the truncation banner. The table row still
  documents `cd <folder>`, and README documents the binding. No existing
  help sentences were removed. The close-hint sentence stays its own short
  paragraph so "to close this help" remains contiguous.
- `:` during an already-active tree search appends to the query. Reservation
  only blocks starting a search, matching other printable bindings (`q`, `/`).
- Manual TUI check was not run (finder after cd, live reload, `cd /`).
- AC6 by inspection only; no automated watcher/finder test after cd.
- Remaining/blocker: none known.
- Next action: none. Coordinator verified 2026-11-19.
