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

	"github.com/smhanov/mdui/internal/tree"
	"github.com/smhanov/mdui/internal/viewer"
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
