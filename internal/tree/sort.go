package tree

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
)

// sortNodes orders directories first, then names naturally and
// case-insensitively. The sort is stable, so names that compare equal (for
// example Foo and foo) keep the order returned by the filesystem.
func sortNodes(nodes []*Node) {
	sortNodesBy(nodes, "name")
}

// sortNodesBy orders directories first and alphabetically, then sorts files by
// mode: natural name (default), modification time (newest first), or size
// (largest first). Equal keys keep their original order.
func sortNodesBy(nodes []*Node, mode string) {
	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].IsDir != nodes[j].IsDir {
			return nodes[i].IsDir
		}
		if nodes[i].IsDir {
			return compareNatural(nodes[i].Name, nodes[j].Name) < 0
		}

		switch mode {
		case "mtime":
			ti, tj := modTime(nodes[i]), modTime(nodes[j])
			if !ti.Equal(tj) {
				return ti.After(tj)
			}
		case "size":
			si, sj := size(nodes[i]), size(nodes[j])
			if si != sj {
				return si > sj
			}
		}
		return compareNatural(nodes[i].Name, nodes[j].Name) < 0
	})
}

func modTime(node *Node) time.Time {
	info, err := os.Stat(node.Path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

func size(node *Node) int64 {
	info, err := os.Stat(node.Path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// hasMarkdownBeneath reports whether path contains a markdown file anywhere
// below it. Unreadable directories count as containing markdown so their error
// stays visible when the user expands them. Symlinked directories are not
// followed, which keeps the walk cycle-safe.
func hasMarkdownBeneath(path string, showHidden bool) bool {
	entries, err := os.ReadDir(path)
	if err != nil {
		return true
	}
	for _, entry := range entries {
		name := entry.Name()
		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		if entry.IsDir() {
			if hasMarkdownBeneath(filepath.Join(path, name), showHidden) {
				return true
			}
			continue
		}
		if isMarkdown(name) {
			return true
		}
		if entry.Type()&os.ModeSymlink != 0 {
			info, statErr := os.Stat(filepath.Join(path, name))
			if statErr == nil && info.IsDir() {
				continue
			}
		}
	}
	return false
}

// compareNatural compares two names rune by rune, treating runs of ASCII
// digits as numbers. Case is ignored; equal names return zero.
func compareNatural(a, b string) int {
	ar, br := []rune(a), []rune(b)

	i, j := 0, 0
	for i < len(ar) && j < len(br) {
		if isDigit(ar[i]) && isDigit(br[j]) {
			aEnd := i
			for aEnd < len(ar) && isDigit(ar[aEnd]) {
				aEnd++
			}
			bEnd := j
			for bEnd < len(br) && isDigit(br[bEnd]) {
				bEnd++
			}

			aNum := trimLeadingZeros(string(ar[i:aEnd]))
			bNum := trimLeadingZeros(string(br[j:bEnd]))
			if len(aNum) != len(bNum) {
				return sign(len(aNum) - len(bNum))
			}
			if aNum != bNum {
				return strings.Compare(aNum, bNum)
			}

			i, j = aEnd, bEnd
			continue
		}

		aRune, bRune := unicode.ToLower(ar[i]), unicode.ToLower(br[j])
		if aRune != bRune {
			if aRune < bRune {
				return -1
			}
			return 1
		}
		i++
		j++
	}

	switch {
	case i < len(ar):
		return 1
	case j < len(br):
		return -1
	default:
		return 0
	}
}

func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

func trimLeadingZeros(s string) string {
	s = strings.TrimLeft(s, "0")
	if s == "" {
		return "0"
	}
	return s
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	default:
		return 0
	}
}
