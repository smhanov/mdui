// Package finder indexes markdown files under a root and ranks them against a
// fuzzy query. It has no terminal or UI dependencies.
package finder

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// File is one indexed markdown file. Path is usable for opening; Rel is the
// display and scoring form relative to the root.
type File struct {
	Path string
	Rel  string
}

// Index is an immutable snapshot of the markdown files under Root.
type Index struct {
	Root   string
	Hidden bool
	Files  []File
}

// Builder reads a root and returns its file index. Tests inject a builder to
// count filesystem walks; the app defaults to Build.
type Builder func(root string, showHidden bool) (Index, error)

// BuildError reports a partial walk. The index returned alongside it still
// contains every file that could be read.
type BuildError struct {
	Count int
	First error
}

// Error implements error.
func (e *BuildError) Error() string {
	if e == nil {
		return ""
	}
	if e.Count == 1 {
		return fmt.Sprintf("1 path could not be read (partial index): %v", e.First)
	}
	return fmt.Sprintf("%d paths could not be read (partial index); first: %v", e.Count, e.First)
}

// Unwrap returns the first underlying path error.
func (e *BuildError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.First
}

// Build walks root and collects markdown files. It follows symlinked
// directories but skips a directory whose identity matches one of its
// ancestors, so symlink cycles terminate. Read failures do not abort the walk:
// the partial index is returned together with a BuildError.
func Build(root string, showHidden bool) (Index, error) {
	root = filepath.Clean(root)
	index := Index{Root: root, Hidden: showHidden}
	var problems []error

	var walk func(dir string, ancestors []os.FileInfo)
	walk = func(dir string, ancestors []os.FileInfo) {
		info, err := os.Stat(dir)
		if err != nil {
			problems = append(problems, err)
			return
		}
		for _, ancestor := range ancestors {
			if os.SameFile(info, ancestor) {
				return
			}
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			problems = append(problems, err)
			return
		}

		stack := append(ancestors, info)
		for _, entry := range entries {
			name := entry.Name()
			if !showHidden && strings.HasPrefix(name, ".") {
				continue
			}
			path := filepath.Join(dir, name)
			isDir := entry.IsDir()
			if entry.Type()&os.ModeSymlink != 0 {
				target, statErr := os.Stat(path)
				if statErr != nil {
					// A broken link cannot be opened; the tree reports it
					// inline, but it has no place in a jump-to index.
					continue
				}
				isDir = target.IsDir()
			}
			if isDir {
				walk(path, stack)
				continue
			}
			if !isMarkdown(name) {
				continue
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				rel = path
			}
			index.Files = append(index.Files, File{Path: path, Rel: rel})
		}
	}
	walk(root, nil)

	sort.Slice(index.Files, func(i, j int) bool {
		a, b := index.Files[i], index.Files[j]
		al, bl := strings.ToLower(a.Rel), strings.ToLower(b.Rel)
		if al != bl {
			return al < bl
		}
		return a.Rel < b.Rel
	})

	if len(problems) > 0 {
		return index, &BuildError{Count: len(problems), First: problems[0]}
	}
	return index, nil
}

func isMarkdown(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".markdown":
		return true
	default:
		return false
	}
}

// Result is one filtered file with its score and matched rune positions.
type Result struct {
	File
	Score     int
	Positions []int
}

// Filter returns the files matching query. An empty query returns every file in
// index order. Non-empty queries are ranked by Score with deterministic
// tie-breaking: shorter relative path, lower-cased relative path, then exact
// relative path.
func Filter(index Index, query string) []Result {
	if query == "" {
		results := make([]Result, len(index.Files))
		for i, file := range index.Files {
			results[i] = Result{File: file}
		}
		return results
	}

	results := make([]Result, 0, min(len(index.Files), 64))
	for _, file := range index.Files {
		score, positions, ok := Score(file.Rel, query)
		if !ok {
			continue
		}
		results = append(results, Result{File: file, Score: score, Positions: positions})
	}

	sort.Slice(results, func(i, j int) bool {
		a, b := results[i], results[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		aLen, bLen := utf8.RuneCountInString(a.Rel), utf8.RuneCountInString(b.Rel)
		if aLen != bLen {
			return aLen < bLen
		}
		aLower, bLower := strings.ToLower(a.Rel), strings.ToLower(b.Rel)
		if aLower != bLower {
			return aLower < bLower
		}
		return a.Rel < b.Rel
	})
	return results
}

const (
	impossible       = math.MinInt / 4
	consecutiveBonus = 16
)

// Score aligns query against candidate as a case-insensitive subsequence and
// returns the best score plus the matched rune indexes. It is path-aware:
// basename and segment starts, consecutive runs and exact-case runes earn
// bonuses. False means candidate does not contain the query.
func Score(candidate, query string) (int, []int, bool) {
	c := []rune(candidate)
	q := []rune(query)
	if len(q) == 0 {
		return 0, nil, true
	}
	if len(q) > len(c) || !isSubsequence(c, q) {
		return 0, nil, false
	}

	baseStart := 0
	for i, r := range c {
		if r == '/' || r == '\\' {
			baseStart = i + 1
		}
	}

	n, m := len(c), len(q)
	prev := make([]int, n)
	for i := range prev {
		prev[i] = impossible
		if runeEqual(c[i], q[0]) {
			prev[i] = runeScore(c, i, baseStart, q[0])
		}
	}

	choices := make([][]int32, m)
	for j := 1; j < m; j++ {
		curr := make([]int, n)
		choice := make([]int32, n)
		for i := range curr {
			curr[i] = impossible
			choice[i] = -1
		}

		// bestK/bestVal hold max(prev[k]+k) over k <= i-2, which turns a
		// gapped transition into prev[k] - (i-k-1) = bestVal - i + 1.
		bestK := int32(-1)
		bestVal := impossible
		for i := 0; i < n; i++ {
			if i >= 2 {
				k := i - 2
				if prev[k] != impossible && prev[k]+k > bestVal {
					bestVal = prev[k] + k
					bestK = int32(k)
				}
			}
			if !runeEqual(c[i], q[j]) {
				continue
			}

			step := runeScore(c, i, baseStart, q[j])
			if i > 0 && prev[i-1] != impossible {
				value := prev[i-1] + step + consecutiveBonus
				if value > curr[i] {
					curr[i] = value
					choice[i] = int32(i - 1)
				}
			}
			if bestK >= 0 {
				value := bestVal - i + 1 + step
				if value > curr[i] {
					curr[i] = value
					choice[i] = bestK
				}
			}
		}
		choices[j] = choice
		prev = curr
	}

	best := impossible
	bestEnd := -1
	for i, value := range prev {
		if value > best {
			best = value
			bestEnd = i
		}
	}
	if bestEnd < 0 {
		return 0, nil, false
	}

	positions := make([]int, m)
	positions[m-1] = bestEnd
	for j := m - 1; j >= 1; j-- {
		parent := choices[j][positions[j]]
		if parent < 0 {
			return 0, nil, false
		}
		positions[j-1] = int(parent)
	}

	return best + suffixBonus(c, q, baseStart, positions), positions, true
}

func runeScore(c []rune, i, baseStart int, queryRune rune) int {
	score := 1
	switch {
	case i == 0:
		score += 14
	case c[i-1] == '/' || c[i-1] == '\\':
		score += 14
	case c[i-1] == '_' || c[i-1] == '-' || c[i-1] == '.' || c[i-1] == ' ':
		score += 6
	}
	if i >= baseStart {
		score += 6
		if i == baseStart {
			score += 14
		}
	}
	if c[i] == queryRune {
		score += 2
	}
	return score
}

func suffixBonus(c, q []rune, baseStart int, positions []int) int {
	base := string(c[baseStart:])
	query := string(q)
	baseLower := strings.ToLower(base)
	queryLower := strings.ToLower(query)

	bonus := 0
	switch {
	case baseLower == queryLower:
		bonus += 80
	case strings.HasPrefix(baseLower, queryLower):
		bonus += 50
	case strings.Contains(baseLower, queryLower):
		bonus += 25
	}
	if len(positions) == 0 {
		return bonus
	}
	allInBase := true
	for _, position := range positions {
		if position < baseStart {
			allInBase = false
			break
		}
	}
	if allInBase {
		bonus += 10
	}
	if positions[0] == 0 {
		bonus += 4
	}
	if positions[len(positions)-1] == len(c)-1 {
		bonus += 4
	}
	return bonus
}

func isSubsequence(c, q []rune) bool {
	j := 0
	for i := 0; i < len(c) && j < len(q); i++ {
		if runeEqual(c[i], q[j]) {
			j++
		}
	}
	return j == len(q)
}

func runeEqual(a, b rune) bool {
	return unicode.ToLower(a) == unicode.ToLower(b)
}
