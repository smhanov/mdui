package finder

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestScoreMatchesSubsequence(t *testing.T) {
	if _, _, ok := Score("docs/readme.md", "rdm"); !ok {
		t.Fatal("rdm should match docs/readme.md")
	}
	if _, _, ok := Score("docs/readme.md", "xyz"); ok {
		t.Fatal("xyz should not match docs/readme.md")
	}
	if _, _, ok := Score("docs/readme.md", "readmefile"); ok {
		t.Fatal("a query longer than the candidate cannot match")
	}
	if _, positions, ok := Score("docs/readme.md", "readme"); !ok {
		t.Fatal("readme should match")
	} else {
		want := []int{5, 6, 7, 8, 9, 10}
		if !equalInts(positions, want) {
			t.Fatalf("positions = %v, want %v", positions, want)
		}
	}
}

func TestScoreBasenameBeatsParentAndPrefixBeatsSubstring(t *testing.T) {
	basename, _, _ := Score("notes/project.md", "project")
	parent, _, _ := Score("project/notes.md", "project")
	if basename <= parent {
		t.Fatalf("basename score %d should beat parent-directory score %d", basename, parent)
	}

	prefix, _, _ := Score("docs/project.md", "proj")
	substring, _, _ := Score("docs/myproject.md", "proj")
	if prefix <= substring {
		t.Fatalf("basename-prefix score %d should beat basename-substring score %d", prefix, substring)
	}

	segment, _, _ := Score("project/docs/notes.md", "proj")
	buried, _, _ := Score("myproject/docs/notes.md", "proj")
	if segment <= buried {
		t.Fatalf("segment-start score %d should beat buried score %d", segment, buried)
	}
}

func TestScoreConsecutiveRunsBeatGaps(t *testing.T) {
	tight, _, _ := Score("docs/abc.md", "abc")
	gaps, _, _ := Score("a/b/c.md", "abc")
	if tight <= gaps {
		t.Fatalf("consecutive score %d should beat gapped score %d", tight, gaps)
	}
}

func TestScoreRepeatedRunesPositionsIncrease(t *testing.T) {
	_, positions, ok := Score("docs/banana.md", "ana")
	if !ok {
		t.Fatal("ana should match docs/banana.md")
	}
	if len(positions) != 3 {
		t.Fatalf("positions = %v, want three", positions)
	}
	for i := 1; i < len(positions); i++ {
		if positions[i] <= positions[i-1] {
			t.Fatalf("positions are not increasing: %v", positions)
		}
	}
}

func TestScoreIsCaseInsensitiveAndRewardsExactCase(t *testing.T) {
	lower, lowerPositions, ok := Score("docs/Readme.md", "readme")
	if !ok || len(lowerPositions) != 6 {
		t.Fatalf("lowercase query did not match mixed-case path: ok=%v positions=%v", ok, lowerPositions)
	}
	exact, _, _ := Score("docs/Readme.md", "Readme")
	if exact <= lower {
		t.Fatalf("exact-case score %d should beat lower-case score %d", exact, lower)
	}
}

func TestFilterDeterministicTies(t *testing.T) {
	index := Index{Files: []File{
		{Path: "/r/b/readme.md", Rel: "b/readme.md"},
		{Path: "/r/a/readme.md", Rel: "a/readme.md"},
		{Path: "/r/c/other.md", Rel: "c/other.md"},
	}}
	results := Filter(index, "readme")
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	if results[0].Rel != "a/readme.md" || results[1].Rel != "b/readme.md" {
		t.Fatalf("tie order = %q, %q, want a/readme.md then b/readme.md", results[0].Rel, results[1].Rel)
	}
}

func TestFilterEmptyQueryAndNoMatches(t *testing.T) {
	index := Index{Files: []File{
		{Path: "/r/b.md", Rel: "b.md"},
		{Path: "/r/a.md", Rel: "a.md"},
	}}
	all := Filter(index, "")
	if len(all) != 2 || all[0].Rel != "b.md" || all[1].Rel != "a.md" {
		t.Fatalf("empty query should return index order, got %+v", all)
	}
	if got := Filter(index, "zzz"); len(got) != 0 {
		t.Fatalf("no-match filter returned %d results", len(got))
	}
}

func TestIndexStaysUsableAfterRootDisappears(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "note.md"), []byte("# note\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	index, err := Build(root, false)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	results := Filter(index, "note")
	if len(results) != 1 || results[0].Rel != "note.md" {
		t.Fatalf("filter touched the filesystem: %+v", results)
	}
}

func TestBuildFiltersHiddenExtensionsAndNestedDirs(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "projects", "deep"))
	mustWrite(t, filepath.Join(root, "top.md"), "# top\n")
	mustWrite(t, filepath.Join(root, "notes.markdown"), "# notes\n")
	mustWrite(t, filepath.Join(root, "projects", "deep", "nested.md"), "# nested\n")
	mustWrite(t, filepath.Join(root, "image.png"), "not markdown")
	mustWrite(t, filepath.Join(root, ".hidden.md"), "# hidden\n")
	mustMkdir(t, filepath.Join(root, ".secret"))
	mustWrite(t, filepath.Join(root, ".secret", "inside.md"), "# inside\n")

	visible, err := Build(root, false)
	if err != nil {
		t.Fatalf("Build(showHidden=false): %v", err)
	}
	assertRels(t, visible.Files, []string{"notes.markdown", filepath.Join("projects", "deep", "nested.md"), "top.md"})

	hidden, err := Build(root, true)
	if err != nil {
		t.Fatalf("Build(showHidden=true): %v", err)
	}
	assertRels(t, hidden.Files, []string{".hidden.md", filepath.Join(".secret", "inside.md"), "notes.markdown", filepath.Join("projects", "deep", "nested.md"), "top.md"})
	if !hidden.Hidden {
		t.Fatal("index did not record the hidden setting")
	}
}

func TestBuildFollowsSymlinksAndStopsCycles(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "real"))
	mustWrite(t, filepath.Join(root, "real", "note.md"), "# note\n")
	if err := os.Symlink(root, filepath.Join(root, "real", "loop")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(filepath.Join(root, "real", "note.md"), filepath.Join(root, "alias.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	index, err := Build(root, false)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	assertRels(t, index.Files, []string{"alias.md", filepath.Join("real", "note.md")})
}

func TestBuildPartialErrorKeepsFiles(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission errors do not apply to root")
	}
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "good.md"), "# good\n")
	blocked := filepath.Join(root, "blocked")
	mustMkdir(t, blocked)
	mustWrite(t, filepath.Join(blocked, "inside.md"), "# inside\n")
	if err := os.Chmod(blocked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o755) })

	index, err := Build(root, false)
	if err == nil {
		t.Fatal("Build returned no error for an unreadable directory")
	}
	var buildErr *BuildError
	if !errors.As(err, &buildErr) {
		t.Fatalf("error = %T, want *BuildError", err)
	}
	if buildErr.Count == 0 {
		t.Fatal("BuildError.Count = 0")
	}
	if got := rels(index.Files); len(got) != 1 || got[0] != "good.md" {
		t.Fatalf("partial index = %q, want good.md", got)
	}
}

func TestBuildAndFilterThousands(t *testing.T) {
	root := t.TempDir()
	const count = 2000
	for i := 0; i < count; i++ {
		dir := filepath.Join(root, "group", pad(i/100))
		if i%100 == 0 {
			mustMkdir(t, dir)
		}
		mustWrite(t, filepath.Join(dir, "note-"+pad(i)+".md"), "# note\n")
	}

	index, err := Build(root, false)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(index.Files) != count {
		t.Fatalf("indexed %d files, want %d", len(index.Files), count)
	}
	results := Filter(index, "note-1999")
	if len(results) != 1 || filepath.Base(results[0].Rel) != "note-1999.md" {
		t.Fatalf("filter results = %+v, want note-1999.md", results)
	}
}

func BenchmarkFilterTwoThousand(b *testing.B) {
	files := make([]File, 2000)
	for i := range files {
		files[i] = File{
			Path: "/root/group/note-" + pad(i) + ".md",
			Rel:  "group/note-" + pad(i) + ".md",
		}
	}
	index := Index{Root: "/root", Files: files}
	b.ResetTimer()
	for range b.N {
		Filter(index, "grpnt1")
	}
}

func rels(files []File) []string {
	out := make([]string, len(files))
	for i, file := range files {
		out[i] = file.Rel
	}
	sort.Strings(out)
	return out
}

func assertRels(t *testing.T, files []File, want []string) {
	t.Helper()
	got := rels(files)
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("files = %q, want %q", got, want)
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func pad(n int) string {
	return fmt.Sprintf("%04d", n)
}
