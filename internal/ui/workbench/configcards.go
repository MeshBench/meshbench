// The Configuration page's own cards: what this run assumes, section by
// section, so an operator can see the model rather than trust it.
package workbench

import (
	"fmt"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

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
		p.mark.brandCard(t),
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
		p.coverageResCard(t, s),
	}
}

// coverageResCard is the raster sharpness: one number, honest about cost.
func (p *configPanel) coverageResCard(t *theme.Theme, s *state.Snapshot) layout.Widget {
	now := 240
	if s.CoverageCells > 0 {
		now = s.CoverageCells
	}
	return comp.Card(t, "Coverage rasters", p.fieldRow(t, &p.covRes, &p.setCovRes,
		fmt.Sprintf("now %d cells on the long edge - the shared grid behind the "+
			"whole-map raster and the network-wide coverage questions. Cost "+
			"scales with the square, times the node count", now)))
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

// rfSimulation is the physics section: which model decides reception, and -
// as the waveform plan lands its later phases - every realism and
// environment switch, each defaulting to the honest baseline.
func (p *configPanel) rfSimulation(t *theme.Theme, s *state.Snapshot) []layout.Widget {
	verdictNote := "calculated: the link budget and the demodulator floor decide - " +
		"fast, and the mode every large scenario should use"
	if s.RFMode == "waveform" {
		verdictNote = "waveform: the channel produces IQ and the full receive " +
			"chain decides - Gray, deinterleave, Hamming FEC, dewhiten, header, " +
			"CRC. Capture, partial overlap and interference alignment are " +
			"emergent; what reaches MeshCore is the decoded bytes. Preamble " +
			"sync is granted, not modelled, until the receiver front end lands"
	}
	return []layout.Widget{
		comp.Card(t, "RF mode", func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.rfModeDD.Layout(t, gtx)
				}),
				layout.Rigid(layout.Spacer{Height: t.Sp.XS}.Layout),
				layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, verdictNote)),
			)
		}),
		comp.Card(t, "Imperfections", func(gtx layout.Context) layout.Dimensions {
			r := s.RFRealism
			rows := []layout.Widget{
				p.fieldRow(t, &p.oscPPM, nil, fmt.Sprintf(
					"now %s ppm - each node gets a deterministic crystal error in "+
						"that range, and the receiver's CFO estimator has to earn it",
					trim0(r.OscPPM))),
				p.fieldRow(t, &p.multipathDB, nil, fmt.Sprintf(
					"now %s dB - one geometric echo per path; fading emerges from "+
						"its phase, not from a dice roll", trim0(r.MultipathDB))),
				p.fieldRow(t, &p.fadingHz, nil, fmt.Sprintf(
					"now %s Hz - how fast the cancellation pattern breathes",
					trim0(r.FadingHz))),
				p.fieldRow(t, &p.implLoss, nil, fmt.Sprintf(
					"now %s dB - the receiver's shortfall from theory, as extra noise",
					trim0(r.ImplLossDB))),
				p.fieldRow(t, &p.satDBm, nil, fmt.Sprintf(
					"now %s dBm - the front end clips above this", trim0(r.SaturationDBm))),
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{}.Layout(gtx,
						layout.Flexed(1, comp.Text(t, t.Sz.Caption, t.P.Faint,
							"all zero is the kind default; every value here is stamped "+
								"into results. Empty boxes leave a knob where it is.")),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return p.setRealism.Layout(t, gtx)
						}),
					)
				},
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				func() []layout.FlexChild {
					kids := make([]layout.FlexChild, len(rows))
					for i, r := range rows {
						kids[i] = layout.Rigid(r)
					}
					return kids
				}()...)
		}),
		p.buildingsCard(t, s),
		comp.Card(t, "Shared physics", comp.Text(t, t.Sz.Caption, t.P.Faint,
			"both modes price the path identically - terrain, budgets, antennas, "+
				"timing, seed - so the same scenario runs under either, and the "+
				"difference between their ledgers measures where the fast model "+
				"diverges from the waveform")),
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
		comp.Card(t, "Terrain downloads", func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.terrainDL.LayoutSwitch(t, gtx)
				}),
				layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, terrainDLNote(s))),
			)
		}),
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
				"restart, in ~/.config/meshbench/workbench2.json; the scenario "+
				"itself deliberately stays in the fixture")),
	}
}

// terrainDLNote says what the switch costs either way.
//
// Both halves matter. Off, links are measured over whatever ground is already
// cached, and over ground that is not there the model falls back to free
// space, which is the most optimistic answer there is. On, a national network
// is several hundred megabytes the first time it is opened.
func terrainDLNote(s *state.Snapshot) string {
	if s != nil && s.TerrainDownloads {
		return "height data is fetched as a study needs it, a few tens of " +
			"kilobytes per tile; a country's worth of links is several hundred " +
			"megabytes the first time, then cached for ever"
	}
	return "nothing is downloaded: links are measured on the ground already " +
		"cached, and where there is none the answer falls back to free space, " +
		"which flatters every link that a hill would have blocked"
}
