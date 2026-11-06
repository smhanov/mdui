package viewer

import (
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/smhanov/mdui/internal/render"
)

func TestWordCount(t *testing.T) {
	tests := []struct {
		name string
		text string
		want int
	}{
		{"empty", "", 0},
		{"whitespace only", " \t\n\r\n  ", 0},
		{"ascii", "A short note with five words.\n\nSecond paragraph, still counting.\n", 10},
		{"cjk", "中文 也有 词.\n", 3},
		{"mixed", "A short note with five words.\n\nSecond paragraph, still counting.  中文 也有 词.\n", 13},
		{"punctuation does not split words", "one,two three...four five-six\n", 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := WordCount(tt.text); got != tt.want {
				t.Errorf("WordCount(%q) = %d, want %d", tt.text, got, tt.want)
			}
		})
	}
}

// TestWordCountMatchesGNUWc cross-checks the fixture against the canonical wc
// implementation. BSD and busybox wc tokenize this CJK text differently, so the
// check is skipped outside GNU coreutils.
func TestWordCountMatchesGNUWc(t *testing.T) {
	version, err := exec.Command("wc", "--version").Output()
	if err != nil || !strings.Contains(string(version), "GNU coreutils") {
		t.Skip("requires GNU coreutils wc")
	}

	const text = "A short note with five words.\n\nSecond paragraph, still counting.  中文 也有 词.\n"
	const want = 13

	cmd := exec.Command("wc", "-w")
	cmd.Stdin = strings.NewReader(text)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("wc -w: %v", err)
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		t.Fatalf("wc -w output = %q", out)
	}
	got, err := strconv.Atoi(fields[0])
	if err != nil {
		t.Fatalf("wc -w output = %q: %v", out, err)
	}
	if got != want {
		t.Fatalf("wc -w = %d, want %d", got, want)
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
