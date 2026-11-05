package app

import "math"

const (
	// TreeMinW is the narrowest the file tree pane may become.
	TreeMinW = 20
	// ViewerMinW is the narrowest the markdown viewer may become.
	ViewerMinW = 30
	// DividerW is the width of the column between the panes.
	DividerW = 1
	// DefaultRatio is the tree's share of the content width on first launch.
	DefaultRatio = 0.3
)

// ClampRatio constrains ratio so that both panes can keep their minimum
// widths. totalW is the width available to the panes, divider included.
//
// When totalW is too small to satisfy both minimums the pane widths cannot be
// expressed as a ratio interval; in that case the requested ratio is returned
// clamped only to [0, 1] and SplitWidths divides the available columns
// proportionally. NaN is treated as DefaultRatio.
func ClampRatio(totalW int, ratio float64) float64 {
	ratio = clamp01(ratio)
	if totalW <= 0 {
		return ratio
	}

	lo := float64(TreeMinW) / float64(totalW)
	hi := 1 - float64(ViewerMinW+DividerW)/float64(totalW)
	if lo > hi {
		return ratio
	}
	return math.Min(hi, math.Max(lo, ratio))
}

// SplitWidths divides totalW into tree, divider, and viewer columns according
// to ratio, honouring the minimum pane widths from the layout spec.
//
// The returned widths always sum to totalW and are never negative. On windows
// too narrow for both minimums (totalW < TreeMinW+ViewerMinW+DividerW) the
// columns are split proportionally instead, so callers can still render a
// coherent frame while showing a "terminal too small" notice.
func SplitWidths(totalW int, ratio float64) (treeW, divW, viewerW int) {
	if totalW <= 0 {
		return 0, 0, 0
	}
	if totalW <= DividerW {
		return 0, totalW, 0
	}

	avail := totalW - DividerW
	minTree := TreeMinW
	maxTree := avail - ViewerMinW
	if maxTree < minTree {
		treeW = int(math.Round(float64(totalW) * clamp01(ratio)))
		treeW = clampInt(treeW, 0, avail)
		return treeW, DividerW, avail - treeW
	}

	treeW = int(math.Round(float64(totalW) * ClampRatio(totalW, ratio)))
	treeW = clampInt(treeW, minTree, maxTree)
	return treeW, DividerW, avail - treeW
}

func clamp01(v float64) float64 {
	if math.IsNaN(v) {
		return DefaultRatio
	}
	return math.Min(1, math.Max(0, v))
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
