package shell

import (
	"image"
	"image/color"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"

	"github.com/A13xB0/meshcoresim/internal/gui/comp"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

// The transport controls: play, pause, step, slower, faster.
//
// In the menu bar, at the top, because what a simulator is doing is the most
// urgent thing on the screen. A simulator whose play button is only a keyboard
// shortcut looks like a simulator that cannot be played.
type transport struct {
	play, step, slow, fast widget.Clickable
}

// transportBar draws the controls. One row tall, said explicitly: as a rigid
// child of a vertical flex it would otherwise inherit the whole remaining
// height and squeeze the body to nothing, which is exactly what it did.
func (sh *Shell) transportBar(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	fire := func(a string) {
		if sh.OnMenu != nil {
			sh.OnMenu(a)
		}
	}
	if sh.tr.play.Clicked(gtx) {
		fire("sim.toggle")
	}
	if sh.tr.step.Clicked(gtx) {
		fire("sim.step")
	}
	if sh.tr.slow.Clicked(gtx) {
		fire("sim.slower")
	}
	if sh.tr.fast.Clicked(gtx) {
		fire("sim.faster")
	}

	side := gtx.Dp(t.RowHeight())
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = side, side

	btn := func(c *widget.Clickable, sym symbol, accent bool) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return c.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				fg := t.P.Ink
				if accent {
					fg = t.P.Accent
				}
				if c.Hovered() {
					comp.FillRect(gtx, image.Pt(side, side), theme.Alpha(t.P.Ink, 0.08))
				}
				drawSymbol(gtx, sym, side, fg)
				return layout.Dimensions{Size: image.Pt(side, side)}
			})
		})
	}
	// The symbol says what pressing it will do, not what the state is: a
	// control showing the current state makes somebody work out which reading
	// it is.
	sym := symPlay
	if s != nil && s.Playing {
		sym = symPause
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		btn(&sh.tr.play, sym, true),
		btn(&sh.tr.step, symStep, false),
		btn(&sh.tr.slow, symSlower, false),
		btn(&sh.tr.fast, symFaster, false),
	)
}

// symbol is a transport glyph, drawn rather than typed.
//
// The media symbols live in ranges the UI face does not carry, so a text
// button would fall back to a box or to the emoji face, and a control that
// renders as a tofu square on somebody else's machine is worse than a word.
type symbol int

const (
	symPlay symbol = iota
	symPause
	symStep
	symSlower
	symFaster
)

func drawSymbol(gtx layout.Context, s symbol, box int, col color.NRGBA) {
	side := float32(box)
	r := side * 0.2
	cx, cy := side/2, side/2

	tri := func(p *clip.Path, ox float32, dir float32) {
		p.MoveTo(f32.Pt(cx+ox-r*0.7*dir, cy-r))
		p.LineTo(f32.Pt(cx+ox+r*0.9*dir, cy))
		p.LineTo(f32.Pt(cx+ox-r*0.7*dir, cy+r))
		p.Close()
	}
	bar := func(p *clip.Path, ox, w float32) {
		p.MoveTo(f32.Pt(cx+ox, cy-r))
		p.LineTo(f32.Pt(cx+ox+w, cy-r))
		p.LineTo(f32.Pt(cx+ox+w, cy+r))
		p.LineTo(f32.Pt(cx+ox, cy+r))
		p.Close()
	}

	var p clip.Path
	p.Begin(gtx.Ops)
	switch s {
	case symPlay:
		tri(&p, 0, 1)
	case symPause:
		bar(&p, -r*0.8, r*0.5)
		bar(&p, r*0.3, r*0.5)
	case symStep:
		tri(&p, -r*0.3, 1)
		bar(&p, r*0.7, r*0.35)
	case symSlower:
		tri(&p, r*0.5, -1)
		tri(&p, -r*0.5, -1)
	case symFaster:
		tri(&p, -r*0.5, 1)
		tri(&p, r*0.5, 1)
	}
	paint.FillShape(gtx.Ops, col, clip.Outline{Path: p.End()}.Op())
}

var _ = op.Offset
