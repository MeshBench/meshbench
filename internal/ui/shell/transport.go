package shell

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// The transport controls: play, pause, step, slower, faster.
//
// In the menu bar, at the top, because what a simulator is doing is the most
// urgent thing on the screen. A simulator whose play button is only a keyboard
// shortcut looks like a simulator that cannot be played.
type transport struct {
	play, step, slow, fast widget.Clickable
	restart, real          widget.Clickable
	// rewarm forces a fresh link measurement without waiting for a run to
	// hit a gap on its own. The identical control already lived in
	// Configuration, buried where nobody thinks to look mid-run - which
	// matters because a carried matrix is a head start, not a guarantee:
	// it can predate what a firmware node's real radio configuration turns
	// out to be, and this is the manual way back once that has happened.
	rewarm widget.Clickable
	// confirmRestart makes the second press the destructive one. A run is
	// cheap to lose and expensive to notice you have lost.
	confirmRestart bool
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
		// sim.start, not sim.toggle: play is one decision, and if this is a
		// real-firmware run it starts MeshCore before advancing the clock.
		fire("sim.start")
	}
	if sh.tr.restart.Clicked(gtx) {
		if sh.tr.confirmRestart {
			sh.tr.confirmRestart = false
			fire("sim.reset")
		} else {
			sh.tr.confirmRestart = true
		}
	}
	if sh.tr.real.Clicked(gtx) {
		fire("ui.toggle_real_firmware")
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
	if sh.tr.rewarm.Clicked(gtx) {
		fire("links.recompute")
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
	// Warming wins over both. Until the matrix is measured a run would spend
	// its first transmission computing every path loss in the network, so the
	// control says what is happening rather than looking idle.
	warm := warmingJob(s)
	if warm != nil {
		sym = symWarming
	}
	// Anything but the transport itself cancels a pending restart, so a
	// half-pressed confirm cannot sit there waiting to catch somebody out.
	if s != nil && s.Playing {
		sh.tr.confirmRestart = false
	}

	children := []layout.FlexChild{
		btn(&sh.tr.play, sym, true),
		btn(&sh.tr.step, symStep, false),
		btn(&sh.tr.restart, symRestart, sh.tr.confirmRestart),
		btn(&sh.tr.slow, symSlower, false),
		btn(&sh.tr.fast, symFaster, false),
	}
	if warm != nil {
		children = append(children,
			layout.Rigid(chip(t, t.P.Warn, warmingWords(warmingNow(s, warm)))))
	} else if sh.tr.confirmRestart {
		children = append(children, layout.Rigid(chip(t, t.P.Bad, "press again to discard the run")))
	} else {
		children = append(children, layout.Rigid(speedChip(t, s)))
		children = append(children, layout.Rigid(realFirmware(t, &sh.tr.real, s)))
		children = append(children, layout.Rigid(rewarmButton(t, &sh.tr.rewarm)))
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
}

// chip is a small piece of text in the bar, vertically centred.
func chip(t *theme.Theme, col color.NRGBA, s string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: t.Sp.S, Right: t.Sp.XS}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Caption, col, s))
			})
	}
}

// speedChip says how fast simulated time is running, in the engine's own unit.
//
// "1x" would be a claim about wall time that a simulator advancing on a ticker
// cannot honestly make; milliseconds per tick is what the two arrows actually
// change.
func speedChip(t *theme.Theme, s *state.Snapshot) layout.Widget {
	ms := uint32(10)
	if s != nil && s.StepMs > 0 {
		ms = s.StepMs
	}
	// Dim rather than Faint: this and the rewarm beside it are the same rank
	// of thing as the node count and the seed at the other end of the bar, and
	// a row of facts set in two greys reads as one of them being disabled.
	return chip(t, t.P.Dim, fmt.Sprintf("%d ms/tick", ms))
}

// realFirmware is what kind of run this is, stated once beside the transport.
//
// Once processes are up it stops being a choice and becomes a fact: changing
// it would mean restarting several hundred of them, so it reports instead.
func realFirmware(t *theme.Theme, c *widget.Clickable, s *state.Snapshot) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if s == nil {
			return layout.Dimensions{}
		}
		if s.FirmwareStarting {
			return chip(t, t.P.Warn, "starting MeshCore...")(gtx)
		}
		if s.FirmwareRunning > 0 {
			return chip(t, t.P.Good,
				fmt.Sprintf("%d on MeshCore", s.FirmwareRunning))(gtx)
		}
		return c.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			fg := t.P.Faint
			if s.RealFirmware {
				fg = t.P.Accent
			}
			if c.Hovered() {
				fg = t.P.Ink
			}
			mark := "OFF"
			if s.RealFirmware {
				mark = "ON"
			}
			return layout.Inset{Left: t.Sp.S, Right: t.Sp.S}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Center.Layout(gtx,
						comp.Text(t, t.Sz.Caption, fg, "real firmware "+mark))
				})
		})
	}
}

// rewarmButton fires links.recompute: the same "measure every link again"
// control Configuration has always had, placed beside the transport too -
// where an operator actually looks once a run has gone quiet - rather than
// only in a settings page nobody opens mid-run.
func rewarmButton(t *theme.Theme, c *widget.Clickable) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return c.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			fg := t.P.Dim
			if c.Hovered() {
				fg = t.P.Ink
			}
			return layout.Inset{Left: t.Sp.S, Right: t.Sp.S}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Center.Layout(gtx,
						comp.Text(t, t.Sz.Caption, fg, "rewarm links"))
				})
		})
	}
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
	symRestart
	// symWarming is a flame: the run cannot start yet because the path-loss
	// matrix is still being measured, and a play button that looks pressable
	// while nothing happens is the thing this exists to prevent.
	symWarming
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
	case symWarming:
		// A flame: two curves meeting at a point, filled. Rough on purpose -
		// eleven pixels of glyph does not need a bezier.
		p.MoveTo(f32.Pt(cx, cy-r*1.1))
		p.LineTo(f32.Pt(cx+r*0.75, cy+r*0.1))
		p.LineTo(f32.Pt(cx+r*0.45, cy+r*0.95))
		p.LineTo(f32.Pt(cx-r*0.45, cy+r*0.95))
		p.LineTo(f32.Pt(cx-r*0.75, cy+r*0.1))
		p.Close()
		p.MoveTo(f32.Pt(cx, cy-r*0.1))
		p.LineTo(f32.Pt(cx+r*0.3, cy+r*0.45))
		p.LineTo(f32.Pt(cx, cy+r*0.95))
		p.LineTo(f32.Pt(cx-r*0.3, cy+r*0.45))
		p.Close()
	case symRestart:
		// An open ring with an arrowhead: three quarters of a circle drawn as
		// a filled annulus, because a stroked arc costs Gio's whole stroke
		// machinery for eleven pixels of glyph.
		const seg = 24
		inner, outer := r*0.62, r*0.92
		start, sweep := -0.4*math.Pi, 1.6*math.Pi
		for i := 0; i <= seg; i++ {
			a := start + sweep*float64(i)/seg
			pt := f32.Pt(cx+outer*float32(math.Cos(a)), cy+outer*float32(math.Sin(a)))
			if i == 0 {
				p.MoveTo(pt)
			} else {
				p.LineTo(pt)
			}
		}
		for i := seg; i >= 0; i-- {
			a := start + sweep*float64(i)/seg
			p.LineTo(f32.Pt(cx+inner*float32(math.Cos(a)), cy+inner*float32(math.Sin(a))))
		}
		p.Close()
		// The head, so it reads as a direction rather than a broken ring.
		hx, hy := cx+outer*float32(math.Cos(start)), cy+outer*float32(math.Sin(start))
		p.MoveTo(f32.Pt(hx-r*0.1, hy-r*0.55))
		p.LineTo(f32.Pt(hx+r*0.5, hy+r*0.05))
		p.LineTo(f32.Pt(hx-r*0.45, hy+r*0.3))
		p.Close()
	}
	paint.FillShape(gtx.Ops, col, clip.Outline{Path: p.End()}.Op())
}

var _ = op.Offset

// warmingJob is the link measurement, if one is running.
func warmingJob(s *state.Snapshot) *state.Job {
	if s == nil {
		return nil
	}
	for i := range s.Jobs {
		if s.Jobs[i].ID == "links" && !s.Jobs[i].Finished {
			return &s.Jobs[i]
		}
	}
	return nil
}

// warmingNow is the job the warming chip should name.
//
// The measurement is what blocks a run, so it is what decides the spinner. It
// is not always what is taking the time: a warm over ground this machine has
// not got stops and downloads that ground first, and the measurement sits at
// zero for as long as that takes. The chip therefore read "measuring every
// link" through five hundred megabytes and never once said a download was
// running, which is exactly the misnaming the status bar was fixed for and the
// shape a stall has. Newest running wins here as it does there, so the strip
// and the bar cannot disagree about what the machine is doing.
func warmingNow(s *state.Snapshot, warm *state.Job) *state.Job {
	for i := len(s.Jobs) - 1; i >= 0; i-- {
		if !s.Jobs[i].Finished {
			return &s.Jobs[i]
		}
	}
	return warm
}

// warmingWords says how far it has got, because "warming up" with no end in
// sight and "warming up, 40,000 of 48,000" are different amounts of patience.
func warmingWords(j *state.Job) string {
	// A percentage rather than a bar: this sits in a one-row strip beside four
	// other controls, and it is a number somebody glances at rather than
	// watches.
	if j.Total > 0 && j.Done > 0 {
		pct := float64(j.Done) / float64(j.Total) * 100
		what := j.What
		if what == "" {
			what = "warming up"
		}
		return fmt.Sprintf("%s - %.0f%%", what, pct)
	}
	if j.What != "" {
		return j.What
	}
	return "warming up - measuring every link"
}
