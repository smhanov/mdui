package viewer

import (
	"io/fs"
	"strings"
	"testing"
)

func TestVanishedFileKeepsLastRenderAndWarns(t *testing.T) {
	f := &fakeRenderer{raw: "last good source"}
	f.render = func(request) string { return "LAST GOOD RENDER" }
	m := newTestViewer(f)
	m = apply(t, m, m.SetSize(80, 20))
	m = apply(t, m, m.Open("notes.md"))

	if !strings.Contains(m.Content(), "LAST GOOD RENDER") {
		t.Fatalf("initial render missing: %q", m.Content())
	}
	if m.Warning() != "" {
		t.Fatalf("unexpected warning before deletion: %q", m.Warning())
	}

	f.err = fs.ErrNotExist
	m = apply(t, m, m.SetSize(100, 20))
	if !strings.Contains(m.Content(), "LAST GOOD RENDER") {
		t.Fatalf("vanished file replaced the last render: %q", m.Content())
	}
	if m.Warning() != "file no longer available" {
		t.Fatalf("warning = %q, want file-no-longer-available", m.Warning())
	}

	// The next successful render recovers.
	f.err = nil
	f.render = func(request) string { return "REFRESHED RENDER" }
	m = apply(t, m, m.SetSize(120, 20))
	if m.Warning() != "" {
		t.Fatalf("warning did not clear: %q", m.Warning())
	}
	if !strings.Contains(m.Content(), "REFRESHED RENDER") {
		t.Fatalf("content did not refresh: %q", m.Content())
	}
}

func TestUnreadableFileShowsInlineErrorAndRecovers(t *testing.T) {
	f := &fakeRenderer{err: fs.ErrPermission}
	m := newTestViewer(f)
	m = apply(t, m, m.SetSize(80, 20))
	m = apply(t, m, m.Open("locked.md"))

	if !strings.Contains(m.Content(), "permission denied") {
		t.Fatalf("inline error missing: %q", m.Content())
	}
	if m.Warning() != "" {
		t.Fatalf("unreadable file should not show the vanished-file warning: %q", m.Warning())
	}

	f.err = nil
	f.render = func(request) string { return "RECOVERED" }
	m = apply(t, m, m.Open("other.md"))
	if strings.Contains(m.Content(), "permission denied") {
		t.Fatalf("error persisted after a new selection: %q", m.Content())
	}
	if !strings.Contains(m.Content(), "RECOVERED") {
		t.Fatalf("new selection did not render: %q", m.Content())
	}
}

func TestMissingFileOnFirstOpenShowsInlineError(t *testing.T) {
	f := &fakeRenderer{err: fs.ErrNotExist}
	m := newTestViewer(f)
	m = apply(t, m, m.SetSize(80, 20))
	m = apply(t, m, m.Open("gone.md"))

	if !strings.Contains(m.Content(), "does not exist") {
		t.Fatalf("inline missing-file error absent: %q", m.Content())
	}
	if m.Warning() != "" {
		t.Fatalf("there is no previous content to preserve: %q", m.Warning())
	}
}

func TestRawModeUsesRawWordCount(t *testing.T) {
	f := &fakeRenderer{raw: strings.Repeat("source ", 300)}
	f.render = func(request) string { return "rendered" }
	m := newTestViewer(f)
	m = apply(t, m, m.SetSize(80, 20))
	m = apply(t, m, m.Open("notes.md"))
	if got := m.Stats().Words; got != 300 {
		t.Fatalf("words = %d, want 300", got)
	}

	m, _ = m.Update(press("r"))
	if !strings.Contains(m.Content(), "source") {
		t.Fatalf("raw content missing: %q", m.Content())
	}
	if got := m.Stats().Words; got != 300 {
		t.Fatalf("raw-mode words = %d, want 300", got)
	}
}
