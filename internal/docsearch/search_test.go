package docsearch

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestProjectPlainTextAndSegments(t *testing.T) {
	line := "\x1b[1mBold\x1b[0m and \x1b[38;5;33mblue\x1b[0m"
	projected := Project(line)

	if got, want := projected.Plain, ansi.Strip(line); got != want {
		t.Fatalf("plain = %q, want %q", got, want)
	}
	if got, want := ansi.StringWidth(projected.Styled), ansi.StringWidth(projected.Plain); got != want {
		t.Fatalf("styled width = %d, plain width = %d", got, want)
	}
	if len(projected.Segments) != 3 {
		t.Fatalf("segments = %d, want 3: %+v", len(projected.Segments), projected.Segments)
	}
	for _, segment := range projected.Segments {
		styled := projected.Styled[segment.StyledStart:segment.StyledEnd]
		plain := projected.Plain[segment.PlainStart:segment.PlainEnd]
		if styled != plain {
			t.Errorf("segment %+v maps %q to %q", segment, styled, plain)
		}
	}
}

func TestProjectOSC8Hyperlink(t *testing.T) {
	line := "see \x1b]8;;https://example.com\x1b\\the docs\x1b]8;;\x1b\\ now"
	projected := Project(line)

	if got, want := projected.Plain, "see the docs now"; got != want {
		t.Fatalf("plain = %q, want %q", got, want)
	}
	if got, want := ansi.StringWidth(projected.Styled), ansi.StringWidth(projected.Plain); got != want {
		t.Fatalf("styled width = %d, plain width = %d", got, want)
	}
	if len(projected.Segments) != 3 {
		t.Fatalf("segments = %d, want 3: %+v", len(projected.Segments), projected.Segments)
	}
}

func TestProjectWideText(t *testing.T) {
	line := "\x1b[32m日本語 👩‍💻\x1b[0m"
	projected := Project(line)

	if got, want := projected.Plain, "日本語 👩‍💻"; got != want {
		t.Fatalf("plain = %q, want %q", got, want)
	}
	if got, want := ansi.StringWidth(projected.Styled), ansi.StringWidth(projected.Plain); got != want {
		t.Fatalf("styled width = %d, plain width = %d", got, want)
	}
}

func TestHighlightPreservesPlainTextAndWidth(t *testing.T) {
	content := strings.Join([]string{
		"\x1b[1mAlpha\x1b[0m beta",
		"plain gamma",
		"\x1b[32m日本語 👩‍💻\x1b[0m and \x1b]8;;https://example.com\x1b\\docs\x1b]8;;\x1b\\",
		"no match here",
	}, "\n")

	for _, query := range []string{"Alpha beta", "gamma", "日本語", "docs", "👩‍💻"} {
		t.Run(query, func(t *testing.T) {
			index := New(content, query)
			if len(index.Matches) == 0 {
				t.Fatalf("query %q produced no matches", query)
			}
			highlighted := index.Highlight()
			if !strings.Contains(highlighted, reverseOn) {
				t.Fatalf("highlighted output has no reverse video: %q", highlighted)
			}

			gotLines := strings.Split(highlighted, "\n")
			wantLines := strings.Split(content, "\n")
			if len(gotLines) != len(wantLines) {
				t.Fatalf("line count = %d, want %d", len(gotLines), len(wantLines))
			}
			for i := range gotLines {
				if got, want := ansi.Strip(gotLines[i]), ansi.Strip(wantLines[i]); got != want {
					t.Errorf("line %d plain text = %q, want %q", i, got, want)
				}
				if got, want := ansi.StringWidth(gotLines[i]), ansi.StringWidth(wantLines[i]); got != want {
					t.Errorf("line %d width = %d, want %d", i, got, want)
				}
			}
			if index.Content != content {
				t.Error("highlighting mutated the original content")
			}
		})
	}
}

func TestHighlightWrapsEveryMatchedSegment(t *testing.T) {
	content := "\x1b[1mAlpha\x1b[0m \x1b[3mbeta\x1b[0m"
	index := New(content, "Alpha beta")
	if len(index.Matches) != 1 {
		t.Fatalf("matches = %d, want 1", len(index.Matches))
	}
	highlighted := index.Highlight()
	if got := strings.Count(highlighted, reverseOn); got != 3 {
		t.Fatalf("reverse-on count = %d, want 3 (one per matched segment): %q", got, highlighted)
	}
	if got := strings.Count(highlighted, reverseOff); got != 3 {
		t.Fatalf("reverse-off count = %d, want 3: %q", got, highlighted)
	}
}

func TestHighlightNoMatchesReturnsOriginal(t *testing.T) {
	content := "\x1b[1mAlpha\x1b[0m"
	index := New(content, "zzz")
	if len(index.Matches) != 0 {
		t.Fatalf("matches = %d, want 0", len(index.Matches))
	}
	if got := index.Highlight(); got != content {
		t.Fatalf("no-match highlight = %q, want original %q", got, content)
	}
}

func TestSmartCase(t *testing.T) {
	content := "Alpha alpha ALPHA"

	lower := New(content, "alpha")
	if lower.CaseSensitive {
		t.Fatal("lowercase query should be case-insensitive")
	}
	if got := len(lower.Matches); got != 3 {
		t.Fatalf("lowercase matches = %d, want 3", got)
	}

	mixed := New(content, "Alpha")
	if !mixed.CaseSensitive {
		t.Fatal("mixed-case query should be case-sensitive")
	}
	if got := len(mixed.Matches); got != 1 {
		t.Fatalf("mixed-case matches = %d, want 1", got)
	}

	upper := New(content, "ALPHA")
	if got := len(upper.Matches); got != 1 {
		t.Fatalf("uppercase matches = %d, want 1", got)
	}
}

func TestLiteralMetacharacters(t *testing.T) {
	content := "a.*b [x] (y) and aXXb"
	cases := map[string]int{
		".*":   1,
		"[":    1,
		"]":    1,
		"(y)":  1,
		"a.*c": 0,
		".*b":  1,
	}
	for query, want := range cases {
		if got := len(New(content, query).Matches); got != want {
			t.Errorf("query %q matches = %d, want %d", query, got, want)
		}
	}
}

func TestEmptyQueryAndContent(t *testing.T) {
	if idx := New("content", ""); len(idx.Matches) != 0 || idx.Highlight() != "content" {
		t.Fatalf("empty query should have no matches: %+v", idx)
	}
	if idx := New("", "query"); len(idx.Matches) != 0 || idx.Highlight() != "" {
		t.Fatalf("empty content should have no matches: %+v", idx)
	}
	var nilIndex *Index
	if got := nilIndex.Highlight(); got != "" {
		t.Fatalf("nil index highlight = %q, want empty", got)
	}
}

func TestMultipleMatchesPerLine(t *testing.T) {
	index := New("one two one two one", "one")
	if got := len(index.Matches); got != 3 {
		t.Fatalf("matches = %d, want 3", got)
	}
	wantStarts := []int{0, 8, 16}
	for i, match := range index.Matches {
		if match.Line != 0 {
			t.Errorf("match %d line = %d, want 0", i, match.Line)
		}
		if match.Start != wantStarts[i] {
			t.Errorf("match %d start = %d, want %d", i, match.Start, wantStarts[i])
		}
		plain := index.Lines[match.Line].Plain
		if got := plain[match.Start:match.End]; got != "one" {
			t.Errorf("match %d does not select %q: %q", i, "one", got)
		}
	}
}

func TestMatchOffsetsWithWideRunes(t *testing.T) {
	index := New("\x1b[32m日本語\x1b[0m", "本")
	if len(index.Matches) != 1 {
		t.Fatalf("matches = %d, want 1", len(index.Matches))
	}
	match := index.Matches[0]
	if got := index.Lines[match.Line].Plain[match.Start:match.End]; got != "本" {
		t.Fatalf("match text = %q, want 本", got)
	}
	if match.Start != len("日") {
		t.Fatalf("match byte start = %d, want %d", match.Start, len("日"))
	}
}
