package mdutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mdui/internal/render"
)

const wideTableSource = `# Wide table

Intro.

| Service | Owner | Status | Latency | Cost | Notes |
|:--|:--|:--|--:|--:|:--|
| gateway | platform team | healthy | 23 ms | $120 | routes all public traffic through the edge |
| billing | finance squad | degraded | 450 ms | $80 | retrying invoices after the outage |
| search | discovery | healthy | 18 ms | $300 | vector index warmup still running |
| identity | security | healthy | 9 ms | $45 | keys rotated weekly |

After.
`

func renderFixture(t *testing.T, src string, width int) string {
	t.Helper()
	rendered, err := render.RenderBytes([]byte(src), width, render.Options{Style: render.MduiDark})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return rendered
}

func TestFindTables(t *testing.T) {
	src := []byte(`# Title

| Name | Status | Notes |
|:--|:--:|--:|
| Backend | shipped | [report](https://example.com/q3) |
| iOS | in review | test plan |
| Web | launched | fast |
`)
	tables, err := FindTables(src)
	if err != nil {
		t.Fatalf("FindTables: %v", err)
	}
	if len(tables) != 1 {
		t.Fatalf("tables = %d, want 1", len(tables))
	}
	table := tables[0]
	if table.Malformed {
		t.Fatal("valid table marked malformed")
	}
	if table.Columns != 3 || table.Rows != 3 {
		t.Fatalf("shape = %d cols x %d rows, want 3x3", table.Columns, table.Rows)
	}
	if got := strings.Join(table.Header, "|"); got != "Name|Status|Notes" {
		t.Fatalf("header = %q", got)
	}
	if got := strings.Join(table.Alignments, ","); got != "left,center,right" {
		t.Fatalf("alignments = %q", got)
	}
	if table.StartLine != 3 || table.EndLine != 7 {
		t.Fatalf("source range = %d..%d, want 3..7", table.StartLine, table.EndLine)
	}
	if table.StartOffset >= table.EndOffset {
		t.Fatalf("byte range = %d..%d, want increasing", table.StartOffset, table.EndOffset)
	}
}

func TestFindTablesMalformedDoesNotPanic(t *testing.T) {
	src := []byte(`| A | B |
| --- |
| one | two |
`)
	tables, err := FindTables(src)
	if err != nil {
		t.Fatalf("FindTables: %v", err)
	}
	if len(tables) != 1 {
		t.Fatalf("malformed tables = %d, want 1: %+v", len(tables), tables)
	}
	if !tables[0].Malformed {
		t.Fatalf("table should be malformed: %+v", tables[0])
	}
	if tables[0].Columns != 1 {
		t.Fatalf("malformed columns = %d, want 1", tables[0].Columns)
	}
}

func TestFindTablesUnicode(t *testing.T) {
	src := []byte("| 名前 | 状態 | 😀 |\n|:--|:--|:--|\n| 東京 | 準備中 | 🚀 |\n")
	tables, err := FindTables(src)
	if err != nil {
		t.Fatalf("FindTables: %v", err)
	}
	if len(tables) != 1 || tables[0].Columns != 3 || tables[0].Rows != 1 {
		t.Fatalf("unexpected table shape: %+v", tables)
	}
	if tables[0].Header[0] != "名前" || tables[0].Header[2] != "😀" {
		t.Fatalf("header = %#v", tables[0].Header)
	}
}

func TestFindTablesMultipleAndEmptyBody(t *testing.T) {
	src := []byte("| A | B |\n|---|---|\n\n| C | D |\n|---|---|\n| 1 | 2 |\n")
	tables, err := FindTables(src)
	if err != nil {
		t.Fatalf("FindTables: %v", err)
	}
	if len(tables) != 2 {
		t.Fatalf("tables = %d, want 2", len(tables))
	}
	if tables[0].Rows != 0 {
		t.Fatalf("empty-body rows = %d, want 0", tables[0].Rows)
	}
	if tables[1].Rows != 1 {
		t.Fatalf("second rows = %d, want 1", tables[1].Rows)
	}
}

func TestTableSpansAndCrushedHeuristic(t *testing.T) {
	fixture := filepath.Join("..", "render", "testdata", "table.md")
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	normal := renderFixture(t, string(data), 60)
	spans := TableSpans(normal)
	if len(spans) != 1 {
		t.Fatalf("normal table spans = %d, want 1\n%s", len(spans), normal)
	}
	if spans[0].Start >= spans[0].End {
		t.Fatalf("invalid span: %+v", spans[0])
	}
	if crushed := CrushedTables(normal, 3); len(crushed) != 0 {
		t.Fatalf("normal table flagged as crushed: %+v", crushed)
	}

	narrow := renderFixture(t, wideTableSource, 30)
	narrowSpans := TableSpans(narrow)
	if len(narrowSpans) != 1 {
		t.Fatalf("narrow table spans = %d, want 1\n%s", len(narrowSpans), narrow)
	}
	if crushed := CrushedTables(narrow, 3); len(crushed) != 1 {
		t.Fatalf("narrow table not flagged as crushed: %+v\n%s", crushed, narrow)
	}

	wide := renderFixture(t, wideTableSource, 100)
	if crushed := CrushedTables(wide, 3); len(crushed) != 0 {
		t.Fatalf("wide render flagged as crushed: %+v", crushed)
	}

	// Source row counts also mark a table that averages more than maxWrap
	// lines per body row.
	if crushed := CrushedTablesWithRows(narrow, []int{4}, 3); len(crushed) != 1 {
		t.Fatal("row-count heuristic did not flag the narrow table")
	}
	if crushed := CrushedTablesWithRows(wide, []int{4}, 3); len(crushed) != 0 {
		t.Fatal("row-count heuristic flagged the wide table")
	}
}

func TestTableSpansFooterAndMalformedRendered(t *testing.T) {
	rendered := renderFixture(t, "| A | B |\n|---|---|\n| [x](https://example.com) | y |\n\nAfter.\n", 80)
	spans := TableSpans(rendered)
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1\n%s", len(spans), rendered)
	}
	extended := TableFootnotes(rendered, spans[0])
	if extended.End <= spans[0].End {
		t.Fatal("footnote block was not included in the display span")
	}
	lines := strings.Split(rendered, "\n")
	if !IsFootnoteLine(lines[spans[0].End+1]) {
		t.Fatalf("expected a footnote after the table, got %q", lines[spans[0].End+1])
	}

	malformed := "| A | B |\n| --- |\n| one |\n"
	got := renderFixture(t, malformed, 80)
	_ = TableSpans(got) // must not panic
	if crushed := CrushedTables(got, 3); len(crushed) != 0 {
		t.Fatalf("malformed render flagged as table: %+v", crushed)
	}
}
