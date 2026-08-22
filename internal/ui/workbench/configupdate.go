// The Configuration panel's interaction pass: every card's clicks and
// edits, in one sweep. Split from panels6c.go at the file limit.
package workbench

import (
	"fmt"
	"strconv"
	"strings"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/shell"
)

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
	// Linux only, and drawn only there (interfaceCards); the update pass
	// follows the same was/now shape as the GPU switch so a preference set
	// from the socket is not fought by a control that thinks it is off.
	if p.keepAbove.Bool.Update(gtx) {
		p.wasKeep = p.keepAbove.Bool.Value
		if p.do != nil {
			p.do("ui.keep_above", map[string]any{"on": p.keepAbove.Bool.Value})
		}
	} else if s.KeepAbove != p.wasKeep {
		p.wasKeep = s.KeepAbove
		p.keepAbove.Bool.Value = s.KeepAbove
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
		{&p.setCovRes, &p.covRes, "coverage.resolution", "cells", 64},
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
	p.wireEnvironSources()
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
