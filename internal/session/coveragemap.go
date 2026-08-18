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

	"github.com/MeshBench/meshbench/internal/coverage"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/scenario"
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
	mapGridMax = 1024
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

	st.Handle("coverage.map", func(w *state.World, _ any) (any, error) {
		infra := infrastructure(s.nodes)
		if len(infra) == 0 {
			return nil, fmt.Errorf("no repeaters or room servers to cover the map with")
		}
		south, north, west, east, gw, gh, err := mapBox(s.nodes, s.coverageCells())
		if err != nil {
			return nil, err
		}
		const id = "coverage-map"
		w.Jobs = append(w.Jobs, state.Job{
			ID: id, What: "coverage: the whole map", Total: len(infra)})
		w.Say(fmt.Sprintf("rasterising the whole map from %d node(s)", len(infra)))
		go func() {
			ctx := context.Background()
			rasters := make([]*coverage.Raster, 0, len(infra))
			for i, n := range infra {
				r, err := s.rasterOnBox(ctx, n, south, north, west, east, gw, gh)
				if err == nil && r != nil {
					rasters = append(rasters, r)
				}
				_, _ = st.Do(ctx, "job.progress", state.Job{
					ID: id, What: "coverage: the whole map",
					Done: i + 1, Total: len(infra)})
			}
			combined, err := coverage.Combine(rasters)
			if err != nil {
				_, _ = st.Do(ctx, "coverage.failed", err.Error())
				return
			}
			// The combined raster's cells already hold each cell's best
			// server, so the per-node painter draws the network-wide truth.
			cov := paintCoverage(&combined.Raster, "the whole network")
			_, _ = st.Do(ctx, "coverage.set", cov)
			_, _ = st.Do(ctx, "coverage.combined",
				map[string]any{"mode": "map", "combined": combined})
		}()
		return map[string]any{"nodes": len(infra), "started": true}, nil
	})
}
