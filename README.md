# mdui

A two-pane terminal viewer for folders of markdown files: browse the file tree
on the left, read the rendered document on the right.

```sh
go run . [folder]
```

The folder defaults to the current directory. Requires Go 1.27+.

## Keys

| Key | Action |
| --- | --- |
| `Tab` / `Shift+Tab` | switch panes |
| `↑` / `↓`, `j` / `k` | move in the tree, scroll the document |
| `←` / `→`, `h` / `l`, `Enter` | collapse / expand / open |
| `H` / `L` | shrink / grow the tree pane by two columns |
| `=` | reset the pane split |
| `r` | toggle raw markdown |
| `q` / `ctrl+c` | quit |

## Mouse

Mouse capture is always enabled. Drag the divider (a one-column hit zone
either side of it) to resize the panes, and use the wheel over either pane to
scroll it. The split ratio is saved to `~/.config/mdui/state.toml` and restored
next time; set `XDG_CONFIG_HOME` to move it.

Enabling mouse capture takes over the terminal's native text selection. Hold
**Shift** while dragging to use the terminal's own selection and copy instead;
the app ignores shifted mouse events.

### tmux

tmux needs mouse reporting enabled in its own config before the app can see
wheel and drag events:

```tmux
set -g mouse on
```

Without it, keyboard controls (`H`, `L`, `=`, `j`, `k`, `PgUp`, `PgDn`) still
work normally.
