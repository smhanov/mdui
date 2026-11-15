# T016: Type-to-search in the file tree
Readiness: ready

## Outcome and scope
GitHub issue #5: with the file tree focused, the user wants to start typing
immediately and have the cursor jump to the best matching entry, with the match
updating on every keystroke. No prefix key, no overlay. Cursor keys, mouse
clicks, and `esc` end the session.

Desired behavior:
- With the tree focused and no overlay, prompt, or link confirmation active,
  the first printable character that is not already a tree/app command starts
  a search session and moves the cursor to the best visible match.
- Every subsequent printable character is appended to the query and re-ranks
  the visible tree rows; `backspace` removes the last rune.
- Ranking uses the existing path-aware fuzzy scorer in `internal/finder`
  (`Score`: basename/segment-start, consecutive-run, and exact-case bonuses,
  deterministic tie-break), so typing `rdm` can reach `docs/readme.md`.
- `↑`/`↓`/`←`/`→`/`pgup`/`pgdn` end the session and perform their normal
  navigation from the landed row. `enter` ends the session and performs the
  normal tree action. Mouse click or wheel ends the session and performs the
  click/wheel action. `esc` ends the session and restores the pre-search
  cursor, like cancelling a find.
- While a session is active the status bar shows `find: <query>` and the
  match count or "no matches"; this is the only chrome change.
- Matching is over rows the tree is currently showing (the root's loaded,
  expanded, non-hidden rows), matching what the user sees. The existing `f`
  finder remains the search over every markdown file, including collapsed
  directories, and is the fallback for a query whose first character is a
  reserved command key.

Explicit non-goals: changing the `f` finder; auto-expanding collapsed
directories while typing; adding a timeout that silently ends the session;
filtering rows out of the tree; highlighting matched runes inside tree rows;
new config options.

## Context and prerequisites
Verified current state (2026-11-15, checkout at `5960a58`):
- `internal/app/app.go:426` `handleKey` handles pending links, overlays, the
  document-search prompt, and global keys, then routes the rest to
  `tree.Update` (`:498`). Global keys already consume `q`, `ctrl+c`, `/`,
  `n`/`N` only while a document search is active, `esc`, `?`, `f`, `T`, `m`,
  `tab`/`shift+tab`, and `H`/`L`/`=`.
- `tree.Model.Update` (`internal/tree/model.go:109`) owns tree keys:
  `up/down/j/k`, `left/h`, `right/l`, `enter`, `g/G`, `pgup/pgdown`, and `.`.
  It does not currently treat any other printable text.
- `tree.Model` keeps the flattened visible rows in `rows` and the cursor index
  (`internal/tree/model.go:32`). `SelectRow` (`internal/tree/click.go:44`)
  moves the cursor and returns the existing `SelectionMsg` command, which the
  root model handles at `internal/app/app.go:414` by previewing the file.
  `Row` exposes `Path`, `Name`, `Depth`, `IsDir`, `Expanded`, `Warning`.
- `internal/finder/finder.go:150` `Score(candidate, query)` returns
  `(score, matchedRunePositions, ok)`, with a documented path-aware ranking;
  `Filter` (`:120`) uses the same scorer. `internal/finder` has no UI or tree
  dependency, so `internal/tree` can import it without a cycle.
- `handleMouseClick` (`:885`), `handleMouseWheel` (`:990`), and
  `handleFsTick` (`:738`, which calls `tree.Reload`) are the places that must
  end a session; `statusLeft` (`:1101`) and `statusRight` (`:1126`) own the
  status bar text.
- Help text lives in `internal/style/help.md`; the README key table is at
  `README.md:58-76` and the finder behavior narrative at `README.md:94-111`.
- Existing tests to mirror: `internal/tree/model_test.go` for cursor moves and
  selection messages, `internal/app/app_test.go:112` for key-driven status
  updates, `internal/app/finder_test.go` for typed-input handling, and
  `internal/app/interaction_test.go` for mouse routing.

Research that shaped the design (2026-11-15):
- `nnn`'s type-to-nav is the closest match to bare typing: entries are
  filtered as the user types, arrow keys navigate, and a unique directory match
  can auto-enter (`https://www.mankier.com/1/nnn`). It works because nnn's
  navigation is arrow-led rather than vim-letter-led.
- `lf` keeps find modal behind `f` and exposes `anchorfind`/`findlen` to tune
  prefix jumps (`https://github.com/gokcehan/lf/issues/68`,
  `https://manpages.ubuntu.com/manpages/resolute/man1/lf.1.html`); ranger is
  similarly modal. That avoids stealing `j`/`k`/`h`/`l` from navigation.
- GUI explorers (Windows Explorer, macOS Finder, VS Code's Explorer Find)
  use a typed prefix buffer and live selection; VS Code needs an explicit
  Explorer Find command (`Ctrl+Alt+F`), while Explorer/Finder select on bare
  typing because they have no vim command keys.
- The conflict is therefore real: mdui already binds printable keys to
  commands. The resolved compromise is in Outcome/scope: bare typing starts a
  session only for characters not already bound, and once a session is active
  every printable character (including `j`/`k`/`h`/`l`) is query text until an
  arrow/click/`esc` ends it. This keeps navigation intact and satisfies "just
  start typing" for the common case; `f` covers names that begin with a
  reserved key.

## Implementation overview
Keep the session state in `internal/app` because that is where key precedence,
the status bar, and mouse/reload cancellation already live. `internal/tree`
gains one pure query method over its visible rows; it does not learn about key
routing or the status bar.

Data flow:
1. `handleKey` first checks active overlays/prompts, then if a tree session is
   active calls `handleTreeSearchKey`. That handler appends/removes query text,
   handles `esc`, and for navigation/enter falls through to the normal key
   pipeline after clearing the session. `ctrl+c` also falls through so it
   always quits.
2. If no session is active and the key is a non-reserved printable character,
   `startTreeSearch` records the current cursor as the origin and starts with
   that character.
3. `applyTreeSearch` calls `tree.Search(query)` and, when the best row differs
   from the cursor, calls `tree.SelectRow(best)`. The existing `SelectionMsg`
   path previews the file; no new viewer plumbing is needed.
4. `esc` restores `tree.SelectRow(origin)`. Arrow/enter/click end the session
   without restoring, so the landed row is kept.
5. Mouse click/wheel, live reload, focus change, quit, and opening an overlay
   clear the session so stale query state never survives a tree rebuild.

`tree.Search(query) (best, count int, ok bool)` iterates visible `rows`
(skipping warning rows and the root row), builds a candidate relative path from
`row.Path` (falling back to `row.Name`), scores it with `finder.Score`, counts
matches, and keeps the highest score. Ties keep the earliest row, matching the
tree's deterministic top-to-bottom order. An empty query returns no match and
does not move the cursor. The method is pure and cheap (visible rows only), so
it runs synchronously on every keystroke; the existing `f` index walk is not
touched.

Reserved first characters are exactly the keys already consumed before the
tree sees them (`q`, `/`, `n`/`N` while a document search is active, `?`, `f`,
`T`, `m`, `tab`/`shift+tab`, `H`, `L`, `=`, `ctrl+c`) plus the tree's own
single-key commands (`j`, `k`, `h`, `l`, `g`, `G`, `enter`, arrow/page keys,
`.`, `esc`). `n`/`N` are allowed to start a session when no document search is
active, because the app otherwise ignores them in that state. Modifier keys
(`ctrl`/`alt`) never start or edit a session.

## Implementation steps
1. Add `internal/tree/search.go`:
   - `func (m Model) Search(query string) (best, count int, ok bool)`.
   - Skip `row.Warning != ""` and the root row; build the candidate with
     `filepath.Rel(m.root.Path, row.Path)` and `filepath.ToSlash` so matching
     is separator-independent, falling back to `row.Name`.
   - Use `finder.Score(candidate, query)`; count every match; retain the first
     highest score. Empty query returns `(-1, 0, false)`.
   - Add `func (m Model) CursorIndex() int` (or use the existing cursor through
     a small getter) so the app can record/restore the origin.
   - Checkpoint: `internal/tree/search_test.go` with a temp tree containing
     `readme.md`, `docs/readme.md`, `notes/project.md`, a collapsed directory,
     and a hidden file. Assert basename-over-parent ranking, path-segment
     matches, deterministic tie order, count, no-match behavior, empty query,
     and that a file inside a collapsed directory is not returned (visible
     rows only).
2. Add tree cursor accessors if needed: `CursorIndex() int` and rely on the
   existing `SelectRow(index) tea.Cmd`. `SelectRow` already clamps and skips
   invalid rows for warning entries; verify a stale origin after a reload is
   handled by clamping or by clearing the session on reload.
3. Extend `Model` in `internal/app/app.go` with the session fields:
   `treeSearching bool`, `treeQuery string`, `treeOrigin int`,
   `treeMatches int`, `treeMatchOK bool`.
4. Add search helpers near `handleKey`:
   - `treeSearchReserved(key string) bool` for the reserved set above.
   - `canStartTreeSearch(msg tea.KeyPressMsg) bool` — tree focus, no overlay,
     no search prompt, no pending link, `msg.Text != ""`, no ctrl/alt, not
     reserved, and not `n`/`N` while `m.viewer.SearchActive()`.
   - `startTreeSearch(msg) (Model, tea.Cmd)` — record origin, set query to
     `msg.Text`, set fields, apply.
   - `handleTreeSearchKey(msg) (Model, tea.Cmd, bool handled)` — `esc`
     restores origin and returns handled; `backspace` removes a rune and
     re-applies; printable text appends and re-applies; any other key ends the
     session without restoring and returns unhandled so the normal pipeline
     continues.
   - `applyTreeSearch() tea.Cmd` — call `tree.Search`; if `best >= 0` and
     `best != tree.CursorIndex()`, return `tree.SelectRow(best)`.
   - `cancelTreeSearch(restore bool) tea.Cmd` — clear fields; optionally
     restore the origin row.
5. Wire into `handleKey` (`:426`) after the pending-link/overlay/search-prompt
   blocks and before `m.notice = ""`:
   - if `m.treeSearching`, run the handler and return when handled;
   - else if `canStartTreeSearch(msg)`, start and return.
   This ordering is deliberate: while a session is active `f`, `?`, `/`, `q`,
   `m`, `T`, `H`, `L`, and `=` are query text, so type-to-search behaves like
   the finder's input; `ctrl+c` and `esc` remain escapes.
6. Cancellation paths:
   - `handleMouseClick` and `handleMouseWheel`: clear without restoring before
     acting (wheel included; the issue says clicking, and a wheel scroll is
     unambiguously an end to typing).
   - `handleFsTick`: clear without restoring before `tree.Reload`.
   - Focus switch (`tab`/`shift+tab`), quit, `openFinder`, `openHelp`,
     `openTableOverlay`, and `startSearch` need no separate call if step 5's
     handler cancels on those unhandled keys; add an explicit clear in the tab
     case and in the overlay/search openers for safety.
7. Status bar:
   - `statusLeft`: after the search-prompt branch, return
     `"find: " + m.treeQuery + "▏"` while a session is active; otherwise the
     existing selected-file label.
   - `statusRight`: before the viewer-search/table/degraded hints, return
     `"no matches · esc cancel"` when the query is non-empty and
     `treeMatches == 0`, `"type to find · esc cancel"` when the query is
     empty, or `"N matches · esc cancel"` otherwise. Use the existing `plural`
     helper and let the existing truncation logic shrink the left block.
   - Checkpoint: app tests assert both strings before/after typing and that
     the normal `% · f find…` hint returns after `esc`.
8. Documentation:
   - Add a "type a name" row/line to `internal/style/help.md` and
     `README.md:58-76` explaining that typing in the tree jumps to the best
     visible match, that arrows/click/`esc` end the session, and that first
     characters already bound to commands remain commands.
   - Add a short note to the finder paragraph (`README.md:94-111`) that `f`
     searches every file while bare typing searches the visible tree.
9. Run the Verification suite and record evidence here.

## Verification
From `/Users/smhanov/mdui`:
- `gofmt -l .` prints nothing; `go vet ./...` exits 0; `go test ./...` passes.
- Focused: `go test ./internal/tree ./internal/app -run 'Search|Tree' -v`
  shows the ranking and key-routing cases.
- Manual check in a terminal (`go run . /path/to/docs`):
  - focus the tree, type `rea`; the cursor jumps to `readme.md` and the viewer
    previews it; keep typing `d m e` and the match narrows;
  - `backspace` widens the match; `esc` restores the original row;
  - after typing, `↓` ends the session and moves down from the landed row;
    `enter` opens/toggles the landed row;
  - clicking another row or using the wheel ends the session and acts on the
    click/wheel;
  - `f` still opens the finder, `j` still moves down when no session is
    active, and `q` still quits;
  - while a session is active, `j`, `k`, `f`, `?`, `/`, and `q` append to the
    query (only `ctrl+c`/`esc` leave the session);
  - `n` starts a tree search when no document search is active, but still
    advances document matches when one is;
  - a live file create/delete during a session cancels it and reloads the
    tree without a stale cursor.
- `go test ./... -count=1` once more after the manual run.

## Acceptance criteria
- [x] AC1: Given the tree focused and no session, when the user types a
      non-reserved printable character, then a session starts, the query is
      shown in the status bar, and the cursor moves to the best visible match
      as scored by `finder.Score`.
- [x] AC2: Given an active session, when the user types more characters or
      presses backspace, then the query updates and the best match/count
      refresh on every keystroke without a filesystem walk.
- [x] AC3: Given an active session, when the user presses `esc`, then the
      cursor returns to the pre-session row, the status bar returns to the
      normal hint, and no query text remains.
- [x] AC4: Given an active session, when the user presses an arrow/page key or
      `enter`, then the session ends without restoring and the key performs
      its normal tree action from the landed row.
- [x] AC5: Given an active session, when the user clicks a tree row, clicks
      elsewhere, or wheels a pane, then the session ends and the click/wheel
      action executes.
- [x] AC6: Given a query with no visible match, the cursor stays on the last
      match, the status bar says "no matches", and backspace can recover; an
      empty query shows "type to find" and moves nothing.
- [x] AC7: Reserved command keys (`j`, `k`, `h`, `l`, `g`, `G`, `enter`, `.`,
      `f`, `?`, `/`, `q`, `T`, `m`, `H`, `L`, `=`, `tab`) keep their existing
      behavior when no session is active, and `ctrl+c` still quits even while
      a session is active.
- [x] AC8: A row inside a collapsed directory is not found until it is
      expanded, and the `f` finder still finds it; hidden rows are never
      matched while hidden.
- [x] AC9: A live reload, focus switch, or overlay open clears the session so
      no query or stale origin is applied to a rebuilt tree.
- [x] AC10: README and help overlay document the feature and its reserved-key
      limitation.

## Risks and recovery
- Risk: users type a reserved first letter (`j`, `f`, `n`, `T`, ...) and
  expect search. Mitigation: document the limitation prominently and point to
  `f`; the reserved keys are exactly the existing commands, so no command is
  silently lost. A future opt-in keymap could move find to a dedicated binding,
  but that is out of scope.
- Risk: `j`/`k` becoming query text once a session is active surprises vim
  users. This is the same trade-off as the finder overlay and is ended by any
  arrow key; README/help state it explicitly.
- Risk: per-keystroke `SelectRow` could re-preview the same file or render at
  high frequency. Guard on `best != CursorIndex()`; only actual row changes
  emit `SelectionMsg`.
- Risk: restoring the origin on `esc` re-opens the previous file and resets its
  scroll. This is the expected "cancel" semantic; if it proves annoying, keep
  the cursor restore but suppress the preview until the next actual move.
- Rollback: the feature is additive app state plus a pure tree method; deleting
  the key hook and helper restores the previous routing exactly.

## Progress and handoff
- [x] Steps 1–2: added the pure visible-row `tree.Search` API and cursor getter;
  tests cover fuzzy ranking, basename preference, match counts, deterministic
  ties, empty/no-match queries, collapsed directories, and hidden files.
- [x] Steps 3–7: added app-managed type-to-search state, key precedence,
  backspace/escape/navigation behavior, cancellation on focus/mouse/reload and
  overlay entry, and status-bar feedback.
- [x] Step 8: documented tree type-to-search and the reserved-first-key
  limitation in the README and help overlay.
- [x] Step 9: focused and full verification suites pass; formatting and vet
  checks pass.
- Evidence: `go test ./internal/tree ./internal/app -run 'Search|Tree' -count=1`
  and `go test ./... -count=1` pass; `go vet ./...` passes; `gofmt -l .`
  prints nothing; `board.py validate` reports no problems; plan-ID grep returns
  no matches.
- Remaining limitation: the interactive terminal walkthrough was not run in
  this non-interactive session; key routing, cancellation, and cursor behavior
  are covered by unit tests.
- Next action: none; ready for coordinator verification and board transition.
