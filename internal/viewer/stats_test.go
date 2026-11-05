package viewer

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/smhanov/mdui/internal/render"
)

func TestWordCountMatchesWc(t *testing.T) {
	text := "A short note with five words.\n\nSecond paragraph, still counting.  中文 也有 词.\n"
	path := filepath.Join(t.TempDir(), "words.md")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}

	want := WordCount(text)
	wc, err := exec.Command("wc", "-w", path).Output()
	if err != nil {
		t.Skipf("wc unavailable: %v", err)
	}
	fields := strings.Fields(string(wc))
	if len(fields) == 0 {
		t.Fatalf("wc output = %q", wc)
	}
	count, err := strconv.Atoi(fields[0])
	if err != nil {
		t.Fatalf("wc output = %q: %v", wc, err)
	}
	if want != count {
		t.Fatalf("WordCount = %d, wc -w = %d", want, count)
	}
}

func TestReadingMinutes(t *testing.T) {
	tests := map[int]int{0: 1, 1: 1, 199: 1, 200: 1, 201: 2, 400: 2, 401: 3}
	for words, want := range tests {
		if got := ReadingMinutes(words); got != want {
			t.Errorf("ReadingMinutes(%d) = %d, want %d", words, got, want)
		}
	}
}

func TestStatsTrackDocumentAndScroll(t *testing.T) {
	f := &fakeRenderer{raw: strings.Repeat("word ", 450)}
	f.render = func(request) string { return strings.Repeat("a rendered line\n", 100) }
	m := newTestViewer(f)
	m = apply(t, m, m.SetSize(80, 12))
	m = apply(t, m, m.Open("notes.md"))

	stats := m.Stats()
	if !stats.Ready {
		t.Fatal("stats are not marked ready after a render")
	}
	if stats.Words != 450 {
		t.Fatalf("words = %d, want 450", stats.Words)
	}
	if stats.ReadingMinutes != 3 {
		t.Fatalf("reading minutes = %d, want 3", stats.ReadingMinutes)
	}
	if stats.ScrollPct != 0 {
		t.Fatalf("top scroll = %d%%, want 0%%", stats.ScrollPct)
	}

	m, _ = m.Update(press("G"))
	if got := m.Stats().ScrollPct; got != 100 {
		t.Fatalf("bottom scroll = %d%%, want 100%%", got)
	}
}

func TestStatsAreNotRepeatedWhenScrollPercentIsUnchanged(t *testing.T) {
	f := &fakeRenderer{raw: strings.Repeat("word ", 500)}
	f.render = func(request) string { return strings.Repeat("line\n", 100) }
	m := newTestViewer(f)
	m = apply(t, m, m.SetSize(80, 12))
	m = apply(t, m, m.Open("notes.md"))

	m, _ = m.Update(press("G"))
	before := m.Stats().ScrollPct
	if before != 100 {
		t.Fatalf("bottom scroll = %d%%, want 100%%", before)
	}

	for range 20 {
		next, cmd := m.Update(press("j"))
		m = next
		if cmd == nil {
			continue
		}
		if msg, ok := cmd().(StatsMsg); ok {
			t.Fatalf("unchanged scroll percent emitted %+v", msg)
		}
	}
}

func TestWrapWidthOverridesRenderWidth(t *testing.T) {
	f := &fakeRenderer{}
	m := NewWithOptions(Options{Style: render.Dark, WrapWidth: 40})
	m.fileCmd = f.file
	m.bytesCmd = f.embedded
	m.rawCmd = f.rawFile

	m = apply(t, m, m.SetSize(80, 20))
	m = apply(t, m, m.Open("notes.md"))
	if got := f.calls[len(f.calls)-1].width; got != 40 {
		t.Fatalf("render width = %d, want 40", got)
	}

	// A pane narrower than the override still renders at the pane width.
	m = apply(t, m, m.SetSize(30, 20))
	if got := f.calls[len(f.calls)-1].width; got != 28 {
		t.Fatalf("narrow pane render width = %d, want 28", got)
	}
}

func TestEmojiOptionReachesRenderer(t *testing.T) {
	f := &fakeRenderer{}
	m := NewWithOptions(Options{Style: render.Dark, Emoji: true})
	m.fileCmd = f.file
	m.bytesCmd = f.embedded
	m.rawCmd = f.rawFile

	m = apply(t, m, m.SetSize(80, 20))
	m = apply(t, m, m.Open("notes.md"))
	if !f.calls[len(f.calls)-1].emoji {
		t.Fatal("emoji setting did not reach the renderer")
	}
}
