package view

import "testing"

// The thumb shows how much of the diff is in view and where, never shrinks out of
// sight, and sits flush with each end when the view does.
func TestScrollThumb(t *testing.T) {
	cases := []struct {
		name                string
		rows, total, offset int
		wantStart, wantSize int
	}{
		{"fits", 10, 10, 0, 0, 0},
		{"top", 10, 40, 0, 0, 2},
		{"bottom", 10, 40, 30, 8, 2},
		{"middle", 10, 40, 15, 4, 2},
		{"never vanishes", 10, 1000, 0, 0, 1},
		{"half in view", 10, 20, 10, 5, 5},
	}
	for _, c := range cases {
		got := scrollThumb(c.rows, c.total, c.offset)
		if got.start != c.wantStart || got.size != c.wantSize {
			t.Errorf("%s: scrollThumb(%d, %d, %d) = %+v, want start %d size %d", c.name, c.rows, c.total, c.offset, got, c.wantStart, c.wantSize)
		}
	}
}
