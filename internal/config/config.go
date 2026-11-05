package config

import (
	"errors"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"mdui/internal/render"
	"mdui/internal/style"
)

const (
	configFile  = "config.toml"
	stylesDir   = "styles"
	sortByName  = "name"
	sortByMtime = "mtime"
	sortBySize  = "size"
)

// Config is the stable preference file at ~/.config/mdui/config.toml.
type Config struct {
	Style         string  `toml:"style"`
	Mouse         bool    `toml:"mouse"`
	Emoji         bool    `toml:"emoji"`
	WrapWidth     int     `toml:"wrap_width"`
	HiddenFiles   bool    `toml:"hidden_files"`
	HideEmptyDirs bool    `toml:"hide_empty_dirs"`
	SortBy        string  `toml:"sort_by"`
	TreeRatio     float64 `toml:"tree_ratio"`
	Editor        string  `toml:"editor"`
}

// DefaultConfig returns the preferences used on first launch or after a bad
// config file.
func DefaultConfig() Config {
	return Config{
		Style:     "auto",
		Mouse:     true,
		Emoji:     true,
		SortBy:    sortByName,
		TreeRatio: DefaultTreeRatio,
	}
}

// ConfigPath returns the absolute path of the config file.
func ConfigPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configFile), nil
}

// StylesPath returns the directory holding user stylesheet overrides.
func StylesPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, stylesDir), nil
}

// StatePath returns the absolute path of the session state file.
func StatePath() (string, error) {
	return DefaultPath()
}

// LoadConfig reads path over the defaults. Missing, unreadable, and malformed
// files yield the defaults, and unknown keys are ignored so a newer config can
// still run on an older build.
func LoadConfig(path string) Config {
	cfg := DefaultConfig()
	if path == "" {
		return cfg
	}
	if _, err := os.Stat(path); err != nil {
		return cfg
	}
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return DefaultConfig()
	}
	cfg.normalize()
	return cfg
}

// LoadOrCreate loads the config at path, creating the config directory, a
// commented example config, and editable copies of the default stylesheets
// when they are missing.
func LoadOrCreate(path string) (Config, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return DefaultConfig(), err
	}

	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		if err := writeFileAtomic(path, []byte(exampleConfig), 0o644); err != nil {
			return DefaultConfig(), err
		}
	}

	if err := installDefaultStyles(filepath.Join(dir, stylesDir)); err != nil {
		return LoadConfig(path), err
	}
	return LoadConfig(path), nil
}

// SaveConfig writes cfg to path atomically, creating parent directories.
func SaveConfig(path string, cfg Config) error {
	if path == "" {
		return errors.New("config: empty config path")
	}
	cfg.normalize()
	return writeTOML(path, cfg)
}

func (c *Config) normalize() {
	c.Style = strings.TrimSpace(c.Style)
	if c.Style == "" {
		c.Style = "auto"
	}
	if c.WrapWidth < 0 {
		c.WrapWidth = 0
	}
	c.SortBy = strings.ToLower(strings.TrimSpace(c.SortBy))
	switch c.SortBy {
	case sortByName, sortByMtime, sortBySize:
	default:
		c.SortBy = sortByName
	}
	if math.IsNaN(c.TreeRatio) || math.IsInf(c.TreeRatio, 0) ||
		c.TreeRatio < 0 || c.TreeRatio > 1 {
		c.TreeRatio = DefaultTreeRatio
	}
	c.Editor = strings.TrimSpace(c.Editor)
}

// StartupTreeRatio picks the initial split ratio: a valid saved session state
// wins, otherwise the config default applies.
func StartupTreeRatio(cfg Config, statePath string) float64 {
	if state, ok := LoadState(statePath); ok {
		return state.TreeRatio
	}
	return cfg.TreeRatio
}

// ResolveStyle applies the documented precedence: CLI flag, config file,
// GLAMOUR_STYLE, then terminal background detection. A stylesheet in
// stylesDir overrides the embedded/built-in style of the same name.
func ResolveStyle(cliStyle, configStyle string, getenv func(string) string, hasDark func() bool, stylesDir string) string {
	chosen := render.ResolveStylePrecedence(cliStyle, configStyle, getenv, hasDark)
	return UserStylesheet(chosen, stylesDir)
}

// UserStylesheet returns a user stylesheet path when one named after docStyle
// exists, and docStyle unchanged otherwise. Explicit paths are never remapped.
func UserStylesheet(docStyle, stylesDir string) string {
	name := strings.TrimSpace(docStyle)
	if name == "" || stylesDir == "" || looksLikePath(name) {
		return name
	}
	path := filepath.Join(stylesDir, strings.TrimSuffix(name, ".json")+".json")
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return name
	}
	return path
}

func looksLikePath(name string) bool {
	return strings.ContainsAny(name, `/\`) || strings.HasSuffix(strings.ToLower(name), ".json")
}

const exampleConfig = `# mdui configuration
#
# Every key below is optional; the value shown is the built-in default.
# Unknown keys are ignored, so this file stays compatible across versions.

# Document style: "auto", "dark", "light", "mdui-dark", "mdui-light", any
# Glamour built-in name, or a path to a JSON stylesheet. "auto" follows the
# terminal background. A file in ~/.config/mdui/styles/<name>.json overrides
# a built-in or embedded style of the same name.
style = "auto"

# Capture the mouse for clicking, wheel scrolling, and divider dragging.
# Press m at runtime to toggle this; shift-click always uses the terminal's
# native selection.
mouse = true

# Render :emoji: shortcodes as unicode emoji.
emoji = true

# Reflow the document at this many columns. 0 uses the viewer pane width.
wrap_width = 0

# Show dot-prefixed files and directories in the tree.
hidden_files = false

# Hide directories that contain no markdown anywhere beneath them.
hide_empty_dirs = false

# Tree order: "name", "mtime", or "size". Directories stay first.
sort_by = "name"

# Initial share of the width given to the tree (0.0–1.0). The last dragged
# ratio is remembered in state.toml and wins on later launches.
tree_ratio = 0.3

# Editor command reserved for opening the current file (not used yet).
editor = ""
`

// installDefaultStyles copies the embedded stylesheets into dir without
// overwriting files the user may have edited.
func installDefaultStyles(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, name := range style.DefaultStyleNames() {
		path := filepath.Join(dir, name+".json")
		if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
			continue
		}
		data, ok := style.LookupStylesheet(name)
		if !ok {
			continue
		}
		if err := writeFileAtomic(path, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}
