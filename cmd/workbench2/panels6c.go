// Configuration and the experiment log (6.17, 6.14).
package main

import (
	"fmt"
	"strconv"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/A13xB0/meshcoresim/internal/gui/comp"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

// configPanel is what this run is, in the terms that change a result (6.17).
//
// Read-only for the values the engine was built with, because changing them
// means rebuilding it, and a field that silently does nothing until a rebuild
// is worse than a value you cannot edit.
type configPanel struct {
	tb    comp.Table
	gpu   comp.Check
	cache comp.Field
	setC  comp.Button
	// OnCache sets the tile cache bound, in GB.
	OnCache func(gb float64)
	// OnGPU turns the graphics path on and off.
	OnGPU func(on bool)
	init  bool
	was   bool
}

func (p *configPanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !p.init {
		p.tb.Cols = []comp.Column{
			{Title: "setting", Width: 240},
			{Title: "value", Width: 200, Mono: true},
			{Title: "why it matters"},
		}
		p.gpu.Label = "measure links on the GPU"
		p.cache.Hint = "tile cache, GB"
		p.cache.Editor.SingleLine = true
		p.setC.Label, p.setC.Kind = "set cache", comp.Secondary
		p.init = true
	}
	if s == nil {
		return layout.Dimensions{}
	}
	// The switch follows the session unless somebody has just moved it, so a
	// run that turned it off is not fought by a checkbox that thinks it is on.
	if p.gpu.Bool.Update(gtx) {
		p.was = p.gpu.Bool.Value
		if p.OnGPU != nil {
			p.OnGPU(p.gpu.Bool.Value)
		}
	} else if s.GPU.Enabled != p.was {
		p.was = s.GPU.Enabled
		p.gpu.Bool.Value = s.GPU.Enabled
	}
	rows := []comp.Row{
		{Key: "seed", Cells: []string{"seed", fmt.Sprintf("%d", s.Seed),
			"two runs of one seed are the same run; a difference across seeds is the model, not the seed"}},
		{Key: "now", Cells: []string{"simulated time", fmt.Sprintf("%.2f s", float64(s.NowMs)/1000),
			"not wall time, and never has been"}},
		{Key: "nodes", Cells: []string{"nodes", fmt.Sprintf("%d", len(s.Nodes)),
			"every one is simulated; none are sampled"}},
		{Key: "links", Cells: []string{"links measured", fmt.Sprintf("%d", len(s.Links)),
			"pairs with a path loss, from the engine rather than from distance"}},
		{Key: "areas", Cells: []string{"study areas", fmt.Sprintf("%d", len(s.Areas)),
			"what bounds the study"}},
		{Key: "margin", Cells: []string{"study margin", fmt.Sprintf("%g km", s.MarginKm),
			"how far outside the boundary a node still matters"}},
		{Key: "playing", Cells: []string{"running", fmt.Sprintf("%t", s.Playing),
			"the engine advances on its own ticker, not on frames"}},
		{Key: "events", Cells: []string{"events", fmt.Sprintf("%d", s.EventTotal),
			"the whole log; the tables show the tail of it"}},
	}
	rows = append(rows, gpuRows(s.GPU)...)
	p.tb.SetRows(rows)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(comp.SectionTitle(t, "this run")),
		layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Warn,
			"results are a best case: no multipath, bare earth, ideal demodulator")),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return p.tb.Layout(t, gtx, nil)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.setC.Click.Clicked(gtx) && p.OnCache != nil {
				if v, err := strconv.ParseFloat(fieldText(&p.cache), 64); err == nil && v > 0 {
					p.OnCache(v)
				}
			}
			bar := actionBar{fields: []*comp.Field{&p.cache},
				buttons: []*comp.Button{&p.setC},
				note: fmt.Sprintf("decoded terrain tiles held in memory - now %.3g GB; "+
					"a cache smaller than the study area re-reads tiles from disk constantly",
					cacheGB(s))}
			return bar.layout(t, gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.gpu.Layout(t, gtx)
		}),
		layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, gpuNote(s.GPU))),
	)
}

// gpuRows say what hardware there is and what the last warm did with it.
//
// What it did, and not only whether it is switched on: a page reading "GPU
// acceleration: on" over a run that quietly fell back to the processor is the
// kind of claim this project does not make.
func gpuRows(g state.GPUState) []comp.Row {
	present := "none found"
	if g.Present {
		present = g.Device + " (" + g.Backend + ")"
	}
	rows := []comp.Row{
		{Key: "gpu", Cells: []string{"graphics device", present,
			"every GPU path has a processor twin that answers the same, more slowly"}},
		{Key: "gpuon", Cells: []string{"links measured on the GPU",
			fmt.Sprintf("%t", g.Enabled),
			"forty-eight thousand independent profiles is the shape a compute shader is for"}},
	}
	if g.Pairs > 0 {
		rows = append(rows, comp.Row{Key: "gpupairs", Cells: []string{
			"last warm on the GPU",
			fmt.Sprintf("%d pairs in %d ms", g.Pairs, g.Ms),
			fmt.Sprintf("sampled at %.0f m per cell", g.CellM)}})
	}
	return rows
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
