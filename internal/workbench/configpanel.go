// The Configuration panel: what this run is, in the terms that change a result.
//
// A sidebar of sections, cards of labelled values with the reason each matters
// underneath, and every value settable where it is shown - through the same verb
// a script would use. The cards themselves are in configcards.go; the interaction
// pass is in configupdate.go.
package workbench

import (
	"image"
	"strings"
	"sync/atomic"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/MeshBench/meshbench/internal/gui/comp"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
)

// configPanel is what this run is, in the terms that change a result -
// redesigned to the mock: a sidebar of sections, cards of labelled values with
// the reason each matters underneath, and every value settable where it is
// shown, through the same verb a script would use.
type configPanel struct {
	do Do
	// choose opens the shell's chooser; the dropdowns hand their choosing to
	// it so there is exactly one way anything picks from a list.
	choose func(title string, opts []string, pick func(string))
	// sets is the interface's own settings - theme, density, scale - which
	// live here now that the separate Settings panel is gone.
	sets *settings

	// secRows are plain clickables rather than buttons: they change which
	// section is open, not the world, so the control audit - which asks
	// whether a control reaches a verb - leaves them to their own test.
	secRows []widget.Clickable
	active  int
	scroll  widget.List

	gpu         comp.Check
	realFW      comp.Check
	seed        comp.Field
	setSeed     comp.Button
	speed       comp.Field
	setSpeed    comp.Button
	margin      comp.Field
	setMargin   comp.Button
	excess      comp.Field
	setExcess   comp.Button
	covRes      comp.Field
	setCovRes   comp.Button
	rfModeDD    comp.Dropdown
	oscPPM      comp.Field
	multipathDB comp.Field
	fadingHz    comp.Field
	implLoss    comp.Field
	satDBm      comp.Field
	setRealism  comp.Button
	envDir      comp.Field
	loadEnv     comp.Button
	dropEnv     comp.Button
	envSrcDD    comp.Dropdown
	envUseDD    comp.Dropdown
	device      comp.Dropdown
	cacheDD     comp.Dropdown
	themeDD     comp.Dropdown
	densityDD   comp.Dropdown
	scale       comp.Field
	setScale    comp.Button
	cacheGBf    comp.Field
	setCache    comp.Button
	cacheDir    comp.Field
	moveCache   comp.Button
	browseCache comp.Button
	// pickedCache carries a browse answer back from the goroutine the
	// dialog blocks on, to be read at the top of the next frame.
	pickedCache atomic.Value
	recomp      comp.Button

	init            bool
	wasGPU, wasReal bool
}

// configSections is the sidebar, in the mock's order. Overview first, then
// the simulation's own terms, then the machine's.
var configSections = []string{
	"Overview", "General", "Nodes", "Links", "Environment", "RF Simulation",
	"Time", "Seed", "Graphics", "Events", "System", "Interface",
}

// configHeads is which sidebar rows get a heading above them.
var configHeads = map[int]string{1: "simulation", 8: "advanced"}

func (p *configPanel) build() {
	p.secRows = make([]widget.Clickable, len(configSections))
	p.gpu.Label = "measure links on the GPU"
	p.realFW.Label = "real firmware on every node"
	p.seed.Hint, p.seed.Label = "seed", "Seed"
	p.setSeed.Label, p.setSeed.Kind = "set seed", comp.Secondary
	p.speed.Hint, p.speed.Label, p.speed.Suffix = "ms per tick", "Speed", "ms"
	p.setSpeed.Label, p.setSpeed.Kind = "set speed", comp.Secondary
	p.margin.Hint, p.margin.Label, p.margin.Suffix = "km", "Study margin", "km"
	p.setMargin.Label, p.setMargin.Kind = "set margin", comp.Secondary
	p.excess.Hint, p.excess.Label, p.excess.Suffix = "dB", "Excess path loss", "dB"
	p.setExcess.Label, p.setExcess.Kind = "set loss", comp.Secondary
	p.covRes.Hint, p.covRes.Label, p.covRes.Suffix = "64 to 1024", "Coverage resolution", "cells"
	p.covRes.Editor.SingleLine = true
	p.setCovRes.Label, p.setCovRes.Kind = "set resolution", comp.Secondary
	p.rfModeDD.Label = "RF mode"
	p.oscPPM.Hint, p.oscPPM.Label, p.oscPPM.Suffix = "0 is perfect", "Oscillator error", "ppm"
	p.multipathDB.Hint, p.multipathDB.Label, p.multipathDB.Suffix = "0 is off", "Multipath echo", "dB down"
	p.fadingHz.Hint, p.fadingHz.Label, p.fadingHz.Suffix = "0 is static", "Fading rate", "Hz"
	p.implLoss.Hint, p.implLoss.Label, p.implLoss.Suffix = "0 is ideal", "Implementation loss", "dB"
	p.satDBm.Hint, p.satDBm.Label, p.satDBm.Suffix = "0 is unmodelled", "Front-end saturation", "dBm"
	for _, f := range []*comp.Field{&p.oscPPM, &p.multipathDB, &p.fadingHz, &p.implLoss, &p.satDBm} {
		f.Editor.SingleLine = true
	}
	p.setRealism.Label, p.setRealism.Kind = "apply realism", comp.Secondary
	p.device.Label = "Graphics device"
	p.cacheDD.Label = "Tile cache"
	p.themeDD.Label = "Theme"
	p.densityDD.Label = "Density"
	p.scale.Hint, p.scale.Label = "scale, 0.5 to 3", "Scale"
	p.scale.Editor.SingleLine = true
	p.setScale.Label, p.setScale.Kind = "set scale", comp.Secondary
	p.cacheGBf.Hint, p.cacheGBf.Label, p.cacheGBf.Suffix = "GB", "Tile cache", "GB"
	p.setCache.Label, p.setCache.Kind = "set cache", comp.Secondary
	p.cacheDir.Hint, p.cacheDir.Label = "a directory path", "Move the cache to"
	p.cacheDir.Editor.SingleLine = true
	p.moveCache.Label, p.moveCache.Kind = "move the cache", comp.Secondary
	p.browseCache.Label, p.browseCache.Kind = "browse...", comp.Quiet
	p.recomp.Label, p.recomp.Kind = "measure every link again", comp.Secondary
	for _, f := range []*comp.Field{&p.seed, &p.speed, &p.margin, &p.excess, &p.cacheGBf} {
		f.Editor.SingleLine = true
	}
	p.scroll.Axis = layout.Vertical
	p.init = true
}

func (p *configPanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !p.init {
		p.build()
	}
	if s == nil {
		return layout.Dimensions{}
	}
	p.update(gtx, s)
	for i := range p.secRows {
		if p.secRows[i].Clicked(gtx) {
			p.active = i
		}
	}
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Dp(170)
			gtx.Constraints.Max.X = gtx.Dp(170)
			return p.sidebar(t, gtx)
		}),
		layout.Rigid(comp.VRule(t)),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: t.Sp.M, Right: t.Sp.M, Top: t.Sp.S}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return p.section(t, gtx, s)
				})
		}),
	)
}

// sidebar is the section list, with the standing honesty note at the bottom.
func (p *configPanel) sidebar(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	var rows []layout.FlexChild
	for i := range p.secRows {
		i := i
		if h, ok := configHeads[i]; ok {
			rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: t.Sp.M, Bottom: t.Sp.XS, Left: t.Sp.S}.Layout(gtx,
					comp.Text(t, t.Sz.Caption, t.P.Faint, strings.ToUpper(h)))
			}))
		}
		rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.sideRow(t, gtx, i)
		}))
	}
	rows = append(rows,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(t.Sp.S).Layout(gtx, comp.Card(t, "Best case",
				comp.Text(t, t.Sz.Caption, t.P.Warn,
					"no multipath, bare earth, ideal demodulator")))
		}),
	)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
}

// sideRow is one section entry, tinted when it is the open one.
func (p *configPanel) sideRow(t *theme.Theme, gtx layout.Context, i int) layout.Dimensions {
	b := &p.secRows[i]
	return b.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		fg := t.P.Dim
		if i == p.active || b.Hovered() {
			fg = t.P.Ink
		}
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		macro := op.Record(gtx.Ops)
		dims := layout.Inset{
			Top: t.Sp.S, Bottom: t.Sp.S, Left: t.Sp.M, Right: t.Sp.S,
		}.Layout(gtx, comp.Text(t, t.Sz.Body, fg, configSections[i]))
		call := macro.Stop()
		if i == p.active {
			comp.FillRect(gtx, dims.Size, theme.Alpha(t.P.Accent, 0.10))
			comp.FillRect(gtx, image.Pt(gtx.Dp(2), dims.Size.Y), t.P.Accent)
		}
		call.Add(gtx.Ops)
		return dims
	})
}

// section draws the open section's cards inside the scroll list.
func (p *configPanel) section(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	var cards []layout.Widget
	switch configSections[p.active] {
	case "Overview":
		cards = p.overview(t, s)
	case "General":
		cards = p.general(t, s)
	case "Nodes":
		cards = p.nodesCards(t, s)
	case "Links":
		cards = p.linksCards(t, s)
	case "Environment":
		cards = p.environment(t, s)
	case "RF Simulation":
		cards = p.rfSimulation(t, s)
	case "Time":
		cards = p.timeCards(t, s)
	case "Seed":
		cards = p.seedCards(t, s)
	case "Graphics":
		cards = p.graphics(t, s)
	case "Events":
		cards = p.eventsCards(t, s)
	case "System":
		cards = p.system(t, s)
	case "Interface":
		cards = p.interfaceCards(t, s)
	}
	return comp.List(t, &p.scroll, len(cards), func(gtx layout.Context, i int) layout.Dimensions {
		return layout.Inset{Bottom: t.Sp.M}.Layout(gtx, cards[i])
	})(gtx)
}

// pillWord is the state of the run, in one word - separate from the drawing
// so the test reads the same decision the pill does.
func pillWord(s *state.Snapshot) string {
	for _, j := range s.Jobs {
		if j.ID == "links" && !j.Finished {
			return "Warming up"
		}
	}
	if s.Playing {
		return "Running"
	}
	return "Ready to run"
}

// runPill is that state as a capsule.
func runPill(t *theme.Theme, s *state.Snapshot) layout.Widget {
	switch word := pillWord(s); word {
	case "Warming up":
		return comp.Pill(t, t.P.Warn, word)
	case "Running":
		return comp.Pill(t, t.P.Accent, word)
	default:
		return comp.Pill(t, t.P.Good, word)
	}
}

// Open opens a named section, for the capture flag: a section that only
// opens on a click is a section nobody can check without a hand on the mouse.
func (p *configPanel) Open(name string) {
	for i, s := range configSections {
		if strings.EqualFold(s, name) {
			p.active = i
		}
	}
}

// auditDraw is every control this panel owns, laid flat with no sections, so
// the audit presses each one regardless of which section happens to be open.
// The real layout's section switching has its own test beside the audit.
func (p *configPanel) auditDraw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !p.init {
		p.build()
	}
	if s == nil {
		return layout.Dimensions{}
	}
	p.update(gtx, s)
	// The Interface dropdowns fill their values inside interfaceCards, which
	// the flat layout also needs before drawing them.
	if p.sets != nil {
		_ = p.interfaceCards(t, s)
	}
	rows := []layout.Widget{
		p.fieldRow(t, &p.seed, &p.setSeed, ""),
		p.fieldRow(t, &p.speed, &p.setSpeed, ""),
		p.fieldRow(t, &p.margin, &p.setMargin, ""),
		p.fieldRow(t, &p.excess, &p.setExcess, ""),
		p.fieldRow(t, &p.oscPPM, &p.setRealism, ""),
		p.fieldRow(t, &p.multipathDB, nil, ""),
		p.fieldRow(t, &p.fadingHz, nil, ""),
		p.fieldRow(t, &p.implLoss, nil, ""),
		p.fieldRow(t, &p.satDBm, nil, ""),
		p.fieldRow(t, &p.envDir, &p.loadEnv, ""),
		func(gtx layout.Context) layout.Dimensions { return p.dropEnv.Layout(t, gtx) },
		func(gtx layout.Context) layout.Dimensions { return p.envSrcDD.Layout(t, gtx) },
		func(gtx layout.Context) layout.Dimensions { return p.envUseDD.Layout(t, gtx) },
		p.fieldRow(t, &p.cacheGBf, &p.setCache, ""),
		p.fieldRow(t, &p.covRes, &p.setCovRes, ""),
		p.fieldRow(t, &p.cacheDir, &p.moveCache, ""),
		func(gtx layout.Context) layout.Dimensions { return p.recomp.Layout(t, gtx) },
		func(gtx layout.Context) layout.Dimensions { return p.gpu.LayoutSwitch(t, gtx) },
		func(gtx layout.Context) layout.Dimensions { return p.realFW.LayoutSwitch(t, gtx) },
		func(gtx layout.Context) layout.Dimensions { return p.device.Layout(t, gtx) },
		func(gtx layout.Context) layout.Dimensions { return p.rfModeDD.Layout(t, gtx) },
		func(gtx layout.Context) layout.Dimensions { return p.cacheDD.Layout(t, gtx) },
		p.fieldRow(t, &p.scale, &p.setScale, ""),
		func(gtx layout.Context) layout.Dimensions { return p.themeDD.Layout(t, gtx) },
		func(gtx layout.Context) layout.Dimensions { return p.densityDD.Layout(t, gtx) },
	}
	// Two columns, because the flat list has outgrown the audit's reach: a
	// control below the pointer sweep reads as unreachable, and it would be
	// the audit that was wrong.
	col := func(rows []layout.Widget) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			var kids []layout.FlexChild
			for _, r := range rows {
				r := r
				kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Bottom: t.Sp.XS}.Layout(gtx, r)
				}))
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
		}
	}
	half := (len(rows) + 1) / 2
	left, right := col(rows[:half]), col(rows[half:])
	return layout.Flex{}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: t.Sp.M}.Layout(gtx, left)
		}),
		layout.Flexed(1, right),
	)
}
