// Package viewer renders the selected markdown document in the right pane.
//
// The model is a deliberately view-only placeholder for now: file loading,
// Glamour rendering, caching, and scrolling arrive with the viewer milestone.
// Keeping the View(width, height) contract stable lets that work land without
// touching the root layout.
package viewer

import "mdui/internal/style"

// Model is the markdown viewer pane.
type Model struct{}

// New returns an empty viewer.
func New() Model {
	return Model{}
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
		Render("no file selected")
}
