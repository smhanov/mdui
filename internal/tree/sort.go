package tree

import (
	"sort"
	"strings"
	"unicode"
)

// sortNodes orders directories first, then names naturally and
// case-insensitively. The sort is stable, so names that compare equal (for
// example Foo and foo) keep the order returned by the filesystem.
func sortNodes(nodes []*Node) {
	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].IsDir != nodes[j].IsDir {
			return nodes[i].IsDir
		}
		return compareNatural(nodes[i].Name, nodes[j].Name) < 0
	})
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
