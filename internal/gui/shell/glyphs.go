// Menu glyphs, drawn.
//
// Drawn rather than taken from an icon font, for the same reason the
// transport's symbols are: a font is a file a machine may not have, and a
// menu whose icons render as boxes on a fresh install is worse than no icons.
// Each glyph is simple geometry in the theme's Dim, sized to the text beside
// it.
package shell

import (
	"image"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

// glyphBox draws a named glyph in a fixed cell, so every label indents the
// same whether or not its row has one.
func glyphBox(t *theme.Theme, gtx layout.Context, name string) layout.Dimensions {
	sz := gtx.Dp(12)
	cell := image.Pt(sz+gtx.Dp(t.Sp.S), sz)
	if name == "" {
		return layout.Dimensions{Size: cell}
	}
	c := t.P.Dim
	s := float32(sz)
	w := float32(gtx.Dp(1))
	if w < 1 {
		w = 1
	}
	stroke := func(p clip.PathSpec) {
		paint.FillShape(gtx.Ops, c, clip.Stroke{Path: p, Width: w}.Op())
	}
	fill := func(p clip.PathSpec) {
		paint.FillShape(gtx.Ops, c, clip.Outline{Path: p}.Op())
	}
	path := func(build func(p *clip.Path)) clip.PathSpec {
		var p clip.Path
		p.Begin(gtx.Ops)
		build(&p)
		return p.End()
	}
	box := func(x0, y0, x1, y1 float32) clip.PathSpec {
		return path(func(p *clip.Path) {
			p.MoveTo(f32.Pt(x0, y0))
			p.LineTo(f32.Pt(x1, y0))
			p.LineTo(f32.Pt(x1, y1))
			p.LineTo(f32.Pt(x0, y1))
			p.Close()
		})
	}
	line := func(x0, y0, x1, y1 float32) {
		stroke(path(func(p *clip.Path) {
			p.MoveTo(f32.Pt(x0, y0))
			p.LineTo(f32.Pt(x1, y1))
		}))
	}
	circle := func(cx, cy, r float32) {
		stroke(path(func(p *clip.Path) {
			p.MoveTo(f32.Pt(cx+r, cy))
			p.ArcTo(f32.Pt(cx, cy), f32.Pt(cx, cy), 2*3.14159274)
		}))
	}
	dot := func(cx, cy, r float32) {
		defer clip.Ellipse(image.Rect(int(cx-r), int(cy-r), int(cx+r), int(cy+r))).
			Push(gtx.Ops).Pop()
		paint.ColorOp{Color: c}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
	}
	arrow := func(x, y0, y1 float32, down bool) {
		line(x, y0, x, y1)
		h := s * 0.22
		tip := y1
		d := -h
		if down {
			d = h
		}
		_ = d
		if down {
			fill(path(func(p *clip.Path) {
				p.MoveTo(f32.Pt(x-h, tip-h))
				p.LineTo(f32.Pt(x+h, tip-h))
				p.LineTo(f32.Pt(x, tip))
				p.Close()
			}))
		} else {
			fill(path(func(p *clip.Path) {
				p.MoveTo(f32.Pt(x-h, y0+h))
				p.LineTo(f32.Pt(x+h, y0+h))
				p.LineTo(f32.Pt(x, y0))
				p.Close()
			}))
		}
	}

	switch name {
	case "folder":
		stroke(path(func(p *clip.Path) {
			p.MoveTo(f32.Pt(s*0.08, s*0.85))
			p.LineTo(f32.Pt(s*0.08, s*0.25))
			p.LineTo(f32.Pt(s*0.4, s*0.25))
			p.LineTo(f32.Pt(s*0.5, s*0.4))
			p.LineTo(f32.Pt(s*0.92, s*0.4))
			p.LineTo(f32.Pt(s*0.92, s*0.85))
			p.Close()
		}))
	case "save", "import":
		arrow(s*0.5, s*0.05, s*0.62, true)
		stroke(path(func(p *clip.Path) {
			p.MoveTo(f32.Pt(s*0.1, s*0.6))
			p.LineTo(f32.Pt(s*0.1, s*0.9))
			p.LineTo(f32.Pt(s*0.9, s*0.9))
			p.LineTo(f32.Pt(s*0.9, s*0.6))
		}))
	case "export":
		arrow(s*0.5, s*0.62, s*0.05, false)
		stroke(path(func(p *clip.Path) {
			p.MoveTo(f32.Pt(s*0.1, s*0.6))
			p.LineTo(f32.Pt(s*0.1, s*0.9))
			p.LineTo(f32.Pt(s*0.9, s*0.9))
			p.LineTo(f32.Pt(s*0.9, s*0.6))
		}))
	case "chip":
		stroke(box(s*0.25, s*0.25, s*0.75, s*0.75))
		for _, f := range []float32{0.35, 0.5, 0.65} {
			line(s*f, 0, s*f, s*0.25)
			line(s*f, s*0.75, s*f, s)
			line(0, s*f, s*0.25, s*f)
			line(s*0.75, s*f, s, s*f)
		}
	case "exit":
		stroke(path(func(p *clip.Path) {
			p.MoveTo(f32.Pt(s*0.55, s*0.1))
			p.LineTo(f32.Pt(s*0.1, s*0.1))
			p.LineTo(f32.Pt(s*0.1, s*0.9))
			p.LineTo(f32.Pt(s*0.55, s*0.9))
		}))
		line(s*0.35, s*0.5, s*0.95, s*0.5)
		fill(path(func(p *clip.Path) {
			p.MoveTo(f32.Pt(s*0.75, s*0.3))
			p.LineTo(f32.Pt(s*0.75, s*0.7))
			p.LineTo(f32.Pt(s*0.98, s*0.5))
			p.Close()
		}))
	case "play":
		fill(path(func(p *clip.Path) {
			p.MoveTo(f32.Pt(s*0.2, s*0.1))
			p.LineTo(f32.Pt(s*0.2, s*0.9))
			p.LineTo(f32.Pt(s*0.9, s*0.5))
			p.Close()
		}))
	case "step":
		fill(path(func(p *clip.Path) {
			p.MoveTo(f32.Pt(s*0.1, s*0.15))
			p.LineTo(f32.Pt(s*0.1, s*0.85))
			p.LineTo(f32.Pt(s*0.7, s*0.5))
			p.Close()
		}))
		line(s*0.85, s*0.15, s*0.85, s*0.85)
	case "back":
		line(s*0.15, s*0.15, s*0.15, s*0.85)
		fill(path(func(p *clip.Path) {
			p.MoveTo(f32.Pt(s*0.9, s*0.15))
			p.LineTo(f32.Pt(s*0.9, s*0.85))
			p.LineTo(f32.Pt(s*0.3, s*0.5))
			p.Close()
		}))
	case "power", "boot":
		circle(s*0.5, s*0.55, s*0.35)
		line(s*0.5, s*0.05, s*0.5, s*0.45)
	case "wipe":
		line(s*0.2, s*0.2, s*0.8, s*0.8)
		line(s*0.8, s*0.2, s*0.2, s*0.8)
	case "spark", "packet":
		dot(s*0.5, s*0.5, s*0.14)
		for _, d := range [][4]float32{
			{0.5, 0.05, 0.5, 0.25}, {0.5, 0.75, 0.5, 0.95},
			{0.05, 0.5, 0.25, 0.5}, {0.75, 0.5, 0.95, 0.5},
		} {
			line(s*d[0], s*d[1], s*d[2], s*d[3])
		}
	case "waterfall":
		for i, h := range []float32{0.45, 0.75, 0.6} {
			x := s * (0.2 + 0.3*float32(i))
			line(x, s*0.9, x, s*(0.9-h))
		}
	case "doc", "log", "pcap":
		stroke(box(s*0.15, s*0.05, s*0.85, s*0.95))
		for _, y := range []float32{0.3, 0.5, 0.7} {
			line(s*0.3, s*y, s*0.7, s*y)
		}
	case "send", "fleet":
		fill(path(func(p *clip.Path) {
			p.MoveTo(f32.Pt(s*0.05, s*0.5))
			p.LineTo(f32.Pt(s*0.95, s*0.1))
			p.LineTo(f32.Pt(s*0.6, s*0.9))
			p.LineTo(f32.Pt(s*0.45, s*0.6))
			p.Close()
		}))
	case "coverage":
		dot(s*0.5, s*0.5, s*0.1)
		circle(s*0.5, s*0.5, s*0.28)
		circle(s*0.5, s*0.5, s*0.45)
	case "route":
		dot(s*0.15, s*0.85, s*0.11)
		dot(s*0.85, s*0.15, s*0.11)
		stroke(path(func(p *clip.Path) {
			p.MoveTo(f32.Pt(s*0.15, s*0.85))
			p.LineTo(f32.Pt(s*0.5, s*0.6))
			p.LineTo(f32.Pt(s*0.55, s*0.35))
			p.LineTo(f32.Pt(s*0.85, s*0.15))
		}))
	case "boundary":
		stroke(box(s*0.12, s*0.12, s*0.88, s*0.88))
		dot(s*0.12, s*0.12, s*0.09)
		dot(s*0.88, s*0.12, s*0.09)
		dot(s*0.12, s*0.88, s*0.09)
		dot(s*0.88, s*0.88, s*0.09)
	case "panel":
		stroke(box(s*0.1, s*0.15, s*0.9, s*0.85))
		line(s*0.1, s*0.35, s*0.9, s*0.35)
	case "grid":
		stroke(box(s*0.1, s*0.1, s*0.42, s*0.42))
		stroke(box(s*0.58, s*0.1, s*0.9, s*0.42))
		stroke(box(s*0.1, s*0.58, s*0.42, s*0.9))
		stroke(box(s*0.58, s*0.58, s*0.9, s*0.9))
	case "sliders", "settings":
		for i, f := range []float32{0.25, 0.5, 0.75} {
			line(s*0.1, s*f, s*0.9, s*f)
			x := s * []float32{0.65, 0.3, 0.55}[i]
			dot(x, s*f, s*0.13)
		}
	case "nodes":
		dot(s*0.5, s*0.2, s*0.12)
		dot(s*0.2, s*0.8, s*0.12)
		dot(s*0.8, s*0.8, s*0.12)
		line(s*0.5, s*0.2, s*0.2, s*0.8)
		line(s*0.5, s*0.2, s*0.8, s*0.8)
	case "help":
		circle(s*0.5, s*0.5, s*0.4)
		dot(s*0.5, s*0.5, s*0.1)
	default:
		// An unnamed glyph draws as a quiet dot rather than nothing, so a
		// typo in an icon name is visible instead of silent.
		dot(s*0.5, s*0.5, s*0.1)
	}
	return layout.Dimensions{Size: cell}
}

var _ = op.Offset(image.Point{})
