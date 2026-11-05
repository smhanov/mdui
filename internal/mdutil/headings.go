package mdutil

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark/ast"
)

// Heading is one ATX or setext heading in the source. Line is 1-based.
type Heading struct {
	Level int
	Text  string
	Slug  string
	Line  int
}

// FindHeadings returns document headings in source order. Duplicate slugs get
// GitHub-style "-1", "-2" suffixes so in-document anchors stay unambiguous.
func FindHeadings(src []byte) []Heading {
	doc := parseDocument(src)
	var headings []Heading
	seen := make(map[string]int)

	_ = ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		heading, ok := node.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}
		text := strings.TrimSpace(nodeText(src, heading))
		slug := Slugify(text)
		if count := seen[slug]; count > 0 {
			seen[slug] = count + 1
			slug = slug + "-" + itoa(count)
		} else {
			seen[slug] = 1
		}
		line := 0
		if lines := heading.Lines(); lines.Len() > 0 {
			line = lineForOffset(src, lines.At(0).Start)
		}
		headings = append(headings, Heading{
			Level: heading.Level,
			Text:  text,
			Slug:  slug,
			Line:  line,
		})
		return ast.WalkContinue, nil
	})
	return headings
}

// Slugify converts heading text into a GitHub-like anchor: lower case,
// letters and digits kept (including CJK), spaces and existing dashes folded
// into single hyphens, punctuation dropped.
func Slugify(text string) string {
	var b strings.Builder
	pendingDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(text)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if pendingDash && b.Len() > 0 {
				b.WriteByte('-')
			}
			pendingDash = false
			b.WriteRune(r)
		case r == ' ' || r == '\t' || r == '-' || r == '_':
			if b.Len() > 0 {
				pendingDash = true
			}
		}
	}
	return b.String()
}

// HeadingBySlug finds a heading by anchor slug, ignoring a leading "#" and
// case.
func HeadingBySlug(headings []Heading, anchor string) (Heading, bool) {
	slug := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(anchor), "#"))
	for _, heading := range headings {
		if strings.EqualFold(heading.Slug, slug) {
			return heading, true
		}
	}
	return Heading{}, false
}

// LocateHeadings maps each heading to its 0-based line in rendered output.
// Matching is sequential so a paragraph that repeats heading text before the
// heading itself cannot steal the anchor. Missing headings get -1.
func LocateHeadings(rendered string, headings []Heading) []int {
	lines := strings.Split(ansi.Strip(rendered), "\n")
	positions := make([]int, len(headings))
	searchFrom := 0
	for i, heading := range headings {
		positions[i] = -1
		text := strings.TrimSpace(heading.Text)
		if text == "" {
			continue
		}
		needle := headingNeedle(text)
		for line := searchFrom; line < len(lines); line++ {
			if strings.Contains(lines[line], needle) {
				positions[i] = line
				searchFrom = line + 1
				break
			}
		}
	}
	return positions
}

// headingNeedle keeps a long heading searchable when Glamour wraps it over
// several rendered lines: the beginning of the heading is enough to find the
// block, and sequential search prevents matching an earlier paragraph.
func headingNeedle(text string) string {
	const runes = 16
	count := 0
	for i := range text {
		if count == runes {
			return strings.TrimSpace(text[:i])
		}
		count++
	}
	return text
}

// AnchorLine resolves an in-document anchor to a rendered line.
func AnchorLine(rendered string, headings []Heading, anchor string) (int, bool) {
	heading, ok := HeadingBySlug(headings, anchor)
	if !ok {
		return 0, false
	}
	positions := LocateHeadings(rendered, headings)
	for i, candidate := range headings {
		if candidate.Slug == heading.Slug && positions[i] >= 0 {
			return positions[i], true
		}
	}
	return 0, false
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	var digits [20]byte
	i := len(digits)
	for value > 0 {
		i--
		digits[i] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[i:])
}
