// The warm's lifecycle: snapshotting the network it measures, running it,
// and the fingerprint that says whether the last warm still stands.
// Split from engine.go at the file limit.
package session

import (
	"context"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"math"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// snapshotNodes copies the nodes themselves, so a worker reading them cannot
// be racing a verb that writes them.
//
// Element-wise, which is what the reported race needed: the fields written
// while a warm is in flight are scalars and strings on the node itself. A
// node's Regions slice is still shared with the original, and deliberately -
// every writer replaces that slice wholesale rather than editing it in place,
// so the copy keeps whichever one it was given.
func snapshotNodes(in []scenario.Node) []scenario.Node {
	return append([]scenario.Node(nil), in...)
}

// warm computes the link margins on a worker and hands them to the store.
//
// One at a time: a second warm while the first is running would compute the
// same thing twice and race to publish it.
func (s *Sim) warm(st *state.Store, nodes int) {
	if s.eng == nil {
		return
	}
	// Cancel whatever is running and start again.
	//
	// Marking it stale and repeating afterwards was not enough: a warm holds
	// no copy of the engine, so one started for a 58-node fixture carried on
	// against the 676-node import that replaced it - 228,000 terrain profiles
	// for a network nobody is looking at, while the 44 nodes that were
	// actually on screen showed no links at all and nothing said why.
	s.warmMu.Lock()
	if s.warmCancel != nil {
		s.warmCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.warmCancel = cancel
	// A warm that is starting is no longer held, whatever held the last one.
	s.warmHeld = false
	s.warmMu.Unlock()

	s.cold = false
	// The network this warm is for, taken now. Everything below runs after
	// the verb that started it has returned, and s.eng/s.nodes belong to
	// whatever has been opened since.
	//
	// A copy of the nodes, not of the slice header. That distinction was the
	// whole of a data race: assigning s.nodes shares the backing array, so a
	// warm reading a node's fields on its worker and a verb writing them on
	// the store goroutine - setFirmware, say - are touching the same memory.
	// The intent here was always a snapshot; this is what makes it one.
	//
	// The fingerprint comes with them, because it names the geometry these
	// nodes are, and the matrix this warm saves is a matrix of that geometry
	// and no other.
	eng, warmNodes, freqMHz, fp := s.eng, snapshotNodes(s.nodes), s.freqMHz, s.geomFP
	go func() {
		defer cancel()
		total := nodes * (nodes - 1) / 2
		_, _ = st.Do(ctx, "job.progress", state.Job{
			ID: "links", What: "measuring every link", Total: total})

		// On by default where there is hardware for it, decided once.
		s.gpuDefault()
		// A matrix restored from disk that already answers every pair leaves
		// the device nothing to measure: the sweep below is map hits, and the
		// warm is over in the time it takes to read them. Without this check a
		// calibration - which rebuilds the engine but changes no geometry -
		// re-measured the whole country to move a constant the cache does not
		// even store.
		primed := eng.LinkCachePairs() >= total
		if !primed {
			// Permission before bandwidth: a first launch opens a national
			// network, and the ground under it is several hundred megabytes
			// nobody agreed to spend.
			if s.heldForTerrain(ctx, st, warmNodes) {
				return
			}
			// The ground first, announced: a walk over a region whose tiles
			// are not down yet otherwise fetches them one by one from the
			// middle of the measurement, which reads as a hang.
			s.prefetchWarmTerrain(ctx, st, warmNodes)
		}
		// The GPU first, if it is switched on and can answer honestly. What
		// it fills, the cores below no longer have to: WarmLinks asks the
		// cache before it measures anything.
		if s.gpuWarm && !primed {
			res := s.warmOnGPU(eng, warmNodes, freqMHz, func(what string, done, total int) {
				_, _ = st.Do(ctx, "job.progress", state.Job{
					ID: "links", What: what, Done: done, Total: total})
			})
			s.gpuMu.Lock()
			s.lastGPU = res
			s.gpuMu.Unlock()
			if res.Used {
				_, _ = st.Do(ctx, "job.progress", state.Job{
					ID: "links", What: "measuring every link on the GPU",
					Done: res.Pairs, Total: total})
			} else if res.Why != "" {
				// Said aloud. A silent fall back is a status frozen on the
				// last phase while every core spikes, which reads as a hang.
				_, _ = st.Do(ctx, "ui.said",
					"the GPU declined this one - "+res.Why+" - measuring on the processor")
			}
			_, _ = st.Do(ctx, "gpu.state", nil)
		}

		eng.WarmLinks(ctx, func(done, of int) {
			// No second throttle here: the engine already reports every 512th
			// pair, and a filter stacked on a filter only let through their
			// common multiples - the first status update came at pair 32,000,
			// which on the processor is most of the warm spent looking hung.
			_, _ = st.Do(ctx, "job.progress", state.Job{
				ID: "links", What: "measuring every link on the processor",
				Done: done, Total: of})
		})

		if ctx.Err() != nil {
			// Superseded: what this measured is about a network that has been
			// replaced, and publishing it would put another network's links
			// on the map.
			//
			// The job row goes with it. Returning silently left "measuring
			// every link" in the list at whatever count it had reached, for
			// ever - and anything waiting for the workbench to go idle waited
			// on a measurement that had been abandoned. The warm that
			// superseded this one owns the row now and will post its own
			// progress under the same id.
			abandonWarmJob(ctx, st)
			return
		}
		links := linksOf(eng, warmNodes)
		if ctx.Err() != nil {
			abandonWarmJob(ctx, st)
			return
		}
		s.warmMu.Lock()
		s.warmed = true
		s.warmMu.Unlock()
		// What was measured survives the process, keyed by the geometry it
		// is about. On its own goroutine already, and after the staleness
		// checks, so what lands on disk is a matrix somebody saw.
		saveMatrix(s.matrixDir(), fp, eng.LinkCacheSnapshot())
		// The warm's own context has been cancelled by the time a rewarm
		// supersedes this one, and the measurement it just finished still has
		// to reach the store - so these inherit it without its cancellation.
		done, release := finishing(ctx)
		defer release()
		_, _ = st.Do(done, "links.set", links)
		// What this warm was actually able to walk, recorded with it: a matrix
		// measured over ground half of which never arrived is a matrix that has
		// to say so wherever it is read.
		_, _ = st.Do(done, "terrain.ground", nil)
		_, _ = st.Do(done, "job.progress", state.Job{
			ID: "links", What: "measuring every link",
			Done: total, Total: total, Finished: true})
	}()
}

// rebuild starts the same network again from the world's seed.
//
// The engine is remade rather than rewound: an engine carries queued packets,
// per-node radio state and the firmware processes' own memory, and there is no
// honest way to unwind those to zero. Firmware is left alone, because
// restarting several hundred processes to change a seed is a different and
// much slower operation than the caller asked for.
func (s *Sim) rebuild(w *state.World) error {
	if len(s.nodes) == 0 {
		return fmt.Errorf("no network loaded")
	}
	seed := w.Seed
	if seed == 0 {
		seed = defaultSeed
	}
	s.buildSeeded(s.nodes, s.freqMHz, seed)
	w.NowMs, w.Seed = 0, seed
	w.Events, w.EventTotal = nil, 0
	return nil
}

// warming reports whether the link matrix is still being measured, which is
// the one thing a run must not start in front of.
//
// A warm held for permission it has not been given is not being measured, so
// this is false for it: leaving it true blocks every run behind a measurement
// that will never finish on its own. linksMeasured is the other half of that
// answer, and is the one to ask before believing a result.
func (s *Sim) warming() bool {
	s.warmMu.Lock()
	defer s.warmMu.Unlock()
	return s.warmCancel != nil && !s.warmed && !s.warmHeld
}

// linksMeasured reports that every pair has actually been walked, and held
// reports a warm that stopped to ask first.
//
// Separate from warming because "not running" and "finished" are different
// answers, and a held warm is the first: nothing is in flight, and nothing has
// been measured either. Marking a held warm as finished is how a session with
// no ground under it came to report itself ready and answer studies over free
// space.
func (s *Sim) linksMeasured() (measured, held bool) {
	s.warmMu.Lock()
	defer s.warmMu.Unlock()
	return s.warmed, s.warmHeld
}

// geometryFingerprint hashes everything the stored matrix depends on.
//
// Deliberately not the excess-loss term: the cache stores the raw physics and
// the term is applied where it is read, so a calibration changes a constant
// rather than the matrix - fingerprinting on it made every validate.calibrate
// throw away half an hour of ground-walking to move a number every path
// shares. Old matrices, keyed under hashes that included the term with it
// baked into every entry, simply never match this fingerprint and are
// remeasured once.
//
// The environment IS in it, because unlike the term it is baked in: a crossed
// rooftop is charged into the cached loss for that pair and cannot be taken
// back out at read time. Without it one key covered two different physics, and
// a session opened over bare earth restored a building-priced matrix from disk
// and reported it as measured - a third of the country's links missing, the
// warm skipped because the cache already answered every pair, and nothing
// saying which model the numbers came from.
func geometryFingerprint(nodes []scenario.Node, freqMHz float64, envDir string) uint64 {
	h := fnv.New64a()
	b := make([]byte, 8)
	put := func(f float64) {
		binary.LittleEndian.PutUint64(b, math.Float64bits(f))
		_, _ = h.Write(b)
	}
	put(freqMHz)
	_, _ = h.Write([]byte(envDir))
	for _, n := range nodes {
		_, _ = h.Write([]byte(n.Name))
		put(n.Position.Lat)
		put(n.Position.Lon)
		put(n.HeightAGLm)
		put(n.TxPowerDBm)
		// Noise figure is in here because the engine's path-loss cull decides
		// against the noise floor, so a node that got quieter can bring a pair
		// back that a previous run discarded. Leaving it out meant a stale
		// matrix loaded from disk and looked authoritative.
		put(n.NoiseFigureDB)
	}
	return h.Sum64()
}

// reFingerprint re-keys the current geometry after something the fingerprint
// covers has changed without the engine being rebuilt.
//
// Switching the environment is the only such change today, and it is why this
// exists: the warm saves what it measured under the fingerprint held when it
// started, so a buildings warm begun under the bare-earth key would file
// building-priced numbers where the bare-earth ones belong, and the disk cache
// would hand them back to the next session that opened over bare earth.
func (s *Sim) reFingerprint() {
	s.geomFP = geometryFingerprint(s.nodes, s.freqMHz, s.envDir)
}

// abandonWarmJob marks the link measurement finished when this warm is not the
// one that will publish it.
//
// Finished rather than removed: a newer warm re-posts the row under the same
// id the moment it starts, so removing it would race with that and leave a
// caller watching a list that flickers. What matters is that nothing waits on
// a measurement nobody is doing any more.
func abandonWarmJob(ctx context.Context, st *state.Store) {
	// The warm's own context, through finishing: it has been cancelled, which
	// is why this is being called at all, and the update still has to reach
	// the store. Same shape as the success path below it.
	done, release := finishing(ctx)
	defer release()
	_, _ = st.Do(done, "job.progress", state.Job{
		ID: "links", What: "measuring every link (superseded)",
		Done: 1, Total: 1, Finished: true})
}
