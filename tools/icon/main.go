// icon draws MeshBench's application icon, and writes it as both SVG and PNG.
//
// One program for both so the two cannot drift: the geometry below is the
// single description, the SVG is that description written out, and the PNGs
// are it rasterised. Everything a desktop needs comes from here - the
// launcher entry, the AppImage, the .deb, the macOS bundle and the Windows
// executable all point at these files.
//
// The mark is the thing MeshBench is about: nodes that can hear each other.
// Four nodes, the links between them drawn at the strengths a real mesh has
// (two strong, two marginal, one missing), and the one node that is talking
// ringed by its transmission. It reads at 512 and still reads at 16, which is
// the only real constraint on an icon.
//
//	go run ./tools/icon
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// The palette is the application's own (internal/gui/theme): if the icon and
// the window disagree about what colour MeshBench is, the icon is wrong.
var (
	ground = color.NRGBA{0x0d, 0x11, 0x13, 0xff} // Palette.Ground
	accent = color.NRGBA{0x4a, 0xc4, 0xa8, 0xff} // Palette.Accent
	dim    = color.NRGBA{0x4a, 0xc4, 0xa8, 0x66} // a link that barely holds
	faint  = color.NRGBA{0xff, 0xff, 0xff, 0x1e} // the ring around the talker
)

// Geometry in a 0..1 square, so one description serves every size.
type node struct{ x, y, r float64 }

var nodes = []node{
	{0.30, 0.32, 0.070}, // the one that is talking
	{0.72, 0.26, 0.055},
	{0.76, 0.70, 0.055},
	{0.27, 0.72, 0.055},
}

// links are indices into nodes, with the strength the mesh actually has.
// Node 1 and 3 cannot hear each other, which is the point: a mesh is not a
// clique, and an icon that draws every edge says nothing about what this is.
var links = []struct {
	a, b   int
	strong bool
}{
	{0, 1, true},
	{0, 3, true},
	{0, 2, false},
	{2, 3, false},
}

func main() {
	out := "packaging/icons"
	if err := os.MkdirAll(out, 0o755); err != nil {
		fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "meshbench.svg"), []byte(svg()), 0o644); err != nil {
		fatal(err)
	}
	// The sizes a Linux hicolor theme, a .desktop entry, an AppImage and a
	// Windows .ico between them ask for.
	for _, px := range []int{16, 24, 32, 48, 64, 128, 256, 512} {
		f, err := os.Create(filepath.Join(out, fmt.Sprintf("meshbench-%d.png", px)))
		if err != nil {
			fatal(err)
		}
		if err := png.Encode(f, raster(px)); err != nil {
			fatal(err)
		}
		if err := f.Close(); err != nil {
			fatal(err)
		}
	}
	fmt.Println("wrote", out+"/meshbench.svg and 8 PNGs")
}

// svg writes the same geometry as scalable markup, for Flathub and for any
// desktop that prefers it.
func svg() string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512" width="512" height="512">` + "\n")
	b.WriteString(`  <title>MeshBench</title>` + "\n")
	fmt.Fprintf(&b, "  <rect width=\"512\" height=\"512\" rx=\"96\" fill=\"%s\"/>\n", hex(ground))
	// The talker's ring, under everything.
	fmt.Fprintf(&b, "  <circle cx=\"%.0f\" cy=\"%.0f\" r=\"%.0f\" fill=\"none\" stroke=\"%s\" stroke-opacity=\"%.2f\" stroke-width=\"10\"/>\n",
		nodes[0].x*512, nodes[0].y*512, nodes[0].r*512*2.6, hex(accent), 0.35)
	for _, l := range links {
		a, c := nodes[l.a], nodes[l.b]
		w, op := 9.0, 0.40
		if l.strong {
			w, op = 14.0, 1.0
		}
		fmt.Fprintf(&b, "  <line x1=\"%.0f\" y1=\"%.0f\" x2=\"%.0f\" y2=\"%.0f\" stroke=\"%s\" stroke-opacity=\"%.2f\" stroke-width=\"%.0f\" stroke-linecap=\"round\"/>\n",
			a.x*512, a.y*512, c.x*512, c.y*512, hex(accent), op, w)
	}
	for _, n := range nodes {
		fmt.Fprintf(&b, "  <circle cx=\"%.0f\" cy=\"%.0f\" r=\"%.0f\" fill=\"%s\"/>\n",
			n.x*512, n.y*512, n.r*512, hex(accent))
	}
	b.WriteString("</svg>\n")
	return b.String()
}

// raster draws the same thing into pixels, supersampled so the edges are not
// a staircase at 16 px - which is the size that decides whether an icon looks
// made or generated.
func raster(px int) image.Image {
	const ss = 4
	big := px * ss
	img := image.NewNRGBA(image.Rect(0, 0, big, big))
	f := float64(big)

	radius := 0.1875 * f // 96/512, the same corner as the SVG
	for y := 0; y < big; y++ {
		for x := 0; x < big; x++ {
			if insideRounded(float64(x)+0.5, float64(y)+0.5, f, radius) {
				img.SetNRGBA(x, y, ground)
			}
		}
	}
	// The ring first, then links, then the nodes on top.
	drawRing(img, nodes[0].x*f, nodes[0].y*f, nodes[0].r*f*2.6, 0.020*f, faintOn(accent, 0.35))
	for _, l := range links {
		a, c := nodes[l.a], nodes[l.b]
		w, col := 0.018*f, dim
		if l.strong {
			w, col = 0.027*f, accent
		}
		drawLine(img, a.x*f, a.y*f, c.x*f, c.y*f, w, col)
	}
	for _, n := range nodes {
		drawDisc(img, n.x*f, n.y*f, n.r*f, accent)
	}
	return downsample(img, px, ss)
}

func insideRounded(x, y, size, r float64) bool {
	cx := math.Min(math.Max(x, r), size-r)
	cy := math.Min(math.Max(y, r), size-r)
	dx, dy := x-cx, y-cy
	return dx*dx+dy*dy <= r*r
}

func drawDisc(img *image.NRGBA, cx, cy, r float64, col color.NRGBA) {
	for y := int(cy - r - 1); y <= int(cy+r+1); y++ {
		for x := int(cx - r - 1); x <= int(cx+r+1); x++ {
			dx, dy := float64(x)+0.5-cx, float64(y)+0.5-cy
			if dx*dx+dy*dy <= r*r {
				blend(img, x, y, col)
			}
		}
	}
}

func drawRing(img *image.NRGBA, cx, cy, r, w float64, col color.NRGBA) {
	inner, outer := r-w/2, r+w/2
	for y := int(cy - outer - 1); y <= int(cy+outer+1); y++ {
		for x := int(cx - outer - 1); x <= int(cx+outer+1); x++ {
			dx, dy := float64(x)+0.5-cx, float64(y)+0.5-cy
			d := math.Hypot(dx, dy)
			if d >= inner && d <= outer {
				blend(img, x, y, col)
			}
		}
	}
}

// drawLine is a capsule: a thick segment with round ends, which is what
// stroke-linecap="round" gives the SVG.
func drawLine(img *image.NRGBA, x0, y0, x1, y1, w float64, col color.NRGBA) {
	r := w / 2
	minX, maxX := int(math.Min(x0, x1)-r-1), int(math.Max(x0, x1)+r+1)
	minY, maxY := int(math.Min(y0, y1)-r-1), int(math.Max(y0, y1)+r+1)
	dx, dy := x1-x0, y1-y0
	len2 := dx*dx + dy*dy
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			t := 0.0
			if len2 > 0 {
				t = ((px-x0)*dx + (py-y0)*dy) / len2
				t = math.Min(math.Max(t, 0), 1)
			}
			ex, ey := x0+t*dx, y0+t*dy
			if math.Hypot(px-ex, py-ey) <= r {
				blend(img, x, y, col)
			}
		}
	}
}

// blend is source-over, so a translucent link crossing another reads as one
// link over another rather than as a brighter patch.
func blend(img *image.NRGBA, x, y int, c color.NRGBA) {
	if !(image.Point{x, y}).In(img.Bounds()) {
		return
	}
	d := img.NRGBAAt(x, y)
	a := float64(c.A) / 255
	img.SetNRGBA(x, y, color.NRGBA{
		R: uint8(float64(c.R)*a + float64(d.R)*(1-a)),
		G: uint8(float64(c.G)*a + float64(d.G)*(1-a)),
		B: uint8(float64(c.B)*a + float64(d.B)*(1-a)),
		A: uint8(math.Min(255, float64(c.A)+float64(d.A)*(1-a))),
	})
}

func downsample(src *image.NRGBA, px, ss int) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, px, px))
	n := float64(ss * ss)
	for y := 0; y < px; y++ {
		for x := 0; x < px; x++ {
			var r, g, b, a float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					c := src.NRGBAAt(x*ss+sx, y*ss+sy)
					// Weight colour by coverage, or the transparent edge
					// pixels drag every rim towards black.
					af := float64(c.A) / 255
					r += float64(c.R) * af
					g += float64(c.G) * af
					b += float64(c.B) * af
					a += float64(c.A)
				}
			}
			av := a / n
			if av < 0.5 {
				out.SetNRGBA(x, y, color.NRGBA{})
				continue
			}
			w := a / 255
			out.SetNRGBA(x, y, color.NRGBA{
				R: uint8(math.Min(255, r/w)),
				G: uint8(math.Min(255, g/w)),
				B: uint8(math.Min(255, b/w)),
				A: uint8(math.Min(255, av)),
			})
		}
	}
	return out
}

func faintOn(c color.NRGBA, alpha float64) color.NRGBA {
	return color.NRGBA{c.R, c.G, c.B, uint8(alpha * 255)}
}

func hex(c color.NRGBA) string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "icon:", err)
	os.Exit(1)
}
