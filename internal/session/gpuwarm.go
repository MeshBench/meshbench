package session

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"sync"
	"time"

	"github.com/MeshBench/meshbench/internal/coverage"
	"github.com/MeshBench/meshbench/internal/dsp"
	"github.com/MeshBench/meshbench/internal/engine"
	"github.com/MeshBench/meshbench/internal/gpu"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/scenario"
	"github.com/MeshBench/meshbench/internal/terrain"
)

// Warming the link matrix with the GPU.
//
// The shape is the old workbench's, because that shape is why it was never
// slow: cull the pairs free space already rules out, walk profiles only over
// ground the surviving paths actually cross - from the same hot tile cache
// the processor uses, on every core - and then do the diffraction arithmetic.
// The one departure is where the arithmetic runs. The first attempt instead
// rasterised the whole bounding box, which downloaded tiles no path crosses
// and outgrew what a card will bind; profiles are small at any span.

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

// gpuMinPairs is the least work worth opening a device for.
const gpuMinPairs = 500

// warmOnGPU fills the engine's link cache, reporting what it did. A result
// with Used false means the processor should do the work instead.
// The engine, the nodes and the frequency are passed in rather than read from
// the Sim, because this runs on a goroutine that outlives the verb that
// started it: opening another network replaces all three while this is still
// measuring, and reading them here is a race with whoever did the replacing.
// A warm measures the network it was started for, or it is cancelled.
func (s *Sim) warmOnGPU(eng *engine.Engine, nodes []scenario.Node, freqMHz float64,
	progress func(what string, done, total int),
) GPUWarmResult {
	var out GPUWarmResult
	n := len(nodes)
	if eng == nil || n < 2 {
		out.Why = "no network"
		return out
	}
	if n*(n-1)/2 < gpuMinPairs {
		out.Why = fmt.Sprintf("%d pairs is not enough work to be worth a device", n*(n-1)/2)
		return out
	}

	dev, err := gpu.Open()
	if err != nil {
		out.Why = err.Error()
		return out
	}
	defer dev.Close()
	out.Device, out.Backend = dev.Name, dev.Backend
	started := time.Now()

	// The cull, exactly as the engine's own pathLoss makes it: terrain only
	// ever adds loss, so a pair that cannot matter over flat vacuum cannot
	// matter at all, and the profile is the expensive part. Culled pairs get
	// their free-space figure, which is what the engine itself caches for
	// them.
	noise := dsp.NoiseFloorDBm(250e3, 6)
	type job struct {
		a, b   int
		distKm float64
	}
	var jobs []job
	loss := make([]float32, n*n)
	for i := range loss {
		loss[i] = coverage.NoDataLoss
	}
	culled := 0
	for a := 0; a < n; a++ {
		for b := a + 1; b < n; b++ {
			na, nb := nodes[a], nodes[b]
			distKm := haversineKmSession(na.Position.Lat, na.Position.Lon,
				nb.Position.Lat, nb.Position.Lon)
			if distKm <= 0 {
				continue
			}
			fspl := terrain.FSPLdB(distKm, freqMHz)
			bestTx := math.Max(na.TxPowerDBm, nb.TxPowerDBm)
			// A generous allowance for antenna gain on both ends, so this
			// cull is never tighter than the engine's own.
			if bestTx+24-fspl < noise-30 {
				loss[a*n+b] = float32(fspl)
				culled++
				continue
			}
			jobs = append(jobs, job{a: a, b: b, distKm: distKm})
		}
	}

	// Profiles, every core, from the tile cache. The same sampling as the
	// engine's own profile: a point per 60 m, at least two, at most 256.
	terr := s.terrain()
	type packed struct {
		idx     int
		heights []float32
		distM   float64
		aglA    float64
		aglB    float64
	}
	results := make([]packed, len(jobs))
	var done64 sync.WaitGroup
	workers := runtime.NumCPU()
	var counter int64
	var mu sync.Mutex
	next := 0
	for w := 0; w < workers; w++ {
		done64.Add(1)
		go func() {
			defer done64.Done()
			for {
				mu.Lock()
				i := next
				next++
				mu.Unlock()
				if i >= len(jobs) {
					return
				}
				j := jobs[i]
				na, nb := nodes[j.a], nodes[j.b]
				steps := int(j.distKm * 1000 / 60)
				if steps < 2 {
					steps = 2
				}
				if steps > 256 {
					steps = 256
				}
				hs := make([]float32, steps+1)
				ok := true
				for k := 0; k <= steps; k++ {
					f := float64(k) / float64(steps)
					h, got := terr.ElevationM(
						na.Position.Lat+(nb.Position.Lat-na.Position.Lat)*f,
						na.Position.Lon+(nb.Position.Lon-na.Position.Lon)*f)
					if !got {
						ok = false
						break
					}
					hs[k] = float32(h)
				}
				if ok {
					results[i] = packed{idx: i, heights: hs, distM: j.distKm * 1000,
						aglA: na.HeightAGLm, aglB: nb.HeightAGLm}
				}
				mu.Lock()
				counter++
				c := counter
				mu.Unlock()
				if progress != nil && (c == 1 || c%512 == 0) {
					progress("walking the ground under each link", int(c), len(jobs))
				}
			}
		}()
	}
	done64.Wait()

	// Pack and dispatch.
	var prof coverage.PairProfiles
	packedIdx := make([]int, 0, len(jobs))
	for i, r := range results {
		if r.heights == nil {
			continue
		}
		prof.Add(r.heights, r.distM, r.aglA, r.aglB)
		packedIdx = append(packedIdx, i)
	}
	if prof.Pairs() > 0 {
		if progress != nil {
			progress("diffraction on the GPU", prof.Pairs(), prof.Pairs())
		}
		res, err := dev.PairProfileLoss(prof, s.freqMHz)
		if err != nil {
			out.Why = err.Error()
			return out
		}
		for k, li := range packedIdx {
			j := jobs[li]
			loss[j.a*n+j.b] = res[k]
		}
	}

	out.Pairs = eng.PrimeLinks(n, loss, coverage.NoDataLoss)
	out.Duration = time.Since(started)
	out.Used = out.Pairs > 0
	if !out.Used {
		out.Why = "nothing could be measured: no elevation data for these paths"
	}
	_ = culled
	return out
}

// haversineKmSession is the same great-circle the engine uses.
func haversineKmSession(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371.0
	rad := math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLon := (lon2 - lon1) * rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * r * math.Asin(math.Min(1, math.Sqrt(a)))
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

// tilesPerGB is how many decoded tiles a gigabyte holds: a tile is 256x256
// float32, a quarter of a megabyte.
const tilesPerGB = 4096

// registerTileCache is the tile cache bound, in the unit people think in, and
// where the cache lives on disk.
func registerTileCache(st *state.Store, s *Sim) {
	st.Handle("terrain.cache", func(w *state.World, p any) (any, error) {
		if v, ok := numField(p, "gb"); ok && v >= 0.25 {
			tiles := int(v * tilesPerGB)
			s.tileCacheTiles = tiles
			if ts, ok := s.terr.(*terrain.TileStore); ok && ts != nil {
				ts.MaxLoadedTiles = tiles
			}
			w.TileCacheGB = v
			s.prefs.TileCacheGB = v
			s.savePrefs()
			w.Say(fmt.Sprintf("the tile cache holds %.3g GB", v))
		}
		if w.TileCacheGB == 0 {
			if s.tileCacheTiles > 0 {
				w.TileCacheGB = float64(s.tileCacheTiles) / tilesPerGB
			} else {
				w.TileCacheGB = float64(terrain.DefaultMaxLoadedTiles) / tilesPerGB
			}
		}
		w.TileCacheDir = s.tileCacheDir()
		return map[string]any{"gb": w.TileCacheGB, "dir": w.TileCacheDir}, nil
	})

	// terrain.cache_dir moves the cache. Gigabytes of tiles, so it runs as a
	// visible job on a worker, and the store only swaps directories after the
	// move has succeeded - the decoded tiles in memory survive throughout.
	st.Handle("terrain.cache_dir", func(w *state.World, p any) (any, error) {
		path := soleString(p)
		if m, ok := p.(map[string]any); ok {
			if v, ok := m["path"].(string); ok {
				path = v
			}
		}
		if path == "" {
			return map[string]any{"dir": s.tileCacheDir()}, nil
		}
		oldDir := s.tileCacheDir()
		newDir, err := validateCacheDir(oldDir, path)
		if err != nil {
			return nil, err
		}
		if s.movingCache.Swap(true) {
			return nil, fmt.Errorf("a cache move is already running")
		}
		w.Say("moving the tile cache to " + newDir)
		go func() {
			defer s.movingCache.Store(false)
			ctx := context.Background()
			_, _ = st.Do(ctx, "job.progress", state.Job{
				ID: "cachemove", What: "moving the tile cache"})
			n, err := moveTree(oldDir, newDir, func(done, total int) {
				_, _ = st.Do(ctx, "job.progress", state.Job{
					ID: "cachemove", What: "moving the tile cache",
					Done: done, Total: total})
			})
			_, _ = st.Do(ctx, "job.done", "cachemove")
			if err != nil {
				_, _ = st.Do(ctx, "ui.said", fmt.Sprintf(
					"the cache move stopped after %d files: %v - "+
						"the cache stays at %s", n, err, oldDir))
				return
			}
			_, _ = st.Do(ctx, "terrain.cache_moved",
				map[string]any{"dir": newDir, "files": n})
		}()
		return map[string]any{"moving": true, "to": newDir}, nil
	})

	// terrain.cache_moved is the worker reporting back on the store's
	// goroutine, which is the only place the swap may happen.
	st.Handle("terrain.cache_moved", func(w *state.World, p any) (any, error) {
		m, _ := p.(map[string]any)
		dir, _ := m["dir"].(string)
		files := 0
		if v, ok := numField(p, "files"); ok {
			files = int(v)
		}
		if dir == "" {
			return nil, fmt.Errorf("terrain.cache_moved needs the directory")
		}
		if ts, ok := s.terr.(*terrain.TileStore); ok && ts != nil {
			ts.SetCacheDir(dir)
		}
		s.prefs.TileCacheDir = dir
		s.savePrefs()
		w.TileCacheDir = dir
		w.Say(fmt.Sprintf("the tile cache lives at %s now - %d files moved, "+
			"nothing needs downloading again", dir, files))
		return map[string]any{"dir": dir}, nil
	})
}

// registerGPU is the switch and what it did.
func registerGPU(st *state.Store, s *Sim) {
	// gpu.set: on or off, said once and remembered.
	st.Handle("gpu.set", func(w *state.World, p any) (any, error) {
		if v, ok := boolField(p, "on"); ok {
			// Chosen beats decided: once somebody has said, the default does
			// not get another go at it - and the choice survives a restart.
			s.gpuAsked = true
			s.gpuWarm = v
			s.prefs.GPU = &v
			s.savePrefs()
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
		p.name, p.backend = d.Name, d.Backend
		// Opening is not the same as being right. A d3d12 adapter opened
		// cleanly, compiled the shaders, ran them, and returned coverage
		// cells nearly ten decibels away from the processor's - a wrong
		// answer with nothing to see. So the device proves itself against
		// the CPU twin on a small problem before it is trusted with the
		// network, and a device that fails says why rather than quietly
		// producing plausible numbers.
		if err := d.SelfCheck(); err != nil {
			p.present, p.why = false, err.Error()
		} else {
			p.present = true
		}
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
