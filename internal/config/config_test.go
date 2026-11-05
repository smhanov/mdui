package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/smhanov/mdui/internal/render"
)

func TestConfigRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.toml")
	want := Config{
		Style:         "light",
		Mouse:         false,
		Emoji:         true,
		WrapWidth:     72,
		HiddenFiles:   true,
		HideEmptyDirs: true,
		SortBy:        "mtime",
		TreeRatio:     0.44,
		Editor:        "vim",
	}
	if err := SaveConfig(path, want); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	if got := LoadConfig(path); got != want {
		t.Fatalf("LoadConfig = %+v, want %+v", got, want)
	}
}

func TestConfigUnknownKeysIgnored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	data := "style = \"light\"\nfuture_key = 42\n[nested]\nvalue = true\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	got := LoadConfig(path)
	if got.Style != "light" {
		t.Fatalf("style = %q, want light", got.Style)
	}
	if got.Mouse != DefaultConfig().Mouse || got.SortBy != DefaultConfig().SortBy {
		t.Fatalf("defaults changed by unknown keys: %+v", got)
	}
}

func TestLoadConfigMissingAndCorruptUseDefaults(t *testing.T) {
	if got := LoadConfig(filepath.Join(t.TempDir(), "missing.toml")); got != DefaultConfig() {
		t.Fatalf("missing config = %+v, want defaults", got)
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("this is [not toml"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := LoadConfig(path); got != DefaultConfig() {
		t.Fatalf("corrupt config = %+v, want defaults", got)
	}
}

func TestLoadOrCreateWritesExampleAndStylesheets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mdui", "config.toml")

	cfg, err := LoadOrCreate(path)
	if err != nil {
		t.Fatalf("LoadOrCreate: %v", err)
	}
	if cfg != DefaultConfig() {
		t.Fatalf("first-run config = %+v, want defaults", cfg)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"style = \"auto\"", "mouse = true", "wrap_width = 0", "sort_by = \"name\""} {
		if !strings.Contains(string(data), want) {
			t.Errorf("example config missing %q:\n%s", want, data)
		}
	}

	for _, name := range []string{"mdui-dark.json", "mdui-light.json"} {
		stylePath := filepath.Join(dir, "mdui", "styles", name)
		if _, err := os.Stat(stylePath); err != nil {
			t.Errorf("default stylesheet %s not installed: %v", name, err)
		}
	}

	// A user edit must survive the next launch.
	stylePath := filepath.Join(dir, "mdui", "styles", "mdui-dark.json")
	if err := os.WriteFile(stylePath, []byte(`{"document":{"color":"1"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreate(path); err != nil {
		t.Fatalf("second LoadOrCreate: %v", err)
	}
	data, err = os.ReadFile(stylePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"document":{"color":"1"}}` {
		t.Fatal("LoadOrCreate overwrote an edited stylesheet")
	}
}

func TestResolveStylePrecedence(t *testing.T) {
	tests := []struct {
		name   string
		cli    string
		config string
		env    string
		dark   bool
		want   string
	}{
		{name: "cli wins", cli: "dracula", config: "light", env: "dark", dark: true, want: "dracula"},
		{name: "config wins over env", config: "light", env: "dracula", dark: true, want: "light"},
		{name: "env wins over detection", config: "auto", env: "dracula", dark: true, want: "dracula"},
		{name: "auto detects light", config: "auto", env: "auto", dark: false, want: render.MduiLight},
		{name: "auto detects dark", config: "auto", dark: true, want: render.MduiDark},
		{name: "cli auto falls through", cli: "auto", config: "light", env: "dark", dark: true, want: "light"},
		{name: "blank env detects", config: "", env: "  ", dark: true, want: render.MduiDark},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveStyle(tt.cli, tt.config, func(string) string { return tt.env }, func() bool { return tt.dark }, "")
			if got != tt.want {
				t.Fatalf("ResolveStyle = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUserStylesheetOverridesEmbeddedAndBuiltIn(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"mdui-dark.json", "dark.json"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(`{"document":{}}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	override := ResolveStyle("", "", nil, func() bool { return true }, dir)
	if want := filepath.Join(dir, "mdui-dark.json"); override != want {
		t.Fatalf("embedded override = %q, want %q", override, want)
	}

	builtIn := ResolveStyle("dark", "", nil, func() bool { return true }, dir)
	if want := filepath.Join(dir, "dark.json"); builtIn != want {
		t.Fatalf("built-in override = %q, want %q", builtIn, want)
	}

	explicit := filepath.Join(dir, "custom.json")
	if got := ResolveStyle(explicit, "", nil, nil, dir); got != explicit {
		t.Fatalf("explicit path = %q, want %q", got, explicit)
	}
	if got := ResolveStyle("dracula", "", nil, nil, dir); got != "dracula" {
		t.Fatalf("non-existent user sheet = %q, want dracula", got)
	}
}

func TestNormalizeConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	data := "style = \"  \"\nsort_by = \"SIZE\"\nwrap_width = -5\ntree_ratio = 2\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	got := LoadConfig(path)
	if got.Style != "auto" || got.SortBy != "size" || got.WrapWidth != 0 || got.TreeRatio != DefaultTreeRatio {
		t.Fatalf("normalized config = %+v", got)
	}
}

func TestStartupTreeRatio(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TreeRatio = 0.42
	missing := filepath.Join(t.TempDir(), "state.toml")
	if got := StartupTreeRatio(cfg, missing); got != 0.42 {
		t.Fatalf("ratio without state = %v, want 0.42", got)
	}

	statePath := filepath.Join(t.TempDir(), "state.toml")
	if err := Save(statePath, State{TreeRatio: 0.55}); err != nil {
		t.Fatal(err)
	}
	if got := StartupTreeRatio(cfg, statePath); got != 0.55 {
		t.Fatalf("ratio with state = %v, want 0.55", got)
	}

	broken := filepath.Join(t.TempDir(), "state.toml")
	if err := os.WriteFile(broken, []byte("tree_ratio = 5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := StartupTreeRatio(cfg, broken); got != 0.42 {
		t.Fatalf("ratio with invalid state = %v, want config 0.42", got)
	}
}
