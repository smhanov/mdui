// Package scrollbar provides terminal-independent scrollbar geometry.
package scrollbar

import "math"

// State describes a vertical scrolling region.
type State struct {
	Total, Visible, Offset, Height int
}

// Shown reports whether the content needs a scrollbar.
func (s State) Shown() bool {
	return s.Total > s.Visible && s.Visible > 0 && s.Height > 1
}

// Thumb returns the thumb's start row and size within the track.
func (s State) Thumb() (start, size int) {
	if s.Height <= 0 {
		return 0, 0
	}
	if s.Total <= 0 || s.Visible <= 0 {
		return 0, s.Height
	}
	size = max(1, int(math.Round(float64(s.Height)*float64(s.Visible)/float64(s.Total))))
	size = min(size, s.Height)
	travel := s.Height - size
	maxOffset := max(1, s.Total-s.Visible)
	start = int(math.Round(float64(travel) * float64(s.Offset) / float64(maxOffset)))
	return clamp(start, 0, travel), size
}

// OffsetForThumbStart maps a thumb start row to the corresponding content offset.
func (s State) OffsetForThumbStart(start int) int {
	maxOffset := max(0, s.Total-s.Visible)
	travel := s.Height
	if s.Total > 0 && s.Visible > 0 {
		size := max(1, int(math.Round(float64(s.Height)*float64(s.Visible)/float64(s.Total))))
		travel = s.Height - min(size, s.Height)
	}
	if travel <= 0 || maxOffset == 0 {
		return 0
	}
	start = clamp(start, 0, travel)
	offset := int(math.Round(float64(start) * float64(maxOffset) / float64(travel)))
	return clamp(offset, 0, maxOffset)
}

func clamp(value, low, high int) int {
	return min(max(value, low), high)
}
