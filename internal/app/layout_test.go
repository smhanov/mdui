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

func TestDividerHitZone(t *testing.T) {
	tests := []struct {
		name   string
		totalW int
		ratio  float64
		x      int
		want   bool
	}{
		{name: "center", totalW: 100, ratio: 0.3, x: 30, want: true},
		{name: "left slop", totalW: 100, ratio: 0.3, x: 29, want: true},
		{name: "right slop", totalW: 100, ratio: 0.3, x: 31, want: true},
		{name: "tree edge", totalW: 100, ratio: 0.3, x: 28, want: false},
		{name: "viewer edge", totalW: 100, ratio: 0.3, x: 32, want: false},
		{name: "negative column", totalW: 100, ratio: 0.3, x: -1, want: false},
		{name: "past content", totalW: 100, ratio: 0.3, x: 100, want: false},
		{name: "zero width", totalW: 0, ratio: 0.3, x: 0, want: false},
		{name: "clamped left", totalW: 100, ratio: 0, x: 20, want: true},
		{name: "clamped right", totalW: 100, ratio: 1, x: 69, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := InDividerZone(tt.totalW, tt.ratio, tt.x); got != tt.want {
				t.Fatalf("InDividerZone(%d, %v, %d) = %v, want %v",
					tt.totalW, tt.ratio, tt.x, got, tt.want)
			}
		})
	}
}

func TestRatioAtColumn(t *testing.T) {
	tests := []struct {
		name   string
		totalW int
		x      int
		want   float64
	}{
		{name: "middle", totalW: 100, x: 40, want: 0.4},
		{name: "left clamp", totalW: 100, x: -10, want: 0.2},
		{name: "right clamp", totalW: 100, x: 200, want: 0.69},
		{name: "zero width defaults", totalW: 0, x: 10, want: DefaultRatio},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RatioAtColumn(tt.totalW, tt.x)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Fatalf("RatioAtColumn(%d, %d) = %v, want %v", tt.totalW, tt.x, got, tt.want)
			}
		})
	}
}

func TestNudgeRatio(t *testing.T) {
	tests := []struct {
		name    string
		totalW  int
		ratio   float64
		columns int
		want    float64
	}{
		{name: "grow two", totalW: 100, ratio: 0.3, columns: 2, want: 0.32},
		{name: "shrink two", totalW: 100, ratio: 0.3, columns: -2, want: 0.28},
		{name: "grow clamps at viewer minimum", totalW: 100, ratio: 0.68, columns: 100, want: 0.69},
		{name: "shrink clamps at tree minimum", totalW: 100, ratio: 0.2, columns: -100, want: 0.2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NudgeRatio(tt.totalW, tt.ratio, tt.columns)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Fatalf("NudgeRatio(%d, %v, %d) = %v, want %v",
					tt.totalW, tt.ratio, tt.columns, got, tt.want)
			}
		})
	}
}
