// The whole map's coverage, as one raster.
//
// Per-repeater coverage answers "what does this mast reach"; this answers the
// operator's real question - "where does the network work" - by rasterising
// every infrastructure node over one shared grid and keeping, per cell, the
// best two-way link anyone offers. One shared grid is not an optimisation:
// rasters over different boxes cannot honestly be combined at all.
package session

import (
	"context"
	"fmt"
	"math"
	"sync"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/rf/environ"
	"github.com/MeshBench/meshbench/internal/rf/propagation"
	"github.com/MeshBench/meshbench/internal/study/coverage"
	"github.com/MeshBench/meshbench/internal/study/linkbudget"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// mapGridDefault is the whole-map raster's longest edge when the operator
// has not chosen one. Finer costs every node's rasterisation again: the
// price scales with the square of this number times the node count.
const mapGridDefault = 240

// mapGridMin and mapGridMax bound the operator's choice: below the floor a
// raster is a rumour, above the ceiling a single pull is minutes of terrain
// profiles nobody sat down for.
const (
	mapGridMin = 64
	mapGridMax = 4096
)

// coverageCells is the long edge the operator chose, or the default.
func (s *Sim) coverageCells() int {
	if s.covCells >= mapGridMin && s.covCells <= mapGridMax {
		return s.covCells
	}
	return mapGridDefault
}

// mapMarginKm is how far past the outermost node the raster looks - coverage
// does not stop at the last mast, and a box cropped to the nodes says it does.
const mapMarginKm = 15

// mapBox is the shared grid every node answers over: the network's bounding
// box plus margin, with the pixel grid matched to its aspect so cells stay
// square-ish rather than stretched.
func mapBox(nodes []scenario.Node, maxEdge int) (south, north, west, east float64, w, h int, err error) {
	if len(nodes) == 0 {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("no nodes to cover")
	}
	south, north = math.Inf(1), math.Inf(-1)
	west, east = math.Inf(1), math.Inf(-1)
	for _, n := range nodes {
		south = math.Min(south, n.Position.Lat)
		north = math.Max(north, n.Position.Lat)
		west = math.Min(west, n.Position.Lon)
		east = math.Max(east, n.Position.Lon)
	}
	midLat := (south + north) / 2
	dLat := mapMarginKm / 111.32
	dLon := mapMarginKm / (111.32 * math.Cos(midLat*math.Pi/180))
	south, north = south-dLat, north+dLat
	west, east = west-dLon, east+dLon
	// Aspect in kilometres, not degrees: a degree of longitude shrinks with
	// latitude, and a grid set by degrees draws Scotland twice as wide.
	spanNS := (north - south) * 111.32
	spanEW := (east - west) * 111.32 * math.Cos(midLat*math.Pi/180)
	edge := float64(maxEdge)
	if spanNS >= spanEW {
		h = maxEdge
		w = int(math.Max(16, math.Round(edge*spanEW/spanNS)))
	} else {
		w = maxEdge
		h = int(math.Max(16, math.Round(edge*spanNS/spanEW)))
	}
	return south, north, west, east, w, h, nil
}

// infrastructure keeps the nodes whose coverage is the network's: repeaters
// and room servers. A companion is somebody's pocket and an observer never
// transmits; drawing their "coverage" would claim service where none stands.
func infrastructure(nodes []scenario.Node) []scenario.Node {
	out := make([]scenario.Node, 0, len(nodes))
	for _, n := range nodes {
		switch n.Kind {
		case scenario.SimpleRepeater, scenario.AdvancedRepeater, scenario.RoomServer:
			out = append(out, n)
		}
	}
	return out
}

func registerCoverageMap(st *state.Store, s *Sim) {
	// coverage.resolution: how sharp the shared-grid rasters are. Persisted,
	// because a resolution is a machine-and-patience choice, not a scenario's.
	st.Handle("coverage.resolution", func(w *state.World, p any) (any, error) {
		if v, ok := numField(p, "cells"); ok {
			cells := int(v)
			if cells < mapGridMin || cells > mapGridMax {
				return nil, fmt.Errorf("coverage resolution is %d to %d cells on the long edge",
					mapGridMin, mapGridMax)
			}
			s.covCells = cells
			w.CoverageCells = cells
			s.prefs.CoverageCells = cells
			s.savePrefs()
			w.Say(fmt.Sprintf("coverage rasters at %d cells on the long edge - "+
				"cost scales with the square", cells))
		}
		return map[string]any{"cells": s.coverageCells()}, nil
	})

	st.Handle("coverage.map", func(w *state.World, p any) (any, error) {
		return s.startCoverageMap(st, w, p)
	})
}

// startCoverageMap is the one raster job: the whole network or a single
// station ("coverage from this node" arrives here too), so there is one
// code path - two would disagree about buildings within a week.
func (s *Sim) startCoverageMap(st *state.Store, w *state.World, p any) (any, error) {
	painted := "the whole network"
	infra := infrastructure(s.nodes)
	// From the params object only: stringField's bare-string case would
	// otherwise read any stray string as a station name.
	mp, _ := p.(map[string]any)
	if name, _ := mp["station"].(string); name != "" {
		infra = infra[:0]
		for i := range s.nodes {
			if s.nodes[i].Name == name ||
				(name == "selected" && i < len(w.Nodes) && w.Nodes[i].Selected) {
				infra = append(infra[:0], s.nodes[i])
				painted = s.nodes[i].Name
				break
			}
		}
		if len(infra) == 0 {
			return nil, fmt.Errorf("no node selected to compute coverage from")
		}
	}
	if len(infra) == 0 {
		return nil, fmt.Errorf("no repeaters or room servers to cover the map with")
	}
	// An explicit box wins - the raster-this-view button sends the
	// borders somebody is actually looking at. Then the study boundary,
	// then the network's own box.
	south, sOK := numField(p, "south")
	north, nOK := numField(p, "north")
	west, wOK := numField(p, "west")
	east, eOK := numField(p, "east")
	if !sOK || !nOK || !wOK || !eOK {
		var ok bool
		south, north, west, east, ok = areasBox(w.Areas)
		if !ok {
			var err error
			south, north, west, east, _, _, err = mapBox(s.nodes, s.coverageCells())
			if err != nil {
				return nil, err
			}
		}
	} else if south >= north || west >= east {
		return nil, fmt.Errorf("that viewport is inside out")
	}
	// A per-run resolution rides along with a viewport box - the map
	// picks what looks sharp on the screen asking - without touching
	// the saved knob.
	edge := s.coverageCells()
	if v, ok := numField(p, "cells"); ok && p != nil {
		if c := int(v); c >= mapGridMin && c <= mapGridMax {
			edge = c
		}
	}
	gw, gh := gridFor(south, north, west, east, edge)
	stations := make([]coverage.Endpoint, 0, len(infra))
	for _, n := range infra {
		n := n
		stations = append(stations, coverage.Endpoint{
			Name: n.Name, Lat: n.Position.Lat, Lon: n.Position.Lon,
			HeightAGLm: n.HeightAGLm, TxPowerDBm: n.TxPowerDBm,
			SensitivityDBm: linkbudget.SensitivityDBm(n),
			GainTowardsDBi: func(b, e float64) float64 { return n.Antenna.GainTowardsDBi(b, e) },
		})
	}
	freq := s.freqMHz
	if freq <= 0 {
		freq = 869.618
	}
	var env environ.Provider
	if s.eng != nil && s.eng.Env != nil {
		env = s.eng.Env
	} else if s.envDir != "" {
		env = environ.OpenTiles(s.envDir)
	}
	// Cache-only terrain: a raster that walks into the sea must draw
	// a gap there, not stall a national job on tile downloads for
	// water nobody radioed across. Missing ground is NoData, counted.
	ground := s.terrainCached()

	const id = "coverage-map"
	// One job, two phases: the terrain sampled once, then the cells.
	// Height rows count as the first half so the bar moves from the
	// first second - a long job with nothing on screen reads as a hang.
	hw, hh := gw*2, gh*2
	if hw > 4096 {
		hw, hh = 4096, int(4096*float64(gh)/float64(gw))
	}
	total := hh + gh
	w.Jobs = append(w.Jobs, state.Job{
		ID: id, What: "coverage: " + painted, Total: total})
	with := "bare earth"
	if env != nil {
		with = "buildings priced"
	}
	if painted == "the whole network" {
		w.Say(fmt.Sprintf("covering the map: %d stations, %dx%d cells, %s",
			len(stations), gw, gh, with))
	} else {
		w.Say(fmt.Sprintf("covering from %s: %dx%d cells, %s",
			painted, gw, gh, with))
	}
	go func() {
		ctx := context.Background()
		// However it ends, the bar comes down: a finished job that
		// keeps owning the status line reads as a hang after the fact.
		defer func() { _, _ = st.Do(ctx, "job.done", id) }()
		grid, frac := propagation.RasteriseHeightsProgress(ground,
			south, north, west, east, hw, hh, func(row, _ int) {
				_, _ = st.Do(ctx, "job.progress", state.Job{
					ID: id, What: "coverage: sampling terrain",
					Done: row, Total: total})
			})
		if frac < 0.05 {
			_, _ = st.Do(ctx, "coverage.failed",
				"the terrain tiles for this area are not downloaded")
			return
		}
		r := &coverage.Raster{South: south, North: north, West: west, East: east,
			Width: gw, Height: gh, FreqMHz: freq}
		var extra func(int, float64, float64, float64, float64, float64) float64
		if env != nil {
			// One spatial index for the whole job. Per-path store
			// queries - even corridor-shaped - ground twelve workers
			// against one mutex for minutes; the index pays one query
			// and answers every path lock-free.
			iSouth, iNorth, iWest, iEast := south, north, west, east
			for _, sta := range stations {
				iSouth = math.Min(iSouth, sta.Lat)
				iNorth = math.Max(iNorth, sta.Lat)
				iWest = math.Min(iWest, sta.Lon)
				iEast = math.Max(iEast, sta.Lon)
			}
			ix := environ.NewPathIndex(env, ground,
				iSouth-0.05, iWest-0.05, iNorth+0.05, iEast+0.05)
			if ix.Buildings() > 0 {
				// Each station's near-set once, up front: the raster
				// asks about the same town a hundred thousand times,
				// and the sector index answers with the handful of
				// footprints the ray could actually cross.
				shadows := make([]*environ.StationPaths, len(stations))
				for i, sta := range stations {
					shadows[i] = ix.Station(sta.Lat, sta.Lon)
				}
				nearMask := ix.NearMask(south, north, west, east, gw, gh, 3)
				var pool = sync.Pool{New: func() any { return &environ.PathScratch{} }}
				extra = func(sti int, cellLat, cellLon, txAsl, rxAsl, distM float64) float64 {
					cx := int((cellLon - west) / (east - west) * float64(gw))
					cy := int((north - cellLat) / (north - south) * float64(gh))
					near := cx >= 0 && cx < gw && cy >= 0 && cy < gh && nearMask[cy*gw+cx]
					// A pool whose New returns *PathScratch cannot hand back
					// anything else, so this one is safe by construction.
					sc := pool.Get().(*environ.PathScratch) //nolint:forcetypeassert // sync.Pool with a typed New
					defer pool.Put(sc)
					return shadows[sti].LossDB(sc, near, txAsl, cellLat, cellLon, rxAsl, distM, freq)
				}
			}
		}
		// The profile step follows the height grid: sampling terrain
		// finer than the grid that answers is precision theatre, paid
		// for in minutes.
		cellM := (east - west) * 111320 * math.Cos((south+north)/2*math.Pi/180) / float64(hw)
		stepM := math.Max(150, cellM)
		opts := coverage.Options{
			RemoteHeightAGLm: 1.5, RemoteTxPowerDBm: 20,
			RemoteSensitivityDBm: -124, ProfileStepM: stepM,
		}
		var combined *coverage.Combined
		if s.gpuWarm {
			// The operator's one GPU switch, the warm's own rule: the
			// device prices each station's whole grid at once, and a
			// missing or dying device hands the job to the CPU twin.
			if c, name, ok := s.coverageMapGPU(grid, stations, r, opts, extra,
				func(what string, done, totalWork int) {
					if totalWork < 1 {
						totalWork = 1
					}
					_, _ = st.Do(ctx, "job.progress", state.Job{
						ID: id, What: what,
						Done: hh + done*gh/totalWork, Total: total})
				}); ok {
				combined = c
				_, _ = st.Do(ctx, "ui.said", "coverage priced on "+name)
			}
		}
		if combined == nil {
			combined = coverage.BestServer(grid, stations, r, opts,
				extra, func(row, _ int) {
					_, _ = st.Do(ctx, "job.progress", state.Job{
						ID: id, What: "coverage: judging every cell",
						Done: hh + row, Total: total})
				})
		}
		cov := paintCoverage(r, painted)
		_, _ = st.Do(ctx, "coverage.set", cov)
		_, _ = st.Do(ctx, "coverage.combined",
			map[string]any{"mode": "map", "combined": combined})
	}()
	return map[string]any{"nodes": len(infra), "started": true}, nil
}

// areasBox is the study boundary's bounding box plus a margin, when a
// boundary exists at all.
func areasBox(areas []state.Area) (south, north, west, east float64, ok bool) {
	south, north = math.Inf(1), math.Inf(-1)
	west, east = math.Inf(1), math.Inf(-1)
	for _, a := range areas {
		for _, ring := range a.Rings {
			for _, p := range ring {
				south = math.Min(south, p.Lat)
				north = math.Max(north, p.Lat)
				west = math.Min(west, p.Lon)
				east = math.Max(east, p.Lon)
			}
		}
	}
	if math.IsInf(south, 1) {
		return 0, 0, 0, 0, false
	}
	midLat := (south + north) / 2
	dLat := 5 / 111.32
	dLon := 5 / (111.32 * math.Cos(midLat*math.Pi/180))
	return south - dLat, north + dLat, west - dLon, east + dLon, true
}

// gridFor matches the pixel grid to the box's aspect on the ground.
func gridFor(south, north, west, east float64, maxEdge int) (w, h int) {
	midLat := (south + north) / 2
	spanNS := (north - south) * 111.32
	spanEW := (east - west) * 111.32 * math.Cos(midLat*math.Pi/180)
	edge := float64(maxEdge)
	if spanNS >= spanEW {
		h = maxEdge
		w = int(math.Max(16, math.Round(edge*spanEW/spanNS)))
	} else {
		w = maxEdge
		h = int(math.Max(16, math.Round(edge*spanNS/spanEW)))
	}
	return w, h
}
