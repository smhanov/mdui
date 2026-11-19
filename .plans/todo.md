# Tasks

## Todo
| Priority | ID | Description / plan | Notes |
| --- | --- | --- | --- |
| P2 | T018 | Tag v0.1.0 so the README's `go install @v0.1.0` works — [plan](T018-tag-v0.1.0.md) | github=smhanov/mdui#3; user deferred tag 2026-11-06 (T011) — confirm before pushing; updated=2026-11-18 |

## In Progress
| Priority | ID | Description / plan | Notes |
| --- | --- | --- | --- |

## Blocked
| Priority | ID | Description / plan | Notes |
| --- | --- | --- | --- |

## Done
| Priority | ID | Description / plan | Notes |
| --- | --- | --- | --- |
| P2 | T017 | Command mode `:` with `cd <folder>` to change the viewed folder in the UI — [plan](T017-command-mode-cd.md) | completed=2026-11-19 |
| P2 | T016 | Type-to-search in the file tree: bare typing fuzzy-jumps to the best visible row; arrows/click/esc end the session — [plan](T016-tree-type-to-search.md) | completed=2026-11-15 |
| P2 | T015 | Thin draggable scrollbars on the tree and viewer right edges, with proportional thumb and mouse drag — [plan](T015-scrollbars.md) | completed=2026-11-15 |
| P2 | T014 | Live filesystem updates for file tree + open document — [plan](T014-live-fs-updates.md) | completed=2026-11-06 |
| P2 | T013 | Fix TestWordCountMatchesWc failing under busybox wc (Alpine Linux) — [plan](T013-linux-wc-wordcount-test.md) | completed=2026-11-06 |
| P2 | T011 | `--version` flag with ldflags/buildinfo fallback, Makefile build target, tag v0.1.0 — [plan](T011-version-and-tags.md) | completed=2026-11-05; code ACs 1-3 done; user declined v0.1.0 tag for now (2026-11-06) - rerun when ready: git tag -a v0.1.0 && git push origin v0.1.0 |
| P1 | T012 | Repro/bisect/mitigate frozen repaints on ptys that never answer startup queries — [plan](T012-pty-repaint-freeze.md) | completed=2026-11-05 |
| P1 | T010 | Friendly one-line error when mdui runs without a TTY, instead of the doubled bubbletea chain — [plan](T010-non-tty-error.md) | completed=2026-11-05 |
| P1 | T009 | Reflow soft-wrapped paragraphs and list items to the frame width — [plan](T009-soft-break-reflow.md) | completed=2026-11-05 |
| P2 | T008 | Fuzzy file finder overlay (f) — [plan](T008-fuzzy-finder.md) | completed=2026-11-05 |
| P2 | T007 | Document search (/ n N) — [plan](T007-doc-search.md) | completed=2026-11-05 |
| P1 | T006 | Table excellence: width ladder, full-width overlay, footnote links — [plan](T006-table-excellence.md) | completed=2026-11-05 |
| P2 | T005 | Polish: help overlay, click routing, status bar content, config, error states — [plan](T005-polish-config.md) | completed=2026-11-05 |
| P1 | T004 | Mouse-resizable splitter, wheel routing, state persist — [plan](T004-splitter-mouse.md) | completed=2026-11-05 |
| P1 | T003 | Markdown viewer: async Glamour render, cache, scroll, dark/light — [plan](T003-markdown-viewer.md) | completed=2026-11-05 |
| P1 | T002 | File tree pane: scan/filter/sort, lazy expand, keyboard nav — [plan](T002-file-tree-pane.md) | completed=2026-11-05 |
| P1 | T001 | App skeleton: two-pane shell, layout clamps, status bar — [plan](T001-app-skeleton.md) | completed=2026-11-05 |
