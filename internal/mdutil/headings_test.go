package mdutil

import (
	"strings"
	"testing"
)

func TestFindHeadingsAndSlugs(t *testing.T) {
	src := []byte(`# Top
## Next quarter
### 日本語 の見出し 🚀
## Next quarter
#### Hello, World!
`)
	headings := FindHeadings(src)
	if len(headings) != 5 {
		t.Fatalf("headings = %d, want 5: %+v", len(headings), headings)
	}
	wantSlugs := []string{"top", "next-quarter", "日本語-の見出し", "next-quarter-1", "hello-world"}
	for i, want := range wantSlugs {
		if headings[i].Slug != want {
			t.Errorf("heading %d slug = %q, want %q", i, headings[i].Slug, want)
		}
	}
	if headings[2].Text != "日本語 の見出し 🚀" {
		t.Errorf("unicode heading text = %q", headings[2].Text)
	}
	if headings[1].Line != 2 {
		t.Errorf("heading line = %d, want 2", headings[1].Line)
	}
}

func TestSlugify(t *testing.T) {
	tests := map[string]string{
		"Next quarter":       "next-quarter",
		"Hello, World!":      "hello-world",
		"  spaced  out  ":    "spaced-out",
		"snake_case":         "snake-case",
		"already-hyphenated": "already-hyphenated",
		"🚀 launch":           "launch",
		"":                   "",
	}
	for input, want := range tests {
		if got := Slugify(input); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestLocateHeadings(t *testing.T) {
	headings := []Heading{
		{Text: "Top", Slug: "top"},
		{Text: "Next quarter", Slug: "next-quarter"},
		{Text: "Missing", Slug: "missing"},
	}
	rendered := strings.Join([]string{
		"  Top",
		"",
		"Top is mentioned in the intro.",
		"",
		"  ## Next quarter",
		"",
		"body",
	}, "\n")

	positions := LocateHeadings(rendered, headings)
	if positions[0] != 0 || positions[1] != 4 || positions[2] != -1 {
		t.Fatalf("positions = %v, want [0 4 -1]", positions)
	}
	if line, ok := AnchorLine(rendered, headings, "#next-quarter"); !ok || line != 4 {
		t.Fatalf("AnchorLine = %d, %v", line, ok)
	}
	if _, ok := AnchorLine(rendered, headings, "#unknown"); ok {
		t.Fatal("unknown anchor resolved")
	}
	if heading, ok := HeadingBySlug(headings, "TOP"); !ok || heading.Slug != "top" {
		t.Fatalf("HeadingBySlug case-insensitive lookup failed: %+v %v", heading, ok)
	}
}
