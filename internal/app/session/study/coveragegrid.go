// The grid a coverage raster answers over: how sharp it is, what ground it
// covers, and which of the two a caller actually asked for.
//
// Kept apart from the job that walks it because one shared grid is not an
// optimisation - rasters over different boxes cannot honestly be combined at
// all - so where the borders and the cell count come from is a decision in its
// own right, and the resolution is the operator's own knob rather than a
// parameter of any one raster.
package study

import (
	"fmt"
	"math"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
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
func coverageCells(s *session.Sim) int {
	if s.CovCells() >= mapGridMin && s.CovCells() <= mapGridMax {
		return s.CovCells()
	}
	return mapGridDefault
}

// registerCoverageResolution adds the operator's sharpness knob. Called from
// registerCoverageMap rather than from the domain's own list, so that the two
// verbs sharing this grid arrive together however the files are arranged.
func registerCoverageResolution(st *state.Store, s *session.Sim) {
	st.Handle("coverage.resolution", func(w *state.World, p any) (any, error) {
		// Asked for, rather than merely readable. No cells at all is a read of
		// the current setting, which is a legitimate call; cells that cannot be
		// read as a number is a caller who meant to change it, and answering
		// them with the unchanged value reports the opposite of what happened.
		v, asked, err := session.NumAsked("coverage.resolution", "cells", p)
		if err != nil {
			return nil, err
		}
		if asked {
			cells := int(v)
			if cells < mapGridMin || cells > mapGridMax {
				return nil, fmt.Errorf("coverage resolution is %d to %d cells on the long edge",
					mapGridMin, mapGridMax)
			}
			s.SetCoverageCells(cells)
			w.CoverageCells = cells
			_ = s.SavePrefs(w)
			w.Say(fmt.Sprintf("coverage rasters at %d cells on the long edge - "+
				"cost scales with the square", cells))
		}
		return map[string]any{"cells": coverageCells(s)}, nil
	})
}

// coverageRequest is what a caller asked a raster for, once every parameter
// has been read and none of them is left to be guessed at.
type coverageRequest struct {
	south, north, west, east float64
	// boxed says the four borders were given. Without it the raster falls back
	// to the study area and then the network's own box, which is a documented
	// default and not a substitution.
	boxed bool
	cells int
}

// coverageAsked reads the raster parameters, refusing what it cannot use.
//
// No viewport at all is the ordinary case - the whole network, or the study
// area - and not an error. Some of a viewport is: three borders and a typo used
// to fail all four reads together and fall through to the network's own box, so
// the raster came back over ground nobody had asked about and looked exactly
// like an answer to the question. The borders are range-checked too, because a
// degree outside the globe is a caller who meant metres.
func coverageAsked(s *session.Sim, p any) (coverageRequest, error) {
	const verb = "coverage.map"
	out := coverageRequest{}
	// Refused when it is outside what a raster can be, rather than replaced by
	// the saved knob: a caller who asked for 30,000 cells and silently got 240
	// has been told a picture is sharp when it is not.
	cells, err := session.NumInRange(verb, "cells", p,
		float64(coverageCells(s)), mapGridMin, mapGridMax)
	if err != nil {
		return out, err
	}
	out.cells = int(cells)

	m, isObject := p.(map[string]any)
	if !isObject {
		return out, nil
	}
	borders := [4]struct {
		name   string
		lo, hi float64
		into   *float64
	}{
		{"south", -90, 90, &out.south}, {"north", -90, 90, &out.north},
		{"west", -180, 180, &out.west}, {"east", -180, 180, &out.east},
	}
	given := 0
	for _, b := range borders {
		if _, has := m[b.name]; !has {
			continue
		}
		given++
		v, err := session.NamedNumInRange(verb, b.name, p, 0, b.lo, b.hi)
		if err != nil {
			return out, err
		}
		*b.into = v
	}
	if given == 0 {
		return out, nil
	}
	if given < 4 {
		return out, session.BadParams(
			"%s: a viewport is all four of south, north, west and east; %d were given",
			verb, given)
	}
	if out.south >= out.north || out.west >= out.east {
		return out, session.BadParams(
			"%s: that viewport is inside out - south %g is not below north %g, "+
				"or west %g is not left of east %g",
			verb, out.south, out.north, out.west, out.east)
	}
	out.boxed = true
	return out, nil
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
