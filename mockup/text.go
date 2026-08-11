package mockup

import (
	"fmt"
	"image"
	"image/color"
	"os"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Weight and family selectors for Face.
type Style int

const (
	Sans Style = iota
	SansBold
	Mono
	MonoBold
)

// Candidate paths, in preference order. Loaded from the system rather than
// embedded: DejaVu is ~750 KB per face and the rendered PNGs are what the repo
// actually needs to carry.
var fontPaths = map[Style][]string{
	Sans:     {"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf", "/Library/Fonts/Arial.ttf"},
	SansBold: {"/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf"},
	Mono:     {"/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf", "/System/Library/Fonts/Menlo.ttc"},
	MonoBold: {"/usr/share/fonts/truetype/dejavu/DejaVuSansMono-Bold.ttf"},
}

var faceCache = map[string]font.Face{}

// Face returns a cached font face at the given logical point size.
func Face(s Style, size float64) font.Face {
	key := fmt.Sprintf("%d/%.1f", s, size)
	if f, ok := faceCache[key]; ok {
		return f
	}
	var data []byte
	var err error
	for _, p := range fontPaths[s] {
		if data, err = os.ReadFile(p); err == nil {
			break
		}
	}
	if data == nil {
		panic(fmt.Sprintf("mockup: no font found for style %d; install fonts-dejavu-core", s))
	}
	f, err := opentype.Parse(data)
	if err != nil {
		panic(err)
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{
		Size: size * Scale, DPI: 72, Hinting: font.HintingFull,
	})
	if err != nil {
		panic(err)
	}
	faceCache[key] = face
	return face
}

// Text draws a string with its baseline at y, returning the x advance so callers
// can chain differently-coloured runs on one line.
func (c *Canvas) Text(x, y int, s string, col color.Color, style Style, size float64) int {
	d := &font.Drawer{
		Dst: c.Img, Src: &image.Uniform{col},
		Face: Face(style, size),
		Dot:  fixed.P(x*Scale, y*Scale),
	}
	d.DrawString(s)
	return d.Dot.X.Round() / Scale
}

// TextRight draws right-aligned to x — for numeric columns, which must align on
// their units to be scannable.
func (c *Canvas) TextRight(x, y int, s string, col color.Color, style Style, size float64) {
	w := c.Measure(s, style, size)
	c.Text(x-w, y, s, col, style, size)
}

// Measure returns the logical width of s.
func (c *Canvas) Measure(s string, style Style, size float64) int {
	return font.MeasureString(Face(style, size), s).Round() / Scale
}
