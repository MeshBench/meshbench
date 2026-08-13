package session

import (
	"fmt"
	"math"
	"time"

	"github.com/A13xB0/meshcoresim/internal/coverage"
	"github.com/A13xB0/meshcoresim/internal/gpu"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
)

// Warming the link matrix on the GPU.
//
// Forty-eight thousand profiles, none of which depends on another, on a
// machine whose cores are already running three hundred firmware processes.
// That is the shape a compute shader is for, and the kernel is held to its CPU
// twin by an equivalence test.
//
// It is not the default, and the reason is honest rather than cautious: the
// kernel reads a rasterised height grid, and a grid is not the DEM. On a
// county the cells come out finer than the elevation data itself and the
// answer is the same to a hundredth of a decibel; on a country-sized import
// the same grid is hundreds of metres per cell, which is a different
// simulation. So the cell size is measured, shown, and refused when it is too
// coarse to be the same answer.

// gpuGridMin and gpuGridMax bound the raster the kernel samples. Square, and
// grown until its cells are fine enough to be the same answer as the DEM: a
// county needs 4096, a country needs 8192, and 8192 squared is 268 MB of
// float32, which is the most worth putting on a graphics card for this.
const (
	gpuGridMin = 2048
	gpuGridMax = 8192
)

// gpuGridFor is the smallest grid whose cells are fine enough for this view.
// The second return is the cell size it achieves, which is reported whether or
// not it is good enough.
func gpuGridFor(south, north float64) (int, float64) {
	spanM := (north - south) * 111320
	for n := gpuGridMin; n <= gpuGridMax; n *= 2 {
		if cell := spanM / float64(n); cell <= gpuMaxCellM {
			return n, cell
		}
	}
	return gpuGridMax, spanM / gpuGridMax
}

// gpuMinPairs is the least work worth moving. Below it the grid costs more to
// build than the pairs cost to measure.
const gpuMinPairs = 4000

// gpuMaxCellM is the coarsest cell this will use. Beyond it the CPU does the
// work, because a profile sampled every quarter of a kilometre is not the
// terrain the rest of the simulator is using.
const gpuMaxCellM = 120.0

// GPUWarmResult is what a warm did, for the page that reports it.
type GPUWarmResult struct {
	Used     bool
	Why      string
	Device   string
	Backend  string
	Pairs    int
	CellM    float64
	Duration time.Duration
}

// gpuDefault decides, once, whether to use the GPU without being asked.
//
// A machine with one gets it: the work is forty-eight thousand independent
// profiles and the cores are wanted for the firmware. A machine without one
// carries on exactly as before, which is the promise the project makes about
// every GPU path.
func (s *Sim) gpuDefault() {
	if s.gpuAsked {
		return
	}
	s.gpuAsked = true
	s.probeGPU()
	s.gpuWarm = s.gpuProbe.present
}

// warmOnGPU fills the engine's link cache from the pair kernel, and reports
// what it did. A false first return means the CPU should do the work.
func (s *Sim) warmOnGPU(progress func(what string, done, total int)) GPUWarmResult {
	var out GPUWarmResult
	if s.eng == nil || len(s.nodes) < 2 {
		out.Why = "no network"
		return out
	}
	south, north := math.Inf(1), math.Inf(-1)
	west, east := math.Inf(1), math.Inf(-1)
	for _, n := range s.nodes {
		south = math.Min(south, n.Position.Lat)
		north = math.Max(north, n.Position.Lat)
		west = math.Min(west, n.Position.Lon)
		east = math.Max(east, n.Position.Lon)
	}
	// A margin, so a node on the edge still has ground beyond it to diffract
	// over.
	padLat := math.Max(0.05, (north-south)*0.05)
	padLon := math.Max(0.05, (east-west)*0.05)
	south, north = south-padLat, north+padLat
	west, east = west-padLon, east+padLon

	// Worth doing at all.
	//
	// The kernel is sixty-five milliseconds for forty-eight thousand pairs;
	// rasterising the grid it reads is sixteen million elevation lookups, and
	// on a small network that trade is the wrong way round - measured at
	// twenty seconds against a couple for the cores. So the grid is cached,
	// and below a few thousand pairs the processor does the work.
	pairs := len(s.nodes) * (len(s.nodes) - 1) / 2
	if pairs < gpuMinPairs && !s.hasGrid(south, north, west, east) {
		out.Why = fmt.Sprintf("%d pairs is less work than building the height "+
			"grid to do it on", pairs)
		return out
	}

	// The cell size, north-south, which is the one that does not change with
	// latitude.
	grid, cell := gpuGridFor(south, north)
	out.CellM = cell
	if out.CellM > gpuMaxCellM {
		out.Why = fmt.Sprintf("this network spans %.0f km, so even a %d cell grid "+
			"is %.0f m per cell - coarser than the terrain the rest of the run uses",
			(north-south)*111.32, gpuGridMax, out.CellM)
		return out
	}

	dev, err := gpu.Open()
	if err != nil {
		out.Why = err.Error()
		return out
	}
	defer dev.Close()
	out.Device, out.Backend = dev.Name, dev.Backend

	// The grid has to fit in one storage binding on this card. Asked, not
	// assumed: at the WebGPU default of 128 MiB an 8192 grid does not bind,
	// and the kernel failing after the grid was built is twenty wasted
	// seconds and a silent fall back.
	if need := uint64(grid) * uint64(grid) * 4 / (1 << 20); need >= dev.MaxStorageMB {
		for grid > gpuGridMin && uint64(grid)*uint64(grid)*4/(1<<20) >= dev.MaxStorageMB {
			grid /= 2
		}
		out.CellM = (north - south) * 111320 / float64(grid)
		if out.CellM > gpuMaxCellM {
			out.Why = fmt.Sprintf("this card binds %d MB at most, so the finest "+
				"grid that fits is %.0f m per cell - coarser than the terrain "+
				"the rest of the run uses", dev.MaxStorageMB, out.CellM)
			return out
		}
	}

	started := time.Now()
	g, ok := s.heightGrid(south, north, west, east, grid, progress)
	if !ok {
		out.Why = "most of this view has no elevation data cached"
		return out
	}

	nodes := make([]coverage.PairNode, 0, len(s.nodes))
	for _, n := range s.nodes {
		nodes = append(nodes, coverage.PairNode{
			Lat: n.Position.Lat, Lon: n.Position.Lon, AGLm: n.HeightAGLm,
		})
	}
	loss, err := dev.PairLoss(g, nodes, coverage.PairParams{
		FreqMHz: s.freqMHz, StepM: 60, StepsCap: 256,
	})
	if err != nil {
		out.Why = err.Error()
		return out
	}
	out.Pairs = s.eng.PrimeLinks(len(nodes), loss, coverage.NoDataLoss)
	out.Duration = time.Since(started)
	out.Used = out.Pairs > 0
	if !out.Used {
		out.Why = "the matrix was about a different network by the time it landed"
	}
	return out
}

// registerGPU is the switch and what it did.
func registerGPU(st *state.Store, s *Sim) {
	// gpu.set: on or off, said once and remembered.
	st.Handle("gpu.set", func(w *state.World, p any) (any, error) {
		if v, ok := boolField(p, "on"); ok {
			// Chosen beats decided: once somebody has said, the default does
			// not get another go at it.
			s.gpuAsked = true
			s.gpuWarm = v
			if v {
				w.Say("the link matrix will be measured on the GPU where it can be")
			} else {
				w.Say("the link matrix will be measured on the processor")
			}
		}
		w.GPU = s.gpuWorldState()
		return s.gpuState(), nil
	})

	// gpu.state: what hardware there is, and what the last warm did with it.
	st.Handle("gpu.state", func(w *state.World, _ any) (any, error) {
		w.GPU = s.gpuWorldState()
		return s.gpuState(), nil
	})
}

// gpuState is the answer both verbs give, and what the Configuration page
// draws: whether it is on, what hardware answered, and what the last warm
// actually did - because "GPU acceleration: on" over a run that quietly fell
// back to the cores is the kind of claim this project does not make.
func (s *Sim) gpuState() map[string]any {
	s.gpuMu.Lock()
	last := s.lastGPU
	s.gpuMu.Unlock()

	out := map[string]any{"enabled": s.gpuWarm}
	s.probeGPU()
	out["present"] = s.gpuProbe.present
	if s.gpuProbe.present {
		out["device"] = s.gpuProbe.name
		out["backend"] = s.gpuProbe.backend
	} else {
		out["why"] = s.gpuProbe.why
	}
	if last.Device != "" || last.Why != "" {
		lastOut := map[string]any{"used": last.Used, "pairs": last.Pairs}
		if last.CellM > 0 {
			lastOut["cell_m"] = math.Round(last.CellM*10) / 10
		}
		if last.Duration > 0 {
			lastOut["ms"] = last.Duration.Milliseconds()
		}
		if last.Why != "" {
			lastOut["why"] = last.Why
		}
		out["last_warm"] = lastOut
	}
	out["grid_max"] = gpuGridMax
	out["coarsest_cell_m"] = gpuMaxCellM
	return out
}

// gpuProbe is what asking the machine for a GPU answered, kept because asking
// twice is a device open twice.
type gpuProbe struct {
	present bool
	name    string
	backend string
	why     string
}

// probeGPU asks the machine what it has, once.
func (s *Sim) probeGPU() {
	if s.gpuProbe != nil {
		return
	}
	p := &gpuProbe{}
	if d, err := gpu.Open(); err == nil {
		p.name, p.backend, p.present = d.Name, d.Backend, true
		d.Close()
	} else {
		p.why = err.Error()
	}
	s.gpuProbe = p
}

// gpuWorldState is the same answer as gpuState, in the shape the interface
// draws rather than the shape a socket reads.
func (s *Sim) gpuWorldState() state.GPUState {
	s.gpuDefault()
	s.gpuMu.Lock()
	last := s.lastGPU
	s.gpuMu.Unlock()
	out := state.GPUState{
		Enabled: s.gpuWarm,
		Present: s.gpuProbe.present,
		Device:  s.gpuProbe.name,
		Backend: s.gpuProbe.backend,
		Why:     s.gpuProbe.why,
		Used:    last.Used,
		Pairs:   last.Pairs,
		CellM:   last.CellM,
		Ms:      last.Duration.Milliseconds(),
	}
	if last.Why != "" {
		out.Why = last.Why
	}
	return out
}

// heightGrid is the raster the kernel reads, built once per view.
//
// Sixteen million elevation lookups is the expensive half of this, and it
// depends on the ground rather than on the nodes: moving a node, changing a
// seed or restarting a run all reuse it, and only a different area pays again.
func (s *Sim) heightGrid(south, north, west, east float64, grid int,
	progress func(what string, done, total int)) (coverage.HeightGrid, bool) {

	if s.hasGrid(south, north, west, east) {
		return s.grid, true
	}
	g, known := coverage.RasteriseHeightsProgress(s.terrain(), south, north, west, east,
		grid, grid, func(done, total int) {
			if progress != nil {
				progress("reading the ground for the GPU", done, total)
			}
		})
	if known < 0.5 {
		return coverage.HeightGrid{}, false
	}
	s.grid = g
	return g, true
}

// hasGrid reports whether the cached raster is about this view.
func (s *Sim) hasGrid(south, north, west, east float64) bool {
	g := s.grid
	return g.W > 0 && g.South == south && g.North == north &&
		g.West == west && g.East == east
}
