package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestValidateDir(t *testing.T) {
	root := t.TempDir()
	if err := validateDir(root); err != nil {
		t.Fatalf("validateDir(%q) = %v, want nil", root, err)
	}

	file := filepath.Join(root, "note.md")
	if err := os.WriteFile(file, []byte("# note\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateDir(file); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("validateDir(file) = %v, want not-a-directory error", err)
	}

	missing := filepath.Join(root, "missing")
	if err := validateDir(missing); err == nil {
		t.Fatal("validateDir(missing) = nil, want error")
	}

	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Log("skipping unreadable-directory check (Windows or root)")
		return
	}

	locked := filepath.Join(root, "locked")
	if err := os.Mkdir(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })
	if err := validateDir(locked); err == nil {
		t.Fatal("validateDir(unreadable) = nil, want error")
	}
}
