package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestRenderShell(t *testing.T) {
	m := New("/tmp/notes")
	m.width, m.height = 100, 30

	out := m.render()

	if got := lipgloss.Width(out); got != 100 {
		t.Fatalf("rendered width = %d, want 100", got)
	}
	if got := lipgloss.Height(out); got != 30 {
		t.Fatalf("rendered height = %d, want 30", got)
	}
	for _, want := range []string{
		"mdui — /tmp/notes",
		"tree",
		"no file selected",
		"tab focus · q quit",
		"│",
		"╭", "╮", "╰", "╯",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered shell does not contain %q", want)
		}
	}
}

func TestRenderTooSmall(t *testing.T) {
	m := New("/tmp/notes")
	m.width, m.height = 50, 12

	out := m.render()

	if !strings.Contains(out, "terminal too small") {
		t.Fatalf("small terminal placeholder missing:\n%s", out)
	}
	if strings.Contains(out, "no file selected") {
		t.Fatal("small terminal should not render the panes")
	}
	if got := lipgloss.Width(out); got != 50 {
		t.Fatalf("placeholder width = %d, want 50", got)
	}
	if got := lipgloss.Height(out); got != 12 {
		t.Fatalf("placeholder height = %d, want 12", got)
	}
}

func TestRenderDegradedSize(t *testing.T) {
	m := New("/tmp/notes")
	m.width, m.height = 70, 20

	out := m.render()

	if strings.Contains(out, "terminal too small") {
		t.Fatal("70x20 should render the shell, not the refusal placeholder")
	}
	if !strings.Contains(out, "resize to ≥80×24") {
		t.Fatalf("degrade hint missing:\n%s", out)
	}
}

func TestResizeReflows(t *testing.T) {
	m := New("/tmp/notes")
	m.width, m.height = 80, 24

	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	resized, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want app.Model", next)
	}
	if resized.width != 120 || resized.height != 40 {
		t.Fatalf("size = %dx%d, want 120x40", resized.width, resized.height)
	}

	out := resized.render()
	if got := lipgloss.Width(out); got != 120 {
		t.Fatalf("reflowed width = %d, want 120", got)
	}
	if got := lipgloss.Height(out); got != 40 {
		t.Fatalf("reflowed height = %d, want 40", got)
	}
}

func TestViewIsAltScreen(t *testing.T) {
	m := New("/tmp/notes")
	m.width, m.height = 100, 30

	v := m.View()
	if !v.AltScreen {
		t.Fatal("View must enable the alternate screen")
	}
	if v.WindowTitle != "mdui — /tmp/notes" {
		t.Fatalf("window title = %q", v.WindowTitle)
	}
}
