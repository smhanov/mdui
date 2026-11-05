# mdui keys

| Keys | Action | Keys | Action |
|:--|:--|:--|:--|
| `q` `ctrl+c` | quit | `?` | this help |
| `tab` `shift+tab` | switch pane | `m` | mouse on / off |
| `↑` `↓` `j` `k` | move / scroll | `g` `G` | top / bottom |
| `←` `h` `→` `l` | collapse / expand | `enter` | open / toggle |
| `ctrl+d` `ctrl+u` | half page | `pgup` `pgdn` | page |
| `H` `L` | resize tree | `=` | reset split |
| `r` | raw markdown | `.` | hidden files |
| `T` | full-width table | `enter` | open first visible link |
| `/` | search document | `n` `N` | next / prev |

`/` opens a prompt on the status bar: type to filter, Enter jumps, `n`/`N`
cycle with wrap, `esc` clears. Matching is literal and smart-case; `r`
re-applies the query to raw markdown.

**Mouse:** click a file to preview, a folder to expand or collapse, or
double-click a folder to open its first item. Drag the divider to resize; the
wheel scrolls the pane under the pointer. Click table footnote links (`[1]`, …)
to follow them; remote URLs ask for `y`/`n`. Hold **shift** for the terminal's
native selection. Press `?` or `esc` to close this help.
