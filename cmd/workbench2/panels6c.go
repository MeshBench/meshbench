// Configuration and the experiment log (6.17, 6.14).
package main

import (
	"fmt"
	"image"
	"strconv"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/A13xB0/meshcoresim/internal/gui/comp"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
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

	// secRows are plain clickables rather than buttons: they change which
	// section is open, not the world, so the control audit - which asks
	// whether a control reaches a verb - leaves them to their own test.
	secRows []widget.Clickable
	active  int
	scroll  widget.List

	gpu       comp.Check
	realFW    comp.Check
	seed      comp.Field
	setSeed   comp.Button
	speed     comp.Field
	setSpeed  comp.Button
	margin    comp.Field
	setMargin comp.Button
	excess    comp.Field
	setExcess comp.Button
	device    comp.Dropdown
	cacheDD   comp.Dropdown
	cacheGBf  comp.Field
	setCache  comp.Button
	cacheDir  comp.Field
	moveCache comp.Button
	recomp    comp.Button

	init             bool
	wasGPU, wasReal  bool
	haveGPU, haveRun bool
}

// configSections is the sidebar, in the mock's order. Overview first, then
// the simulation's own terms, then the machine's.
var configSections = []string{
	"Overview", "General", "Nodes", "Links", "Environment", "Time", "Seed",
	"Graphics", "Events", "System",
}

// configHeads is which sidebar rows get a heading above them.
var configHeads = map[int]string{1: "simulation", 7: "advanced"}

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
	p.device.Label = "Graphics device"
	p.cacheDD.Label = "Tile cache"
	p.cacheGBf.Hint, p.cacheGBf.Label, p.cacheGBf.Suffix = "GB", "Tile cache", "GB"
	p.setCache.Label, p.setCache.Kind = "set cache", comp.Secondary
	p.cacheDir.Hint, p.cacheDir.Label = "a directory path", "Move the cache to"
	p.cacheDir.Editor.SingleLine = true
	p.moveCache.Label, p.moveCache.Kind = "move the cache", comp.Secondary
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

func (p *configPanel) overview(t *theme.Theme, s *state.Snapshot) []layout.Widget {
	grid := func(cells ...layout.Widget) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			return comp.CellGrid(t, gtx, 190, cells)
		}
	}
	lastWarm := "not yet"
	warmCap := "nothing has been measured on it"
	if s.GPU.Pairs > 0 {
		lastWarm = fmt.Sprintf("%d pairs in %d ms", s.GPU.Pairs, s.GPU.Ms)
		warmCap = "what the last warm actually did"
	}
	return []layout.Widget{
		comp.Card(t, "", func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(comp.Text(t, t.Sz.Section, t.P.Ink, "Run profile")),
						layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Warn,
							"optimised for the cleanest possible channel: no multipath, bare earth, ideal demodulator")),
					)
				}),
				layout.Rigid(runPill(t, s)),
			)
		}),
		comp.Card(t, "Simulation scope", grid(
			comp.StatCell(t, "Study areas", itoa(len(s.Areas)), "what bounds the study"),
			comp.StatCell(t, "Study margin", fmt.Sprintf("%g km", s.MarginKm),
				"how far outside the boundary a node still matters"),
			comp.StatCell(t, "Nodes", itoa(len(s.Nodes)),
				"every one is simulated; none are sampled"),
		)),
		comp.Card(t, "Links & measurement", func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(grid(
					comp.StatCell(t, "Links measured", itoa(len(s.Links)),
						"pairs with a path loss from the engine"),
					comp.StatCell(t, "Last warm on the GPU", lastWarm, warmCap),
					comp.StatCell(t, "Running", yesNo(s.Playing),
						"the engine advances on its own ticker"),
					comp.StatCell(t, "Simulated time",
						fmt.Sprintf("%.2f s", float64(s.NowMs)/1000),
						"not wall time, and never has been"),
				)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: t.Sp.S}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							return p.gpu.LayoutSwitch(t, gtx)
						})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: t.Sp.XS}.Layout(gtx,
						comp.Text(t, t.Sz.Caption, t.P.Faint, gpuNote(s.GPU)))
				}),
			)
		}),
		comp.Card(t, "Randomness & variance", grid(
			comp.StatCell(t, "Seed", fmt.Sprintf("%d", s.Seed),
				"two runs with one seed are identical"),
			comp.StatCell(t, "Events", itoa(s.EventTotal),
				"the whole log; tables show the tail of it"),
		)),
		comp.Card(t, "Graphics & performance", func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Right: t.Sp.M}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return p.device.Layout(t, gtx)
								}),
								layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint,
									"every GPU path has a processor twin")),
							)
						})
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return p.cacheDD.Layout(t, gtx)
						}),
						layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint,
							"decoded terrain tiles held in memory")),
					)
				}),
			)
		}),
		comp.Card(t, "About the tile cache", comp.Text(t, t.Sz.Caption, t.P.Faint,
			fmt.Sprintf("a smaller cache uses less memory but may re-read tiles "+
				"from disk constantly - current cache %.3g GB at %s",
				cacheGB(s), orUnset(s.TileCacheDir)))),
	}
}

func (p *configPanel) general(t *theme.Theme, s *state.Snapshot) []layout.Widget {
	return []layout.Widget{
		comp.Card(t, "Run kind", func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.realFW.LayoutSwitch(t, gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: t.Sp.XS}.Layout(gtx,
						comp.Text(t, t.Sz.Caption, t.P.Faint,
							"on, play starts MeshCore on every node and each relay decision "+
								"is the firmware's own; off, the channel is real but nothing relays"))
				}),
			)
		}),
		comp.Card(t, "Speed", p.fieldRow(t, &p.speed, &p.setSpeed,
			fmt.Sprintf("simulated milliseconds per tick - now %d ms; independent "+
				"of the frame rate by design", s.StepMs))),
		comp.Card(t, "Study margin", p.fieldRow(t, &p.margin, &p.setMargin,
			fmt.Sprintf("now %g km - how far outside the boundary a node still matters", s.MarginKm))),
	}
}

func (p *configPanel) nodesCards(t *theme.Theme, s *state.Snapshot) []layout.Widget {
	kinds := map[string]int{}
	running := 0
	for i := range s.Nodes {
		kinds[s.Nodes[i].Kind]++
	}
	running = s.FirmwareRunning
	cells := []layout.Widget{
		comp.StatCell(t, "Nodes", itoa(len(s.Nodes)), "every one is simulated"),
		comp.StatCell(t, "Running firmware", itoa(running),
			"processes up right now"),
	}
	for _, k := range []string{"simple-repeater", "advanced-repeater", "companion",
		"room-server", "sdr-observer", "emitter"} {
		if kinds[k] > 0 {
			cells = append(cells, comp.StatCell(t, shortKind(k), itoa(kinds[k]), ""))
		}
	}
	return []layout.Widget{
		comp.Card(t, "The network", func(gtx layout.Context) layout.Dimensions {
			return comp.CellGrid(t, gtx, 170, cells)
		}),
	}
}

func (p *configPanel) linksCards(t *theme.Theme, s *state.Snapshot) []layout.Widget {
	return []layout.Widget{
		comp.Card(t, "The link matrix", func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return comp.CellGrid(t, gtx, 190, []layout.Widget{
						comp.StatCell(t, "Links measured", itoa(len(s.Links)),
							"pairs with a path loss from the engine, weighted by the weaker direction"),
						comp.StatCell(t, "Measured on", gpuOrCPU(s.GPU),
							"where the last warm ran"),
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: t.Sp.S}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							return p.recomp.Layout(t, gtx)
						})
				}),
			)
		}),
	}
}

func (p *configPanel) environment(t *theme.Theme, s *state.Snapshot) []layout.Widget {
	return []layout.Widget{
		comp.Card(t, "Excess path loss", p.fieldRow(t, &p.excess, &p.setExcess,
			"everything the bare-earth model does not contain: vegetation, buildings, "+
				"the ground not being a knife edge - fitted at 20 dB against 118 real "+
				"receptions; setting it rebuilds and re-measures")),
		comp.Card(t, "Elevation", comp.Text(t, t.Sz.Caption, t.P.Faint,
			"terrarium tiles at zoom 12, about 30 m per pixel at UK latitudes - "+
				"missing tiles answer \"no data\", which is bare earth for that "+
				"profile and says so")),
	}
}

func (p *configPanel) timeCards(t *theme.Theme, s *state.Snapshot) []layout.Widget {
	return []layout.Widget{
		comp.Card(t, "The clock", func(gtx layout.Context) layout.Dimensions {
			return comp.CellGrid(t, gtx, 190, []layout.Widget{
				comp.StatCell(t, "Simulated time",
					fmt.Sprintf("%.2f s", float64(s.NowMs)/1000),
					"not wall time, and never has been"),
				comp.StatCell(t, "Speed", fmt.Sprintf("%d ms/tick", s.StepMs),
					"how much simulated time one tick advances"),
			})
		}),
		comp.Card(t, "Speed", p.fieldRow(t, &p.speed, &p.setSpeed,
			"independent of the frame rate: the run must not go faster on a "+
				"better graphics card")),
	}
}

func (p *configPanel) seedCards(t *theme.Theme, s *state.Snapshot) []layout.Widget {
	return []layout.Widget{
		comp.Card(t, "Seed", p.fieldRow(t, &p.seed, &p.setSeed,
			fmt.Sprintf("now %d - two runs with one seed are identical by design; "+
				"setting it rebuilds the engine, and the measured matrix carries "+
				"over when the geometry has not changed", s.Seed))),
	}
}

func (p *configPanel) graphics(t *theme.Theme, s *state.Snapshot) []layout.Widget {
	g := s.GPU
	present := "none found"
	if g.Present {
		present = g.Device + " (" + g.Backend + ")"
	}
	cells := []layout.Widget{
		comp.StatCell(t, "Graphics device", present,
			"every GPU path has a processor twin that answers the same, more slowly"),
		comp.StatCell(t, "Links measured on the GPU", yesNo(g.Enabled),
			"forty-eight thousand independent profiles is the shape a compute shader is for"),
	}
	if g.Pairs > 0 {
		cells = append(cells, comp.StatCell(t, "Last warm",
			fmt.Sprintf("%d pairs in %d ms", g.Pairs, g.Ms),
			"what the last warm actually did"))
	}
	return []layout.Widget{
		comp.Card(t, "The graphics path", func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return comp.CellGrid(t, gtx, 210, cells)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: t.Sp.S}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							return p.gpu.LayoutSwitch(t, gtx)
						})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: t.Sp.XS}.Layout(gtx,
						comp.Text(t, t.Sz.Caption, t.P.Faint, gpuNote(s.GPU)))
				}),
			)
		}),
	}
}

func (p *configPanel) eventsCards(t *theme.Theme, s *state.Snapshot) []layout.Widget {
	return []layout.Widget{
		comp.Card(t, "The event log", func(gtx layout.Context) layout.Dimensions {
			return comp.CellGrid(t, gtx, 190, []layout.Widget{
				comp.StatCell(t, "Events", itoa(s.EventTotal),
					"everything the engine did; the tables show the most recent"),
				comp.StatCell(t, "In the tables", itoa(len(s.Events)),
					"the tail, oldest first"),
			})
		}),
		comp.Card(t, "", comp.Text(t, t.Sz.Caption, t.P.Faint,
			"export the whole log from File > Export the event log; capture a "+
				"pcapng from Simulation > Capture to a pcapng file")),
	}
}

func (p *configPanel) system(t *theme.Theme, s *state.Snapshot) []layout.Widget {
	return []layout.Widget{
		comp.Card(t, "Tile cache size", p.fieldRow(t, &p.cacheGBf, &p.setCache,
			fmt.Sprintf("decoded terrain tiles held in memory - now %.3g GB; a cache "+
				"smaller than the study area re-reads tiles from disk constantly",
				cacheGB(s)))),
		comp.Card(t, "Tile cache location", func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim,
					"now at "+orUnset(s.TileCacheDir))),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: t.Sp.S}.Layout(gtx,
						p.fieldRow(t, &p.cacheDir, &p.moveCache,
							"moved as a visible job - renamed on the same filesystem, "+
								"copied then deleted across; nothing re-downloads"))
				}),
			)
		}),
		comp.Card(t, "Saved settings", comp.Text(t, t.Sz.Caption, t.P.Faint,
			"the GPU choice, the cache size and the cache location survive a "+
				"restart, in ~/.config/meshcoresim/workbench2.json; the scenario "+
				"itself deliberately stays in the fixture")),
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
	rows := []layout.Widget{
		p.fieldRow(t, &p.seed, &p.setSeed, ""),
		p.fieldRow(t, &p.speed, &p.setSpeed, ""),
		p.fieldRow(t, &p.margin, &p.setMargin, ""),
		p.fieldRow(t, &p.excess, &p.setExcess, ""),
		p.fieldRow(t, &p.cacheGBf, &p.setCache, ""),
		p.fieldRow(t, &p.cacheDir, &p.moveCache, ""),
		func(gtx layout.Context) layout.Dimensions { return p.recomp.Layout(t, gtx) },
		func(gtx layout.Context) layout.Dimensions { return p.gpu.LayoutSwitch(t, gtx) },
		func(gtx layout.Context) layout.Dimensions { return p.realFW.LayoutSwitch(t, gtx) },
		func(gtx layout.Context) layout.Dimensions { return p.device.Layout(t, gtx) },
		func(gtx layout.Context) layout.Dimensions { return p.cacheDD.Layout(t, gtx) },
	}
	var kids []layout.FlexChild
	for _, r := range rows {
		r := r
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: t.Sp.XS}.Layout(gtx, r)
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
}

// fieldRow is a field, its button and the reason underneath - the shape every
// settable value here takes.
func (p *configPanel) fieldRow(t *theme.Theme, f *comp.Field, b *comp.Button, why string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.End}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(280))
						return f.Layout(t, gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: t.Sp.S, Bottom: t.Sp.XXS}.Layout(gtx,
							func(gtx layout.Context) layout.Dimensions {
								return b.Layout(t, gtx)
							})
					}),
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: t.Sp.XS}.Layout(gtx,
					comp.Text(t, t.Sz.Caption, t.P.Faint, why))
			}),
		)
	}
}

// gpuNote is why the last warm did not use the GPU, when it did not.
func gpuNote(g state.GPUState) string {
	switch {
	case !g.Present:
		return "no graphics device: " + g.Why
	case g.Enabled && g.Why != "":
		return "the last warm used the processor: " + g.Why
	case g.Enabled:
		return "the kernel is held to its processor twin by an equivalence " +
			"test, and refuses a grid too coarse to be the same answer"
	}
	return "off: every link is measured on the processor, across every core"
}

func gpuOrCPU(g state.GPUState) string {
	if g.Used {
		return "the GPU"
	}
	return "the processor"
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func orUnset(s string) string {
	if s == "" {
		return "the default cache directory"
	}
	return s
}

func trim0(f float64) string {
	if f == float64(int(f)) {
		return fmt.Sprintf("%d", int(f))
	}
	return fmt.Sprintf("%g", f)
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

// cacheGB is the bound as the session reports it, or the default before it
// has said anything.
func cacheGB(s *state.Snapshot) float64 {
	if s != nil && s.TileCacheGB > 0 {
		return s.TileCacheGB
	}
	return 10
}
