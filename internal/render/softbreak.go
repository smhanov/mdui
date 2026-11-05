package render

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// softBreakParser mirrors the goldmark configuration Glamour uses (GFM,
// definition lists, auto heading IDs) so both passes see the same document
// shape. See glamour v2 NewTermRenderer.
var softBreakParser = goldmark.New(
	goldmark.WithExtensions(extension.GFM, extension.DefinitionList),
	goldmark.WithParserOptions(parser.WithAutoHeadingID()),
)

// joinSoftBreaks replaces the newline ending every soft line break with a
// single space so the renderer reflows hard-wrapped paragraphs and list items
// to the wrap width.
//
// Goldmark sets Text.SoftLineBreak only where the same inline text flow
// continues on the next line, and never together with a hard line break, so
// the rewrite joins exactly the newlines a reflow should join. Newlines that
// end a block (code fences, table rows, headings), that a following block
// interrupts (a nested list, a heading), or that the author made hard (two
// trailing spaces or a backslash) carry no soft-break flag and are left in
// place. Insignificant bytes around the newline — trailing spaces on the
// breaking line, the continuation line's indentation, and blockquote ">"
// markers — collapse into the single joining space so the reflowed text reads
// with one space between words. The parse is a pure function of the input, so
// the render cache keyed by source path, mtime, width, style, and emoji stays
// valid.
func joinSoftBreaks(source []byte) []byte {
	if len(source) == 0 {
		return source
	}

	doc := softBreakParser.Parser().Parse(text.NewReader(source))

	type gap struct{ start, end int }
	var cuts []gap
	err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		t, ok := n.(*ast.Text)
		if !ok || !t.SoftLineBreak() {
			return ast.WalkContinue, nil
		}
		// The gap between this segment and the next text line is the line
		// ending, possibly preceded by insignificant trailing spaces and
		// followed by indentation or blockquote markers. Anything else means
		// the source shape surprises us; leave the break alone.
		stop := t.Segment.Stop
		start := stop
		for start > 0 && (source[start-1] == ' ' || source[start-1] == '\t') {
			start--
		}
		end := stop
		if end < len(source) && source[end] == '\r' {
			end++
		}
		if end >= len(source) || source[end] != '\n' {
			return ast.WalkContinue, nil
		}
		end++
		for end < len(source) && (source[end] == ' ' || source[end] == '\t' || source[end] == '>') {
			end++
		}
		cuts = append(cuts, gap{start: start, end: end})
		return ast.WalkContinue, nil
	})
	if err != nil || len(cuts) == 0 {
		return source
	}

	out := make([]byte, 0, len(source))
	prev := 0
	for _, cut := range cuts {
		if cut.start < prev {
			continue
		}
		out = append(out, source[prev:cut.start]...)
		out = append(out, ' ')
		prev = cut.end
	}
	out = append(out, source[prev:]...)
	return out
}
