# T015: Thin draggable scrollbars for the tree and viewer panes
Readiness: ready

## Outcome and scope
GitHub issue #4: scrolling a pane gives no visual indication of position, and
neither pane can be dragged to a position the way a GUI scrollbar allows.

Desired behavior:
- Each pane shows a thin vertical scrollbar on its right edge whenever its
  content is taller than the pane. The track is dim; the thumb is visibly
  brighter and at least one cell tall.
- The thumb's size and position track `visible/total` and `offset` live while
  keys, the mouse wheel, search jumps, document re-renders, and live reloads
  change the scroll position.
- With mouse capture on, clicking the track jumps to that position and
  dragging the thumb scrolls continuously. Releasing ends the drag without
  changing the pane split.
- The bar is purely visual when mouse capture is off (`m`); it still shows
  position but does not capture clicks.
- No bar is drawn when all content fits, so short documents and small trees
  look unchanged.
- Text layout does not shift: the bar occupies the pane's existing right-hand
  padding cell, next to the divider (tree) or inside the app frame (viewer).

Explicit non-goals: horizontal scrollbars; a scrollbar in overlays, the help
screen, or the table overlay; changing wheel/keyboard scroll semantics.

## Context and prerequisites
Verified current state (2026-11-15, checkout at `5960a58`):
- `specs.md:416` already lists this as an optional phase-1.5 feature:
  "optional thin scrollbar on the viewer's right edge (`▐` markers or
  `bubbles/scrollbar`)". Extend the idea to the tree as the issue asks.
- `internal/tree/view.go:13` renders the tree through `style.Pane`; the pane
  style (`internal/style/styles.go:37`) has one cell of padding on each side.
  `visibleLines` (`internal/tree/view.go:31`) pads text to the inner width and
  the pane supplies vertical padding. `windowStart` (`internal/tree/view.go:54`)
  already computes the first visible row from `cursor` and `height`, and
  `VisibleStart` (`internal/tree/click.go:24`) exposes it.
- `internal/viewer/model.go:398` renders `m.viewport.View()` through
  `style.Pane`. `internal/viewer/model.go:553-562` exposes the viewport offset
  through `ScrollOffset`/`SetScrollOffset`; `Stats()` (`:579`) already derives
  the status-bar percentage from `viewport.ScrollPercent()`.
- The pinned `charm.land/bubbles/v2 v2.2.1` viewport exposes
  `TotalLineCount()`, `VisibleLineCount()`, `Height()`, `YOffset()`, and
  `SetYOffset()`; soft wrap is enabled (`m.viewport.SoftWrap = true` at
  `internal/viewer/model.go:111`), so `TotalLineCount` is the wrapped-line
  count the bar should use.
- Mouse routing lives in `internal/app/app.go`: `regionAt` (`:614`) maps a
  terminal cell to tree/divider/viewer, and the divider hit zone is
  `treeW-1 .. treeW+1` (`:629`). The tree's rightmost cell is therefore
  currently part of the divider drag zone; a visible tree scrollbar must take
  precedence there while the divider keeps its own column and viewer-side
  slop.
- Mouse handlers: click `:885`, motion `:966`, release `:978`, wheel `:990`.
  `handleMouseMotion`/`handleMouseRelease` currently only track the splitter
  drag (`m.dragging`); a second drag state is needed so the two cannot be
  confused. Shift-click already bypasses capture before region routing
  (`:899`).
- Tests to keep green: `internal/app/mouse_test.go` (divider zones, drag
  debounce, wheel), `internal/app/layout_test.go`, `internal/tree/view_test.go`,
  `internal/viewer/model_test.go`, and the render-width/height assertions in
  `internal/app/app_test.go`.

## Implementation overview
Add a small terminal-independent `internal/scrollbar` package that owns the
geometry: given `total`, `visible`, `offset`, and `height`, it reports whether
a bar is shown, the thumb start/length, and the offset corresponding to a
dragged thumb position. Both panes use the same math. Rendering stays in the
pane packages, which already import `internal/style`.

Each pane's `View` keeps its text width unchanged. When the bar is shown it
swaps `style.Pane` for an asymmetric `PaneScroll` style with only left padding,
then appends one styled glyph to every visible row, including rows padded below
the content. The resulting block is exactly the same width and height as today,
so no layout or hit-test column moves. `internal/style` gains `ScrollTrack`
(dim `│`) and `ScrollThumb` (bold `▐`) styles; the thumb glyph reads as a
right-edge slider without competing with the single-column divider.

`tree.Model` gains `ScrollState()` (total rows, visible rows, window start,
pane height) and `ScrollTo(offset)`; `ScrollTo` clamps the offset, keeps the
cursor inside the new window, and returns the existing `SelectionMsg` command
only if the cursor moved, so dragging previews at most one file per changed
row. `viewer.Model` gains the same two methods backed by the viewport's
`TotalLineCount`/`Height`/`YOffset`/`SetYOffset`. Viewer scroll changes reuse
`statsCommand(true)` so the status-bar percentage follows the bar.

`internal/app` adds a scrollbar drag state (pane + grabbed row offset) separate
from `m.dragging`. `regionAt` reports `regionTreeScroll`/`regionViewerScroll`
before the divider zone when the corresponding bar is shown. Clicking the thumb
starts a drag without jumping; clicking the track centers the thumb on the
click and starts a drag. Motion maps the pointer through
`scrollbar.OffsetForThumbStart`; release clears the drag. Wheel events over a
scrollbar column are treated as wheel events for that pane, not as drags.
Shift-click and mouse-off paths remain pass-through.

## Implementation steps
1. Add `internal/scrollbar/scrollbar.go`:
   - `type State struct { Total, Visible, Offset, Height int }`.
   - `Shown() bool` — true only when `Total > Visible`, `Visible > 0`, and
     `Height > 1`.
   - `Thumb() (start, size int)` — thumb size
     `max(1, round(Height*Visible/Total))`, clamped to `Height`; travel
     `Height-size`; start `round(travel*Offset/max(1,Total-Visible))`, clamped
     to `[0, travel]`.
   - `OffsetForThumbStart(start int)` — clamp `start` to `[0, travel]`, then
     invert the formula with rounding and clamp to `[0, Total-Visible]`.
   - All methods are pure and deterministic; no terminal or style imports.
   - Checkpoint: table-driven unit tests in
     `internal/scrollbar/scrollbar_test.go` for short content, one-line thumb,
     top/middle/bottom offsets, clamping, and round-trip
     `OffsetForThumbStart(Thumb()) == Offset`.
2. Add `ScrollTrack` and `ScrollThumb` styles and a `PaneScroll` style with
   left-only padding to `internal/style/styles.go`. Keep `Pane` unchanged for
   every other caller.
3. Tree geometry and scroll API:
   - Add `internal/tree/scroll.go` (or methods beside `windowStart`) with
     `ScrollState() scrollbar.State` using `len(m.rows)`, `min(m.height,
     len(m.rows))`, `m.windowStart(m.height)`, and `m.height`.
   - Add `ScrollTo(offset int) tea.Cmd`: clamp to `[0, max(0, len(rows)-height)]`;
     if the cursor is outside the new window, move it to the nearest edge row
     (skipping warning rows via `nearestSelectable`); set `m.offset`; return
     `selectionCmd()` only when the cursor changed.
   - Checkpoint: `internal/tree/scroll_test.go` asserts window start changes,
     the cursor stays inside, and dragging to the bottom/top clamps.
4. Render the tree bar in `internal/tree/view.go`:
   - Compute `state := m.ScrollState()`.
   - When `state.Shown()`, render row text at `width-2` as today, pad every
     line to that width, append the track/thumb glyph for each of `height`
     rows, and wrap with `style.PaneScroll` at the same `width/height`.
   - When not shown, keep the current `style.Pane` path byte-for-byte.
   - Checkpoint: `internal/tree/view_test.go` asserts the rightmost cell is a
     bar glyph only when rows exceed height, rendered width is still `width`,
     and a short tree has a space in that cell.
5. Viewer geometry and render:
   - Add `ScrollState() scrollbar.State` and `ScrollTo(offset int) tea.Cmd` to
     `internal/viewer/model.go`; `ScrollTo` uses `viewport.SetYOffset` and
     returns `statsCommand(true)`.
   - In `View` (`:398`), keep the viewport at `innerWidth(width)` (text width),
     split its output into lines, pad to `innerHeight(height)`, append the
     glyph per row when shown, and wrap with `style.PaneScroll`; otherwise use
     the current `style.Pane` path.
   - Checkpoint: `internal/viewer/model_test.go` sets a long document, renders
     at a fixed size, and asserts the last cell of every body row is a track or
     thumb glyph; a short document leaves it blank; soft-wrapped content
     produces a thumb proportional to wrapped lines.
6. App hit-testing and dragging:
   - Extend `paneRegion` with `regionTreeScroll` and `regionViewerScroll`.
     In `regionAt`, after the `contentX` bounds check and before the divider
     branch, return the scrollbar region when the pane's `Shown()` is true and
     `contentX` equals `treeW-1` (tree) or `innerW-1` (viewer). Keep the
     divider at its existing columns; the tree bar only shadows the
     tree-side slop cell.
   - Add fields for scroll-drag state, e.g. `scrollPane paneRegion` and
     `scrollGrab int`, plus `scrollDragging() bool`.
   - `handleMouseClick`: for a scrollbar region, clear double-click state,
     compute `start,size := state.Thumb()`, set `grab = 0` when the click is on
     the thumb or `size/2` on the track (so a track click centers the thumb),
     apply `State.OffsetForThumbStart(y-bodyTop-grab)` through `tree.ScrollTo`
     or `viewer.ScrollTo`, and store the drag state.
   - `handleMouseMotion`: if a scrollbar drag is active and `overlayNone` and
     mouse capture is on, clamp `y-bodyTop` to the pane height and apply the
     same mapping; otherwise preserve the splitter path.
   - `handleMouseRelease`: clear either drag state; save the split ratio only
     for the splitter.
   - `handleMouseWheel`: route the two scrollbar regions to the same pane
     scroll path as `regionTree`/`regionViewer`.
   - `toggleMouse` and quit clear the scrollbar drag state. Shift-click still
     returns before region routing.
   - Checkpoint: `internal/app/mouse_test.go` covers track click, thumb drag,
     release, top/bottom clamping, wheel over the bar, divider still draggable
     at `treeW`/`treeW+1`, and no scrollbar capture when the pane fits or
     mouse is off.
7. Add a short line to `internal/style/help.md` and the README's Mouse table
   describing the bar ("thin scrollbar: click track / drag thumb"), and the
   README feature bullet. Keep the help overlay text within its 80-column
   layout.
8. Run the full gates in Verification and record evidence in this plan.

## Verification
From `/Users/smhanov/mdui`:
- `gofmt -l .` prints nothing.
- `go vet ./...` exits 0.
- `go test ./...` passes, including the new scrollbar, tree, viewer, and app
  mouse tests.
- `go test ./internal/scrollbar ./internal/tree ./internal/viewer ./internal/app -run 'Scrollbar|Scroll|Divider|View|Wheel' -v`
  shows geometry and mouse cases passing.
- Manual check with a folder of many markdown files:
  `go run . /path/to/many-docs` in a ≥80×24 terminal. Verify:
  - both panes show a dim track with a brighter thumb;
  - the thumb grows/shrinks when the pane is resized and moves with `j`/`k`,
    wheel, `g`/`G`, search jumps, and live file edits;
  - clicking the tree track jumps the list, dragging the thumb scrolls and
    previews rows without moving the divider; the viewer behaves the same;
  - a folder with one short file and a one-page document shows no bars;
  - `m` turns bar interaction off while leaving the bar visible;
  - shift-drag still hands selection to the terminal.
- `git diff --stat` contains only scrollbar-related code, styles, docs, and
  tests; no change to divider math or viewer render width.

## Acceptance criteria
- [x] AC1: Given content taller than a pane, when the app renders at width W
      and height H, then the pane's last column contains a track with a
      contiguous thumb of at least one cell, the pane is exactly W×H, and
      content text is not narrowed by more than the existing padding.
- [x] AC2: Given the thumb state for offsets at top/middle/bottom, the thumb
      start and size match the `visible/total` and `offset` formulas within one
      cell, and the pane can be scrolled to the true top and bottom.
- [x] AC3: Given mouse capture on, when the user clicks the track, the pane
      jumps so the thumb centers at the click; when the user drags the thumb,
      the pane scrolls continuously and the thumb follows; release ends the
      drag and does not alter the split ratio.
- [x] AC4: Given the tree scrollbar is visible at `treeW-1`, a click there
      scrolls the tree rather than starting a divider drag, while clicks at
      `treeW` and `treeW+1` still drag the divider.
- [x] AC5: Given content that fits, no bar is drawn and every existing
      divider/wheel/render test passes unchanged.
- [x] AC6: Given mouse capture off or a shift-click, scrollbar clicks are
      passed through and do not scroll or start a drag.
- [x] AC7: The status-bar scroll percentage continues to track viewer offset
      while the viewer bar is dragged.

## Risks and recovery
- Risk: ANSI-styled markdown lines end with escape sequences; appending a
  glyph could inherit a background. Mitigate by appending a fully styled
  `style.ScrollTrack/ScrollThumb` cell (lipgloss emits its own reset) and
  testing with a code-block document.
- Risk: reserving the tree's rightmost cell breaks divider hit-test
  assumptions. Mitigate with explicit `regionAt` order tests and keep the
  divider at its own column plus viewer-side slop.
- Risk: dragging the tree bar emits a preview per row change, causing render
  churn on large files. `ScrollTo` only emits when the cursor row changes;
  motion is already coalesced by the event loop. If profiling shows cost,
  defer the preview command to release without changing geometry.
- Rollback: the scrollbar package and styles are additive; reverting the View
  call sites removes all visible behavior, and mouse regions fall back to the
  old divider logic.

## Progress and handoff
- Completed: implementation steps 1–8. Added shared scrollbar geometry, tree
  and viewer rendering/APIs, mouse hit-testing and drag handling, help/README
  notes, and geometry/render/mouse tests.
- Evidence: `go test ./...`, `go vet ./...`, and `gofmt -l .` pass; focused
  scrollbar/tree/viewer/app tests pass. `git diff --check` is clean. Automated
  tests cover dimensions, fit/no-fit rendering, top/bottom drag behavior,
  divider hit regions, and viewer stats updates.
- Remaining/blocker: interactive terminal verification with a large real
  folder was not run in this session.
- Next action: manually verify both bars in a terminal when convenient; no
  code work remains.
