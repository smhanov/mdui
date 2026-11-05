package tree

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestViewRendersMarkers(t *testing.T) {
	m := treeFixture(t)

	out := m.View(40, 10)
	if got := lipgloss.Width(out); got != 40 {
		t.Fatalf("width = %d, want 40", got)
	}
	if got := lipgloss.Height(out); got != 10 {
		t.Fatalf("height = %d, want 10", got)
	}
	for _, want := range []string{"▾ ", "▸ dir/", "• first.md", "• second.md"} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing %q:\n%s", want, out)
		}
	}

	m = press(t, m, "down")
	m = press(t, m, "right")
	if out := m.View(40, 10); !strings.Contains(out, "• nested.md") {
		t.Fatalf("expanded view missing nested file:\n%s", out)
	}
}

func TestViewEmptyState(t *testing.T) {
	root := t.TempDir()
	writeNote(t, root, "photo.png")
	writeNote(t, root, "notes.txt")

	m := New(root)
	out := m.View(30, 6)
	if !strings.Contains(out, "no markdown files found") {
		t.Fatalf("empty state missing:\n%s", out)
	}
	if strings.Contains(out, "•") {
		t.Fatalf("empty state rendered file rows:\n%s", out)
	}
	if got := lipgloss.Width(out); got != 30 {
		t.Fatalf("width = %d, want 30", got)
	}
	if got := lipgloss.Height(out); got != 6 {
		t.Fatalf("height = %d, want 6", got)
	}
}

func TestViewTruncatesLongNames(t *testing.T) {
	root := t.TempDir()
	writeNote(t, root, strings.Repeat("long", 30)+".md")
	m := New(root)
	m.SetHeight(5)

	out := m.View(24, 5)
	if got := lipgloss.Width(out); got != 24 {
		t.Fatalf("width = %d, want 24", got)
	}
	if !strings.Contains(out, "…") {
		t.Fatalf("long name was not truncated:\n%s", out)
	}
}

func TestViewSanitizesControlCharacters(t *testing.T) {
	root := t.TempDir()
	writeNote(t, root, "line\nbreak\x1b.md")
	m := New(root)
	m.SetHeight(5)

	out := m.View(30, 5)
	if got := lipgloss.Height(out); got != 5 {
		t.Fatalf("height = %d, want 5; control characters broke the row", got)
	}
	if strings.Contains(out, "\x1b.md") {
		t.Fatalf("raw escape sequence in rendered row:\n%q", out)
	}
	if !strings.Contains(out, "?") {
		t.Fatalf("control character was not made visible:\n%s", out)
	}
}

func TestLazyStartupReadsOnlyRootAndViewIsWindowed(t *testing.T) {
	root := t.TempDir()
	bulk := mkdir(t, root, "bulk")
	for i := range 2000 {
		writeNote(t, bulk, fmt.Sprintf("note-%04d.md", i))
	}

	counter := &scanCounter{}
	m := newModel(root, counter.scan)
	m.SetHeight(10)

	if counter.calls != 1 {
		t.Fatalf("startup scans = %d, want 1", counter.calls)
	}
	if counter.paths[0] != root {
		t.Fatalf("startup scanned %q, want root", counter.paths[0])
	}

	m = press(t, m, "down") // bulk
	m = press(t, m, "right")
	if counter.calls != 2 {
		t.Fatalf("expanding bulk scans = %d, want 2", counter.calls)
	}
	if got := len(m.rows); got != 2002 {
		t.Fatalf("expanded rows = %d, want 2002", got)
	}

	out := m.View(40, 10)
	if got := lipgloss.Width(out); got != 40 {
		t.Fatalf("width = %d, want 40", got)
	}
	if got := lipgloss.Height(out); got != 10 {
		t.Fatalf("height = %d, want 10", got)
	}
	if got := strings.Count(out, "•"); got > 10 {
		t.Fatalf("rendered %d file rows in a 10-row pane:\n%s", got, out)
	}
	if !strings.Contains(out, "note-0000.md") {
		t.Fatalf("first visible note missing:\n%s", out)
	}
	if strings.Contains(out, "note-0100.md") {
		t.Fatalf("view rendered far-away row:\n%s", out)
	}

	m = press(t, m, "G")
	out = m.View(40, 10)
	if !strings.Contains(out, "note-1999.md") {
		t.Fatalf("bottom row missing after G:\n%s", out)
	}
	if strings.Contains(out, "note-0000.md") {
		t.Fatalf("view still renders top row after G:\n%s", out)
	}
}
