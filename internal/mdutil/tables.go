package mdutil

import (
	"sort"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
)

// Span is a half-open range of rendered line indexes: Start is included, End
// is excluded. Lines are 0-based, matching strings.Split and viewport offsets.
type Span struct {
	Start int
	End   int
}

// Table describes a table found in markdown source. StartLine/EndLine are
// 1-based source line numbers. Rows counts body rows; the header is separate.
// Malformed is set for table-like delimiter lines that goldmark could not
// parse as a table (for example when the header has a different column count).
type Table struct {
	StartLine   int
	EndLine     int
	StartOffset int
	EndOffset   int
	Columns     int
	Rows        int
	Header      []string
	Alignments  []string
	Malformed   bool
}

// FindTables returns every GFM table in src in document order. It never
// returns a parse error: malformed table syntax is reported through
// Table.Malformed instead. The error return is reserved for future parsers.
func FindTables(src []byte) ([]Table, error) {
	doc := parseDocument(src)
	var tables []Table

	_ = ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		table, ok := node.(*extast.Table)
		if !ok {
			return ast.WalkContinue, nil
		}
		t := Table{Columns: len(table.Alignments)}
		for _, alignment := range table.Alignments {
			t.Alignments = append(t.Alignments, alignment.String())
		}

		start := -1
		end := -1
		for row := table.FirstChild(); row != nil; row = row.NextSibling() {
			if _, ok := row.(*extast.TableHeader); ok {
				for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
					t.Header = append(t.Header, strings.TrimSpace(nodeText(src, cell)))
				}
			}
			if _, ok := row.(*extast.TableRow); ok {
				t.Rows++
			}
			for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
				lines := cell.Lines()
				for i := 0; i < lines.Len(); i++ {
					segment := lines.At(i)
					if start < 0 || segment.Start < start {
						start = segment.Start
					}
					if segment.Stop > end {
						end = segment.Stop
					}
				}
			}
		}
		if start >= 0 {
			t.StartOffset = start
			t.EndOffset = end
			t.StartLine = lineForOffset(src, start)
			t.EndLine = lineForOffset(src, end)
			// The rule line always follows the header. Header-only tables
			// have no body cell to widen the range, so include it here.
			if t.EndLine <= t.StartLine {
				t.EndLine = t.StartLine + 1
			}
		}
		tables = append(tables, t)
		return ast.WalkContinue, nil
	})

	tables = append(tables, malformedTables(src, tables)...)
	sort.SliceStable(tables, func(i, j int) bool {
		if tables[i].StartLine == tables[j].StartLine {
			return tables[i].Malformed && !tables[j].Malformed
		}
		return tables[i].StartLine < tables[j].StartLine
	})
	return tables, nil
}

// malformedTables finds delimiter-looking lines that are not inside a parsed
// table. This keeps malformed tables visible to callers without failing the
// document.
func malformedTables(src []byte, parsed []Table) []Table {
	lines := strings.Split(string(src), "\n")
	var out []Table
	for i, line := range lines {
		if !looksLikeTableDelimiter(line) {
			continue
		}
		lineNo := i + 1
		covered := false
		for _, table := range parsed {
			if table.StartLine <= lineNo && lineNo <= table.EndLine {
				covered = true
				break
			}
		}
		if covered {
			continue
		}
		out = append(out, Table{
			StartLine: lineNo,
			EndLine:   lineNo,
			Columns:   delimiterColumns(line),
			Malformed: true,
		})
	}
	return out
}

func looksLikeTableDelimiter(line string) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.Contains(trimmed, "|") || !strings.Contains(trimmed, "-") {
		return false
	}
	for _, r := range trimmed {
		switch r {
		case '|', '-', ':', ' ', '\t':
		default:
			return false
		}
	}
	return true
}

func delimiterColumns(line string) int {
	columns := 0
	for _, cell := range strings.Split(strings.TrimSpace(line), "|") {
		if strings.TrimSpace(cell) != "" {
			columns++
		}
	}
	return columns
}

// TableSpans locates rendered table blocks in Glamour output. A block starts
// at the header line, includes the rule line Glamour emits below the header,
// and ends at the blank line after the body. Footer link lines are excluded;
// callers that display a table can extend the span with TableFootnotes.
//
// The scanner is intentionally conservative: it only accepts rule lines made
// of Glamour's table drawing glyphs immediately below a non-empty header line.
// If a pathological document defeats it, callers fall back to showing the
// full-width document.
func TableSpans(rendered string) []Span {
	lines := renderedLines(rendered)
	var spans []Span
	for i := 1; i < len(lines); i++ {
		if !isTableRule(lines[i]) {
			continue
		}
		header := i - 1
		if strings.TrimSpace(lines[header]) == "" {
			continue
		}
		end := i + 1
		for end < len(lines) && strings.TrimSpace(lines[end]) != "" {
			end++
		}
		spans = append(spans, Span{Start: header, End: end})
		i = end
	}
	return spans
}

// TableFootnotes extends span to include the footnote block that follows a
// table, if present. Glamour separates the table from its footnotes with one
// blank line.
func TableFootnotes(rendered string, span Span) Span {
	lines := renderedLines(rendered)
	j := span.End
	for j < len(lines) && strings.TrimSpace(lines[j]) == "" {
		j++
	}
	if j >= len(lines) || !IsFootnoteLine(lines[j]) {
		return span
	}
	for j < len(lines) && IsFootnoteLine(lines[j]) {
		j++
	}
	return Span{Start: span.Start, End: j}
}

// CrushedTables reports rendered table blocks that are likely unreadable at
// the current width. The heuristic looks for wrapping: a row continues when
// the first rendered column is blank while later columns still carry text. A
// block is crushed when one such row spans more than maxWrap lines.
//
// A table whose rows all fit on one rendered line never reports blanks in the
// first column, so it is never flagged. That keeps the hint conservative on
// ordinary tables.
func CrushedTables(rendered string, maxWrap int) []Span {
	spans := TableSpans(rendered)
	var crushed []Span
	lines := renderedLines(rendered)
	for _, span := range spans {
		if tableCrushed(lines, span, maxWrap) {
			crushed = append(crushed, span)
		}
	}
	return crushed
}

// CrushedTablesWithRows is CrushedTables with source row counts for each
// rendered span. When the counts line up it also flags a table whose average
// wrapped lines per row exceeds maxWrap; otherwise it uses the text-only
// heuristic.
func CrushedTablesWithRows(rendered string, rows []int, maxWrap int) []Span {
	spans := TableSpans(rendered)
	lines := renderedLines(rendered)
	var crushed []Span
	for i, span := range spans {
		bodyLines := span.End - (span.Start + 2)
		if i < len(rows) && rows[i] > 0 && bodyLines > rows[i]*maxWrap {
			crushed = append(crushed, span)
			continue
		}
		if tableCrushed(lines, span, maxWrap) {
			crushed = append(crushed, span)
		}
	}
	return crushed
}

func tableCrushed(lines []string, span Span, maxWrap int) bool {
	if maxWrap < 1 {
		maxWrap = 1
	}
	bodyStart := span.Start + 2
	if bodyStart >= span.End {
		return false
	}

	started := false
	sawContinuation := false
	previousBlank := false
	current := 0
	longest := 0
	for i := bodyStart; i < span.End; i++ {
		first := firstCell(lines[i])
		blank := strings.TrimSpace(ansi.Strip(first)) == ""
		if blank {
			if started {
				current++
			}
			previousBlank = true
			sawContinuation = true
			continue
		}
		if !started || previousBlank {
			if started && current > longest {
				longest = current
			}
			current = 1
			started = true
		} else {
			current++
		}
		previousBlank = false
	}
	if started && current > longest {
		longest = current
	}
	if !sawContinuation {
		return false
	}
	return longest > maxWrap
}

// firstCell returns the text before the first column separator. Single-column
// tables have no separators, so the whole line is the first cell.
func firstCell(line string) string {
	if i := strings.IndexRune(line, '│'); i >= 0 {
		return line[:i]
	}
	return line
}

func renderedLines(rendered string) []string {
	return strings.Split(ansi.Strip(rendered), "\n")
}

func isTableRule(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	dashes := 0
	for _, r := range trimmed {
		switch r {
		case '─':
			dashes++
		case '┼', '┬', '┴', '├', '┤', '┌', '┐', '└', '┘', ' ':
		default:
			return false
		}
	}
	return dashes >= 3
}
