# mdui

A fast two-pane terminal viewer for folders of markdown files: browse the file
tree on the left, read the rendered document on the right.

```sh
go build -o mdui .
./mdui [flags] [folder]
```

The folder defaults to the current directory. Go 1.27 or newer is required.
Prebuilt binaries can go anywhere on `PATH`; from a checkout, `go install .`
also puts the binary in `$GOBIN`. mdui reads its configuration from
`~/.config/mdui/` (or `$XDG_CONFIG_HOME/mdui/`).

## Keys

| Key | Action |
| --- | --- |
| `q` / `ctrl+c` | quit |
| `?` | toggle this help overlay (`?` or `esc` closes) |
| `Tab` / `Shift+Tab` | switch pane focus |
| `↑` / `↓`, `j` / `k` | move in the tree / scroll the document |
| `←` / `h` | collapse a directory / jump to its parent |
| `→` / `l`, `Enter` | expand a directory / open a file |
| `g` / `G` | jump to the top / bottom |
| `PageUp` / `PageDown`, `ctrl+u` / `ctrl+d` | move or scroll one page / half page |
| `H` / `L` | shrink / grow the tree pane by two columns |
| `=` | reset the pane split |
| `r` | toggle raw markdown in the viewer |
| `T` | open the table under the viewport at full width; any key closes |
| `.` | show or hide dot-prefixed files in the tree |
| `m` | toggle terminal mouse capture |

Moving the tree cursor previews files immediately; `Enter` toggles a
directory or re-opens a file without changing panes. In the viewer, `Enter`
follows the first table footnote link visible in the viewport. When a table
row wraps to more than three rendered lines, the status bar offers `T` to
show that table at full terminal width.

## Mouse

While mouse capture is on:

| Event | Action |
| --- | --- |
| click a file row | select and preview it |
| click a directory row | expand or collapse it |
| double-click a directory | expand it and move to its first child |
| drag the divider | resize the panes |
| wheel over a pane | scroll that pane |
| click a table footnote `[n]` | follow the link (relative `.md` in-app, `#anchor` scroll, http(s) after `y` confirmation) |
| shift-click / shift-drag | pass through to the terminal's native selection |

Press `m` to turn capture off when you want to select and copy text normally.
A yellow hint bar appears while it is off; press `m` again to restore clicking,
wheel scrolling, and divider dragging. The pane split is saved to
`state.toml`.

### tmux

tmux needs its own mouse reporting enabled before mdui can see wheel, click,
and drag events:

```tmux
set -g mouse on
```

Without it, every keyboard binding still works. Holding `shift` also bypasses
mouse capture in terminals that support the convention.

## Configuration

Preferences live in `~/.config/mdui/config.toml` (honouring
`XDG_CONFIG_HOME`). On first launch mdui creates the directory, a commented
example config, and editable copies of its stylesheets under `styles/`.

```toml
# Document style: "auto", "dark", "light", "mdui-dark", "mdui-light",
# any Glamour built-in name, or a path to a JSON stylesheet.
style = "auto"

# Capture the mouse. Toggle at runtime with m.
mouse = true

# Render :emoji: shortcodes as unicode emoji.
emoji = true

# Reflow the document at this many columns. 0 uses the viewer pane width.
wrap_width = 0

# Show dot-prefixed files and directories.
hidden_files = false

# Hide directories with no markdown anywhere beneath them.
hide_empty_dirs = false

# Tree order: "name", "mtime", or "size". Directories stay first.
sort_by = "name"

# Initial tree share of the width. The last dragged ratio in state.toml wins.
tree_ratio = 0.3

# Reserved for an "open in editor" command; not used yet.
editor = ""
```

Unknown keys are ignored, so a config written by a newer mdui still starts an
older build. A missing or malformed config falls back to the defaults above.

### Flags

Flags override the config file:

| Flag | Effect |
| --- | --- |
| `--style NAME\|PATH` | document style, same values as `style` |
| `--mouse` / `--no-mouse` | force mouse capture on or off |
| `--width N` | reflow width in columns; `0` uses the pane width |
| `--emoji` / `--no-emoji` | force emoji rendering on or off |
| `--hidden` | show dot-prefixed tree entries |
| `--sort name\|mtime\|size` | tree order |

Style precedence is **flag → config → `GLAMOUR_STYLE` → terminal background
detection**. `auto` means "fall through to the next source".

## Stylesheets

mdui ships two tuned Glamour stylesheets, `mdui-dark` and `mdui-light`:
coloured, bold tables with Unicode rules and the rest of Glamour's dark/light
palette. `style = "auto"` picks one from the terminal background; `dark` and
`light` select Glamour's built-in sheets instead.

To customise a sheet, edit the copy mdui created:

```
~/.config/mdui/styles/mdui-dark.json
~/.config/mdui/styles/mdui-light.json
```

A JSON file named after the selected style overrides both the embedded sheet
and a Glamour built-in: `styles/mdui-dark.json` replaces `mdui-dark`, and
`styles/dracula.json` replaces Glamour's `dracula` when `style = "dracula"`.
If the file is not there, the built-in or embedded sheet is used. An explicit
path in `style` (for example `/home/me/themes/solar.light.json`) is always
used directly.

Restart mdui after editing a stylesheet; renders are cached by file, width,
style, and emoji setting.

## Status bar

The bottom row shows the selected path (relative to the opened folder), word
count, estimated reading time at 200 words per minute (minimum one minute),
the scroll position as a percentage, and short key hints. If the open file is
deleted during the session, mdui keeps the last rendered page and shows a
warning in the status bar until you select another file. Unreadable files show
an inline error in the viewer.

## Notes and limits

- Markdown files are `.md` and `.markdown`, case-insensitive.
- Documents over 2 MiB are not rendered automatically; press `r` to read the
  raw source.
- Markdown tables are rendered with wrapped cells and footnote-style links.
  `T` shows a table at full terminal width; horizontal panning inside that
  overlay is not implemented yet.
- Relative `.md` links open in the viewer (the tree cursor is not moved),
  `#anchors` scroll within the current document, and http(s) links require
  confirmation before the system opener runs. Other schemes are refused.
- Document search, a fuzzy file finder, an outline sidebar, and opening files
  in `$EDITOR` are not implemented yet.
