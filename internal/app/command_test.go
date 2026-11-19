package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func cdFixture(t *testing.T) (root, sub string) {
	t.Helper()
	root = t.TempDir()
	sub = filepath.Join(root, "docs")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		filepath.Join(root, "top.md"),
		filepath.Join(sub, "inner.md"),
	} {
		if err := os.WriteFile(name, []byte("# doc\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root, sub
}

func TestCommandPromptEditAndEscape(t *testing.T) {
	root, _ := cdFixture(t)
	m := New(root)
	m.width, m.height = 100, 30

	m = pressKey(t, m, tea.KeyPressMsg{Code: ':', Text: ":"})
	if !m.commandPrompt {
		t.Fatal(": did not open the command prompt")
	}
	if m.treeSearching {
		t.Fatal(": started a tree search")
	}
	rendered := m.render()
	if !strings.Contains(rendered, ":") || !strings.Contains(rendered, "▏") || !strings.Contains(rendered, "enter runs") {
		t.Fatalf("command prompt missing from render:\n%s", rendered)
	}

	m = pressKey(t, m, tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = pressKey(t, m, tea.KeyPressMsg{Code: 'd', Text: "d"})
	if m.commandInput != "cd" {
		t.Fatalf("commandInput = %q, want cd", m.commandInput)
	}
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	if m.commandInput != "c" {
		t.Fatalf("backspace left %q, want c", m.commandInput)
	}
	m = pressKey(t, m, tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	if m.commandInput != "" {
		t.Fatalf("ctrl+u left %q", m.commandInput)
	}
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.commandPrompt {
		t.Fatal("esc did not close the command prompt")
	}
	if strings.Contains(m.render(), "▏") {
		t.Fatal("esc left the command cursor in the render")
	}
}

func TestCommandCdChangesRoot(t *testing.T) {
	root, sub := cdFixture(t)
	m := New(root)
	m.width, m.height = 240, 30

	m = pressKey(t, m, tea.KeyPressMsg{Code: ':', Text: ":"})
	for _, key := range []tea.KeyPressMsg{
		{Code: 'c', Text: "c"},
		{Code: 'd', Text: "d"},
		{Code: ' ', Text: " "},
		{Code: 'd', Text: "d"},
		{Code: 'o', Text: "o"},
		{Code: 'c', Text: "c"},
		{Code: 's', Text: "s"},
	} {
		m = pressKey(t, m, key)
	}
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})

	if m.root != sub {
		t.Fatalf("root = %q, want %q", m.root, sub)
	}
	if m.selected != "" {
		t.Fatalf("selected = %q, want empty", m.selected)
	}
	if m.commandPrompt {
		t.Fatal("enter left the command prompt open")
	}
	rendered := m.render()
	if !strings.Contains(rendered, "mdui — "+sub) {
		t.Fatalf("title missing new root:\n%s", rendered)
	}
	if !strings.Contains(rendered, "inner.md") {
		t.Fatalf("tree missing inner.md:\n%s", rendered)
	}
	if strings.Contains(rendered, "top.md") {
		t.Fatalf("tree still shows top.md:\n%s", rendered)
	}
}

func TestCommandCdParentAndHome(t *testing.T) {
	root, _ := cdFixture(t)
	m := New(root)
	m.width, m.height = 100, 30

	m = runCommandLine(t, m, "cd ..")
	parent := filepath.Dir(root)
	if m.root != parent {
		t.Fatalf("cd .. root = %q, want %q", m.root, parent)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Logf("skipping home assertion: %v", err)
		return
	}
	m = runCommandLine(t, m, "cd ~")
	if m.root != home {
		t.Fatalf("cd ~ root = %q, want %q", m.root, home)
	}
}

func TestCommandCdErrors(t *testing.T) {
	root, _ := cdFixture(t)
	m := New(root)
	m.width, m.height = 120, 30

	m = runCommandLine(t, m, "cd nope")
	if m.commandPrompt {
		t.Fatal("failed cd left the prompt open")
	}
	if !strings.Contains(m.notice, "no such directory") {
		t.Fatalf("notice = %q, want no such directory", m.notice)
	}
	if m.root != root {
		t.Fatalf("failed cd changed root to %q", m.root)
	}
	if !strings.Contains(m.render(), "⚠") {
		t.Fatalf("render missing warning for missing directory:\n%s", m.render())
	}

	m = runCommandLine(t, m, "cd top.md")
	if !strings.Contains(m.notice, "not a directory") {
		t.Fatalf("notice = %q, want not a directory", m.notice)
	}
	if m.root != root {
		t.Fatalf("cd of a file changed root to %q", m.root)
	}

	m = runCommandLine(t, m, "cd")
	if !strings.Contains(m.notice, "usage: cd <folder>") {
		t.Fatalf("notice = %q, want usage", m.notice)
	}
	if m.root != root {
		t.Fatalf("bare cd changed root to %q", m.root)
	}

	m = runCommandLine(t, m, "frobnicate x")
	if !strings.Contains(m.notice, "unknown command") {
		t.Fatalf("notice = %q, want unknown command", m.notice)
	}
	if m.root != root {
		t.Fatalf("unknown command changed root to %q", m.root)
	}
	if m.commandPrompt {
		t.Fatal("unknown command left the prompt open")
	}
}

func TestCommandColonReservedFromTreeSearch(t *testing.T) {
	root, _ := cdFixture(t)
	m := New(root)
	m = pressKey(t, m, tea.KeyPressMsg{Code: ':', Text: ":"})
	if !m.commandPrompt {
		t.Fatal(": did not open the command prompt")
	}
	if m.treeSearching {
		t.Fatal(": seeded a tree search")
	}
}

func TestCommandMouseIgnoredWhilePromptOpen(t *testing.T) {
	root, _ := cdFixture(t)
	m := New(root)
	m.width, m.height = 100, 30
	m = pressKey(t, m, tea.KeyPressMsg{Code: ':', Text: ":"})

	// Row 2 is top.md. A click there selects the file when the prompt is closed.
	x := m.frameLeft() + 3
	y := m.bodyTop() + 2
	next, cmd := m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	m = next.(Model)
	if cmd != nil {
		if msg := cmd(); msg != nil {
			next, _ = m.Update(msg)
			m = next.(Model)
		}
	}
	if m.selected != "" {
		t.Fatalf("mouse click selected %q while the command prompt was open", m.selected)
	}
	if !m.commandPrompt {
		t.Fatal("mouse click closed the command prompt")
	}
}

func TestResolveCdPath(t *testing.T) {
	home, homeErr := os.UserHomeDir()
	tests := []struct {
		name    string
		root    string
		arg     string
		want    string
		wantErr bool
		skip    bool
	}{
		{name: "empty", root: "/abs/base", arg: "", wantErr: true},
		{name: "relative", root: "/abs/base", arg: "x", want: "/abs/base/x"},
		{name: "clean", root: "/abs/base", arg: "x/../y", want: "/abs/base/y"},
		{name: "home", root: "/abs/base", arg: "~", want: home, skip: homeErr != nil},
		{name: "home sub", root: "/etc", arg: "~/sub", want: filepath.Join(home, "sub"), skip: homeErr != nil},
		{name: "absolute", root: "/etc", arg: "/var", want: "/var"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skip {
				t.Skip("home directory unavailable")
			}
			got, err := resolveCdPath(tt.root, tt.arg)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("resolveCdPath(%q, %q) = %q, want error", tt.root, tt.arg, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveCdPath(%q, %q) error: %v", tt.root, tt.arg, err)
			}
			if got != tt.want {
				t.Fatalf("resolveCdPath(%q, %q) = %q, want %q", tt.root, tt.arg, got, tt.want)
			}
		})
	}
}

func runCommandLine(t *testing.T, m Model, line string) Model {
	t.Helper()
	m = pressKey(t, m, tea.KeyPressMsg{Code: ':', Text: ":"})
	for _, r := range line {
		m = pressKey(t, m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
}
