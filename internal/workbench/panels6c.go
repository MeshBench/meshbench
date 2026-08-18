// Configuration and the experiment log (6.17, 6.14).
package workbench

import (
	"fmt"
	"image"
	"strconv"
	"strings"
	"sync/atomic"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/MeshBench/meshbench/internal/gui/comp"
	"github.com/MeshBench/meshbench/internal/gui/shell"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
)

// configPanel is what this run is, in the terms that change a result (6.17) -
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
	p.envDir.Hint, p.envDir.Label = "a tile directory from tools/envgen", "Environment tiles"
	p.envDir.Editor.SingleLine = true
	p.loadEnv.Label, p.loadEnv.Kind = "load buildings", comp.Secondary
	p.dropEnv.Label, p.dropEnv.Kind = "bare earth", comp.Quiet
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

// update reconciles the switches with the store and fires the controls.
func (p *configPanel) update(gtx layout.Context, s *state.Snapshot) {
	// A switch follows the session unless somebody has just moved it, so a
	// run that turned the GPU off is not fought by a control that thinks it
	// is on.
	if p.gpu.Bool.Update(gtx) {
		p.wasGPU = p.gpu.Bool.Value
		if p.do != nil {
			p.do("gpu.set", map[string]any{"on": p.gpu.Bool.Value})
		}
	} else if s.GPU.Enabled != p.wasGPU {
		p.wasGPU = s.GPU.Enabled
		p.gpu.Bool.Value = s.GPU.Enabled
	}
	if p.realFW.Bool.Update(gtx) {
		p.wasReal = p.realFW.Bool.Value
		if p.do != nil {
			p.do("sim.kind", map[string]any{"real": p.realFW.Bool.Value})
		}
	} else if s.RealFirmware != p.wasReal {
		p.wasReal = s.RealFirmware
		p.realFW.Bool.Value = s.RealFirmware
	}

	numeric := []struct {
		btn   *comp.Button
		field *comp.Field
		verb  string
		key   string
		min   float64
	}{
		{&p.setSeed, &p.seed, "sim.seed", "seed", 1},
		{&p.setSpeed, &p.speed, "sim.speed", "step_ms", 1},
		{&p.setMargin, &p.margin, "study.margin", "km", 0},
		{&p.setExcess, &p.excess, "rf.excess_loss", "db", 0},
		{&p.setCache, &p.cacheGBf, "terrain.cache", "gb", 0.25},
	}
	for _, n := range numeric {
		if !n.btn.Click.Clicked(gtx) || p.do == nil {
			continue
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(fieldText(n.field)), 64)
		if err != nil || v < n.min {
			n.field.Error = "a number, at least " + trim0(n.min)
			continue
		}
		n.field.Error = ""
		p.do(n.verb, map[string]any{n.key: v})
	}
	// The realism switches travel as one apply: empty boxes leave a knob
	// where it is, so one change does not restate the rest.
	if p.setRealism.Click.Clicked(gtx) && p.do != nil {
		params := map[string]any{}
		for _, f := range []struct {
			field *comp.Field
			key   string
		}{
			{&p.oscPPM, "osc_ppm"}, {&p.multipathDB, "multipath_db"},
			{&p.fadingHz, "fading_hz"}, {&p.implLoss, "impl_loss_db"},
			{&p.satDBm, "saturation_dbm"},
		} {
			if v, err := strconv.ParseFloat(strings.TrimSpace(fieldText(f.field)), 64); err == nil {
				params[f.key] = v
			}
		}
		// Fired even with every box empty: an apply that changes nothing is
		// a no-op at the session, and a button that sometimes reaches no
		// verb is what the control audit exists to refuse.
		p.do("rf.realism", params)
	}
	if p.loadEnv.Click.Clicked(gtx) && p.do != nil {
		if dir := strings.TrimSpace(fieldText(&p.envDir)); dir != "" {
			p.do("rf.environment", map[string]any{"dir": dir})
		} else {
			p.do("rf.environment", map[string]any{"on": false})
		}
	}
	if p.dropEnv.Click.Clicked(gtx) && p.do != nil {
		p.do("rf.environment", map[string]any{"on": false})
	}
	// A directory is a thing to point at, not a path to remember and type.
	if p.browseCache.Click.Clicked(gtx) && shell.Browse != nil {
		start := strings.TrimSpace(fieldText(&p.cacheDir))
		go func() {
			got, err := shell.Browse("Where should the tiles live?", start,
				shell.PathAsk{Kind: shell.PathDirectory})
			p.pickedCache.Store(&pickResult{path: got, err: err})
		}()
	}
	if r, _ := p.pickedCache.Swap((*pickResult)(nil)).(*pickResult); r != nil {
		switch {
		case r.err != nil:
			p.cacheDir.Error = "could not open a file dialog: " + r.err.Error()
		case r.path != "":
			p.cacheDir.Editor.SetText(r.path)
			p.cacheDir.Error = ""
		}
	}
	if p.moveCache.Click.Clicked(gtx) && p.do != nil {
		dir := strings.TrimSpace(fieldText(&p.cacheDir))
		if dir == "" {
			p.cacheDir.Error = "where should the tiles live?"
		} else {
			p.cacheDir.Error = ""
			p.do("terrain.cache_dir", map[string]any{"path": dir})
		}
	}
	if p.recomp.Click.Clicked(gtx) && p.do != nil {
		p.do("links.recompute", nil)
	}
	if p.setScale.Click.Clicked(gtx) && p.sets != nil {
		if v, err := strconv.ParseFloat(strings.TrimSpace(fieldText(&p.scale)), 64); err == nil && v >= 0.5 && v <= 3 {
			p.scale.Error = ""
			p.sets.setScale(v)
		} else {
			p.scale.Error = "between 0.5 and 3"
		}
	}

	// The dropdowns: current value from the snapshot, choosing through the
	// shell's chooser.
	if s.GPU.Present {
		if s.GPU.Enabled {
			p.device.Value = s.GPU.Device + " (" + s.GPU.Backend + ")"
		} else {
			p.device.Value = "processor only - " + s.GPU.Device + " idle"
		}
	} else {
		p.device.Value = "processor only"
	}
	if s.RFMode == "waveform" {
		p.rfModeDD.Value = "waveform - demodulator verdicts"
	} else {
		p.rfModeDD.Value = "calculated - link-budget verdicts"
	}
	p.rfModeDD.OnOpen = func() {
		if p.choose == nil {
			return
		}
		p.choose("Reception is decided by", []string{
			"calculated - fast link budgets, scales to thousands of nodes",
			"waveform - IQ through the channel, verdict by the demodulator",
		}, func(picked string) {
			if p.do == nil {
				return
			}
			mode := "calculated"
			if strings.HasPrefix(picked, "waveform") {
				mode = "waveform"
			}
			p.do("rf.mode", map[string]any{"mode": mode})
		})
	}
	p.device.OnOpen = func() {
		if p.choose == nil || !s.GPU.Present {
			return
		}
		gpuOpt := s.GPU.Device + " (" + s.GPU.Backend + ")"
		p.choose("Measure links on", []string{gpuOpt, "processor only"},
			func(picked string) {
				if p.do != nil {
					p.do("gpu.set", map[string]any{"on": picked == gpuOpt})
				}
			})
	}
	p.cacheDD.Value = fmt.Sprintf("%.3g GB", cacheGB(s))
	p.cacheDD.OnOpen = func() {
		if p.choose == nil {
			return
		}
		p.choose("Tile cache size", []string{"2 GB", "5 GB", "10 GB", "20 GB"},
			func(picked string) {
				if v, err := strconv.ParseFloat(strings.Fields(picked)[0], 64); err == nil && p.do != nil {
					p.do("terrain.cache", map[string]any{"gb": v})
				}
			})
	}
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
		p.fieldRow(t, &p.cacheGBf, &p.setCache, ""),
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

// logPanel is what has happened in this session, newest last (6.14).
//
// The store's own log rather than a second one kept by the interface: every
// verb that says something says it there, so a script and a click leave the
// same trace.
type logPanel struct {
	list widget.List
}

func (p *logPanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if s == nil || len(s.Log) == 0 {
		return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Body, t.P.Dim,
			"nothing has happened yet"))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(comp.SectionTitle(t, "experiment log")),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			p.list.Axis = layout.Vertical
			return comp.List(t, &p.list, len(s.Log),
				func(gtx layout.Context, i int) layout.Dimensions {
					// Oldest first, so reading downwards is reading forwards.
					return comp.Mono(t, t.Sz.Caption, t.P.Dim, s.Log[i])(gtx)
				})(gtx)
		}),
	)
}
