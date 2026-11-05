// Package mdutil extracts structure from markdown and from Glamour's rendered
// output: tables, headings, and table footnote links. It deliberately reuses
// goldmark (Glamour's parser) instead of adding a second markdown parser.
package mdutil

import (
	"bytes"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

// markdownParser includes the GFM extensions mdui documents use. Tables are
// the only extension needed for structural extraction; inline formatting is
// handled by goldmark's core parser.
func markdownParser() goldmark.Markdown {
	return goldmark.New(goldmark.WithExtensions(extension.Table))
}

func parseDocument(src []byte) ast.Node {
	return markdownParser().Parser().Parse(text.NewReader(src))
}

// lineForOffset converts a byte offset in src into a 1-based line number.
func lineForOffset(src []byte, offset int) int {
	if offset < 0 {
		return 0
	}
	if offset > len(src) {
		offset = len(src)
	}
	return bytes.Count(src[:offset], []byte{'\n'}) + 1
}

// nodeText returns the visible text below node, ignoring inline markdown
// punctuation. It covers the node kinds that appear in headings and table
// cells.
func nodeText(src []byte, node ast.Node) string {
	var b bytes.Buffer
	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.Text:
			b.Write(n.Segment.Value(src))
		case *ast.String:
			b.Write(n.Value)
		case *ast.CodeSpan:
			// CodeSpan children are text nodes, so they are handled below.
		}
		return ast.WalkContinue, nil
	})
	return string(b.Bytes())
}
