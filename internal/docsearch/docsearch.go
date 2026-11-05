// Package docsearch provides ANSI-aware literal search over rendered terminal
// text. Rendered markdown may contain SGR attribute runs and OSC 8 hyperlinks,
// so matches are found in a plain-text projection and mapped back to byte
// ranges in the styled line before reverse-video is injected.
package docsearch

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

const (
	reverseOn  = "\x1b[7m"
	reverseOff = "\x1b[27m"
)

// Segment maps a contiguous run of visible bytes in a line's plain projection
// to the same bytes in the styled line. Offsets are byte offsets and ranges
// are half-open.
type Segment struct {
	PlainStart  int
	PlainEnd    int
	StyledStart int
	StyledEnd   int
}

// Line is one rendered line split into visible text and a byte-range map.
type Line struct {
	Styled   string
	Plain    string
	Segments []Segment
}

// Project converts one ANSI-styled line into its plain visible text and the
// segments that map it back to the styled bytes. Escape, control and OSC
// sequences contribute no plain text. Adjacent printable sequences with no
// escape between them are merged into one segment.
func Project(styled string) Line {
	line := Line{Styled: styled}
	var plain strings.Builder
	var state byte

	for i := 0; i < len(styled); {
		seq, _, n, newState := ansi.DecodeSequence(styled[i:], state, nil)
		state = newState
		if n <= 0 {
			break
		}
		if isPrintable(seq) {
			start := plain.Len()
			plain.WriteString(seq)
			end := plain.Len()

			if len(line.Segments) > 0 {
				last := &line.Segments[len(line.Segments)-1]
				if last.StyledEnd == i && last.PlainEnd == start {
					last.PlainEnd = end
					last.StyledEnd = i + n
					i += n
					continue
				}
			}
			line.Segments = append(line.Segments, Segment{
				PlainStart:  start,
				PlainEnd:    end,
				StyledStart: i,
				StyledEnd:   i + n,
			})
		}
		i += n
	}

	line.Plain = plain.String()
	return line
}

// isPrintable reports whether seq is visible text rather than a control or
// escape sequence. DecodeSequence already groups escape sequences; checking
// the first rune filters the remaining C0/C1 controls it returns byte-wise.
func isPrintable(seq string) bool {
	if seq == "" {
		return false
	}
	switch seq[0] {
	case 0x1b, 0x07, 0x08, 0x09, 0x0a, 0x0d, 0x7f,
		0x90, 0x98, 0x9b, 0x9d, 0x9e, 0x9f:
		return false
	}
	r, _ := utf8.DecodeRuneInString(seq)
	return !unicode.IsControl(r)
}

// Match identifies one literal occurrence of a query in a projected document.
// Line is a 0-based rendered line; Start and End are byte offsets in that
// line's Plain text.
type Match struct {
	Line  int
	Start int
	End   int
}

// Index is a projected document plus every match for one query.
type Index struct {
	Content       string
	Query         string
	CaseSensitive bool
	Lines         []Line
	Matches       []Match
}

// New projects content and finds literal matches for query. An empty query or
// empty content produces an empty index. Matching is smart-case: a query made
// entirely of lowercase characters (and non-letters) ignores case, while a
// query containing an uppercase letter is case-sensitive.
func New(content, query string) *Index {
	index := &Index{
		Content:       content,
		Query:         query,
		CaseSensitive: HasUpper(query),
	}
	if content == "" || query == "" {
		return index
	}

	lines := strings.Split(content, "\n")
	index.Lines = make([]Line, len(lines))
	for number, line := range lines {
		index.Lines[number] = Project(line)
		for _, span := range findInLine(index.Lines[number].Plain, query, index.CaseSensitive) {
			index.Matches = append(index.Matches, Match{
				Line:  number,
				Start: span[0],
				End:   span[1],
			})
		}
	}
	return index
}

// HasUpper reports whether s contains an uppercase letter. It drives the
// smart-case policy for a query.
func HasUpper(s string) bool {
	for _, r := range s {
		if unicode.IsUpper(r) {
			return true
		}
	}
	return false
}

// Highlight returns a copy of the indexed content with reverse video around
// every match. Escape inserts are zero-width, so the plain text and cell width
// of each line are unchanged.
func (i *Index) Highlight() string {
	if i == nil || i.Content == "" || len(i.Matches) == 0 || len(i.Lines) == 0 {
		if i == nil {
			return ""
		}
		return i.Content
	}

	byLine := make(map[int][]Match, len(i.Lines))
	for _, match := range i.Matches {
		byLine[match.Line] = append(byLine[match.Line], match)
	}

	lines := make([]string, len(i.Lines))
	for number, line := range i.Lines {
		matches := byLine[number]
		if len(matches) == 0 {
			lines[number] = line.Styled
			continue
		}
		lines[number] = highlightLine(line, matches)
	}
	return strings.Join(lines, "\n")
}

type styledSpan struct {
	start int
	end   int
}

// highlightLine injects reverse video into one styled line. Each matched
// segment is wrapped separately because an SGR reset between segments would
// otherwise cancel reverse video for the remainder of a match.
func highlightLine(line Line, matches []Match) string {
	spans := make([]styledSpan, 0, len(matches))
	for _, match := range matches {
		for _, segment := range line.Segments {
			if match.End <= segment.PlainStart || match.Start >= segment.PlainEnd {
				continue
			}
			start := max(match.Start, segment.PlainStart)
			end := min(match.End, segment.PlainEnd)
			spans = append(spans, styledSpan{
				start: segment.StyledStart + (start - segment.PlainStart),
				end:   segment.StyledStart + (end - segment.PlainStart),
			})
		}
	}
	if len(spans) == 0 {
		return line.Styled
	}
	sort.Slice(spans, func(a, b int) bool { return spans[a].start < spans[b].start })

	var b strings.Builder
	last := 0
	for _, span := range spans {
		if span.start < last {
			// Defensive: overlapping matches should not duplicate bytes.
			if span.end > last {
				last = span.end
			}
			continue
		}
		b.WriteString(line.Styled[last:span.start])
		b.WriteString(reverseOn)
		b.WriteString(line.Styled[span.start:span.end])
		b.WriteString(reverseOff)
		last = span.end
	}
	b.WriteString(line.Styled[last:])
	return b.String()
}

// findInLine returns non-overlapping byte ranges in plain where query occurs.
// Comparison is rune-by-rune so case folding and UTF-8 boundaries stay valid.
func findInLine(plain, query string, caseSensitive bool) [][2]int {
	if plain == "" || query == "" {
		return nil
	}
	haystack := []rune(plain)
	needle := []rune(query)
	if len(needle) == 0 || len(needle) > len(haystack) {
		return nil
	}

	offsets := make([]int, len(haystack)+1)
	offset := 0
	for i, r := range haystack {
		offsets[i] = offset
		offset += utf8.RuneLen(r)
	}
	offsets[len(haystack)] = len(plain)

	var matches [][2]int
	for i := 0; i+len(needle) <= len(haystack); {
		if runesMatch(haystack[i:i+len(needle)], needle, caseSensitive) {
			matches = append(matches, [2]int{offsets[i], offsets[i+len(needle)]})
			i += len(needle)
			continue
		}
		i++
	}
	return matches
}

func runesMatch(haystack, needle []rune, caseSensitive bool) bool {
	for i, want := range needle {
		got := haystack[i]
		if got == want {
			continue
		}
		if caseSensitive || unicode.ToLower(got) != unicode.ToLower(want) {
			return false
		}
	}
	return true
}
