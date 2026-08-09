// Package mockup renders the UX wireframes in docs/ux to PNG.
//
// The designs are generated from code rather than drawn by hand so they can be
// regenerated when the design changes, and so a diff shows what moved. It is a
// drawing toolkit, not application code — nothing here is used at runtime.
package mockup

import (
	"image"
	"image/color"
	"image/draw"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// Palette for a dark engineering UI. Named by role, not by colour, so the
// wireframes stay readable if the palette changes.
var (
	Bg      = color.RGBA{0x14, 0x17, 0x1a, 0xff}
	Panel   = color.RGBA{0x1c, 0x21, 0x26, 0xff}
	Border  = color.RGBA{0x33, 0x3b, 0x44, 0xff}
	Text    = color.RGBA{0xe6, 0xea, 0xef, 0xff}
	Muted   = color.RGBA{0x8b, 0x97, 0xa5, 0xff}
	Accent  = color.RGBA{0x4c, 0x9a, 0xff, 0xff}
	Good    = color.RGBA{0x4a, 0xde, 0x80, 0xff}
	Warn    = color.RGBA{0xfb, 0xbf, 0x24, 0xff}
	Bad     = color.RGBA{0xf8, 0x71, 0x71, 0xff}
	Terrain = color.RGBA{0x3a, 0x4a, 0x3f, 0xff}
	Fresnel = color.RGBA{0x25, 0x3a, 0x52, 0xff}
)

// Canvas is a fixed-size RGBA image with the few primitives the wireframes need.
type Canvas struct {
	Img  *image.RGBA
	W, H int
}

func New(w, h int) *Canvas {
	c := &Canvas{Img: image.NewRGBA(image.Rect(0, 0, w, h)), W: w, H: h}
	draw.Draw(c.Img, c.Img.Bounds(), &image.Uniform{Bg}, image.Point{}, draw.Src)
	return c
}

func (c *Canvas) Fill(x, y, w, h int, col color.Color) {
	draw.Draw(c.Img, image.Rect(x, y, x+w, y+h), &image.Uniform{col}, image.Point{}, draw.Src)
}

// Rect draws a 1px outlined box, optionally filled.
func (c *Canvas) Rect(x, y, w, h int, stroke color.Color, fill color.Color) {
	if fill != nil {
		c.Fill(x, y, w, h, fill)
	}
	c.Fill(x, y, w, 1, stroke)
	c.Fill(x, y+h-1, w, 1, stroke)
	c.Fill(x, y, 1, h, stroke)
	c.Fill(x+w-1, y, 1, h, stroke)
}

// Line draws a 1px line via Bresenham. Dash of 0 is solid; >0 alternates every
// dash pixels, which is how "marginal" and "no path" links stay distinguishable
// without relying on colour.
func (c *Canvas) Line(x0, y0, x1, y1 int, col color.Color, dash int) {
	dx, dy := abs(x1-x0), -abs(y1-y0)
	sx, sy := sign(x1-x0), sign(y1-y0)
	err := dx + dy
	n := 0
	for {
		if dash == 0 || (n/dash)%2 == 0 {
			c.Img.Set(x0, y0, col)
		}
		n++
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

// Text draws with the 7x13 bitmap face. Returns the x advance so callers can
// chain coloured segments on one line.
func (c *Canvas) Text(x, y int, s string, col color.Color) int {
	d := &font.Drawer{
		Dst:  c.Img,
		Src:  &image.Uniform{col},
		Face: basicfont.Face7x13,
		Dot:  fixed.P(x, y),
	}
	d.DrawString(s)
	return d.Dot.X.Round()
}

// Panel draws a titled panel and returns the inner content origin.
func (c *Canvas) Panel(x, y, w, h int, title string) (int, int) {
	c.Rect(x, y, w, h, Border, Panel)
	c.Fill(x+1, y+1, w-2, 18, color.RGBA{0x24, 0x2b, 0x33, 0xff})
	c.Text(x+8, y+14, title, Text)
	return x + 8, y + 34
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
func sign(v int) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}
