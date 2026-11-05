package render

import (
	"strings"
	"testing"
)

func TestJoinSoftBreaksListItems(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "tight bullet item joins continuation",
			in:   "- item alpha one two\n  three four five\n",
			want: "- item alpha one two three four five\n",
		},
		{
			name: "loose bullet item joins continuation",
			in:   "- item alpha one two\n  three four five\n\n- second item\n",
			want: "- item alpha one two three four five\n\n- second item\n",
		},
		{
			name: "ordered item joins continuation",
			in:   "1. item alpha one two\n   three four five\n",
			want: "1. item alpha one two three four five\n",
		},
		{
			name: "nested item joins within itself",
			in:   "- parent alpha one two\n  three four\n  - nested alpha one\n    two three\n",
			want: "- parent alpha one two three four\n  - nested alpha one two three\n",
		},
		{
			name: "distinct items stay distinct",
			in:   "- first item\n- second item\n",
			want: "- first item\n- second item\n",
		},
		{
			name: "nested list after item text is not joined into the item",
			in:   "- parent text\n  - child text\n",
			want: "- parent text\n  - child text\n",
		},
		{
			name: "paragraph joins",
			in:   "alpha one two\nthree four five\n",
			want: "alpha one two three four five\n",
		},
		{
			name: "blockquote joins",
			in:   "> alpha one two\n> three four\n",
			want: "> alpha one two three four\n",
		},
		{
			name: "emphasis across the break keeps its markers",
			in:   "text *bold one\ntwo* rest\n",
			want: "text *bold one two* rest\n",
		},
		{
			name: "hard break with two spaces stays",
			in:   "alpha one  \ntwo three\n",
			want: "alpha one  \ntwo three\n",
		},
		{
			name: "hard break with backslash stays",
			in:   "alpha one\\\ntwo three\n",
			want: "alpha one\\\ntwo three\n",
		},
		{
			name: "fenced code stays verbatim",
			in:   "```go\nalpha := 1\ntwo := 2\n```\n",
			want: "```go\nalpha := 1\ntwo := 2\n```\n",
		},
		{
			name: "table rows stay verbatim",
			in:   "| a | b |\n| --- | --- |\n| one | two |\n",
			want: "| a | b |\n| --- | --- |\n| one | two |\n",
		},
		{
			name: "heading stays verbatim, setext text joins into one line",
			in:   "# Title\n\nsetext stays\ntoo\n=====\n",
			want: "# Title\n\nsetext stays too\n=====\n",
		},
		{
			name: "invalid setext underline is a lazy continuation and joins",
			in:   "Title text\n===== sub\n",
			want: "Title text ===== sub\n",
		},
		{
			name: "crlf breaks join to one space",
			in:   "- item alpha\r\n  three four\r\n",
			want: "- item alpha three four\r\n",
		},
		{
			name: "item code fence inside a list stays verbatim",
			in:   "- item text\n  ```\n  code one\n  code two\n  ```\n",
			want: "- item text\n  ```\n  code one\n  code two\n  ```\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := string(joinSoftBreaks([]byte(tt.in))); got != tt.want {
				t.Errorf("joinSoftBreaks(%q) =\n%q\nwant\n%q", tt.in, got, tt.want)
			}
		})
	}
}

func TestJoinSoftBreaksNoBreaksReturnsInput(t *testing.T) {
	src := []byte("# Only single lines here\n\n- one\n- two\n")
	got := joinSoftBreaks(src)
	if &got[0] != &src[0] {
		t.Errorf("joinSoftBreaks copied a buffer with no soft breaks")
	}
}

func TestJoinSoftBreaksEmpty(t *testing.T) {
	if got := joinSoftBreaks(nil); len(got) != 0 {
		t.Errorf("joinSoftBreaks(nil) = %q, want empty", got)
	}
}

// TestJoinSoftBreaksPlansStyle exercises the .plans formatting that reported
// the bug: hard-wrapped bullet items with two-space continuations.
func TestJoinSoftBreaksPlansStyle(t *testing.T) {
	in := "## Context and prerequisites\n\n" +
		"- Depends: S001 (shell); integrates the `tree.SelectionMsg` emitted by S002\n" +
		"  (status-bar-only until this task wires it into the viewer).\n" +
		"- Spec source of truth: `specs.md` §6.2 (viewer), §7.1 (Glamour config),\n" +
		"  §7.3 (colours), §8 (performance), §9 (edge cases).\n"
	want := "## Context and prerequisites\n\n" +
		"- Depends: S001 (shell); integrates the `tree.SelectionMsg` emitted by S002 (status-bar-only until this task wires it into the viewer).\n" +
		"- Spec source of truth: `specs.md` §6.2 (viewer), §7.1 (Glamour config), §7.3 (colours), §8 (performance), §9 (edge cases).\n"
	if got := string(joinSoftBreaks([]byte(in))); got != want {
		t.Errorf("joinSoftBreaks plans style =\n%q\nwant\n%q", got, want)
	}
}

func TestJoinSoftBreaksIdempotent(t *testing.T) {
	src := "- item alpha one two\n  three four\n\npara one two\nthree four\n"
	once := string(joinSoftBreaks([]byte(src)))
	twice := string(joinSoftBreaks([]byte(once)))
	if once != twice {
		t.Errorf("joinSoftBreaks not idempotent:\nonce  = %q\ntwice = %q", once, twice)
	}
	if strings.Count(once, "\n") != 3 {
		t.Errorf("joined output = %q, want exactly three newlines", once)
	}
}
