// Package config persists mdui's preferences and session state between
// launches.
package config

import (
	"errors"
	"math"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// DefaultTreeRatio is the tree pane's share of the content width when no state
// has been saved yet.
const DefaultTreeRatio = 0.3

const stateFile = "state.toml"

// State is the small amount of per-session state mdui remembers.
type State struct {
	TreeRatio float64 `toml:"tree_ratio"`
}

// Default returns the state used on first launch or after a bad state file.
func Default() State {
	return State{TreeRatio: DefaultTreeRatio}
}

// Dir returns mdui's configuration directory. It honours XDG_CONFIG_HOME and
// otherwise uses ~/.config/mdui, including on macOS where os.UserConfigDir
// would prefer ~/Library/Application Support.
func Dir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "mdui"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "mdui"), nil
}

// DefaultPath returns the absolute path of the state file.
func DefaultPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, stateFile), nil
}

// Load reads state from path. Missing, unreadable, and malformed files all
// yield the defaults so a damaged state file can never keep mdui from starting.
func Load(path string) State {
	state, _ := LoadState(path)
	return state
}

// LoadState reads state from path and reports whether a valid file was found.
// Callers use the flag to decide whether a config-file default should apply.
func LoadState(path string) (State, bool) {
	if path == "" {
		return Default(), false
	}
	state := Default()
	if _, err := toml.DecodeFile(path, &state); err != nil {
		return Default(), false
	}
	if math.IsNaN(state.TreeRatio) || math.IsInf(state.TreeRatio, 0) ||
		state.TreeRatio < 0 || state.TreeRatio > 1 {
		return Default(), false
	}
	return state, true
}

// LoadDefault loads state from the default configuration path.
func LoadDefault() State {
	path, err := DefaultPath()
	if err != nil {
		return Default()
	}
	return Load(path)
}

// Save writes state to path atomically: the TOML goes to a temporary file in
// the same directory, is flushed, and is renamed over the destination. Missing
// parent directories are created.
func Save(path string, state State) error {
	if path == "" {
		return errors.New("config: empty state path")
	}
	return writeTOML(path, state)
}

// writeTOML atomically encodes value as TOML at path.
func writeTOML(path string, value any) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".mdui-*.toml")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmpName)
		}
	}()

	if err := toml.NewEncoder(tmp).Encode(value); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	committed = true
	return nil
}

// writeFileAtomic writes data to path through a temporary file and rename so
// readers never observe a partial stylesheet or config.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".mdui-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	committed = true
	return nil
}
