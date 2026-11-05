package config

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "state.toml")

	if err := Save(path, State{TreeRatio: 0.42}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got := Load(path)
	if math.Abs(got.TreeRatio-0.42) > 1e-9 {
		t.Fatalf("Load ratio = %v, want 0.42", got.TreeRatio)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "tree_ratio") {
		t.Fatalf("state file missing tree_ratio key:\n%s", data)
	}
}

func TestLoadMissingUsesDefaults(t *testing.T) {
	got := Load(filepath.Join(t.TempDir(), "does-not-exist.toml"))
	if got != Default() {
		t.Fatalf("Load(missing) = %+v, want %+v", got, Default())
	}
	if got := Load(""); got != Default() {
		t.Fatalf("Load(empty path) = %+v, want %+v", got, Default())
	}
}

func TestLoadCorruptUsesDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.toml")
	if err := os.WriteFile(path, []byte("this is [not toml"), 0o600); err != nil {
		t.Fatal(err)
	}

	got := Load(path)
	if got != Default() {
		t.Fatalf("Load(corrupt) = %+v, want %+v", got, Default())
	}
}

func TestLoadRejectsImpossibleRatios(t *testing.T) {
	for _, value := range []string{"nan", "-0.1", "1.5"} {
		t.Run(value, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.toml")
			if err := os.WriteFile(path, []byte("tree_ratio = "+value+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if got := Load(path); got != Default() {
				t.Fatalf("Load(%s) = %+v, want defaults", value, got)
			}
		})
	}
}

func TestSaveLeavesNoTemporaryFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.toml")
	if err := Save(path, State{TreeRatio: 0.25}); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "state.toml" {
		t.Fatalf("state directory contains %v, want only state.toml", entryNames(entries))
	}
}

func TestDefaultPathHonoursXDG(t *testing.T) {
	home := t.TempDir()
	xdg := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)

	got, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(xdg, "mdui", "state.toml")
	if got != want {
		t.Fatalf("DefaultPath() = %q, want %q", got, want)
	}
}

func TestDefaultPathFallsBackToDotConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	got, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".config", "mdui", "state.toml")
	if got != want {
		t.Fatalf("DefaultPath() = %q, want %q", got, want)
	}
}

func entryNames(entries []os.DirEntry) []string {
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name()
	}
	return names
}
