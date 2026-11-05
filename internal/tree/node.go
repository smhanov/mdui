// Package tree implements the left-hand file tree pane: one-level directory
// scans, lazy expansion, keyboard navigation, and windowed rendering.
package tree

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Node is one entry in the tree. Children stay nil until a directory is
// expanded for the first time; Loaded records whether a scan has happened.
type Node struct {
	Name     string
	Path     string
	IsDir    bool
	Expanded bool
	Loaded   bool
	Children []*Node
	Err      string

	parent *Node
	cycle  bool
}

// Scanner reads one directory level and returns its visible children. Tests
// inject a Scanner to count reads; production uses ScanDir.
type Scanner func(path string, showHidden bool) ([]*Node, error)

// ScanDir reads a single directory level. It keeps every directory plus files
// with a .md or .markdown extension (case-insensitive). Dot-prefixed entries
// are included only when showHidden is true. The result is sorted
// directories-first with a natural, case-insensitive name order.
func ScanDir(path string, showHidden bool) ([]*Node, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	nodes := make([]*Node, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}

		fullPath := filepath.Join(path, name)
		isDir := entry.IsDir()
		if !isDir && entry.Type()&os.ModeSymlink != 0 {
			// A symlinked directory is navigable like a real one. Broken and
			// looping links fail here and are kept only when their name looks
			// like markdown.
			if info, statErr := os.Stat(fullPath); statErr == nil && info.IsDir() {
				isDir = true
			}
		}
		if !isDir && !isMarkdown(name) {
			continue
		}

		nodes = append(nodes, &Node{Name: name, Path: fullPath, IsDir: isDir})
	}

	sortNodes(nodes)
	return nodes, nil
}

func isMarkdown(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".markdown":
		return true
	default:
		return false
	}
}

// errorText strips the operation/path wrapper so inline rows read as
// "permission denied" rather than repeating the node's own path.
func errorText(err error) string {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		err = pathErr.Err
	}
	return err.Error()
}
