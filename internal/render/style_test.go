package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestResolveStylePrecedence(t *testing.T) {
	tests := []struct {
		name   string
		cli    Style
		config Style
		env    string
		dark   bool
		want   Style
	}{
		{name: "cli wins", cli: "dracula", config: "light", env: "dark", dark: true, want: "dracula"},
		{name: "config wins over env", config: "light", env: "dracula", dark: true, want: Light},
		{name: "env wins over detection", config: "auto", env: "dracula", dark: true, want: "dracula"},
		{name: "detect light", config: "auto", env: "auto", dark: false, want: MduiLight},
		{name: "detect dark", config: "auto", dark: true, want: MduiDark},
		{name: "cli auto falls through", cli: "auto", config: "light", dark: true, want: Light},
		{name: "empty falls through", config: "", env: "", dark: false, want: MduiLight},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveStylePrecedence(tt.cli, tt.config, func(string) string { return tt.env }, func() bool { return tt.dark })
			if got != tt.want {
				t.Fatalf("style = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEmbeddedStylesheetRendersTable(t *testing.T) {
	fixture := filepath.Join("testdata", "table.md")
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := RenderBytes(data, 60, Options{Style: MduiDark})
	if err != nil {
		t.Fatalf("RenderBytes(MduiDark): %v", err)
	}
	plain := ansi.Strip(rendered)
	for _, want := range []string{"Quarterly review", "┼", "│", "─"} {
		if !strings.Contains(plain, want) {
			t.Errorf("mdui-dark table missing %q:\n%s", want, plain)
		}
	}
	if !strings.Contains(rendered, "38;5;252;1m") && !strings.Contains(rendered, "38;5;252") {
		t.Error("mdui-dark table cells are not coloured")
	}
}

func TestEmojiOption(t *testing.T) {
	data := []byte("Hello :rocket: world\n")
	plain, err := RenderBytes(data, 40, Options{Style: MduiDark})
	if err != nil {
		t.Fatal(err)
	}
	emoji, err := RenderBytes(data, 40, Options{Style: MduiDark, Emoji: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ansi.Strip(plain), ":rocket:") {
		t.Fatalf("plain render changed the shortcode:\n%s", ansi.Strip(plain))
	}
	if strings.Contains(ansi.Strip(emoji), ":rocket:") {
		t.Fatalf("emoji render left the shortcode in place:\n%s", ansi.Strip(emoji))
	}
}

func TestCacheSeparatesEmojiSetting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "emoji.md")
	if err := os.WriteFile(path, []byte("Hello :rocket:\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	plain := NewCmdWith(1, path, 40, Options{Style: MduiDark})().(RenderedMsg)
	emoji := NewCmdWith(2, path, 40, Options{Style: MduiDark, Emoji: true})().(RenderedMsg)
	if plain.Err != nil || emoji.Err != nil {
		t.Fatalf("render failed: plain=%v emoji=%v", plain.Err, emoji.Err)
	}
	if plain.Rendered == emoji.Rendered {
		t.Fatal("emoji and plain renders shared a cache entry or produced identical output")
	}
}
