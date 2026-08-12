package comp

import (
	"math"

	"gioui.org/f32"
	"gioui.org/op/clip"
)

// Cheap geometry for the map, where the same shape is drawn hundreds of times
// per frame.
//
// Gio re-tessellates every path on every frame; there is no cache to hit and
// nothing here is a Gio shortcoming. The saving comes from asking it to
// tessellate less. Two shapes dominate the map, and both had a cheaper form
// that looks the same at the size they are actually drawn.

// segment adds a line from a to b as a filled quad, half a pixel of width
// either side of the centre line.
//
// The obvious spelling is clip.Stroke over a path of MoveTo/LineTo pairs, and
// that is what this replaced. A stroke is general - it has to handle joins,
// caps and curvature - so Gio expands each segment through its arc machinery,
// which showed up as stroke.ArcTransform and approxCubeTo in the profile. A
// straight line of constant width is a rectangle, and a rectangle is four
// points.
func segment(p *clip.Path, a, b f32.Point, width float32) {
	dx, dy := b.X-a.X, b.Y-a.Y
	l := f32.Point{X: dx, Y: dy}
	n := length(l)
	if n == 0 {
		return
	}
	// The normal, scaled to half the width.
	h := width / 2
	nx, ny := -dy/n*h, dx/n*h

	p.MoveTo(f32.Pt(a.X+nx, a.Y+ny))
	p.LineTo(f32.Pt(b.X+nx, b.Y+ny))
	p.LineTo(f32.Pt(b.X-nx, b.Y-ny))
	p.LineTo(f32.Pt(a.X-nx, a.Y-ny))
	p.Close()
}

// dot adds a node marker: an octagon rather than a circle.
//
// A circle is four cubic Beziers, and Gio flattens each into a run of quads
// before it can fill anything - 311 nodes cost 1,244 curve flattenings a
// frame. An octagon is eight straight lines and needs no flattening at all. At
// the four-pixel radius a node is actually drawn, the two are the same picture.
func dot(p *clip.Path, c f32.Point, r float32) {
	// cos(22.5 degrees) and sin(22.5 degrees), so the octagon is drawn with a
	// flat edge uppermost and reads as round rather than as a stop sign.
	const (
		co = 0.9238795
		si = 0.3826834
	)
	pts := [8]f32.Point{
		{X: c.X + r*co, Y: c.Y + r*si},
		{X: c.X + r*si, Y: c.Y + r*co},
		{X: c.X - r*si, Y: c.Y + r*co},
		{X: c.X - r*co, Y: c.Y + r*si},
		{X: c.X - r*co, Y: c.Y - r*si},
		{X: c.X - r*si, Y: c.Y - r*co},
		{X: c.X + r*si, Y: c.Y - r*co},
		{X: c.X + r*co, Y: c.Y - r*si},
	}
	p.MoveTo(pts[0])
	for _, q := range pts[1:] {
		p.LineTo(q)
	}
	p.Close()
}

func length(p f32.Point) float32 {
	return float32(math.Sqrt(float64(p.X*p.X + p.Y*p.Y)))
}
