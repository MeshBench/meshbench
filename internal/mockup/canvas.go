// Package mockup renders the UX designs in docs/ux to PNG.
//
// The designs are generated from code rather than drawn by hand so they can be
// regenerated when the design changes, and so a review sees a diff rather than a
// new picture. It is a drawing toolkit for documentation — nothing here runs at
// application runtime.
package mockup

import (
	"image"
	"image/color"
	"image/draw"

	"golang.org/x/image/vector"
)

// Scale renders at 2x and the PNG is downsampled on write, which is what makes
// text and hairlines look crisp rather than aliased.
const Scale = 2

// Palette — a modern dark UI. Named by role so the designs survive a repaint.
var (
	BgDeep    = color.NRGBA{0x0b, 0x0f, 0x14, 0xff}
	BgSurface = color.NRGBA{0x12, 0x18, 0x20, 0xff}
	BgRaised  = color.NRGBA{0x18, 0x20, 0x2b, 0xff}
	BgInset   = color.NRGBA{0x0d, 0x12, 0x18, 0xff}
	Border    = color.NRGBA{0x23, 0x2d, 0x3a, 0xff}
	BorderLit = color.NRGBA{0x31, 0x3f, 0x50, 0xff}

	TextHi  = color.NRGBA{0xe6, 0xed, 0xf3, 0xff}
	TextMid = color.NRGBA{0xa8, 0xb4, 0xc0, 0xff}
	TextLo  = color.NRGBA{0x6e, 0x7d, 0x8c, 0xff}

	Accent = color.NRGBA{0x4c, 0x8d, 0xff, 0xff}
	Good   = color.NRGBA{0x3f, 0xb9, 0x50, 0xff}
	Warn   = color.NRGBA{0xd2, 0x99, 0x22, 0xff}
	Bad    = color.NRGBA{0xf8, 0x51, 0x49, 0xff}
	Violet = color.NRGBA{0xa3, 0x71, 0xf7, 0xff}
	Teal   = color.NRGBA{0x39, 0xc5, 0xcf, 0xff}

	Terrain = color.NRGBA{0x2b, 0x3a, 0x33, 0xff}
	Sky     = color.NRGBA{0x0e, 0x14, 0x1b, 0xff}
)

// NRGBA is re-exported so figure files need only one import.
type NRGBA = color.NRGBA

// Alpha returns c with the given opacity, for washes and fills under strokes.
func Alpha(c color.NRGBA, a uint8) color.NRGBA {
	c.A = a
	return c
}

// Canvas is a 2x-oversampled RGBA image with anti-aliased primitives.
// All coordinates passed in are in logical (1x) units.
type Canvas struct {
	Img  *image.RGBA
	W, H int // logical size
}

func New(w, h int) *Canvas {
	c := &Canvas{Img: image.NewRGBA(image.Rect(0, 0, w*Scale, h*Scale)), W: w, H: h}
	c.Fill(0, 0, w, h, BgDeep)
	return c
}

// Fill paints an axis-aligned rectangle with no anti-aliasing needed.
func (c *Canvas) Fill(x, y, w, h int, col color.Color) {
	r := image.Rect(x*Scale, y*Scale, (x+w)*Scale, (y+h)*Scale)
	draw.Draw(c.Img, r, &image.Uniform{col}, image.Point{}, draw.Over)
}

// fillPath rasterises a closed path with anti-aliasing and composites col
// through the coverage mask. Points are in device (scaled) units.
func (c *Canvas) fillPath(pts [][2]float32, col color.Color) {
	if len(pts) < 3 {
		return
	}
	minX, minY, maxX, maxY := pts[0][0], pts[0][1], pts[0][0], pts[0][1]
	for _, p := range pts {
		minX, minY = min32(minX, p[0]), min32(minY, p[1])
		maxX, maxY = max32(maxX, p[0]), max32(maxY, p[1])
	}
	ox, oy := int(minX)-1, int(minY)-1
	w, h := int(maxX)-ox+2, int(maxY)-oy+2
	if w <= 0 || h <= 0 {
		return
	}
	r := vector.NewRasterizer(w, h)
	r.MoveTo(pts[0][0]-float32(ox), pts[0][1]-float32(oy))
	for _, p := range pts[1:] {
		r.LineTo(p[0]-float32(ox), p[1]-float32(oy))
	}
	r.ClosePath()
	mask := image.NewAlpha(image.Rect(0, 0, w, h))
	r.Draw(mask, mask.Bounds(), image.Opaque, image.Point{})
	draw.DrawMask(c.Img, image.Rect(ox, oy, ox+w, oy+h),
		&image.Uniform{col}, image.Point{}, mask, image.Point{}, draw.Over)
}

// RoundRect draws a rounded rectangle: fill first, then a 1px inner stroke.
func (c *Canvas) RoundRect(x, y, w, h, radius int, fill, stroke color.Color) {
	if fill != nil {
		c.fillPath(roundRectPath(float32(x*Scale), float32(y*Scale),
			float32(w*Scale), float32(h*Scale), float32(radius*Scale)), fill)
	}
	if stroke != nil {
		outer := roundRectPath(float32(x*Scale), float32(y*Scale),
			float32(w*Scale), float32(h*Scale), float32(radius*Scale))
		inner := roundRectPath(float32(x*Scale)+Scale, float32(y*Scale)+Scale,
			float32(w*Scale)-2*Scale, float32(h*Scale)-2*Scale, float32(radius*Scale)-Scale)
		// Ring = outer path followed by the inner path reversed, so the
		// non-zero winding rule leaves a hollow outline.
		ring := append(outer, reverse(inner)...)
		c.fillPath(ring, stroke)
	}
}

// Line draws an anti-aliased line of the given width. dash > 0 alternates every
// dash logical units, which is how link states stay distinguishable without
// relying on colour alone.
func (c *Canvas) Line(x0, y0, x1, y1 int, col color.Color, width float32, dash int) {
	if dash <= 0 {
		c.segment(float32(x0*Scale), float32(y0*Scale), float32(x1*Scale), float32(y1*Scale), col, width)
		return
	}
	dx, dy := float32((x1-x0)*Scale), float32((y1-y0)*Scale)
	length := sqrt32(dx*dx + dy*dy)
	step := float32(dash * Scale)
	for t := float32(0); t < length; t += step * 2 {
		e := t + step
		if e > length {
			e = length
		}
		c.segment(float32(x0*Scale)+dx*t/length, float32(y0*Scale)+dy*t/length,
			float32(x0*Scale)+dx*e/length, float32(y0*Scale)+dy*e/length, col, width)
	}
}

func (c *Canvas) segment(x0, y0, x1, y1 float32, col color.Color, width float32) {
	w := width * Scale / 2
	dx, dy := x1-x0, y1-y0
	l := sqrt32(dx*dx + dy*dy)
	if l == 0 {
		return
	}
	nx, ny := -dy/l*w, dx/l*w
	c.fillPath([][2]float32{{x0 + nx, y0 + ny}, {x1 + nx, y1 + ny}, {x1 - nx, y1 - ny}, {x0 - nx, y0 - ny}}, col)
}

// Polyline draws a connected anti-aliased path, and optionally fills beneath it
// to a baseline — how the terrain profile and the battery curve are drawn.
func (c *Canvas) Polyline(pts [][2]int, col color.Color, width float32, fillTo int, fill color.Color) {
	if fill != nil && len(pts) > 1 {
		poly := make([][2]float32, 0, len(pts)+2)
		for _, p := range pts {
			poly = append(poly, [2]float32{float32(p[0] * Scale), float32(p[1] * Scale)})
		}
		poly = append(poly,
			[2]float32{float32(pts[len(pts)-1][0] * Scale), float32(fillTo * Scale)},
			[2]float32{float32(pts[0][0] * Scale), float32(fillTo * Scale)})
		c.fillPath(poly, fill)
	}
	for i := 1; i < len(pts); i++ {
		c.segment(float32(pts[i-1][0]*Scale), float32(pts[i-1][1]*Scale),
			float32(pts[i][0]*Scale), float32(pts[i][1]*Scale), col, width)
	}
}

// Dot draws a filled circle — nodes, emitters, status indicators.
func (c *Canvas) Dot(x, y int, r float32, col color.Color) {
	pts := make([][2]float32, 0, 24)
	for i := 0; i < 24; i++ {
		a := float64(i) / 24 * 2 * pi
		pts = append(pts, [2]float32{
			float32(x*Scale) + r*Scale*cos32(a),
			float32(y*Scale) + r*Scale*sin32(a),
		})
	}
	c.fillPath(pts, col)
}
