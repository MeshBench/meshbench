// The whole map's coverage, as one raster.
//
// Per-repeater coverage answers "what does this mast reach"; this answers the
// operator's real question - "where does the network work" - by rasterising
// every infrastructure node over one shared grid and keeping, per cell, the
// best two-way link anyone offers. The grid itself, and the knob that sets how
// sharp it is, are in coveragegrid.go.
package study

import (
	"context"
	"fmt"
	"math"
	"sync"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/rf/environ"
	"github.com/MeshBench/meshbench/internal/rf/propagation"
	"github.com/MeshBench/meshbench/internal/study/coverage"
	"github.com/MeshBench/meshbench/internal/study/linkbudget"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

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

func registerCoverageMap(st *state.Store, s *session.Sim) {
	registerCoverageResolution(st, s)

	st.Handle("coverage.map", func(w *state.World, p any) (any, error) {
		return startCoverageMap(s, st, w, p)
	})
}

// startCoverageMap is the one raster job: the whole network or a single
// station ("coverage from this node" arrives here too), so there is one
// code path - two would disagree about buildings within a week.
func startCoverageMap(s *session.Sim, st *state.Store, w *state.World, p any) (any, error) {
	// Every parameter read and checked before anything is decided about the
	// network. A refusal about a viewport that arrives only once the network
	// turns out to have a repeater in it is a refusal that changes with the
	// scenario, and a caller cannot tell their own mistake from ours.
	asked, err := coverageAsked(s, p)
	if err != nil {
		return nil, err
	}
	painted := "the whole network"
	infra := infrastructure(s.Nodes())
	// From the params object only: stringField's bare-string case would
	// otherwise read any stray string as a station name.
	mp, _ := p.(map[string]any)
	if name, _ := mp["station"].(string); name != "" {
		infra = infra[:0]
		for i := range s.Nodes() {
			if s.Nodes()[i].Name == name ||
				(name == "selected" && i < len(w.Nodes) && w.Nodes[i].Selected) {
				infra = append(infra[:0], s.Nodes()[i])
				painted = s.Nodes()[i].Name
				break
			}
		}
		// "selected" is the map asking about whatever is under the cursor, so
		// nothing selected is a state and not a mistake. A name is a caller
		// naming a node, and a name this network has not got is theirs.
		if len(infra) == 0 {
			if name == "selected" {
				return nil, fmt.Errorf("no node selected to compute coverage from")
			}
			return nil, session.UnknownNames("coverage.map", w.Nodes, []string{name})
		}
	}
	if len(infra) == 0 {
		return nil, fmt.Errorf("no repeaters or room servers to cover the map with")
	}
	// An explicit box wins - the raster-this-view button sends the
	// borders somebody is actually looking at. Then the study boundary,
	// then the network's own box.
	south, north, west, east := asked.south, asked.north, asked.west, asked.east
	if !asked.boxed {
		var ok bool
		south, north, west, east, ok = areasBox(w.Areas)
		if !ok {
			south, north, west, east, _, _, err = mapBox(s.Nodes(), coverageCells(s))
			if err != nil {
				return nil, err
			}
		}
	}
	// The ground before the raster, not after it: a cell painted over free
	// space is a cell that says a ridge is not there, and this raster is read
	// as a picture of where the network works.
	under := s.GroundOver(south, north, west, east)
	if err := session.StudyGround(w, "coverage.map", under); err != nil {
		return nil, err
	}
	gw, gh := gridFor(south, north, west, east, asked.cells)
	stations := make([]coverage.Endpoint, 0, len(infra))
	for _, n := range infra {
		n := n
		stations = append(stations, coverage.Endpoint{
			Name: n.Name, Lat: n.Position.Lat, Lon: n.Position.Lon,
			HeightAGLm: n.HeightAGLm, TxPowerDBm: n.TxPowerDBm,
			SensitivityDBm: linkbudget.SensitivityDBm(n),
			UncertaintyKm:  n.UncertaintyKm,
			GainTowardsDBi: func(b, e float64) float64 { return n.Antenna.GainTowardsDBi(b, e) },
		})
	}
	freq := s.FreqMHz()
	if freq <= 0 {
		freq = 869.618
	}
	var env environ.Provider
	if s.Engine() != nil && s.Engine().Env != nil {
		env = s.Engine().Env
	} else if s.EnvDir() != "" {
		env = environ.OpenTiles(s.EnvDir())
	}
	// Cache-only terrain: a raster that walks into the sea must draw
	// a gap there, not stall a national job on tile downloads for
	// water nobody radioed across. Missing ground is NoData, counted.
	ground := s.TerrainCached()

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
				nearMask := ix.NearMask(south, north, west, east, gw, gh)
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
		if s.GPUWarm() {
			// The operator's one GPU switch, the warm's own rule: the
			// device prices each station's whole grid at once, and a
			// missing or dying device hands the job to the CPU twin.
			if c, name, ok := coverageMapGPU(s, grid, stations, r, opts, extra,
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
			c, err := coverage.BestServer(grid, stations, r, opts,
				extra, func(row, _ int) {
					_, _ = st.Do(ctx, "job.progress", state.Job{
						ID: id, What: "coverage: judging every cell",
						Done: hh + row, Total: total})
				})
			if err != nil {
				_, _ = st.Do(ctx, "coverage.failed", err.Error())
				return
			}
			combined = c
		}
		cov := paintCoverage(r, painted)
		_, _ = st.Do(ctx, "coverage.set", cov)
		_, _ = st.Do(ctx, "coverage.combined",
			map[string]any{"mode": "map", "combined": combined})
	}()
	return map[string]any{"nodes": len(infra), "started": true,
		"ground": under.Map()}, nil
}
