package app

import (
	"math"
	"testing"
)

func TestClampRatio(t *testing.T) {
	tests := []struct {
		name   string
		totalW int
		ratio  float64
		want   float64
	}{
		{name: "default stays", totalW: 100, ratio: 0.3, want: 0.3},
		{name: "zero clamps to tree minimum", totalW: 100, ratio: 0, want: 0.2},
		{name: "one clamps to viewer minimum", totalW: 100, ratio: 1, want: 0.69},
		{name: "below zero clamps", totalW: 100, ratio: -1, want: 0.2},
		{name: "above one clamps", totalW: 100, ratio: 2, want: 0.69},
		{name: "narrow window low clamp", totalW: 60, ratio: 0.1, want: 1.0 / 3.0},
		{name: "narrow window high clamp", totalW: 60, ratio: 0.9, want: 1 - 31.0/60.0},
		{name: "exact minimum window", totalW: 51, ratio: 0.3, want: 20.0 / 51.0},
		{name: "too narrow keeps ratio", totalW: 40, ratio: 0.3, want: 0.3},
		{name: "zero width keeps ratio", totalW: 0, ratio: 0.3, want: 0.3},
		{name: "NaN becomes default", totalW: 100, ratio: math.NaN(), want: DefaultRatio},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClampRatio(tt.totalW, tt.ratio)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Fatalf("ClampRatio(%d, %v) = %v, want %v", tt.totalW, tt.ratio, got, tt.want)
			}
		})
	}
}

func TestSplitWidths(t *testing.T) {
	tests := []struct {
		name                          string
		totalW                        int
		ratio                         float64
		wantTree, wantDiv, wantViewer int
	}{
		{name: "default ratio", totalW: 100, ratio: 0.3, wantTree: 30, wantDiv: 1, wantViewer: 69},
		{name: "default ratio at 80", totalW: 80, ratio: 0.3, wantTree: 24, wantDiv: 1, wantViewer: 55},
		{name: "ratio zero clamps to tree minimum", totalW: 100, ratio: 0, wantTree: 20, wantDiv: 1, wantViewer: 79},
		{name: "ratio one clamps to viewer minimum", totalW: 100, ratio: 1, wantTree: 69, wantDiv: 1, wantViewer: 30},
		{name: "exact minimum window", totalW: 51, ratio: 0.3, wantTree: 20, wantDiv: 1, wantViewer: 30},
		{name: "narrow window clamps left", totalW: 60, ratio: 0.1, wantTree: 20, wantDiv: 1, wantViewer: 39},
		{name: "narrow window clamps right", totalW: 60, ratio: 0.9, wantTree: 29, wantDiv: 1, wantViewer: 30},
		{name: "below minimums proportional", totalW: 40, ratio: 0.3, wantTree: 12, wantDiv: 1, wantViewer: 27},
		{name: "below minimums zero ratio", totalW: 40, ratio: 0, wantTree: 0, wantDiv: 1, wantViewer: 39},
		{name: "zero width", totalW: 0, ratio: 0.3, wantTree: 0, wantDiv: 0, wantViewer: 0},
		{name: "divider only", totalW: 1, ratio: 0.3, wantTree: 0, wantDiv: 1, wantViewer: 0},
		{name: "two columns", totalW: 2, ratio: 0.3, wantTree: 1, wantDiv: 1, wantViewer: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			treeW, divW, viewerW := SplitWidths(tt.totalW, tt.ratio)
			if treeW != tt.wantTree || divW != tt.wantDiv || viewerW != tt.wantViewer {
				t.Fatalf(
					"SplitWidths(%d, %v) = (%d, %d, %d), want (%d, %d, %d)",
					tt.totalW, tt.ratio, treeW, divW, viewerW,
					tt.wantTree, tt.wantDiv, tt.wantViewer,
				)
			}
			if tt.totalW > 0 {
				if sum := treeW + divW + viewerW; sum != tt.totalW {
					t.Fatalf("widths sum to %d, want %d", sum, tt.totalW)
				}
				if treeW < 0 || divW < 0 || viewerW < 0 {
					t.Fatalf("negative width in (%d, %d, %d)", treeW, divW, viewerW)
				}
			}
		})
	}
}
