// Package tree renders the markdown file tree in the left pane.
//
// The model is a deliberately view-only placeholder for now: scanning,
// filtering, expansion, and navigation arrive with the tree milestone. Keeping
// the View(width, height) contract stable lets that work land without touching
// the root layout.
package tree

import "mdui/internal/style"

// Model is the tree pane.
type Model struct {
	root string
}

// New returns a tree pane rooted at root.
func New(root string) Model {
	return Model{root: root}
}

// Root reports the folder the tree is scoped to.
func (m Model) Root() string {
	return m.root
}

// View renders the placeholder at exactly width by height cells.
func (m Model) View(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	return style.Pane.
		Width(width).
		Height(height).
		MaxHeight(height).
		Render("tree")
}
