package scrollbar

import "testing"

func TestScrollbarStateGeometry(t *testing.T) {
	tests := []struct {
		name         string
		state        State
		shown        bool
		start, size  int
		offsetForTop int
	}{
		{"fits", State{Total: 8, Visible: 8, Height: 8}, false, 0, 8, 0},
		{"one line thumb", State{Total: 100, Visible: 1, Height: 10}, true, 0, 1, 0},
		{"top", State{Total: 100, Visible: 20, Height: 10}, true, 0, 2, 0},
		{"middle", State{Total: 100, Visible: 20, Offset: 40, Height: 10}, true, 4, 2, 0},
		{"bottom", State{Total: 100, Visible: 20, Offset: 80, Height: 10}, true, 8, 2, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.Shown(); got != tt.shown {
				t.Fatalf("Shown() = %v, want %v", got, tt.shown)
			}
			start, size := tt.state.Thumb()
			if start != tt.start || size != tt.size {
				t.Fatalf("Thumb() = (%d, %d), want (%d, %d)", start, size, tt.start, tt.size)
			}
			if got := tt.state.OffsetForThumbStart(-100); got != tt.offsetForTop {
				t.Fatalf("top clamp = %d, want %d", got, tt.offsetForTop)
			}
		})
	}
}

func TestScrollbarThumbRoundTrip(t *testing.T) {
	for _, offset := range []int{0, 1, 20, 40, 79, 80} {
		state := State{Total: 100, Visible: 20, Offset: offset, Height: 10}
		start, _ := state.Thumb()
		got := state.OffsetForThumbStart(start)
		if abs(got-offset) > 1 {
			t.Errorf("offset %d round-tripped as %d", offset, got)
		}
	}
	state := State{Total: 100, Visible: 20, Height: 10}
	if got := state.OffsetForThumbStart(100); got != 80 {
		t.Fatalf("bottom clamp = %d, want 80", got)
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
