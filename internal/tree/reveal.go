package tree

import (
	"os"
	"path/filepath"
	"strings"
)

// Hidden reports whether dot-prefixed entries are currently visible.
func (m Model) Hidden() bool {
	return m.showHidden
}

// Reveal expands every ancestor of path, loading directory levels as needed,
// moves the cursor to the file and scrolls it into view. It returns false and
// leaves the tree untouched when path is outside the root, is the root itself,
// is a directory, no longer exists, or is not visible under the current
// hidden-file setting. Reveal never emits a SelectionMsg; the caller already
// knows which file it is opening.
func (m *Model) Reveal(path string) bool {
	if path == "" || m.root == nil {
		return false
	}

	root := filepath.Clean(m.root.Path)
	target := filepath.Clean(path)
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == "." || rel == ".." {
		return false
	}
	if filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	info, err := os.Stat(target)
	if err != nil || info.IsDir() {
		return false
	}

	parts := strings.Split(rel, string(filepath.Separator))
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}

	node := m.root
	ancestors := []*Node{node}
	for _, part := range parts[:len(parts)-1] {
		if !node.Loaded {
			m.load(node)
		}
		child := childNamed(node, part)
		if child == nil || !child.IsDir {
			return false
		}
		ancestors = append(ancestors, child)
		node = child
	}
	if !node.Loaded {
		m.load(node)
	}
	file := childNamed(node, parts[len(parts)-1])
	if file == nil || file.IsDir {
		return false
	}

	for _, ancestor := range ancestors {
		ancestor.Expanded = true
	}
	m.rebuild()
	index := m.rowIndexByPath(file.Path)
	if index < 0 {
		return false
	}
	m.cursor = index
	m.ensureCursorVisible()
	return true
}

func childNamed(node *Node, name string) *Node {
	for _, child := range node.Children {
		if child.Name == name {
			return child
		}
	}
	return nil
}
