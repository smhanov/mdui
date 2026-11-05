package render

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

var updateGolden = flag.Bool("update", false, "update golden files")

func TestResolveStyle(t *testing.T) {
	tests := []struct {
		name       string
		env        string
		dark       bool
		want       Style
		wantDetect bool
	}{
		{name: "dark terminal", dark: true, want: Dark, wantDetect: true},
		{name: "light terminal", dark: false, want: Light, wantDetect: true},
		{name: "env overrides light", env: "dracula", dark: false, want: "dracula"},
		{name: "env path overrides dark", env: "/tmp/theme.json", dark: true, want: "/tmp/theme.json"},
		{name: "auto detects", env: "auto", dark: false, want: Light, wantDetect: true},
		{name: "blank env detects", env: "   ", dark: true, want: Dark, wantDetect: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detected := false
			got := ResolveStyle(
				func(key string) string {
					if key != "GLAMOUR_STYLE" {
						t.Fatalf("unexpected environment key %q", key)
					}
					return tt.env
				},
				func() bool {
					detected = true
					return tt.dark
				},
			)
			if got != tt.want {
				t.Errorf("style = %q, want %q", got, tt.want)
			}
			if detected != tt.wantDetect {
				t.Errorf("background detection called = %v, want %v", detected, tt.wantDetect)
			}
		})
	}
}

func TestGoldenTable(t *testing.T) {
	// Glamour v2 writes ANSI directly (256-colour styles and a terminal16m
	// Chroma formatter), so this golden does not depend on TERM or NO_COLOR.
	// Regenerate with: go test ./internal/render -run TestGoldenTable -update
	fixture := filepath.Join("testdata", "table.md")
	msg, ok := NewCmd(1, fixture, 60, Dark)().(RenderedMsg)
	if !ok {
		t.Fatal("render command returned an unexpected message type")
	}
	if msg.Err != nil {
		t.Fatalf("render failed: %v", msg.Err)
	}

	plain := ansi.Strip(msg.Rendered)
	for _, want := range []string{
		"Quarterly review",
		"│",
		"─",
		"[1]: report",
		"[2]: test plan",
		"https://example.com/q3-backend",
		"https://example.com/q3-ios",
	} {
		if !strings.Contains(plain, want) {
			t.Errorf("rendered table does not contain %q", want)
		}
	}
	if !strings.Contains(msg.Rendered, "38;2;") {
		t.Error("code block is missing truecolour syntax highlighting")
	}

	goldenPath := filepath.Join("testdata", "table.golden")
	if *updateGolden {
		if err := os.WriteFile(goldenPath, []byte(msg.Rendered), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (run go test ./internal/render -run TestGoldenTable -update to create it): %v", err)
	}
	if msg.Rendered != string(want) {
		t.Errorf("rendered output differs from %s\nrun: go test ./internal/render -run TestGoldenTable -update",
			goldenPath)
	}
}

func TestLargeFileIsNotRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "huge.md")
	data := bytes.Repeat([]byte("x"), MaxRenderBytes+1)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	msg, ok := NewCmd(7, path, 80, Dark)().(RenderedMsg)
	if !ok {
		t.Fatal("render command returned an unexpected message type")
	}
	if !msg.Large {
		t.Fatal("large file was not reported as Large")
	}
	if msg.Raw != "" || msg.Rendered != "" {
		t.Fatalf("large file should not be read, got %d raw and %d rendered bytes", len(msg.Raw), len(msg.Rendered))
	}
	if msg.Size != int64(len(data)) {
		t.Fatalf("size = %d, want %d", msg.Size, len(data))
	}
}

func TestRenderCacheKeyedByFileMetadata(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(path, []byte("# Cached\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}

	first := NewCmd(1, path, 80, Dark)().(RenderedMsg)
	if first.Err != nil {
		t.Fatalf("first render failed: %v", first.Err)
	}
	info, err := os.Stat(absPath)
	if err != nil {
		t.Fatal(err)
	}
	key := Key{Path: absPath, ModTime: info.ModTime().UnixNano(), Width: 80, Style: Dark}
	if _, ok := defaultCache.Get(key); !ok {
		t.Fatal("render was not stored in the cache")
	}

	widthKey := key
	widthKey.Width = 81
	if _, ok := defaultCache.Get(widthKey); ok {
		t.Fatal("a different width should not hit the same cache entry")
	}

	newTime := info.ModTime().Add(2 * time.Second)
	if err := os.Chtimes(absPath, newTime, newTime); err != nil {
		t.Fatal(err)
	}
	info, err = os.Stat(absPath)
	if err != nil {
		t.Fatal(err)
	}
	mtimeKey := key
	mtimeKey.ModTime = info.ModTime().UnixNano()
	if mtimeKey.ModTime == key.ModTime {
		t.Fatal("test could not change the file mtime")
	}
	if _, ok := defaultCache.Get(mtimeKey); ok {
		t.Fatal("an mtime change should not hit the old cache entry")
	}

	second := NewCmd(2, path, 80, Dark)().(RenderedMsg)
	if second.Err != nil {
		t.Fatalf("render after mtime change failed: %v", second.Err)
	}
	if second.Rendered != first.Rendered {
		t.Fatal("unchanged content rendered differently after mtime bump")
	}
	if _, ok := defaultCache.Get(mtimeKey); !ok {
		t.Fatal("render after mtime change was not cached under the new key")
	}
}

func TestRenderWidthChangesOutput(t *testing.T) {
	fixture := filepath.Join("testdata", "table.md")
	narrow := NewCmd(1, fixture, 40, Dark)().(RenderedMsg)
	wide := NewCmd(2, fixture, 90, Dark)().(RenderedMsg)
	if narrow.Err != nil || wide.Err != nil {
		t.Fatalf("render failed: narrow=%v wide=%v", narrow.Err, wide.Err)
	}
	if narrow.Rendered == wide.Rendered {
		t.Fatal("different widths produced identical output")
	}
}
