package comp

import (
	"image"
	"testing"

	"gioui.org/layout"
	"gioui.org/unit"
)

// A radius larger than half the shorter side describes no rounded rectangle.
// Gio does not reject it - the arcs overrun each other and the path sprays
// stray strokes right across the panel, nowhere near the widget that asked
// for them. A pill is a legitimate thing to want, so the clamp lives in the
// primitive rather than in every caller's head.
func TestCornerRadiusIsClampedToTheBox(t *testing.T) {
	gtx := layout.Context{Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	for _, c := range []struct {
		name string
		sz   image.Point
		r    unit.Dp
		want int
	}{
		{"pill radius on a short box", image.Pt(120, 16), 999, 8},
		{"pill radius on a narrow box", image.Pt(14, 90), 999, 7},
		{"a radius that fits is untouched", image.Pt(200, 100), 8, 8},
		{"exactly half is allowed", image.Pt(40, 40), 20, 20},
		{"a zero-sized box rounds to nothing", image.Pt(0, 0), 12, 0},
		{"a negative radius is not a hole", image.Pt(50, 50), -4, 0},
	} {
		if got := cornerRadius(gtx, c.sz, c.r); got != c.want {
			t.Errorf("%s: cornerRadius(%v, %v) = %d, want %d", c.name, c.sz, c.r, got, c.want)
		}
	}
}
