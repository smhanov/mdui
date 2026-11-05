package mdutil

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestParseLinkFootnotesVariants(t *testing.T) {
	rendered := "  [1]: report https://example.com/q3\n" +
		"  [ 2]: https://example.com/bare\n" +
		"\t[3]: jump #section\n" +
		"[4]: site https://example.com/trunc…\n" +
		"not a footnote\n" +
		"  [5]: image https://example.com/image.png\n"
	links := ParseLinkFootnotes(rendered)
	if len(links) != 5 {
		t.Fatalf("links = %d, want 5: %+v", len(links), links)
	}
	want := []struct {
		number int
		text   string
		url    string
		line   int
		trunc  bool
	}{
		{1, "report", "https://example.com/q3", 0, false},
		{2, "", "https://example.com/bare", 1, false},
		{3, "jump", "#section", 2, false},
		{4, "site", "https://example.com/trunc…", 3, true},
		{5, "image", "https://example.com/image.png", 5, false},
	}
	for i, expected := range want {
		got := links[i]
		if got.Number != expected.number || got.Text != expected.text || got.URL != expected.url ||
			got.Line != expected.line || got.Truncated != expected.trunc {
			t.Errorf("link %d = %+v, want %+v", i, got, expected)
		}
	}

	if link, ok := LinkAtLine(links, 3); !ok || link.Number != 4 {
		t.Fatalf("LinkAtLine(3) = %+v, %v", link, ok)
	}
	if _, ok := LinkAtLine(links, 4); ok {
		t.Fatal("non-footnote line matched a link")
	}
}

func TestParseLinkFootnotesFromGlamour(t *testing.T) {
	src := []byte("# Links\n\n| Name | Target |\n|:--|:--|\n" +
		"| local | [other](./other.md) |\n| remote | [site](https://example.com/path) |\n")
	rendered := renderFixture(t, string(src), 80)
	links := ParseLinkFootnotes(rendered)
	if len(links) != 2 {
		t.Fatalf("links = %d, want 2\n%s", len(links), rendered)
	}
	if links[0].URL != "./other.md" || links[1].URL != "https://example.com/path" {
		t.Fatalf("links = %+v", links)
	}
	lines := bytes.Split([]byte(rendered), []byte("\n"))
	for _, link := range links {
		if link.Line < 0 || link.Line >= len(lines) {
			t.Fatalf("line %d out of range", link.Line)
		}
		if !bytes.Contains(lines[link.Line], []byte("[1]")) && !bytes.Contains(lines[link.Line], []byte("[2]")) {
			t.Fatalf("line %d is not the rendered footnote: %q", link.Line, lines[link.Line])
		}
	}
}

func TestClassifyLink(t *testing.T) {
	tests := []struct {
		raw  string
		want LinkKind
	}{
		{"../x.md", LinkFile},
		{"./x.md", LinkFile},
		{"x.md", LinkFile},
		{"x.md#section", LinkFile},
		{"notes/X.MD", LinkFile},
		{"#section", LinkAnchor},
		{"#", LinkAnchor},
		{"https://example.com", LinkRemote},
		{"http://example.com/x", LinkRemote},
		{"HTTPS://EXAMPLE.COM", LinkRemote},
		{"mailto:a@example.com", LinkOther},
		{"ftp://example.com", LinkOther},
		{"javascript:alert(1)", LinkOther},
		{"file:///tmp/x.md", LinkOther},
		{"//example.com/x", LinkOther},
		{"README", LinkOther},
		{"image.png", LinkOther},
		{"", LinkOther},
	}
	for _, tt := range tests {
		if got := ClassifyLink(tt.raw); got != tt.want {
			t.Errorf("ClassifyLink(%q) = %v, want %v", tt.raw, got, tt.want)
		}
	}
}

func TestResolveLink(t *testing.T) {
	current := filepath.Join("/docs", "guide", "intro.md")

	target, err := ResolveLink(current, "../x.md")
	if err != nil {
		t.Fatal(err)
	}
	if target.Kind != LinkFile || target.Path != filepath.Join("/docs", "x.md") {
		t.Fatalf("parent target = %+v", target)
	}

	target, err = ResolveLink(current, "./x.md#frag")
	if err != nil {
		t.Fatal(err)
	}
	if target.Path != filepath.Join("/docs", "guide", "x.md") || target.Anchor != "frag" {
		t.Fatalf("sibling target = %+v", target)
	}

	target, err = ResolveLink(current, "#section%20one")
	if err != nil {
		t.Fatal(err)
	}
	if target.Kind != LinkAnchor || target.Anchor != "section one" {
		t.Fatalf("anchor target = %+v", target)
	}

	target, err = ResolveLink(current, "https://example.com/x")
	if err != nil {
		t.Fatal(err)
	}
	if target.Kind != LinkRemote || target.URL != "https://example.com/x" {
		t.Fatalf("remote target = %+v", target)
	}

	target, err = ResolveLink(current, "mailto:a@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if target.Kind != LinkOther {
		t.Fatalf("mailto kind = %v", target.Kind)
	}

	if _, err := ResolveLink("", "x.md"); err == nil {
		t.Fatal("relative link without a current document should be an error")
	}
}
