# mdui — Terminal Markdown Viewer

**Spec & Design Document**

A TUI markdown viewer/browser written in Go. Give it a folder, get a two-pane
interface: a file tree of markdown content on the left, beautifully rendered
markdown on the right. Panes are resizable with the mouse. Rendering is
optimized for tables.

---

## 1. Overview

### Goals

- `mdui [folder]` launches a full-screen (alt-screen) TUI.
- Left pane: a tree browser of the folder, showing **only directories and
  markdown files** (`.md`, `.markdown`). Hidden files and everything else are
  filtered out.
- Right pane: the selected markdown file, rendered with a nice colour scheme
  (headings, code blocks, lists, and especially **tables**).
- Selecting a file (click or keyboard) renders it immediately.
- Panes are resizable by **dragging the divider with the mouse**; keyboard
  resize also supported.
- Fast: renders large files without stutter; scrolling is smooth.

### Non-goals (v1)

- Editing markdown (open in `$EDITOR` instead — see nice-to-haves).
- Fetching remote markdown over HTTP.
- Rendering Mermaid diagrams or images.
- Windows-specific features beyond what Bubble Tea gives us for free.

### Design principles

1. **Vim-friendly** — all functionality reachable from the keyboard; mouse is
   an additive layer, never required.
2. **Fast by default** — render cache + async rendering; the UI never blocks
   on file I/O or markdown rendering.
3. **Tables are first-class** — the rendering pipeline is tuned so tables are
   readable at any pane width.
4. **Degrade gracefully** — no colour terminal? Use `ascii` style. Tiny pane?
   Switch strategies instead of wrapping garbage.

---

## 2. Research Summary

### 2.1 Prior art

| Tool | What it is | What we take / avoid |
|---|---|---|
| [Glow](https://github.com/charmbracelet/glow) (v3, `charm.land/glow/v3`) | Charm's terminal markdown reader. TUI lists *discovered* markdown files (no folder tree), renders via Glamour. | Take: pager keybindings, `?` help overlay, dark/light auto-detect, JSON stylesheets, config file pattern. Avoid: its flat discovery list — we want a **directory tree** scoped to the argument folder. |
| `glow`/Glamour table redesign | Glamour v0.10.0 reworked tables: links are pulled out of cells and listed under the table for legibility/clickability (done with the GitHub CLI team). | Take: this is the default table behaviour we want (`WithInlineTableLinks(false)`). |
| [bubbles/filepicker](https://github.com/charmbracelet/bubbles) | Built-in directory navigation component. | Good reference, but it's a modal picker, not a persistent tree pane. We build a custom tree. |
| [tree-bubble](https://github.com/savannahostrowski/tree-bubble) | Community tree component for Bubble Tea. | Reference for tree node expansion logic; may be outdated vs v2 — likely fork/rewrite. |
| [bubblezone](https://github.com/charmbracelet/bubblezone) | Mouse-zone tracking for Bubble Tea v1. | v1-oriented; with Bubble Tea v2 we can hit-test clicks ourselves via coordinates (our layout is simple). Skip. |
| [leg100's Bubble Tea tips](https://leg100.github.io/en/posts/building-bubbletea-programs/) | Patterns for multi-pane apps with resizable splits. | Confirms the "root model owns layout + splitter, children own content" pattern. |

### 2.2 Key research findings that shape the design

1. **The Charm v2 generation is stable.** Bubble Tea v2, Lip Gloss v2, and
   Bubbles v2 left beta on 23 Feb 2026. They power Charm's own production tools.
   We start on v2 — no reason to begin a new project on v1.
   - New "Cursed Renderer" (ncurses-style cell diffing) → dramatically faster
     rendering, which matters for a viewer app.
   - Declarative `tea.View` fields replace v1 option/command soup
     (`v.AltScreen = true`, `v.MouseMode = tea.MouseModeCellMotion`).
2. **Bubble Tea v2 mouse API is event-typed.** `tea.MouseMsg` is now an
   interface; match on `tea.MouseClickMsg`, `tea.MouseReleaseMsg`,
   `tea.MouseWheelMsg`, `tea.MouseMotionMsg`; get coordinates with
   `msg.Mouse()`. Buttons: `tea.MouseLeft`, `tea.MouseRight`, … This maps
   perfectly onto drag-to-resize: click → motion (while held) → release.
3. **Glamour v2 exists** (`github.com/charmbracelet/glamour/v2`, built on
   Lip Gloss v2). Notable API: `WithWordWrap`, `WithStandardStyle`,
   `WithStylePath`, `WithStylesFromJSONFile/Bytes`, `WithInlineTableLinks`,
   `WithTableWrap`, `WithEmoji`, `WithChromaFormatter`. **Glamour v2 no
   longer auto-detects the terminal background** — the app must query the
   terminal itself (e.g. `charmbracelet/x/term.HasDarkBackground()`) and pick
   `dark` vs `light`.
4. **Glamour + width:** with `WithWordWrap(w)`, tables expand to the full
   given width. This is exactly what we want — pass the *right pane's inner
   width*, not the terminal width. When the user drags the divider, we
   re-render at the new width (cheap enough with caching; see §9).

---

## 3. Technology Stack

| Dependency | Import path | Role |
|---|---|---|
| Bubble Tea v2 | `charm.land/bubbletea/v2` | TUI framework (Elm architecture) |
| Lip Gloss v2 | `github.com/charmbracelet/lipgloss/v2` | Styling, borders, layout (`JoinHorizontal`) |
| Bubbles v2 | `charm.land/bubbles/v2` (`/viewport`, `/table`, `/key`, `/help`…) | Viewport with mouse-wheel + high-perf mode, help bar |
| Glamour v2 | `github.com/charmbracelet/glamour/v2` | Markdown → ANSI rendering |
| Chroma (indirect via Glamour) | — | Syntax highlighting in code blocks |
| fsnotify | `github.com/fsnotify/fsnotify` | Live file reload (nice-to-have, phase 2) |

**Go version:** latest stable (>= 1.23).

**Decision — Bubble Tea v2 vs v1:** v2. Mouse drag-resize and wheel routing are
cleaner, the renderer is much faster, and the declarative `View` fields make
alt-screen/mouse-mode handling trivial. Ecosystem (Bubbles v2, Lip Gloss v2) is
stable.

**Decision — Glamour vs hand-rolled renderer:** Glamour. It's the best ANSI
markdown renderer in the Go ecosystem, stylesheet-driven (JSON), and its
v0.10.0+ table engine (link footnoting, wrapping) is precisely the "good at
tables" requirement. We customize via a JSON stylesheet rather than forking.

**Terminal support:** anything ANSI-ish — iTerm2, Kitty, Ghostty, WezTerm,
Alacritty, Windows Terminal, macOS Terminal. tmux works with `set -g mouse on`
(cell-motion tracking passes through). TrueColour when available, automatic
downsampling otherwise (handled by Lip Gloss/termenv colour profiles).

---

## 4. Architecture

Standard Elm architecture. A single root model owns layout and input routing;
two child models own their content. Children never know about each other.

```
┌───────────────────────── root model ─────────────────────────┐
│  layout: width/height, splitterRatio, focus (L/R)           │
│  theme, config, help overlay state                           │
│                                                              │
│  ┌── tree.Model ─────┐   ┌─── splitter (state) ─┐           │
│  │ root path         │   │ dragging bool        │           │
│  │ nodes (lazy)      │   │ hover bool           │           │
│  │ cursor, expanded  │   └──────────────────────┘           │
│  │ selection         │                                       │
│  └───────────────────┘   ┌── viewer.Model ──────┐           │
│                          │ path, mtime, raw md  │           │
│                          │ rendered cache       │           │
│                          │ viewport (bubbles)   │           │
│                          │ mode: rendered|raw   │           │
│                          └──────────────────────┘           │
└──────────────────────────────────────────────────────────────┘
```

### Message flow

```
user input ──► tea.Program ──► root.Update(msg)
                                │
                                ├─ tea.WindowSizeMsg  → resize both panes, clamp ratio
                                ├─ tea.MouseClickMsg  → hit-test: divider? tree row? viewer?
                                ├─ tea.MouseMotionMsg → divider drag / hover cursor
                                ├─ tea.MouseReleaseMsg→ end drag
                                ├─ tea.MouseWheelMsg  → scroll pane under cursor
                                ├─ tea.KeyPressMsg    → route to focused pane / global keys
                                └─ internal msgs (loadedFileMsg, renderedMsg, tickMsg…)
```

- File reads and Glamour rendering run inside `tea.Cmd`s (goroutines) and come
  back as messages — **the Update loop never touches disk or renders markdown
  synchronously**.
- A monotonically increasing render request ID guards against out-of-order
  async renders (fast file switching): stale results are dropped.

### Package layout

```
mdui/
├── main.go              # flag parsing, program bootstrap
├── internal/
│   ├── app/             # root model, layout, input routing, splitter
│   ├── tree/            # file tree model (scan, filter, expand/collapse, render)
│   ├── viewer/          # markdown viewer model (glamour integration, viewport)
│   ├── render/          # glamour renderer pool + cache (keyed by path+mtime+width+style)
│   ├── config/          # load/save TOML config, state persistence
│   ├── style/           # lipgloss styles, embedded glamour JSON stylesheets
│   └── mdutil/          # TOC extraction, word count, front-matter stripping
├── styles/              # glamour JSON stylesheets (dark.json, light.json, mdui-dark.json…)
├── go.mod
└── README.md
```

---

## 5. UI Layout

```
┌─ mdui ─ ~/notes ──────────────────────────────────────────────────────┐
│ ▾ notes/                      │  # Quarterly Review                  │
│   ▾ projects/                 │                                      │
│     • dispatch.md             │  Summary of Q3 progress across       │
│     • roadmap.md             │  teams.                               │
│   ▸ archive/                  │                                      │
│   • readme.md                 │  | Team    | Status | Notes          │
│   • todo.md                   │  |---------|--------|-----------------|
│                               │  | Backend | ✅     | Shipped v2     │
│                               │  | iOS     | 🚧     | In review      │
│                               │  | Web     | ✅     | Launched       │
│                               │                                      │
│                               │  Links:                              │
│                               │  [1] github.com/acme/q3-report       │
│                               │                                      │
│                               │  ## Next quarter                     │
│───────────────────────────────│                                      │
│ readme.md · 342 words · 2 min │ 67%                    (?) help, (q) quit │
└──────────────────────────────────────────────────────────────────────┘
          ▲ tree pane (30%)                        viewer pane (70%) ▲
                              drag divider ↕
```

- **Outer chrome**: 1-cell border around the whole app (rounded corners when
  the terminal supports unicode), title bar showing app name + root folder.
- **Tree pane**: indented tree, `▾`/`▸` for expanded/collapsed dirs,
  files highlighted when selected; directories dimmer/cyan, files default.
- **Viewer pane**: Glamour-rendered markdown in a `bubbles/viewport`
  (high-performance mode), with a 1-cell inner padding.
- **Divider**: 1 column, rendered as `│` (dim, or highlighted while dragging).
  A 3-column hit zone (divider ± 1 cell) makes grabbing easy.
- **Status bar** (1 line, bottom, above/below the border): left = file path,
  word count, estimated reading time; right = scroll position %, key hints.
- **Help overlay**: `?` toggles a centered keybinding cheatsheet (rendered
  with — what else — Glamour, from an embedded `help.md`).

### Layout math

```
totalW  = window width
divW    = 1
treeW   = clamp(round(totalW * ratio), treeMinW, totalW - viewerMinW - divW)
viewerW = totalW - treeW - divW
treeMinW = 20 ; viewerMinW = 30 ; default ratio = 0.3
```

Rendered with `lipgloss.JoinHorizontal(lipgloss.Top, treeView, divider, viewerView)`.

---

## 6. Core Features

### 6.1 File tree pane

- **Scan**: walk the argument folder (default `.`). Show only directories and
  files with extensions `.md` / `.markdown` (case-insensitive). Hidden files
  (dot-prefixed) hidden by default, toggleable.
- **Lazy directories**: directory children are read on first expansion, not on
  startup — a huge tree must not slow launch. Directories with no markdown
  anywhere beneath them are shown greyed or hidden (config: `tree.hide_empty`).
- **Sort**: dirs-first alphabetical (natural/case-aware sort so `a10` > `a2`),
  configurable.
- **Navigation**: ↑/↓ or j/k move, →/l or Enter expands dir / opens file,
  ←/h collapses or jumps to parent, `g`/`G` top/bottom, PageUp/PageDown.
- **Selection opens the file** in the viewer (preview on move, like a file
  manager — no Enter needed to preview; Enter "pins"/focuses the viewer).
- Symlinks: followed but cycle-safe (skip already-visited inodes).
- Errors (permission denied, vanished file) show inline in the pane, never
  crash.

### 6.2 Viewer pane

- Glamour render at the viewer's **inner width** (`viewerW - 2`, accounting
  for padding) with `WithWordWrap(innerW)`.
- Scroll: ↑/↓, j/k, PageUp/PageDown, `g`/`G` (top/bottom), ctrl+d/ctrl+u
  (half-page), mouse wheel when hovered.
- **TOC mode**: `t` toggles a heading outline sidebar/overlay; selecting a
  heading scrolls the document to it (see §10.6).
- **Raw mode**: `r` toggles between rendered view and raw markdown in a
  monospace viewport (useful to see the source of a table).
- **Relative link following**: links to sibling `.md` files (or anchors) are
  clickable / Enter-openable (see §10.4).
- Empty state (no file selected): a short welcome/how-to rendered as markdown.
- Binary/absurdly large files (> N MB, config): show a warning + raw view
  offer instead of rendering.

### 6.3 Splitter (mouse resize)

- Mouse down within the divider hit zone → `dragging = true`; while dragging,
  every `MouseMotionMsg` sets `ratio = x / totalW` (clamped to
  `[treeMinW/totalW, 1 - (viewerMinW+divW)/totalW]`), and the divider renders
  highlighted (`┃`). Mouse up → `dragging = false`.
- While dragging, the viewer does **not** re-render markdown per pixel;
  re-render is throttled (e.g. at most every 100 ms via a timer command) and
  once more on release. See §9.
- Keyboard equivalents: `H`/`L` (or `<`/`>`) nudge the divider by 2 columns;
  `=` or `ctrl+r` resets to default ratio. `Tab`/`Shift+Tab` move focus
  between panes.
- The ratio persists in the config file across sessions.

### 6.4 Mouse behaviour table

| Event | Location | Action |
|---|---|---|
| Left click | divider zone | begin drag |
| Left drag | anywhere | move divider |
| Release | anywhere | end drag, final re-render |
| Left click | tree row | select + preview file (dir: toggle expand) |
| Double click | tree row (dir) | expand + descend |
| Left click | viewer link | open relative `.md` link |
| Wheel up/down | tree pane | scroll tree |
| Wheel up/down | viewer pane | scroll document |
| Right click | — | reserved (context menu later); default passthrough where possible |

Mouse capture note: enabling mouse modes disables the terminal's native
text selection inside the app. Two mitigations: `m` toggles mouse capture off
(and shows a hint bar explaining how to re-enable), and holding `shift`
bypasses mouse in most terminals anyway.

### 6.5 Keybindings (global)

```
q / ctrl+c     quit                            ?         help overlay
Tab / Shift+Tab  cycle pane focus               m         toggle mouse capture
H / L          shrink/grow tree pane            =         reset pane split
t              toggle outline (TOC)             r         toggle raw/rendered
f              fuzzy file finder (§10.2)        /         search in document
e              open current file in $EDITOR     .         toggle hidden files
1..9 or [ ]    cycle theme (§8)                 ctrl+r    reload file
```

All keybindings listed in the `?` overlay via `bubbles/help`.

---

## 7. Markdown Rendering & Colour Scheme

### 7.1 Glamour configuration

```go
r, err := glamour.NewTermRenderer(
    glamour.WithStandardStyle("dark"),        // or "light", chosen at startup
    glamour.WithWordWrap(viewerInnerWidth),
    glamour.WithInlineTableLinks(false),      // links as footnotes under tables
    glamour.WithTableWrap(true),              // wrap long cell content
    glamour.WithEmoji(),                      // :emoji: → unicode (config-gated)
    glamour.WithChromaFormatter("terminal16m"), // truecolour code highlights
)
```

- **Style selection at startup**: query terminal background
  (`charmbracelet/x/term.HasDarkBackground()`); dark → `dark`, light →
  `light`. Overridable via config/CLI (`--style dark|light|ascii|…`). Glamour
  v2 does *not* do this detection itself — this is our job.
- **Custom stylesheets**: we ship `mdui-dark.json` / `mdui-light.json`
  (tweaked copies of Glamour's) as our default, and load user stylesheets from
  `~/.config/mdui/styles/*.json`, so the theme is fully user-editable without
  recompiling.
- `GLAMOUR_STYLE` env var respected as a stylesheet path (same convention as
  Glow), lowest priority.

### 7.2 Table excellence (the differentiator)

Glamour's v0.10+ table engine gives us: column alignment from the source
(`|:---|` vs `|---:|`), cell wrapping at the pane width, and link footnoting.
On top of that we add:

1. **Stylesheet tuning** in `mdui-*.json`: bold/coloured table header row,
   subtle border colour (dimmed foreground), row banding not possible in
   Glamour's model — instead distinct border vs. text colours for scanability.
2. **Width-strategy ladder** as the viewer pane narrows:
   - Full width ≥ longest logical cell: normal wrapped table.
   - Very narrow (< ~40 cols): Glamour still wraps; if a table would become
     unreadable (heuristic: > 3 wrapped lines per row), offer `T` to pop the
     table out into a **full-width overlay** (temporarily borrow the whole
     window), any key dismisses. This is our signature table feature.
3. **Wide tables (many columns)**: horizontal scroll inside a table overlay
   (`<`/`>` or wheel-left/right) using the `bubbles/viewport`-style
   horizontal panning over the rendered block. (Phase 2 — see nice-to-haves.)
4. Clickable footnote links: after Glamour footnotes links under a table, we
   keep an index of link rects so a click on `[1]` opens the URL via
   `open`/`xdg-open` (with a confirm status-bar prompt for remote URLs).

### 7.3 Colour scheme

- Follow the chosen Glamour style (dark/light) for document content so the
   whole app feels coherent.
- Chrome colours (borders, status bar, tree) defined as Lip Gloss styles with
   **adaptive colours** (dark/light variants per style) and degrade cleanly:
   truecolour → 256 → 16 → mono (`NoColour` profile automatically).
- Precedence: user stylesheet JSON > user config overrides > our embedded
  default > Glamour built-in `dark`/`light`.

---

## 8. Performance

| Concern | Mitigation |
|---|---|
| Rendering markdown blocks the UI | Render in `tea.Cmd` goroutines; show existing (stale) content or a spinner until `renderedMsg` arrives. |
| Same file re-rendered on every scroll/resize | Render cache keyed by `(absPath, mtime, width, styleName)` — an LRU of ~8 entries. Scrolling only drives the viewport (string slicing), zero markdown work. |
| Divider drag re-renders per pixel | Debounce: during drag use last-good render (truncated/wrapped by viewport); re-render at most every ~100 ms and once on release. |
| Huge files | Cap synchronous path at ~2 MB; larger files still render async with a spinner; if render exceeds ~2 s, offer raw mode. |
| Tree with thousands of files | Lazy dir expansion; tree viewport renders only visible slice. |
| Terminal flicker | Bubble Tea v2's Cursed Renderer already cell-diffs; use bubbles viewport high-performance mode. |

---

## 9. Error Handling & Edge Cases

- Missing/unreadable argument folder → friendly inline error, offer `.` fallback? No: exit with a clear message on stderr, non-zero code.
- File deleted while open → status-bar warning, keep last render, auto-refresh on fsnotify re-create.
- Truncated/invalid markdown → Glamour is lenient (goldmark-based); worst case renders as text — fine.
- Terminal too small (< ~60×15) → render a "terminal too small" placeholder instead of garbage.
- No colour support / piped output → refuse to start TUI unless `--force`; print plain-rendered file instead (useful for scripting: `mdui --cat file.md`).
- Weird filenames (spaces, emoji, newlines) → always quote paths in status bar; tree truncates with `…`.
- Windows path separators → use `filepath` everywhere, never string-concat slashes.

---

## 10. Nice-to-Have Features (think about these)

Roughly in priority order. Phase 1 = core (§5–8); everything below is
optional/phase 2+ unless marked otherwise.

### Phase 1.5 — cheap, high value
1. **Search within document** (`/` + `n`/`N`): highlight matches (ANS-aware
   search over the rendered string; map match rows to scroll).
2. **Fuzzy file finder** (`f`): overlay input, fuzzy-matches all markdown
   files under root (sahilm/fuzzy or sedm0784/fuzzy), Enter to jump. Mirrors
   what the `bubbles/list` filter does, but as an overlay.
3. **Reading progress**: status bar percentage + optional thin scrollbar on
   the viewer's right edge (`▐` markers or `bubbles/scrollbar`).
4. **Open in `$EDITOR`** (`e`): suspend the TUI (tea v2 supports
   release/restore of the terminal), run editor, resume + reload file.
5. **Word count / reading time** in status bar (from raw markdown).

### Phase 2 — bigger lifts
6. **Outline / TOC sidebar** (`t`): parse H1–H3 into a list overlay; selecting
   scrolls to heading; auto-highlight current section while scrolling
   ("scrollspy").
7. **Live reload** via fsnotify: re-render on save; brief "reloaded" toast.
8. **Full-width table overlay** (`T` on a table, or auto-prompt when crushed)
   with horizontal scroll for wide tables — see §7.2.
9. **Link navigation**: number links document-wide; `Enter` + link number, or
   click; relative `.md` links open in-app, anchors scroll, URLs open
   externally after confirm.
10. **Bookmarks / history**: `b` to bookmark file (persisted), `ctrl+o`
    back/forward navigation stack like a browser (`,`.` / `;`' style).
11. **Session persistence**: last root, expanded tree nodes, last file, pane
    ratio, theme — in `~/.config/mdui/state.toml`.
12. **Multiple roots** (`mdui docs wiki`): roots listed at tree top, or tabs.
13. **Git-aware tree**: respect `.gitignore` for hiding (config), show file
    mtime/size columns (config).
14. **Sort/mode toggles in-tree**: `s` cycles sort (name/mtime/size),
    `.` toggles hidden, `/`-prefixed filter inline.

### Phase 3 — exploratory
15. **Themes menu**: `1`–`9` cycle Glamour built-ins (dark, light, dracula,
    pink, tokyonight, ascii…) + user stylesheets; live preview.
16. **Inline images** in terminals that support it (Kitty/iTerm2 graphics) —
    Bubble Tea v2 has primitives for this.
17. **Reading queue / tabs**: open several files in tabs (`gt`/`gT`).
18. **Serve mode**: `mdui --serve` renders a folder as a local web page for
    sharing (probably a different tool — resist scope creep).
19. **Front-matter awareness**: parse YAML front-matter, show
    title/date/tags in status bar or a header strip.
20. **Statistics view**: `i` overlay — headings, words, links, tables count.
21. **Fish-shell style auto-suggestions**: typing `/command`… overkill, but a
    mini command palette (`:`) is a fun extension point.

**Explicitly rejected / deferred:** editing, remote fetching, mermaid
rendering, plugin system. Keep the tool focused on *viewing well*.

---

## 11. Configuration & Persistence

- `~/.config/mdui/config.toml` — stable prefs:
  `style` (`"auto"|"dark"|"light"|path`), `mouse` (bool), `emoji` (bool),
  `wrap_width` (0 = pane width), `hidden_files` (bool), `hide_empty_dirs`,
  `sort_by`, `tree_ratio`, `editor` override.
- `~/.config/mdui/state.toml` — session state: last root/file, expanded nodes
  (pruned to ≤ 100), bookmarks. Written on quit, debounced.
- CLI flags mirror config (`--style`, `--no-mouse`, `--width`…) and win over
  config. Flags/env/config precedence documented in README.
- First run: create config dir with commented example + drop our default
  stylesheets there for easy editing.

---

## 12. Testing

- **Unit**: tree filtering/sorting (table-driven tests with temp dirs),
  layout math + clamping, cache key behaviour, TOC extraction, link
  resolution (relative/anchor/URL), path handling on `/` and Windows.
- **Golden tests**: Glamour output snapshots for a corpus of markdown
  fixtures (tables, code, nested lists, CJK, emoji) at several widths —
  catches renderer regressions.
- **TUI tests**: Bubble Tea v2's `tea.WithColorProfile` +
  `tea.WithWindowSize` and the `teatest` package drive the model through
  scripted key/mouse sequences (select file, drag divider, resize window).
- Manual matrix: iTerm2, Kitty, Ghostty, Terminal.app, tmux (`mouse on`),
  Windows Terminal; 80-col minimum.

---

## 13. Build & Distribution

- Single static binary; `go build ./...`; no cgo.
- goreleaser for tagged releases: Homebrew tap, scoop bucket, .deb/.rpm,
  macOS/Linux/Windows/ARM builds (mirror Glow's release matrix).
- `Makefile`: `make test lint run`; golangci-lint in CI; GoReleaser on tag.

---

## 14. Milestones

| # | Deliverable | Done when |
|---|---|---|
| M0 | Skeleton: alt-screen app, arg parsing, two empty panes, status bar | boots on a folder |
| M1 | Tree pane: scan/filter/sort/lazy expand, keyboard nav, selection | can browse any folder |
| M2 | Viewer: async Glamour render, dark/light detect, viewport scroll | can read files nicely |
| M3 | Splitter: mouse drag + clamps + keyboard nudges + persistence | drag is smooth |
| M4 | Polish: help overlay, mouse wheel/click routing everywhere, status bar content, error states, config file | daily-drivable |
| M5 | Table features: overlay, width ladder, footnote-link clicks | tables look great at any width |
| M6+ | Nice-to-haves by priority (§10) | — |

---

## 15. Open Questions

1. Preview-on-move (file renders as cursor moves) vs select-then-render?
   Default: preview-on-move with ~150 ms debounce; make it a config
   (`preview_on_move`). Confirm feels right in practice.
2. Should the tree show a flat "all files" mode (`~` toggle) for huge flat
   folders where a tree is noise?
3. Full-width table overlay — modal (any-key dismiss) or a pinned layout mode?
   Lean modal first (simpler).
4. Minimum terminal size to hard-refuse vs. degrade gracefully at? Propose:
   degrade below 80×24, refuse below 60×15.
5. Name/logo. "mdui" is the working name — check collisions on GitHub/brew
   before release.

---

## 16. References

- Bubble Tea — https://github.com/charmbracelet/bubbletea · v2 upgrade guide: `UPGRADE_GUIDE_V2.md` (import `charm.land/bubbletea/v2`)
- Lip Gloss v2 — https://github.com/charmbracelet/lipgloss (`github.com/charmbracelet/lipgloss/v2`)
- Bubbles v2 — https://github.com/charmbracelet/bubbles (`charm.land/bubbles/v2`); viewport, table, filepicker, help components
- Glamour v2 — https://github.com/charmbracelet/glamour (`github.com/charmbracelet/glamour/v2`) · table redesign blog: https://charm.land/blog/glamour-tables/
- Glow (prior art) — https://github.com/charmbracelet/glow
- Charm v2 announcement — https://charm.land/blog/v2/
- Bubble Tea v2 "What's New" — https://github.com/charmbracelet/bubbletea/discussions/1374
- Bubble Tea program-building patterns — https://leg100.github.io/en/posts/building-bubbletea-programs/
- tree-bubble (reference tree component) — https://github.com/savannahostrowski/tree-bubble
